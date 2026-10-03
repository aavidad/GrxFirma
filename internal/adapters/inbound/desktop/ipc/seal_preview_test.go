// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"math"
	"testing"
)

func TestDespachar_SealPreview_DevuelveElSelloReal(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{
		VisibleSeal: map[string]any{
			"page": "1", "x": 0.5, "y": 0.05, "w": 0.45, "h": 0.1,
			"pageWidth": 595.0, "pageHeight": 842.0, "rotation": 0, "keepText": true,
		},
		QRContent:    "https://sede.ejemplo/verificar",
		ExtraOptions: map[string]string{"visibleSealLogo": "institucional"},
	}))
	if !resp.OK {
		t.Fatalf("seal_preview: %s", resp.Error)
	}
	datos, ok := resp.Data.(resultadoVistaPreviaSello)
	if !ok {
		t.Fatalf("datos inesperados: %T", resp.Data)
	}
	raw, err := base64.StdEncoding.DecodeString(datos.Image)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if got, want := float64(b.Dx())/float64(b.Dy()), (0.45*595)/(0.1*842); math.Abs(got-want)/want > 0.03 {
		t.Fatalf("proporción %.3f, se esperaba %.3f", got, want)
	}
}

func TestDespachar_SealPreview_SinSelloEsError(t *testing.T) {
	t.Parallel()
	if resp := manejadorVacio().despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{})); resp.OK {
		t.Fatal("sin datos del sello debe fallar")
	}
}

func TestDespachar_SealPreview_UsaNombreElegidoPorElPortal(t *testing.T) {
	if got := nombreFirmanteVistaPrevia("", " Nombre elegido ", "Ejemplo"); got != "Nombre elegido" {
		t.Fatalf("nombre=%q", got)
	}
	if got := nombreFirmanteVistaPrevia("cert-id", "Nombre elegido", "Certificado real"); got != "Certificado real" {
		t.Fatalf("prioridad=%q", got)
	}
	m := manejadorVacio()
	seal := map[string]any{"page": "1", "x": 0.5, "y": 0.05, "w": 0.4, "h": 0.1,
		"pageWidth": 595.0, "pageHeight": 842.0, "rotation": 0, "keepText": true}
	preview := func(name string) string {
		resp := m.despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{VisibleSeal: seal, SignerName: name}))
		if !resp.OK {
			t.Fatalf("seal_preview: %s", resp.Error)
		}
		return resp.Data.(resultadoVistaPreviaSello).Image
	}
	if preview("Nombre elegido") == preview("Otro certificado") {
		t.Fatal("el nombre elegido no cambió el sello previsualizado")
	}
}

// El PNG conserva la tarjeta y ocupa la caja envolvente de la anotación PDF.
func TestDespachar_SealPreview_GeometriaRotada(t *testing.T) {
	for _, rotation := range []int{0, 15, 30, 45, 80, 90, 180, 270} {
		t.Run(fmt.Sprint(rotation), func(t *testing.T) {
			resp := manejadorVacio().despachar(context.Background(), peticionJSON(t, "seal_preview", paramsVistaPreviaSello{
				VisibleSeal: map[string]any{
					"page": "1", "x": 0.5, "y": 0.05, "w": 0.4, "h": 0.1,
					"pageWidth": 600.0, "pageHeight": 800.0, "rotation": rotation, "keepText": true,
				},
			}))
			if !resp.OK {
				t.Fatalf("seal_preview: %s", resp.Error)
			}
			datos := resp.Data.(resultadoVistaPreviaSello)
			raw, err := base64.StdEncoding.DecodeString(datos.Image)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			b := img.Bounds()
			r := float64(rotation) * math.Pi / 180
			want := (240*math.Abs(math.Cos(r)) + 80*math.Abs(math.Sin(r))) /
				(240*math.Abs(math.Sin(r)) + 80*math.Abs(math.Cos(r)))
			if math.Abs(float64(b.Dx())/float64(b.Dy())-want) > 0.04 {
				t.Fatalf("giro %d: PNG %dx%d; se espera proporción %.3f", rotation, b.Dx(), b.Dy(), want)
			}
		})
	}
}
