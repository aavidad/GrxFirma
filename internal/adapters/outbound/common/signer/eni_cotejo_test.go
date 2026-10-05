// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

func TestCotejarCAdESExplicitaConElOriginal(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	original := []byte("%PDF-1.7\ncontenido del acta")
	doc, _ := domain.NewDocument("acta.pdf", original, "application/pdf")
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign, Options: map[string]string{},
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	if err := commonsigner.CotejarCAdESExplicita(res.Data, original); err != nil {
		t.Fatalf("la firma de este original debe cotejar: %v", err)
	}
	otro := []byte("%PDF-1.7\ncontenido de otra acta")
	for nombre, datos := range map[string][]byte{"otro documento": otro, "vacío": nil, "un byte cambiado": append(bytes.Clone(original[:len(original)-1]), 'X')} {
		if err := commonsigner.CotejarCAdESExplicita(res.Data, datos); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	if err := commonsigner.CotejarCAdESExplicita([]byte{0x30, 0x03, 0x02, 0x01, 0x01}, original); err == nil {
		t.Error("CMS no válido aceptado")
	}
}

func TestCotejarXAdESSeparadaConElOriginal(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	casos := map[string]struct {
		nombre, mime string
		datos, otro  []byte
	}{
		// Texto: el contenido va en Base64 dentro de CONTENT (modo Java).
		"contenido interno": {"acta.txt", "text/plain", []byte("acta de la sesión"), []byte("acta de otra sesión")},
		// XML: la referencia apunta al documento externo.
		"referencia externa": {"datos.xml", "application/xml", []byte(`<?xml version="1.0"?><datos><importe>10</importe></datos>`), []byte(`<datos><importe>99</importe></datos>`)},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			doc, _ := domain.NewDocument(c.nombre, c.datos, c.mime)
			res, err := commonsigner.NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
				Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign, Options: map[string]string{},
			}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
			if err != nil {
				t.Fatal(err)
			}
			if err := commonsigner.CotejarXAdESSeparada(res.Data, c.datos); err != nil {
				t.Fatalf("la firma de este original debe cotejar: %v", err)
			}
			if err := commonsigner.CotejarXAdESSeparada(res.Data, c.otro); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
				t.Fatalf("original ajeno aceptado: %v", err)
			}
		})
	}
}

func TestComprobarIntegridadPAdESSinRed(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	if err := commonsigner.ComprobarIntegridadPAdES(context.Background(), firmado); err != nil {
		t.Fatalf("PDF firmado intacto rechazado: %v", err)
	}
	alterado := bytes.Clone(firmado)
	i := bytes.Index(alterado, []byte("/Type"))
	if i < 0 {
		t.Fatal("no se encontró /Type en el PDF")
	}
	alterado[i+1] = 'X'
	if err := commonsigner.ComprobarIntegridadPAdES(context.Background(), alterado); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
		t.Fatalf("PDF alterado aceptado: %v", err)
	}
}
