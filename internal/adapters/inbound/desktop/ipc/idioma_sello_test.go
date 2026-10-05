// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"testing"

	"grxfirma/internal/ports"
)

func TestIdiomaSelloConfiguradoPrevaleceSobreElDeLaInterfaz(t *testing.T) {
	conSello := map[string]string{"visibleSeal": "true", "sealLanguage": "en"}
	casos := []struct {
		nombre    string
		preferida any
		formato   string
		opciones  map[string]string
		quiere    string
		cargaDoc  bool
	}{
		{"idioma fijo", "es", "pades", conSello, "es", true},
		{"seguir a la interfaz", ports.IdiomaSelloInterfaz, "pades", conSello, "en", true},
		{"sin preferencia", nil, "pades", conSello, "en", true},
		{"valor raro se ignora", "../es", "pades", conSello, "en", true},
		{"sin sello no consulta ajustes", "es", "pades", map[string]string{"sealLanguage": "en"}, "en", false},
		{"otro formato", "es", "cades", conSello, "en", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			doc := ports.DocumentoConfiguracionUsuario{Extras: map[string]any{}}
			if c.preferida != nil {
				doc.Extras[ports.ClaveIdiomaSello] = c.preferida
			}
			ajustes := &stubTypedSettings{doc: doc}
			m := &Manejador{Settings: ajustes}
			got := m.aplicarIdiomaSelloConfigurado(context.Background(), c.formato, c.opciones)
			if got["sealLanguage"] != c.quiere {
				t.Fatalf("sealLanguage = %q; quiere %q", got["sealLanguage"], c.quiere)
			}
			if ajustes.usedLoadDoc != c.cargaDoc {
				t.Fatalf("usedLoadDoc = %t; quiere %t", ajustes.usedLoadDoc, c.cargaDoc)
			}
			if c.opciones["sealLanguage"] != "en" {
				t.Fatal("no debe modificar el mapa recibido")
			}
		})
	}
}
