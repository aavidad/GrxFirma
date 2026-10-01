// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

func firmarXMLDSig(t *testing.T, datos []byte, mime string, accion domain.SignatureAction, opciones map[string]string, nombre string) []byte {
	t.Helper()
	priv, cert := certForTest(t, nombre)
	doc, _ := domain.NewDocument("entrada", datos, mime)
	res, err := NewXMLDSigDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: formatXMLDSig, Action: accion, Options: opciones,
	}, &LocalSigningKey{ID: nombre, Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("%s: %v", accion, err)
	}
	return res.Data
}

func exigirXMLDSigValida(t *testing.T, firmado []byte, firmantes int) {
	t.Helper()
	doc, _ := domain.NewDocument("f.xml", firmado, "application/xml")
	vr, signers, err := NewXMLDSigVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil || !vr.Valid || len(signers) != firmantes {
		t.Fatalf("XMLDSig: err=%v valid=%v firmantes=%d (%s)\n%s", err, vr.Valid, len(signers), vr.Reason, firmado)
	}
}

func TestXMLDSig_VariantesDeJava(t *testing.T) {
	for _, c := range []struct {
		formato, mime string
		datos         []byte
	}{
		{"XMLDSig Enveloping", "application/pdf", []byte("%PDF binario")},
		{"XMLDSig Enveloping", "text/xml", []byte(`<a><b>1</b></a>`)},
		{"XMLDSig Enveloped", "application/xml", []byte(`<pedido><n>7</n></pedido>`)},
	} {
		t.Run(c.formato+" "+c.mime, func(t *testing.T) {
			firma := firmarXMLDSig(t, c.datos, c.mime, domain.ActionSign, map[string]string{"format": c.formato}, "A")
			if strings.Contains(string(firma), "QualifyingProperties") {
				t.Fatal("XMLDSig no debe llevar propiedades XAdES")
			}
			exigirXMLDSigValida(t, firma, 1)
			cofirma := firmarXMLDSig(t, firma, "application/xml", domain.ActionCoSign, map[string]string{"format": c.formato}, "B")
			exigirXMLDSigValida(t, cofirma, 2)
		})
	}
}

func TestXAdES_NodeToSign(t *testing.T) {
	xmlDoc := []byte(`<expediente><cabecera>libre</cabecera><doc Id="n1"><importe>10</importe></doc></expediente>`)
	for _, formato := range []string{"XAdES Enveloped", "XAdES Enveloping"} {
		t.Run(formato, func(t *testing.T) {
			firmado := operarXAdES(t, xmlDoc, "application/xml", domain.ActionSign, map[string]string{"format": formato, "nodeToSign": "n1"}, "Nodo")
			if !strings.Contains(string(firmado), `URI="#n1"`) {
				t.Fatalf("la referencia debe apuntar al nodo:\n%s", firmado)
			}
			exigirFirmantesXAdES(t, firmado, 1)
			fuera := bytes.Replace(firmado, []byte("libre"), []byte("otro!"), 1)
			exigirFirmantesXAdES(t, fuera, 1)
			dentro := bytes.Replace(firmado, []byte("<importe>10<"), []byte("<importe>99<"), 1)
			doc, _ := domain.NewDocument("f.xml", dentro, "application/xml")
			if vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{}); err == nil && vr.Valid {
				t.Fatal("alterar el nodo firmado debe invalidar la firma")
			}
		})
	}
	priv, cert := certForTest(t, "sin-nodo")
	doc, _ := domain.NewDocument("a.xml", xmlDoc, "application/xml")
	if _, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": "XAdES Enveloped", "nodeToSign": "no-existe"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert}); err == nil {
		t.Fatal("un nodeToSign inexistente debe rechazarse")
	}
}
