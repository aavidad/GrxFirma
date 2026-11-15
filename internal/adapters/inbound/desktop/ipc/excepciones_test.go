// SPDX-License-Identifier: EUPL-1.2
package ipc

import (
	"context"
	"encoding/json"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"testing"
)

func TestUnsafeExceptionsIPCListSetAndPolicyBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	m := &Manejador{Loc: localizador.Para("es")}
	call := func(action, raw string) respuesta {
		return m.despachar(conEstadoInterfaz(context.Background(), interfazVerificada), peticion{Action: action, Params: json.RawMessage(raw)})
	}
	list := call("unsafe_exceptions_list", "{}")
	if !list.OK || len(list.Data.([]excepciones.State)) != 26 {
		t.Fatalf("list: %+v", list)
	}
	changed := call("unsafe_exceptions_activate", `{"id":"tsa_http","accepted":true,"acceptanceVersion":"2026-11-15.1"}`)
	if !changed.OK || excepciones.Check("tsa_http") != nil {
		t.Fatalf("set: %+v", changed)
	}
	if call("unsafe_exceptions_set", `{"id":"tsa_http"}`).OK {
		t.Fatal("enabled ausente aceptado")
	}
	if call("unsafe_exceptions_set", `{"id":"desconocida","enabled":true}`).OK {
		t.Fatal("id desconocido aceptado")
	}
	machinepolicy.SetForTest(t, "permitir_tsa_http", false)
	blocked := call("unsafe_exceptions_activate", `{"id":"tsa_http","accepted":true,"acceptanceVersion":"2026-11-15.1"}`)
	if blocked.OK || blocked.ExceptionID != "tsa_http" || blocked.BlockKey != "excepciones.tsa_http.bloqueo" || blocked.ErrorCode != "unsafe_exception_blocked" || blocked.SettingKey == "" {
		t.Fatalf("bloqueo sin contrato: %+v", blocked)
	}
	if blocked.Error == blocked.BlockKey {
		t.Fatal("error sin traducción")
	}
}
