// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package usersettings_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/ports"
)

func TestGuardarOpacidadLogoSello_RechazaFueraDeRangoSinSobrescribir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := usersettings.New(t.TempDir())
	if err := store.Guardar(ctx, map[string]any{"signSealLogoOpacityPercent": 30}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{-1, 101, 30.5, "30"} {
		if err := store.Guardar(ctx, map[string]any{"signSealLogoOpacityPercent": value}); err == nil {
			t.Fatalf("Guardar aceptó %#v", value)
		}
	}
	invalid := 101
	if err := store.GuardarDocumento(ctx, ports.DocumentoConfiguracionUsuario{
		PAdESVisible: ports.ConfiguracionUsuarioPAdESVisible{LogoOpacityPercent: &invalid},
	}); err == nil {
		t.Fatal("GuardarDocumento aceptó opacidad inválida")
	}
	data, err := store.Cargar(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if data["signSealLogoOpacityPercent"] != float64(30) {
		t.Fatalf("ajuste previo sobrescrito: %#v", data)
	}
}
