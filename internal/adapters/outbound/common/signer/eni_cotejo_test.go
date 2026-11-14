// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"bytes"
	"context"
	"encoding/base64"
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

// KD-02: una firma XAdES con el contenido dentro (AFIRMA/CONTENT, modo de
// AutoFirma) solo coteja con el original si cada firma lo referencia desde
// su ds:SignedInfo. Un CONTENT añadido fuera de lo firmado, con una
// «Reference» suelta que lo nombra, no hace que la firma ajena pase por
// firma del documento B.
func TestCotejarXAdESSeparadaRechazaContenidoNoFirmadoAnadido(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	documentoA := []byte("acta de la sesión del lunes")
	documentoB := []byte("reconozco una deuda de 90.000 euros")
	doc, _ := domain.NewDocument("acta.txt", documentoA, "text/plain")
	res, err := commonsigner.NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign, Options: map[string]string{},
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(res.Data, []byte("<AFIRMA>")) || !bytes.Contains(res.Data, []byte("</AFIRMA>")) {
		t.Fatalf("se esperaba el modo AFIRMA/CONTENT: %.200s", res.Data)
	}
	if err := commonsigner.CotejarXAdESSeparada(res.Data, documentoA); err != nil {
		t.Fatalf("la firma intacta debe cotejar con A: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(documentoB)
	variantes := map[string]string{
		"referencia_suelta":              `<X><Reference URI="#EVIL"/></X><CONTENT Id="EVIL" Encoding="Base64">` + b64 + `</CONTENT>`,
		"signedinfo_falso":               `<X><SignedInfo><Reference URI="#EVIL"/></SignedInfo></X><CONTENT Id="EVIL" Encoding="Base64">` + b64 + `</CONTENT>`,
		"signedinfo_dsig_fuera_de_firma": `<ds:SignedInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:Reference URI="#EVIL"/></ds:SignedInfo><CONTENT Id="EVIL" Encoding="Base64">` + b64 + `</CONTENT>`,
	}
	for nombre, anadido := range variantes {
		t.Run(nombre, func(t *testing.T) {
			atacada := bytes.Replace(res.Data, []byte("</AFIRMA>"), []byte(anadido+"</AFIRMA>"), 1)
			// La firma sigue verificando sola (cubre A): el ataque solo
			// funciona si el cotejo da B por firmado.
			if err := commonsigner.CotejarXAdESSeparada(atacada, documentoA); err != nil {
				t.Logf("A deja de cotejar en la variante (aceptable): %v", err)
			}
			if err := commonsigner.CotejarXAdESSeparada(atacada, documentoB); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
				t.Fatalf("el cotejo con B debe rechazarse: %v", err)
			}
		})
	}
}

// La cofirma de un AFIRMA/CONTENT añade otra firma que referencia el mismo
// CONTENT: sigue cotejando con el original.
func TestCotejarXAdESSeparadaAdmiteCofirma(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	original := []byte("acta de la sesión del martes")
	doc, _ := domain.NewDocument("acta.txt", original, "text/plain")
	clave := &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}
	res, err := commonsigner.NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign, Options: map[string]string{},
	}, clave)
	if err != nil {
		t.Fatal(err)
	}
	firmado, _ := domain.NewDocument("acta.xsig", res.Data, "application/xml")
	cofirma, err := commonsigner.NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: firmado, Format: domain.FormatXAdES, Action: domain.ActionCoSign, Options: map[string]string{},
	}, clave)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(cofirma.Data, []byte("<ds:SignedInfo")); n < 2 {
		t.Fatalf("se esperaban dos firmas, hay %d", n)
	}
	if err := commonsigner.CotejarXAdESSeparada(cofirma.Data, original); err != nil {
		t.Fatalf("la cofirma debe cotejar con el original: %v", err)
	}
	if err := commonsigner.CotejarXAdESSeparada(cofirma.Data, []byte("otra acta")); !errors.Is(err, commonsigner.ErrFirmaNoCorrespondeOriginal) {
		t.Fatalf("original ajeno aceptado: %v", err)
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
