// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/ports"
)

type proxySecretStoreMockREST struct {
	status ports.ProxySecretStoreStatus
	err    error
}

func (m proxySecretStoreMockREST) Store(context.Context, string, ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	return ports.ProxySecretDescriptor{}, errors.New("no debe invocarse Store en este test")
}

func (m proxySecretStoreMockREST) Load(context.Context, string) (ports.ProxySecretMaterial, error) {
	return ports.ProxySecretMaterial{}, errors.New("no debe invocarse Load en este test")
}

func (m proxySecretStoreMockREST) Delete(context.Context, string) error {
	return errors.New("no debe invocarse Delete en este test")
}

func (m proxySecretStoreMockREST) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	return m.status, m.err
}

func TestRoutes_ProxySecretStoreStatus(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithProxySecretStore(proxySecretStoreMockREST{
		status: ports.ProxySecretStoreStatus{
			Available: true,
			Platform:  "linux",
			Backend:   "secret-service",
		},
	})

	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/settings/proxy/secret-store/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	if resp["ok"] != true || resp["available"] != true || resp["platform"] != "linux" || resp["backend"] != "secret-service" {
		t.Fatalf("respuesta inesperada: %#v", resp)
	}
}

func TestRoutes_ProxySecretStoreStatusNoConfigurado(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)

	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/settings/proxy/secret-store/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	if resp["ok"] != true || resp["available"] != false || resp["reason"] != "proxy secret store no configurado" {
		t.Fatalf("respuesta inesperada: %#v", resp)
	}
}

func TestRoutes_ProxySecretStoreStatusBackendError(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithProxySecretStore(proxySecretStoreMockREST{
		err: errors.New("dbus no disponible"),
	})

	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/settings/proxy/secret-store/status", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
}
