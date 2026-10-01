// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxyhttp

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/ports"
)

type secretStoreStub struct {
	status   ports.ProxySecretStoreStatus
	material ports.ProxySecretMaterial
}

func (s secretStoreStub) Store(context.Context, string, ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	return ports.ProxySecretDescriptor{}, errors.New("not implemented")
}
func (s secretStoreStub) Load(context.Context, string) (ports.ProxySecretMaterial, error) {
	return s.material, nil
}
func (s secretStoreStub) Delete(context.Context, string) error {
	return errors.New("not implemented")
}
func (s secretStoreStub) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	return s.status, nil
}

func TestTransportWithDependencies_ConsumeSecretoYRespetaExclusion(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "2001:db8::1"
	port := 3128
	secretID := "secret-1"
	realm := "corp-proxy"
	load := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return ports.DocumentoConfiguracionUsuario{Proxy: ports.ConfiguracionUsuarioProxy{
			Enabled:      &enabled,
			Type:         &typ,
			Host:         &host,
			Port:         &port,
			SecretID:     &secretID,
			Realm:        &realm,
			ExcludedURLs: []string{"intra.example"},
		}}, nil
	}
	store := secretStoreStub{
		status: ports.ProxySecretStoreStatus{Available: true},
		material: ports.ProxySecretMaterial{
			Realm:    realm,
			Username: "usuario",
			Password: []byte("secreto"),
		},
	}

	transport := transportWithDependencies("/config", load, store)
	req, _ := http.NewRequest(http.MethodGet, "https://servicio.example", nil)
	got, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if got == nil || got.Host != "[2001:db8::1]:3128" || got.User == nil {
		t.Fatalf("proxy inesperado: %#v", got)
	}
	if user := got.User.Username(); user != "usuario" {
		t.Fatalf("username = %q", user)
	}
	if password, ok := got.User.Password(); !ok || password != "secreto" {
		t.Fatalf("password inesperado")
	}

	excluded, _ := http.NewRequest(http.MethodGet, "https://intra.example/recurso", nil)
	if got, err := transport.Proxy(excluded); err != nil || got != nil {
		t.Fatalf("proxy excluido = %#v, %v", got, err)
	}
}

func TestTransportWithDependencies_SecretStoreNoDisponibleFallaCerrado(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	load := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return ports.DocumentoConfiguracionUsuario{Proxy: ports.ConfiguracionUsuarioProxy{
			Enabled:  &enabled,
			Type:     &typ,
			Host:     &host,
			Port:     &port,
			SecretID: &secretID,
		}}, nil
	}
	store := secretStoreStub{
		status: ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  "windows",
			Backend:   "dpapi-user",
			Reason:    "no disponible",
		},
	}

	transport := transportWithDependencies("/config", load, store)
	req, _ := http.NewRequest(http.MethodGet, "https://servicio.example", nil)
	if got, err := transport.Proxy(req); err == nil || got != nil {
		t.Fatalf("Proxy() = %#v, %v; debe fallar cerrado", got, err)
	}
}

func TestTransportWithDependencies_ErrorDeSettingsFallaCerrado(t *testing.T) {
	t.Parallel()

	load := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return ports.DocumentoConfiguracionUsuario{}, errors.New("settings corruptos")
	}
	transport := transportWithDependencies("/config", load, secretStoreStub{})
	req, _ := http.NewRequest(http.MethodGet, "https://servicio.example", nil)
	if got, err := transport.Proxy(req); err == nil || got != nil {
		t.Fatalf("Proxy() = %#v, %v; debe fallar cerrado", got, err)
	}
}

func TestTransportWithDependencies_NoneDeshabilitaProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy-entorno.local:8080")

	enabled := true
	typ := "none"
	load := func(context.Context, string) (ports.DocumentoConfiguracionUsuario, error) {
		return ports.DocumentoConfiguracionUsuario{Proxy: ports.ConfiguracionUsuarioProxy{
			Enabled: &enabled,
			Type:    &typ,
		}}, nil
	}
	transport := transportWithDependencies("/config", load, secretStoreStub{})
	if transport.Proxy != nil {
		t.Fatal("proxyType=none no debe conservar el proxy del entorno")
	}
}

func TestManualProxyURL_RechazaHostConUserInfo(t *testing.T) {
	t.Parallel()

	cfg := structRuntimeConfig("usuario@proxy.local")
	cfg.Password = []byte("secreto")
	password := cfg.Password
	if _, err := manualProxyURL(cfg); err == nil {
		t.Fatal("se esperaba rechazo de host con userinfo")
	}
	for i, b := range password {
		if b != 0 {
			t.Fatalf("password no borrado en byte %d", i)
		}
	}
}

func structRuntimeConfig(host string) proxysecretstore.RuntimeProxyConfig {
	return proxysecretstore.RuntimeProxyConfig{
		Enabled: true,
		Type:    "manual",
		Host:    host,
		Port:    3128,
	}
}
