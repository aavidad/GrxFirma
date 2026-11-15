// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 39f: la interfaz dice si la persona aceptó en el aviso de un bloqueo o
// en el apartado Seguridad, y el recibo lo guarda. Sin origen, con uno
// desconocido o con un reintento que no viene de un aviso, no se activa nada.
func TestV39FActivarPorIPCGuardaElOrigenDeLaInterfaz(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	for _, origin := range []string{"aviso", "ajustes"} {
		t.Run(origin, func(t *testing.T) {
			dir := t.TempDir()
			m := &Manejador{ConfigDir: dir, Loc: localizador.Para("es")}
			ctx := conEstadoInterfaz(context.Background(), interfazVerificada)
			raw, _ := json.Marshal(map[string]any{"id": "tsa_http", "accepted": true,
				"acceptanceVersion": excepciones.AcceptanceVersion, "origin": origin})
			r := m.despachar(ctx, peticion{Action: "unsafe_exceptions_activate", Params: raw})
			if !r.OK {
				t.Fatalf("activación: %+v", r)
			}
			receipt, ok := r.Data.(excepciones.Acceptance)
			if !ok || receipt.Origin != origin || receipt.Channel != "desktop" {
				t.Fatalf("recibo: %#v", r.Data)
			}
			var listed *excepciones.Acceptance
			for _, s := range excepciones.ListAt(dir) {
				if s.ID == "tsa_http" {
					listed = s.LastAcceptance
				}
			}
			if listed == nil || listed.Origin != origin {
				t.Fatalf("la lista no da el origen guardado: %+v", listed)
			}
		})
	}
}

func TestV39FActivarPorIPCRechazaOrigenAusenteODesconocido(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	for name, extra := range map[string]map[string]any{
		"sin origen":                  {},
		"vacío":                       {"origin": ""},
		"línea de órdenes":            {"origin": "linea_ordenes"},
		"texto antiguo":               {"origin": "aceptado desde el aviso"},
		"canal":                       {"origin": "desktop"},
		"no es texto":                 {"origin": 1},
		"reintento desde los ajustes": {"origin": "ajustes", "retryId": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			m := &Manejador{ConfigDir: dir, Loc: localizador.Para("es")}
			ctx := conEstadoInterfaz(context.Background(), interfazVerificada)
			params := map[string]any{"id": "tsa_http", "accepted": true, "acceptanceVersion": excepciones.AcceptanceVersion}
			for k, v := range extra {
				params[k] = v
			}
			raw, _ := json.Marshal(params)
			r := m.despachar(ctx, peticion{Action: "unsafe_exceptions_activate", Params: raw})
			if r.OK {
				t.Fatalf("activó con %v", extra)
			}
			if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
				t.Fatalf("escribió ajustes: %v", err)
			}
			if excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), dir)) == nil {
				t.Fatal("la excepción quedó activa")
			}
		})
	}
}
