// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "testing"

func TestOpacidadLogoSello_ProyeccionYValidacion(t *testing.T) {
	t.Parallel()
	for _, value := range []int{0, 30, 100} {
		input := map[string]any{"signSealLogoOpacityPercent": value}
		if err := ValidarOpacidadLogoSello(input); err != nil {
			t.Fatalf("opacidad %d rechazada: %v", value, err)
		}
		doc := DocumentoConfiguracionUsuarioDesdeMapa(input)
		if doc.PAdESVisible.LogoOpacityPercent == nil || *doc.PAdESVisible.LogoOpacityPercent != value {
			t.Fatalf("opacidad %d no proyectada: %#v", value, doc.PAdESVisible.LogoOpacityPercent)
		}
		if got := doc.Mapa()["signSealLogoOpacityPercent"]; got != value {
			t.Fatalf("opacidad %d no conservada: %#v", value, got)
		}
	}
	if doc := DocumentoConfiguracionUsuarioDesdeMapa(nil); doc.PAdESVisible.LogoOpacityPercent != nil {
		t.Fatal("ausencia debe conservar el valor predeterminado del motor")
	}
	for _, value := range []any{-1, 101, 30.5, "30", true} {
		if err := ValidarOpacidadLogoSello(map[string]any{"signSealLogoOpacityPercent": value}); err == nil {
			t.Fatalf("valor inválido aceptado: %#v", value)
		}
	}
}
