// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

// Prueba de regresión de E2-04: la imagen del sello que manda el portal se
// rechaza por sus dimensiones antes de decodificarla.

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image/png"
	"strings"
	"testing"
)

// pngDeclarado devuelve un PNG cuya cabecera declara ancho x alto píxeles
// con datos mínimos: pesa unos bytes y decodificado ocuparía gigabytes.
func pngDeclarado(t *testing.T, ancho, alto uint32) []byte {
	t.Helper()
	data := pngPequeno(t)
	binary.BigEndian.PutUint32(data[16:20], ancho)
	binary.BigEndian.PutUint32(data[20:24], alto)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestPAdESLimites_ImagenDelSelloConDimensionesEnormes(t *testing.T) {
	img := pngDeclarado(t, 30000, 30000)
	if cfg, err := png.DecodeConfig(bytes.NewReader(img)); err != nil || cfg.Width != 30000 {
		t.Fatalf("PNG de prueba mal construido: %v %+v", err, cfg)
	}
	err := firmarConPlazo(t, pdfConCount(1, 0), opcionesImagen(img, "1"))
	if err == nil || !strings.Contains(err.Error(), "dimensiones permitidas") {
		t.Fatalf("se esperaba el rechazo por dimensiones de la imagen: %v", err)
	}
}

// Con una imagen normal la firma con estampado sigue funcionando.
func TestPAdESLimites_ImagenNormalSeEstampa(t *testing.T) {
	if err := firmarConPlazo(t, pdfConCount(1, 0), opcionesImagen(pngPequeno(t), "1")); err != nil {
		t.Fatalf("firma con imagen pequeña: %v", err)
	}
}
