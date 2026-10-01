// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

func decodificarPrueba(t *testing.T, raw []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// El sello conserva la proporción pedida, también en vertical.
func TestSelloModerno_ConservaLaProporcion(t *testing.T) {
	for _, c := range [][2]float64{{240, 80}, {120, 260}, {150, 45}} {
		raw, err := generarImagenSelloPAdES(testInfoPAdES(), c[0], c[1], true, "https://sede.ejemplo/v", "", emblemaFirmaPNG, estiloTextoSello{})
		if err != nil {
			t.Fatal(err)
		}
		b := decodificarPrueba(t, raw).Bounds()
		if got, want := float64(b.Dx())/float64(b.Dy()), c[0]/c[1]; math.Abs(got-want)/want > 0.02 {
			t.Errorf("%vx%v: proporción %.3f, se esperaba %.3f", c[0], c[1], got, want)
		}
	}
}

// El emblema institucional se dibuja con sus colores corporativos.
func TestSelloModerno_EmblemaInstitucional(t *testing.T) {
	raw, err := generarImagenSelloPAdES(testInfoPAdES(), 260, 80, true, "", "", emblemaFirmaPNG, estiloTextoSello{})
	if err != nil {
		t.Fatal(err)
	}
	img := decodificarPrueba(t, raw)
	b := img.Bounds()
	verdes := 0
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X + b.Dx()/20; x < b.Min.X+b.Dx()/3; x += 2 {
			r, g, bb, a := img.At(x, y).RGBA()
			if a > 0xC000 && g > 0xB000 && r > 0x9000 && r < 0xC000 && bb < 0x7000 {
				verdes++
			}
		}
	}
	if verdes < 50 {
		t.Fatalf("no se aprecia el emblema institucional (%d píxeles verdes)", verdes)
	}
}

// Con un giro libre las esquinas quedan transparentes y el centro no.
func TestSelloModerno_GiroLibre(t *testing.T) {
	raw, err := generarImagenSelloPAdES(testInfoPAdES(), 300, 150, true, "", "", nil, estiloTextoSello{})
	if err != nil {
		t.Fatal(err)
	}
	girada, err := girarImagenPNGLibre(raw, 20)
	if err != nil {
		t.Fatal(err)
	}
	img := decodificarPrueba(t, girada)
	b := img.Bounds()
	if _, _, _, a := img.At(b.Min.X+2, b.Min.Y+2).RGBA(); a != 0 {
		t.Error("la esquina de un sello girado debe ser transparente")
	}
	if _, _, _, a := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA(); a == 0 {
		t.Error("el centro del sello girado no puede ser transparente")
	}
}

// El giro libre debe ir en el mismo sentido (horario) que los giros rápidos:
// una marca en la esquina superior izquierda acaba arriba a la derecha.
func TestSelloModerno_GiroLibreMismoSentidoQueNoventa(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	for _, grados := range []float64{89, 90} {
		var img image.Image
		if grados == 90 {
			img = rotarImagen(src, 90)
		} else {
			img = rotarImagenLibre(src, grados, 100, 100)
		}
		if r, _, _, a := img.At(85, 12).RGBA(); a == 0 || r < 0x8000 {
			t.Errorf("giro %.0f°: la marca debería quedar arriba a la derecha", grados)
		}
		if _, _, _, a := img.At(12, 12).RGBA(); a != 0 {
			if r, g, _, _ := img.At(12, 12).RGBA(); r > 0x8000 && g < 0x4000 {
				t.Errorf("giro %.0f°: la marca no debería seguir arriba a la izquierda", grados)
			}
		}
	}
}
