// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/exttools"
)

// TestMotorFirmaGo_PAdESVisible_CorpusVariado firma con sello visible un corpus
// de PDFs estructuralmente diversos (multipágina, con imágenes y ya firmados)
// y exige que el resultado sea estructuralmente válido según
// qpdf y contenga exactamente una firma nueva según pdfsig.
//
// El corpus cubre la colocación en una página y en todas las páginas,
// así como la firma incremental de un documento ya firmado.
func TestMotorFirmaGo_PAdESVisible_CorpusVariado(t *testing.T) {
	fixtures := []struct {
		nombre string
		ruta   string
	}{
		{"propio_multipagina", filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2.pdf")},
		{"firmado_fnmt_pruebas", filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_signed.pdf")},
		{"multipagina", filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "multiple_pages.pdf")},
		{"simple_1", filepath.Join("..", "..", "..", "..", "..", "test", "prueba1.pdf")},
		{"simple_2", filepath.Join("..", "..", "..", "..", "..", "test", "prueba2.pdf")},
		{"simple_3", filepath.Join("..", "..", "..", "..", "..", "test", "prueba3.pdf")},
		{"simple_ya_firmado", filepath.Join("..", "..", "..", "..", "..", "test", "prueba1_signed.pdf")},
	}
	modos := []struct {
		nombre string
		page   string
	}{
		{"pagina_1", "1"},
		{"todas_las_paginas", "all"},
	}

	priv, cert := generarCertRSA(t)
	clave := signer.NuevaClaveLocal(priv, cert)
	motor := signer.NuevoMotorFirmaGo(nil)

	for _, fixture := range fixtures {
		original, err := os.ReadFile(fixture.ruta)
		if err != nil {
			if os.IsNotExist(err) && filepath.Base(fixture.ruta) == "2_signed.pdf" {
				t.Run(fixture.nombre, func(t *testing.T) {
					t.Skip("fixture 2_signed.pdf pendiente: firmar el 2.pdf sintético con el certificado FNMT de pruebas")
				})
				continue
			}
			t.Fatalf("no se pudo leer el fixture %s: %v", fixture.ruta, err)
		}
		firmasPrevias := bytes.Count(original, []byte("/ByteRange"))
		for _, modo := range modos {
			t.Run(fixture.nombre+"/"+modo.nombre, func(t *testing.T) {
				doc, err := domain.NewDocument(filepath.Base(fixture.ruta), original, "application/pdf")
				if err != nil {
					t.Fatalf("NewDocument() error = %v", err)
				}
				job := domain.SignatureJob{
					Document: doc,
					Format:   domain.FormatPAdES,
					Action:   domain.ActionSign,
					Options: map[string]string{
						"visibleSeal":      "true",
						"visibleSealRectX": "36",
						"visibleSealRectY": "36",
						"visibleSealRectW": "220",
						"visibleSealRectH": "70",
						"page":             modo.page,
					},
				}
				resultado, err := motor.Sign(context.Background(), job, clave)
				if err != nil {
					t.Fatalf("Sign devolvió error inesperado: %v", err)
				}
				if !bytes.HasPrefix(resultado.Data, []byte("%PDF-")) {
					t.Fatal("la salida debe seguir siendo un PDF real")
				}

				signedPath := filepath.Join(t.TempDir(), "firmado.pdf")
				if err := os.WriteFile(signedPath, resultado.Data, 0o600); err != nil {
					t.Fatalf("no se pudo escribir el PDF firmado temporal: %v", err)
				}

				if exttools.Available(t, "qpdf") {
					salida, err := exec.Command("qpdf", "--check", signedPath).CombinedOutput()
					if err != nil {
						t.Fatalf("qpdf --check falló (el fixture original estaba limpio): %v\n%s", err, salida)
					}
				}

				// Conteo estructural de firmas independiente de la versión de
				// pdfsig: cada diccionario de firma tiene exactamente un
				// /ByteRange. Debe haber una firma nueva respecto al original.
				gotSigs := bytes.Count(resultado.Data, []byte("/ByteRange"))
				wantSigs := firmasPrevias + 1
				if gotSigs != wantSigs {
					t.Fatalf("el PDF firmado tiene %d diccionarios de firma (/ByteRange), se esperaban %d (previas %d + 1 nueva)", gotSigs, wantSigs, firmasPrevias)
				}

				if exttools.Available(t, "pdfsig") {
					salida, _ := exec.Command("pdfsig", signedPath).CombinedOutput()
					out := string(salida)
					// "Impossible" delata múltiples campos /FT /Sig apuntando al
					// mismo /V (el bug de AllPages). El resto de la salida de
					// pdfsig depende de la versión de poppler: las versiones
					// antiguas no resuelven /V cuando está en el campo padre con
					// widgets /Kids, así que no exigimos aquí su validación
					// criptográfica (la cubren los tests de página única y qpdf).
					if strings.Contains(out, "Impossible") {
						t.Fatalf("pdfsig reporta firmas inconsistentes:\n%s", out)
					}
				}
			})
		}
	}
}
