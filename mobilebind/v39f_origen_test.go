// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"encoding/json"
	"testing"

	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 39f: Android dice si la persona aceptó en el aviso de un bloqueo o en
// Preferencias. El recibo lo guarda con canal android. Un origen ausente o
// desconocido no activa nada.
func TestV39FAndroidGuardaElOrigenDeLaAceptacion(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	for _, origin := range []string{"aviso", "ajustes"} {
		f := &Facade{configDir: t.TempDir(), platform: "android"}
		raw, err := f.ActivateUnsafeExceptionJSON(mustJSON(t, map[string]any{
			"id": "tsa_http", "accepted": true, "acceptanceVersion": excepciones.AcceptanceVersion, "origin": origin,
		}))
		if err != nil {
			t.Fatal(origin, err)
		}
		var receipt excepciones.Acceptance
		if json.Unmarshal([]byte(raw), &receipt) != nil || receipt.Origin != origin || receipt.Channel != "android" {
			t.Fatalf("recibo %s", raw)
		}
	}
	for _, bad := range []map[string]any{
		{},
		{"origin": ""},
		{"origin": "linea_ordenes"},
		{"origin": "aceptado desde el aviso"},
		{"origin": "AVISO"},
	} {
		f := &Facade{configDir: t.TempDir(), platform: "android"}
		params := map[string]any{"id": "tsa_http", "accepted": true, "acceptanceVersion": excepciones.AcceptanceVersion}
		for k, v := range bad {
			params[k] = v
		}
		if _, err := f.ActivateUnsafeExceptionJSON(mustJSON(t, params)); err == nil {
			t.Fatalf("activó con %v", bad)
		}
		if excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), f.configDir)) == nil {
			t.Fatalf("quedó activa con %v", bad)
		}
	}
}
