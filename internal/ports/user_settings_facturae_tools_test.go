// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "testing"

func TestFacturaeToolsEnabledEsPreferenciaDesktopTipada(t *testing.T) {
	doc := DocumentoConfiguracionUsuarioDesdeMapa(map[string]any{
		"facturaeToolsEnabled": true,
		"otraClave":            "se conserva",
	})

	if doc.Desktop.FacturaeToolsEnabled == nil ||
		!*doc.Desktop.FacturaeToolsEnabled {
		t.Fatal("facturaeToolsEnabled no se tipó como preferencia desktop")
	}
	if _, existe := doc.Extras["facturaeToolsEnabled"]; existe {
		t.Fatal("facturaeToolsEnabled no debe quedar duplicada en Extras")
	}
	if doc.Extras["otraClave"] != "se conserva" {
		t.Fatal("la migración no debe eliminar extras desconocidos")
	}

	salida := doc.Mapa()
	if enabled, ok := salida["facturaeToolsEnabled"].(bool); !ok || !enabled {
		t.Fatalf("roundtrip inesperado: %#v", salida["facturaeToolsEnabled"])
	}
}
