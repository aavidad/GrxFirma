// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/ports"
)

type proxySecretStoreMock struct {
	loadCalls []string
	material  ports.ProxySecretMaterial
	err       error
	status    ports.ProxySecretStoreStatus
	statusErr error
}

func (m *proxySecretStoreMock) Store(context.Context, string, ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	return ports.ProxySecretDescriptor{}, errors.New("not implemented")
}

func (m *proxySecretStoreMock) Load(_ context.Context, id string) (ports.ProxySecretMaterial, error) {
	m.loadCalls = append(m.loadCalls, id)
	if m.err != nil {
		return ports.ProxySecretMaterial{}, m.err
	}
	return m.material, nil
}

func (m *proxySecretStoreMock) Delete(context.Context, string) error {
	return errors.New("not implemented")
}

func (m *proxySecretStoreMock) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	if m.statusErr != nil {
		return ports.ProxySecretStoreStatus{}, m.statusErr
	}
	if m.status.Platform == "" && m.status.Backend == "" && !m.status.Available {
		return ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  "linux",
			Backend:   "secret-service",
		}, nil
	}
	return m.status, nil
}

func TestResolveRuntimeProxy_ManualConSecretoCargado(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	realm := "corp-proxy"
	store := &proxySecretStoreMock{
		status: ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  "linux",
			Backend:   "secret-service",
		},
		material: ports.ProxySecretMaterial{
			Realm:    "corp-proxy",
			Username: "alberto",
			Password: []byte("secreto"),
		},
	}

	cfg, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:      &enabled,
		Type:         &typ,
		Host:         &host,
		Port:         &port,
		SecretID:     &secretID,
		Realm:        &realm,
		ExcludedURLs: []string{"https://intra.local", "*.dipgra.es"},
	}, store)
	if err != nil {
		t.Fatalf("ResolveRuntimeProxy() error = %v", err)
	}
	if !cfg.Enabled || cfg.Type != "manual" || cfg.Host != "proxy.local" || cfg.Port != 3128 {
		t.Fatalf("cfg base inesperada: %+v", cfg)
	}
	if cfg.SecretID != "secret-1" || cfg.Realm != "corp-proxy" || cfg.Username != "alberto" || string(cfg.Password) != "secreto" {
		t.Fatalf("cfg secreto inesperada: %+v", cfg)
	}
	if len(cfg.ExcludedURLs) != 2 || cfg.ExcludedURLs[0] != "https://intra.local" || cfg.ExcludedURLs[1] != "*.dipgra.es" {
		t.Fatalf("excluded urls inesperadas: %#v", cfg.ExcludedURLs)
	}
	if len(store.loadCalls) != 1 || store.loadCalls[0] != "secret-1" {
		t.Fatalf("load calls inesperadas: %#v", store.loadCalls)
	}
}

func TestResolveRuntimeProxy_ManualSinSecretoNoCargaStore(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 8080
	store := &proxySecretStoreMock{}

	cfg, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled: &enabled,
		Type:    &typ,
		Host:    &host,
		Port:    &port,
	}, store)
	if err != nil {
		t.Fatalf("ResolveRuntimeProxy() error = %v", err)
	}
	if !cfg.Enabled || cfg.Type != "manual" || cfg.Host != "proxy.local" || cfg.Port != 8080 {
		t.Fatalf("cfg inesperada: %+v", cfg)
	}
	if cfg.Username != "" || len(cfg.Password) != 0 {
		t.Fatalf("no deberia haber credenciales cargadas: %+v", cfg)
	}
	if len(store.loadCalls) != 0 {
		t.Fatalf("load no deberia llamarse: %#v", store.loadCalls)
	}
}

func TestResolveRuntimeProxy_SistemaNoCargaSecretos(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "system"
	secretID := "secret-1"
	store := &proxySecretStoreMock{}

	cfg, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		SecretID: &secretID,
	}, store)
	if err != nil {
		t.Fatalf("ResolveRuntimeProxy() error = %v", err)
	}
	if !cfg.Enabled || cfg.Type != "system" {
		t.Fatalf("cfg inesperada: %+v", cfg)
	}
	if len(store.loadCalls) != 0 {
		t.Fatalf("load no deberia llamarse en proxy de sistema: %#v", store.loadCalls)
	}
}

func TestResolveRuntimeProxy_NoneDeshabilitaYTipoInvalidoFalla(t *testing.T) {
	t.Parallel()

	enabled := true
	none := "none"
	cfg, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled: &enabled,
		Type:    &none,
	}, &proxySecretStoreMock{})
	if err != nil {
		t.Fatalf("ResolveRuntimeProxy(none) error = %v", err)
	}
	if cfg.Enabled || cfg.Type != "none" {
		t.Fatalf("cfg none inesperada: %+v", cfg)
	}

	invalid := "proxy-inventado"
	_, err = ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled: &enabled,
		Type:    &invalid,
	}, &proxySecretStoreMock{})
	if err == nil || err.Error() != "proxysecretstore: tipo de proxy invalido" {
		t.Fatalf("error tipo invalido = %v", err)
	}
}

func TestResolveRuntimeProxy_SecretIDSinStoreFalla(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		Host:     &host,
		Port:     &port,
		SecretID: &secretID,
	}, nil)
	if !errors.Is(err, ErrRuntimeProxyStoreRequired) {
		t.Fatalf("error = %v, want ErrRuntimeProxyStoreRequired", err)
	}
}

func TestResolveRuntimeProxy_ManualInvalidoFalla(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	port := 0

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled: &enabled,
		Type:    &typ,
		Port:    &port,
	}, &proxySecretStoreMock{})
	if err == nil || err.Error() != "proxysecretstore: proxy manual sin host" {
		t.Fatalf("error = %v, want host error", err)
	}

	host := "proxy.local"
	_, err = ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled: &enabled,
		Type:    &typ,
		Host:    &host,
		Port:    &port,
	}, &proxySecretStoreMock{})
	if err == nil || err.Error() != "proxysecretstore: proxy manual sin puerto valido" {
		t.Fatalf("error = %v, want port error", err)
	}
}

func TestResolveRuntimeProxy_ValidaHostManual(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	port := 3128
	for _, host := range []string{
		"usuario@proxy.local",
		"proxy.local/ruta",
		"proxy.local\nHost: atacante",
		"proxy.local:8080",
	} {
		host := host
		_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
			Enabled: &enabled,
			Type:    &typ,
			Host:    &host,
			Port:    &port,
		}, &proxySecretStoreMock{})
		if err == nil || err.Error() != "proxysecretstore: host de proxy invalido" {
			t.Fatalf("host %q error = %v", host, err)
		}
	}

	ipv6 := "[2001:db8::1]"
	cfg, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled: &enabled,
		Type:    &typ,
		Host:    &ipv6,
		Port:    &port,
	}, &proxySecretStoreMock{})
	if err != nil {
		t.Fatalf("IPv6 valido rechazado: %v", err)
	}
	if cfg.Host != "2001:db8::1" {
		t.Fatalf("host IPv6 = %q", cfg.Host)
	}
}

func TestResolveRuntimeProxy_PropagaErrorDeCarga(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	store := &proxySecretStoreMock{err: errors.New("secret backend failure")}

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		Host:     &host,
		Port:     &port,
		SecretID: &secretID,
	}, store)
	if err == nil || err.Error() != "proxysecretstore: resolviendo secreto: secret backend failure" {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestResolveRuntimeProxy_RechazaSecretoDeOtroRealm(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	realm := "corp-proxy"
	password := []byte("secreto")
	store := &proxySecretStoreMock{
		material: ports.ProxySecretMaterial{
			Realm:    "otro-proxy",
			Username: "alberto",
			Password: password,
		},
	}

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		Host:     &host,
		Port:     &port,
		SecretID: &secretID,
		Realm:    &realm,
	}, store)
	if err == nil || err.Error() != "proxysecretstore: el realm del secreto no coincide con el proxy configurado" {
		t.Fatalf("error inesperado: %v", err)
	}
	for i, b := range password {
		if b != 0 {
			t.Fatalf("password no borrado en byte %d", i)
		}
	}
}

func TestResolveRuntimeProxy_RechazaSecretIDNoOpaco(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := `..\..\otro-usuario`

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		Host:     &host,
		Port:     &port,
		SecretID: &secretID,
	}, &proxySecretStoreMock{})
	if !errors.Is(err, ErrInvalidSecretID) {
		t.Fatalf("error = %v, want ErrInvalidSecretID", err)
	}
}

func TestResolveRuntimeProxy_BackendNoDisponibleFallaConErrorTipado(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	store := &proxySecretStoreMock{
		status: ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  "darwin",
			Backend:   "keychain",
			Reason:    "proxysecretstore: backend Keychain requiere build con cgo en macOS",
		},
	}

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		Host:     &host,
		Port:     &port,
		SecretID: &secretID,
	}, store)
	if !IsRuntimeProxyBackendUnavailable(err) {
		t.Fatalf("error = %v, want IsRuntimeProxyBackendUnavailable", err)
	}
	if len(store.loadCalls) != 0 {
		t.Fatalf("load no deberia llamarse con backend no disponible: %#v", store.loadCalls)
	}
}

func TestResolveRuntimeProxy_ErrorConsultandoEstadoNoDegradaATipadoBackend(t *testing.T) {
	t.Parallel()

	enabled := true
	typ := "manual"
	host := "proxy.local"
	port := 3128
	secretID := "secret-1"
	store := &proxySecretStoreMock{statusErr: errors.New("status backend failure")}

	_, err := ResolveRuntimeProxy(context.Background(), ports.ConfiguracionUsuarioProxy{
		Enabled:  &enabled,
		Type:     &typ,
		Host:     &host,
		Port:     &port,
		SecretID: &secretID,
	}, store)
	if err == nil || err.Error() != "proxysecretstore: consultando estado del backend seguro: status backend failure" {
		t.Fatalf("error inesperado: %v", err)
	}
	if IsRuntimeProxyBackendUnavailable(err) {
		t.Fatalf("error no deberia marcar backend unavailable: %v", err)
	}
}
