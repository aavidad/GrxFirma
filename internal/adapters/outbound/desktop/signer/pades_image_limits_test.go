// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"math"
	"strings"
	"testing"

	pdfsign "github.com/digitorus/pdfsign/sign"
)

func TestDimensionesRasterSelloAcotadasSinReservarPixeles(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]float64{{220, 70}, {595.28, 841.89}, {1e12, 1e12}, {1e308, 1e308}, {1e308, 1}, {1, 1e308}, {math.SmallestNonzeroFloat64, 1}} {
		for _, minimum := range [][2]int{{1, 1}, {420, 96}} {
			w, h, err := dimensionesRasterSello(size[0], size[1], minimum[0], minimum[1])
			if err != nil {
				t.Fatal(size, minimum, err)
			}
			if w < 1 || h < 1 || w > maxSealRasterEdge || h > maxSealRasterEdge || w*h > maxSealRasterPixels {
				t.Fatalf("dimensiones fuera de límite para %v: %dx%d", size, w, h)
			}
		}
	}
	w, h, err := dimensionesRasterSello(220, 70, 1, 1)
	if err != nil || w != 660 || h != 210 {
		t.Fatalf("un sello normal conserva la resolución original: %dx%d, %v", w, h, err)
	}
}

func TestSelloRechazaGeometriaInvalidaAntesDeGenerarImagen(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"0", "-1", "NaN", "+Inf", "-Inf", "1e309", "no-numero"} {
		t.Run(raw, func(t *testing.T) {
			data := pdfsign.SignData{}
			err := aplicarOpcionesAparienciaPdfsign(&data, map[string]string{"visibleSeal": "true", "visibleSealRectW": raw})
			if err == nil || len(data.Appearance.Image) != 0 {
				t.Fatalf("geometría inválida no rechazada: %v", err)
			}
		})
	}
	if err := validarGeometriaSello(nil, math.MaxFloat64, 0, math.MaxFloat64, 1); err == nil {
		t.Fatal("la suma de coordenadas no puede ser infinita")
	}
}

func TestCabeceraImagenSelloSeValidaAntesDeDecodificarPixeles(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := decodificarImagenSello(buffer.Bytes()); err != nil {
		t.Fatal(err)
	}
	// Cambiar solo una cabecera de 1x1: nunca se construye una imagen enorme.
	for _, dimensions := range [][2]uint32{{maxSealImageEdge + 1, 1}, {8192, 8192}} {
		headerOnly := append([]byte(nil), buffer.Bytes()...)
		binary.BigEndian.PutUint32(headerOnly[16:20], dimensions[0])
		binary.BigEndian.PutUint32(headerOnly[20:24], dimensions[1])
		binary.BigEndian.PutUint32(headerOnly[29:33], crc32.ChecksumIEEE(headerOnly[12:29]))
		if _, err := decodificarImagenSello(headerOnly); err == nil || !strings.Contains(err.Error(), "dimensiones") {
			t.Fatalf("esperada denegación por cabecera antes del decoder: %v", err)
		}
	}
	if err := validarPixelesSello(math.MaxInt, math.MaxInt, maxSealImageEdge, maxSealImagePixels); err == nil {
		t.Fatal("dimensiones de cabecera desbordadas no rechazadas")
	}
}

func TestImagenSelloInvalidaNoSeIgnoraSilenciosamente(t *testing.T) {
	t.Parallel()
	data := pdfsign.SignData{}
	err := aplicarOpcionesAparienciaPdfsign(&data, map[string]string{
		"visibleSeal": "true", "visibleSealImageBase64": base64.StdEncoding.EncodeToString([]byte("imagen sintética no válida")),
	})
	if err == nil || len(data.Appearance.Image) != 0 {
		t.Fatalf("una imagen inválida no debe convertirse en firma sin el sello pedido: %v", err)
	}
}
