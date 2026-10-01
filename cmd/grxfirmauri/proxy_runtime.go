// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/net/http/httpproxy"
	"grxfirma/internal/appdirs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/ports"
)

type proxySettingsLoader func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error)

var errProxyRuntimeFailClosed = errors.New("proxy runtime: credenciales configuradas no disponibles")

const proxyRuntimeResolveTimeout = 5 * time.Second

var usersettingsLoaderForProxyTests proxySettingsLoader
var secretStoreForProxyTests ports.ProxySecretStore

type runtimeProxyMode string

const (
	runtimeProxyModeDefaultEnvironment runtimeProxyMode = "default-environment"
	runtimeProxyModeDisabled           runtimeProxyMode = "disabled"
	runtimeProxyModeSystem             runtimeProxyMode = "system"
	runtimeProxyModeManualNoSecret     runtimeProxyMode = "manual-no-secret"
	runtimeProxyModeManualSecureStore  runtimeProxyMode = "manual-secure-store"
	runtimeProxyModeFailClosed         runtimeProxyMode = "fail-closed"
)

type runtimeProxyDiagnostic struct {
	Mode     runtimeProxyMode
	Enabled  bool
	Type     string
	Host     string
	Port     int
	SecretID string
	Reason   string
}

func construirClienteTrifasico(configDir string) *http.Client {
	return construirClienteTrifasicoConDependencias(configDir, effectiveProxySettingsLoader(), effectiveProxySecretStore())
}

func construirClienteTrifasicoConDependencias(configDir string, loadSettings proxySettingsLoader, secretStore ports.ProxySecretStore) *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: construirTransporteHTTPCompartidoConDependencias(configDir, loadSettings, secretStore),
	}
}

func effectiveProxySettingsLoader() proxySettingsLoader {
	if usersettingsLoaderForProxyTests != nil {
		return usersettingsLoaderForProxyTests
	}
	return usersettings.CargarDocumentoCompat
}

func effectiveProxySecretStore() ports.ProxySecretStore {
	if secretStoreForProxyTests != nil {
		return secretStoreForProxyTests
	}
	return proxysecretstore.New()
}

func construirTransporteHTTPCompartidoConDependencias(configDir string, loadSettings proxySettingsLoader, secretStore ports.ProxySecretStore) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	ensureTransportProxyFromEnvironment(transport)
	cfg, diag := resolverProxyRuntimeConDependencias(configDir, loadSettings, secretStore)
	switch diag.Mode {
	case runtimeProxyModeDefaultEnvironment:
		return transport
	case runtimeProxyModeDisabled:
		disableTransportProxy(transport)
		return transport
	case runtimeProxyModeFailClosed:
		failClosedTransportProxy(transport, diag.Reason)
		return transport
	}
	if !cfg.Enabled {
		disableTransportProxy(transport)
		return transport
	}
	typ := strings.ToLower(strings.TrimSpace(cfg.Type))
	if typ == "system" {
		ensureTransportProxyFromEnvironment(transport)
		return transport
	}
	if typ != "manual" || strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 {
		disableTransportProxy(transport)
		return transport
	}

	proxyURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
	}
	if cfg.Username != "" || len(cfg.Password) > 0 {
		proxyURL.User = url.UserPassword(cfg.Username, string(cfg.Password))
		zeroProxyPassword(cfg.Password)
	}

	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if shouldBypassProxy(req.URL, cfg.ExcludedURLs) {
			return nil, nil
		}
		return proxyURL, nil
	}
	return transport
}

func diagnosticarClienteHTTPRuntimeSeguro(configDir string) runtimeProxyDiagnostic {
	return diagnosticarClienteHTTPRuntimeSeguroConDependencias(configDir, effectiveProxySettingsLoader(), effectiveProxySecretStore())
}

func diagnosticarClienteHTTPRuntimeSeguroConDependencias(configDir string, loadSettings proxySettingsLoader, secretStore ports.ProxySecretStore) runtimeProxyDiagnostic {
	_, diag := resolverProxyRuntimeConDependencias(configDir, loadSettings, secretStore)
	return diag
}

func resolverProxyRuntimeConDependencias(configDir string, loadSettings proxySettingsLoader, secretStore ports.ProxySecretStore) (proxysecretstore.RuntimeProxyConfig, runtimeProxyDiagnostic) {
	diag := runtimeProxyDiagnostic{Mode: runtimeProxyModeDefaultEnvironment}
	if loadSettings == nil {
		return proxysecretstore.RuntimeProxyConfig{}, diag
	}
	ctx, cancel := context.WithTimeout(context.Background(), proxyRuntimeResolveTimeout)
	defer cancel()
	doc, err := loadSettings(ctx, resolveProxyConfigDir(configDir))
	if err != nil {
		diag.Mode = runtimeProxyModeFailClosed
		diag.Reason = err.Error()
		return proxysecretstore.RuntimeProxyConfig{}, diag
	}
	diag.Enabled = doc.Proxy.Enabled != nil && *doc.Proxy.Enabled
	diag.Type = strings.ToLower(strings.TrimSpace(ptrStringProxy(doc.Proxy.Type)))
	diag.Host = strings.TrimSpace(ptrStringProxy(doc.Proxy.Host))
	diag.Port = ptrIntProxy(doc.Proxy.Port)
	diag.SecretID = strings.TrimSpace(ptrStringProxy(doc.Proxy.SecretID))
	if !diag.Enabled {
		diag.Mode = runtimeProxyModeDisabled
		return proxysecretstore.RuntimeProxyConfig{}, diag
	}
	cfg, err := proxysecretstore.ResolveRuntimeProxy(ctx, doc.Proxy, secretStore)
	if err == nil {
		return cfg, classifyResolvedRuntimeProxyDiagnostic(cfg, diag)
	}
	diag.Mode = runtimeProxyModeFailClosed
	diag.Reason = err.Error()
	return proxysecretstore.RuntimeProxyConfig{}, diag
}

func classifyResolvedRuntimeProxyDiagnostic(cfg proxysecretstore.RuntimeProxyConfig, diag runtimeProxyDiagnostic) runtimeProxyDiagnostic {
	diag.Enabled = cfg.Enabled
	diag.Type = strings.ToLower(strings.TrimSpace(cfg.Type))
	diag.Host = strings.TrimSpace(cfg.Host)
	diag.Port = cfg.Port
	diag.SecretID = strings.TrimSpace(cfg.SecretID)
	if !diag.Enabled {
		diag.Mode = runtimeProxyModeDisabled
		return diag
	}
	switch diag.Type {
	case "system":
		diag.Mode = runtimeProxyModeSystem
	case "manual":
		if diag.SecretID != "" {
			diag.Mode = runtimeProxyModeManualSecureStore
		} else {
			diag.Mode = runtimeProxyModeManualNoSecret
		}
	default:
		diag.Mode = runtimeProxyModeDefaultEnvironment
	}
	return diag
}

func formatRuntimeProxyDiagnostic(diag runtimeProxyDiagnostic) string {
	parts := []string{"mode=" + string(diag.Mode)}
	if diag.Type != "" {
		parts = append(parts, "type="+diag.Type)
	}
	if diag.Host != "" && diag.Port > 0 {
		parts = append(parts, fmt.Sprintf("endpoint=%s:%d", diag.Host, diag.Port))
	}
	if diag.SecretID != "" {
		parts = append(parts, "secret=store")
	}
	if diag.Reason != "" {
		parts = append(parts, "reason="+diag.Reason)
	}
	return strings.Join(parts, " ")
}

func formatRuntimeProxyStartupNotice(diag runtimeProxyDiagnostic) string {
	switch diag.Mode {
	case runtimeProxyModeManualSecureStore, runtimeProxyModeManualNoSecret, runtimeProxyModeSystem, runtimeProxyModeFailClosed:
		return "proxy runtime: " + formatRuntimeProxyDiagnostic(diag)
	default:
		return ""
	}
}

func ptrStringProxy(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptrIntProxy(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func resolveProxyConfigDir(configDir string) string {
	if strings.TrimSpace(configDir) != "" {
		return strings.TrimSpace(configDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return appdirs.Config(home)
}

func shouldBypassProxy(target *url.URL, excluded []string) bool {
	if target == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(target.Hostname()))
	if host == "" {
		return false
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	for _, raw := range excluded {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		if pattern == "" {
			continue
		}
		if strings.Contains(pattern, "://") {
			if parsed, err := url.Parse(pattern); err == nil {
				pattern = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
			}
		}
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(host, suffix) {
				return true
			}
			continue
		}
		if host == pattern {
			return true
		}
	}
	return false
}

func zeroProxyPassword(password []byte) {
	for i := range password {
		password[i] = 0
	}
}

func disableTransportProxy(transport *http.Transport) {
	if transport == nil {
		return
	}
	transport.Proxy = nil
}

func failClosedTransportProxy(transport *http.Transport, reason string) {
	if transport == nil {
		return
	}
	cause := errProxyRuntimeFailClosed
	if reason = strings.TrimSpace(reason); reason != "" {
		cause = fmt.Errorf("%w: %s", cause, reason)
	}
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		return nil, cause
	}
}

func ensureTransportProxyFromEnvironment(transport *http.Transport) {
	if transport == nil {
		return
	}
	// Usamos httpproxy.FromEnvironment() en lugar del http.ProxyFromEnvironment global
	// porque este último cachea el entorno via sync.Once y no refleja cambios en tests.
	// En producción el entorno no cambia, así que el comportamiento es equivalente.
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}
}
