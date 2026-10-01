// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"testing"
)

func TestOpcionesSelloIPC_OpacidadLogo(t *testing.T) {
	t.Parallel()
	seal := map[string]any{"pageWidth": 595.0, "pageHeight": 842.0, "logoOpacityPercent": 30.0}
	got, err := construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", nil, "pades")
	if err != nil || got["visibleSealLogoOpacityPercent"] != "30" {
		t.Fatalf("opacidad no propagada: %#v, %v", got, err)
	}
	seal["logoOpacityPercent"] = 0.0
	got, err = construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", nil, "pades")
	if err != nil || got["visibleSealLogoOpacityPercent"] != "0" {
		t.Fatalf("cero no propagado: %#v, %v", got, err)
	}
	delete(seal, "logoOpacityPercent")
	got, err = construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", nil, "pades")
	if err != nil {
		t.Fatal(err)
	}
	if _, present := got["visibleSealLogoOpacityPercent"]; present {
		t.Fatal("la ausencia debe conservar el comportamiento anterior")
	}
	for _, value := range []any{-1.0, 101.0, 30.5, "30"} {
		seal["logoOpacityPercent"] = value
		if _, err := construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", nil, "pades"); err == nil {
			t.Fatalf("valor IPC inválido aceptado: %#v", value)
		}
	}
	delete(seal, "logoOpacityPercent")
	if _, err := construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", map[string]string{"visibleSealLogoOpacityPercent": "101"}, "pades"); err == nil {
		t.Fatal("extraOptions inválido aceptado")
	}
	if _, err := construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", map[string]string{
		"visibleSealLogoOpacityPercent": "30", "VISIBLESEALLOGOOPACITYPERCENT": "100",
	}, "pades"); err == nil {
		t.Fatal("extraOptions duplicado con distinta capitalización aceptado")
	}
}

func TestSealPreview_OpacidadLogoInvalida(t *testing.T) {
	t.Parallel()
	resp := manejadorVacio().despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{
		VisibleSeal: map[string]any{"pageWidth": 595.0, "pageHeight": 842.0, "logoOpacityPercent": 101},
	}))
	if resp.OK {
		t.Fatal("seal_preview aceptó opacidad inválida")
	}
}

func TestSealPreview_OpacidadLogoTreinta(t *testing.T) {
	t.Parallel()
	resp := manejadorVacio().despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{
		VisibleSeal:  map[string]any{"pageWidth": 595.0, "pageHeight": 842.0, "logoOpacityPercent": 30},
		ExtraOptions: map[string]string{"visibleSealLogo": "institucional"},
	}))
	if !resp.OK {
		t.Fatalf("seal_preview con 30 %%: %s", resp.Error)
	}
	data, ok := resp.Data.(resultadoVistaPreviaSello)
	if !ok || data.Image == "" {
		t.Fatalf("vista previa vacía: %#v", resp.Data)
	}
}

func TestSealPreview_OpacidadSelloSinLogoDesdeIPC(t *testing.T) {
	resp := manejadorVacio().despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{
		VisibleSeal: map[string]any{"pageWidth": 595.0, "pageHeight": 842.0, "logoOpacityPercent": 40},
	}))
	if !resp.OK {
		t.Fatalf("vista previa sin logo: %s", resp.Error)
	}
	data := resp.Data.(resultadoVistaPreviaSello)
	raw, err := base64.StdEncoding.DecodeString(data.Image)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var maxAlpha uint32
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > maxAlpha {
				maxAlpha = a
			}
		}
	}
	if maxAlpha < 25000 || maxAlpha > 27000 {
		t.Fatalf("IPC no propagó 40 %% al sello completo: alfa=%d", maxAlpha)
	}
}

func TestSaveSettings_OpacidadLogoFueraDeRangoNoSeGuarda(t *testing.T) {
	t.Parallel()
	store := &stubTypedSettings{}
	m := &Manejador{Settings: store}
	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"signSealLogoOpacityPercent": 101,
	}))
	if resp.OK || store.usedSaveDoc {
		t.Fatalf("ajuste inválido guardado: %#v", resp)
	}
	resp = m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"signSealLogoOpacityPercent": 0,
	}))
	if !resp.OK || !store.usedSaveDoc || store.lastSavedDoc.PAdESVisible.LogoOpacityPercent == nil || *store.lastSavedDoc.PAdESVisible.LogoOpacityPercent != 0 {
		t.Fatalf("opacidad cero no guardada: %#v, %#v", resp, store.lastSavedDoc.PAdESVisible.LogoOpacityPercent)
	}
}
