// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func operarXAdES(t *testing.T, datos []byte, mime string, accion domain.SignatureAction, opciones map[string]string, nombre string) []byte {
	t.Helper()
	priv, cert := certForTest(t, nombre)
	doc, err := domain.NewDocument("entrada", datos, mime)
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: accion, Options: opciones,
	}, &LocalSigningKey{ID: nombre, Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("%s (%s): %v", accion, nombre, err)
	}
	return res.Data
}

func exigirFirmantesXAdES(t *testing.T, firmado []byte, esperados int) {
	t.Helper()
	doc, _ := domain.NewDocument("f.xsig", firmado, "application/xml")
	vr, firmantes, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificación: %v\n%s", err, firmado)
	}
	if !vr.Valid || len(firmantes) != esperados {
		t.Fatalf("valid=%v firmantes=%d (want %d): %s %v\n%s", vr.Valid, len(firmantes), esperados, vr.Reason, vr.Integrity.Details, firmado)
	}
}

// Igual que AutoFirma Java: cofirma y contrafirma (a las hojas) sobre todas
// las variantes XAdES, verificables de forma autónoma.
func TestXAdES_CofirmaYContrafirmaEnTodasLasVariantes(t *testing.T) {
	casos := []struct {
		nombre, mime, formato string
		datos                 []byte
	}{
		{"detached texto", "text/plain", "XAdES Detached", []byte("Texto de prueba XAdES")},
		{"detached XML", "application/xml", "XAdES Detached", []byte(`<doc><a>1</a></doc>`)},
		{"enveloping binario", "application/pdf", "XAdES Enveloping", []byte("%PDF-1.4 datos\x00")},
		{"enveloping XML", "text/xml", "XAdES Enveloping", []byte(`<doc xmlns="urn:x"><a>1</a></doc>`)},
		{"enveloped", "application/xml", "XAdES Enveloped", []byte(`<?xml version="1.0"?><f:factura xmlns:f="urn:f"><f:total>10</f:total></f:factura>`)},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			firma := operarXAdES(t, c.datos, c.mime, domain.ActionSign, map[string]string{"format": c.formato}, "A")
			exigirFirmantesXAdES(t, firma, 1)
			cofirma := operarXAdES(t, firma, "application/xml", domain.ActionCoSign, map[string]string{"algorithm": "SHA512withRSA"}, "B")
			exigirFirmantesXAdES(t, cofirma, 2)
			contrafirma := operarXAdES(t, cofirma, "application/xml", domain.ActionCounterSign, nil, "C")
			exigirFirmantesXAdES(t, contrafirma, 4)
			if n := strings.Count(string(contrafirma), "CounterSignature>"); n != 4 {
				t.Fatalf("etiquetas CounterSignature = %d, want 4 (2 contrafirmas)", n)
			}
			doble := operarXAdES(t, contrafirma, "application/xml", domain.ActionCounterSign, nil, "D")
			exigirFirmantesXAdES(t, doble, 6)
		})
	}
}

func TestXAdES_CofirmaRechazaDatosSinFirma(t *testing.T) {
	priv, cert := certForTest(t, "X")
	doc, _ := domain.NewDocument("a.xml", []byte(`<a/>`), "application/xml")
	for _, accion := range []domain.SignatureAction{domain.ActionCoSign, domain.ActionCounterSign} {
		_, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
			Document: doc, Format: domain.FormatXAdES, Action: accion,
		}, &LocalSigningKey{ID: "x", Signer: priv, Certificate: cert})
		if err == nil {
			t.Fatalf("%s sobre un XML sin firmas debe fallar", accion)
		}
	}
}
