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

func firmarXAdESVariante(t *testing.T, datos []byte, mime, formato string) []byte {
	t.Helper()
	priv, cert := certForTest(t, "XAdES-"+formato)
	doc, err := domain.NewDocument("entrada", datos, mime)
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": formato, "algorithm": "SHA512withRSA"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatalf("%s: %v", formato, err)
	}
	return res.Data
}

func verificarXAdESValida(t *testing.T, firmado []byte) domain.VerificationResult {
	t.Helper()
	doc, err := domain.NewDocument("firma.xsig", firmado, "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("verificación: %v\n%s", err, firmado)
	}
	return vr
}

// Variantes de AutoFirma Java: Enveloping (por defecto en Java) sobre XML y
// binario, y Enveloped sobre XML con espacios de nombres y raíz vacía.
func TestXAdES_EnvelopingYEnveloped(t *testing.T) {
	casos := []struct {
		nombre, formato, mime string
		datos                 []byte
		contiene              string
	}{
		{"enveloping XML", "XAdES Enveloping", "text/xml", []byte(`<?xml version="1.0"?><datos xmlns="urn:x"><a>1</a></datos>`), `<datos xmlns="urn:x">`},
		{"enveloping binario", "XAdES Enveloping", "application/pdf", []byte("%PDF-1.4 binario\x00\x01"), algBase64Transform},
		{"enveloped XML", "XAdES Enveloped", "application/xml", []byte("<?xml version=\"1.0\"?>\n<f:factura xmlns:f=\"urn:f\"><f:total>10</f:total></f:factura>\n"), `</ds:Signature></f:factura>`},
		{"enveloped raíz vacía", "XAdES Enveloped", "application/xml", []byte(`<vacio atributo="1"/>`), `</ds:Signature></vacio>`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			firmado := firmarXAdESVariante(t, c.datos, c.mime, c.formato)
			if !bytes.Contains(firmado, []byte(c.contiene)) {
				t.Fatalf("estructura inesperada, falta %q:\n%s", c.contiene, firmado)
			}
			if vr := verificarXAdESValida(t, firmado); !vr.Valid {
				t.Fatalf("firma no válida: %s\n%s", vr.Reason, firmado)
			}
		})
	}
}

func TestXAdES_EnvelopedDetectaManipulacion(t *testing.T) {
	firmado := firmarXAdESVariante(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", "XAdES Enveloped")
	alterado := bytes.Replace(firmado, []byte("<importe>10<"), []byte("<importe>99<"), 1)
	doc, _ := domain.NewDocument("f.xsig", alterado, "application/xml")
	vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil && vr.Valid {
		t.Fatal("una firma enveloped con el documento alterado no puede ser válida")
	}
}

func TestXAdES_EnvelopingBinarioDetectaManipulacion(t *testing.T) {
	firmado := firmarXAdESVariante(t, []byte("datos binarios originales"), "application/octet-stream", "XAdES Enveloping")
	i := bytes.Index(firmado, []byte(`Encoding="`+algBase64Transform+`">`)) + len(`Encoding="`+algBase64Transform+`">`)
	alterado := append([]byte(nil), firmado...)
	if alterado[i] == 'Z' {
		alterado[i] = 'Y'
	} else {
		alterado[i] = 'Z'
	}
	doc, _ := domain.NewDocument("f.xsig", alterado, "application/xml")
	vr, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil && vr.Valid {
		t.Fatal("un objeto Base64 alterado no puede verificar")
	}
}

func TestXAdES_EnvelopedRechazaDatosNoXML(t *testing.T) {
	priv, cert := certForTest(t, "no-xml")
	doc, _ := domain.NewDocument("a.txt", []byte("texto plano"), "text/plain")
	_, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
		Options: map[string]string{"format": "XAdES Enveloped"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err == nil || !strings.Contains(err.Error(), "solo pueden realizarse sobre datos XML") {
		t.Fatalf("err = %v", err)
	}
}
