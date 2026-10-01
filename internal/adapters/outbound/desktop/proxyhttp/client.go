// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package proxyhttp construye el cliente HTTP de producto a partir de las
// preferencias no sensibles y del almacén seguro de credenciales del SO.
package proxyhttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/ports"
)

const (
	defaultTimeout      = 30 * time.Second
	secretResolveWindow = 5 * time.Second
)

type settingsLoader func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error)

// New crea el cliente HTTP compartido para TSA, OCSP/CRL y servicios remotos.
func New(configDir string) *http.Client {
	return newWithDependencies(configDir, usersettings.CargarDocumentoCompat, proxysecretstore.New())
}

func newWithDependencies(configDir string, load settingsLoader, store ports.ProxySecretStore) *http.Client {
	return &http.Client{
		Timeout:   defaultTimeout,
		Transport: transportWithDependencies(configDir, load, store),
	}
}

func transportWithDependencies(configDir string, load settingsLoader, store ports.ProxySecretStore) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	setEnvironmentProxy(transport)
	if load == nil {
		return transport
	}

	ctx, cancel := context.WithTimeout(context.Background(), secretResolveWindow)
	defer cancel()
	doc, err := load(ctx, strings.TrimSpace(configDir))
	if err != nil {
		transport.Proxy = failClosedProxy(fmt.Errorf("proxyhttp: cargando configuracion: %w", err))
		return transport
	}
	if doc.Proxy.Enabled == nil || !*doc.Proxy.Enabled {
		transport.Proxy = nil
		return transport
	}

	cfg, err := proxysecretstore.ResolveRuntimeProxy(ctx, doc.Proxy, store)
	if err != nil {
		// Un proxy configurado con proxySecretId nunca se degrada a tráfico
		// directo, al proxy de entorno ni al mismo proxy sin autenticación.
		transport.Proxy = failClosedProxy(err)
		return transport
	}
	if !cfg.Enabled {
		transport.Proxy = nil
		return transport
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "system":
		setEnvironmentProxy(transport)
		return transport
	case "manual":
		// continúa abajo
	default:
		transport.Proxy = nil
		return transport
	}

	proxyURL, err := manualProxyURL(cfg)
	if err != nil {
		transport.Proxy = failClosedProxy(err)
		return transport
	}
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if req != nil && bypass(req.URL, cfg.ExcludedURLs) {
			return nil, nil
		}
		return proxyURL, nil
	}
	return transport
}

func manualProxyURL(cfg proxysecretstore.RuntimeProxyConfig) (*url.URL, error) {
	defer zero(cfg.Password)
	host := strings.TrimSpace(cfg.Host)
	if host == "" || cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, errors.New("proxyhttp: proxy manual incompleto")
	}
	if strings.ContainsAny(host, `/\\@?#`) || strings.ContainsAny(host, "\r\n\t ") {
		return nil, errors.New("proxyhttp: host de proxy invalido")
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return nil, errors.New("proxyhttp: host de proxy invalido")
	}
	proxyURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(cfg.Port)),
	}
	if cfg.Username != "" || len(cfg.Password) > 0 {
		proxyURL.User = url.UserPassword(cfg.Username, string(cfg.Password))
	}
	return proxyURL, nil
}

func failClosedProxy(cause error) func(*http.Request) (*url.URL, error) {
	return func(*http.Request) (*url.URL, error) {
		return nil, cause
	}
}

func setEnvironmentProxy(transport *http.Transport) {
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if req == nil || req.URL == nil {
			return nil, nil
		}
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}
}

func bypass(target *url.URL, excluded []string) bool {
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
		if strings.Contains(pattern, "://") {
			if parsed, err := url.Parse(pattern); err == nil {
				pattern = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
			}
		}
		if strings.HasPrefix(pattern, "*.") {
			if strings.HasSuffix(host, strings.TrimPrefix(pattern, "*")) {
				return true
			}
			continue
		}
		if pattern != "" && host == pattern {
			return true
		}
	}
	return false
}

func zero(secret []byte) {
	for i := range secret {
		secret[i] = 0
	}
}
