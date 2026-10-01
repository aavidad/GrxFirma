// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "testing"

func TestCheckForUpdatesEsPreferenciaGeneralTipada(t *testing.T) {
	doc := DocumentoConfiguracionUsuarioDesdeMapa(map[string]any{
		"checkForUpdates": false,
		"otraClave":       "se conserva",
	})
	if doc.General.CheckForUpdates == nil || *doc.General.CheckForUpdates {
		t.Fatalf("CheckForUpdates = %#v; want false", doc.General.CheckForUpdates)
	}
	if _, presente := doc.Extras["checkForUpdates"]; presente {
		t.Fatal("checkForUpdates no debe permanecer duplicado en Extras")
	}
	mapa := doc.Mapa()
	if valor, ok := mapa["checkForUpdates"].(bool); !ok || valor {
		t.Fatalf("mapa checkForUpdates = %#v; want false", mapa["checkForUpdates"])
	}
	if mapa["otraClave"] != "se conserva" {
		t.Fatal("la proyección tipada no debe borrar claves compatibles")
	}
}
