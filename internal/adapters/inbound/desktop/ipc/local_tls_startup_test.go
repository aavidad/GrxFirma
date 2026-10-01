// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"testing"
)

func TestLocalTLSStartupStatus_ConsultaResultadoYEstadoInicial(t *testing.T) {
	manager := &Manejador{}
	server := New(manager)
	query := func() LocalTLSStartupStatus {
		t.Helper()
		response := manager.despachar(context.Background(), peticion{Action: "local_tls_startup_status"})
		if !response.OK || response.Action != "local_tls_startup_status" {
			t.Fatalf("respuesta inesperada: %+v", response)
		}
		status, ok := response.Data.(LocalTLSStartupStatus)
		if !ok {
			t.Fatalf("tipo de respuesta: %T", response.Data)
		}
		return status
	}
	if got := query(); got.State != "unknown" || got.Changed {
		t.Fatalf("estado inicial: %+v", got)
	}
	server.SetLocalTLSStartupStatus(LocalTLSStartupStatus{State: "ready", Changed: true})
	if got := query(); got.State != "ready" || !got.Changed {
		t.Fatalf("instalación comunicada: %+v", got)
	}
	if got := query(); got.State != "ready" || got.Changed {
		t.Fatalf("el aviso de instalación se repite: %+v", got)
	}
	server.SetLocalTLSStartupStatus(LocalTLSStartupStatus{State: "ready", Changed: true})
	refreshes := 0
	server.SetLocalTLSStartupRefresh(func(context.Context) LocalTLSStartupStatus {
		refreshes++
		return LocalTLSStartupStatus{State: "ready"}
	})
	if got := query(); got.State != "ready" || !got.Changed || refreshes != 1 {
		t.Fatalf("revalidación conserva el aviso inicial: status=%+v refreshes=%d", got, refreshes)
	}
	if got := query(); got.State != "ready" || got.Changed || refreshes != 2 {
		t.Fatalf("una segunda Qt no recibe el aviso antiguo: status=%+v refreshes=%d", got, refreshes)
	}
	server.SetLocalTLSStartupRefresh(func(context.Context) LocalTLSStartupStatus {
		return LocalTLSStartupStatus{State: "error"}
	})
	if got := query(); got.State != "error" || got.Changed {
		t.Fatalf("error de revalidación comunicado: %+v", got)
	}
	server.SetLocalTLSStartupRefresh(nil)
	server.SetLocalTLSStartupStatus(LocalTLSStartupStatus{State: "error", Changed: false})
	if got := query(); got.State != "error" || got.Changed {
		t.Fatalf("error comunicado: %+v", got)
	}
	server.SetLocalTLSStartupStatus(LocalTLSStartupStatus{State: "otro", Changed: true})
	if got := query(); got.State != "unknown" || got.Changed {
		t.Fatalf("estado inválido aceptado: %+v", got)
	}
}
