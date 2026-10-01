// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"encoding/asn1"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"grxfirma/internal/domain"
)

func firmarCAdESParaPrueba(t *testing.T, datos []byte, accion domain.SignatureAction, nombre string, opciones map[string]string) ([]byte, *LocalSigningKey) {
	t.Helper()
	priv, cert := certForTest(t, nombre)
	clave := &LocalSigningKey{ID: nombre, Signer: priv, Certificate: cert}
	doc, err := domain.NewDocument("datos.bin", datos, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: accion, Options: opciones,
	}, clave)
	if err != nil {
		t.Fatalf("%s %s: %v", accion, nombre, err)
	}
	return res.Data, clave
}

// Flujo de Canarias ("firma secuencial") y portafirmas: firma, cofirma y
// contrafirma sobre la misma firma CAdES explícita.
func TestCAdES_CofirmaYContrafirma(t *testing.T) {
	contenido := []byte("Texto de prueba CAdES con certificado FNMT")
	firma, _ := firmarCAdESParaPrueba(t, contenido, domain.ActionSign, "Firmante A", nil)
	cofirma, _ := firmarCAdESParaPrueba(t, firma, domain.ActionCoSign, "Cofirmante B", nil)

	vr, firmantes, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), cofirma, contenido)
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la cofirma no verifica: %v %+v", err, vr.Integrity)
	}
	if len(firmantes) != 2 {
		t.Fatalf("firmantes = %d, want 2", len(firmantes))
	}
	if !bytes.Contains(cofirma, firma[len(firma)-256:]) {
		t.Fatal("la cofirma debe conservar intacto el valor de firma original")
	}

	contrafirma, claveC := firmarCAdESParaPrueba(t, cofirma, domain.ActionCounterSign, "Contrafirmante C", nil)
	vr, _, err = NewCAdESVerifier().VerifyDetachedCMS(context.Background(), contrafirma, contenido)
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la firma contrafirmada no verifica: %v %+v", err, vr.Integrity)
	}

	sd, err := parseSignedDataParts(contrafirma)
	if err != nil {
		t.Fatal(err)
	}
	contrafirmas := 0
	for _, siDER := range sd.signerInfos {
		var si signerInfoRaw
		if _, err := asn1.Unmarshal(siDER, &si); err != nil {
			t.Fatal(err)
		}
		attrs, err := splitElements(si.UnsignedAttributes.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		for _, attrDER := range attrs {
			var attr attribute
			if _, err := asn1.Unmarshal(attrDER, &attr); err != nil || !attr.Type.Equal(oidCounterSignature) {
				continue
			}
			var cs signerInfoRaw
			if _, err := asn1.Unmarshal(attr.Values[0].FullBytes, &cs); err != nil {
				t.Fatal(err)
			}
			md, err := extractMessageDigestAttribute(cs.SignedAttributes.Bytes)
			if err != nil || !bytes.Equal(md, cmsDigest(crypto.SHA256, si.Signature)) {
				t.Fatal("la contrafirma no firma el valor de firma del firmante")
			}
			if err := verifyCMSignature(claveC.Certificate, cs, crypto.SHA256); err != nil {
				t.Fatalf("contrafirma criptográficamente inválida: %v", err)
			}
			attrsSET, _ := asSetDER(cs.SignedAttributes)
			if bytes.Contains(attrsSET, mustMarshalOID(t, oidContentType)) {
				t.Fatal("una contrafirma no debe llevar content-type (RFC 5652 11.4)")
			}
			contrafirmas++
		}
	}
	if contrafirmas != 2 {
		t.Fatalf("contrafirmas = %d, want 2 (una por firmante hoja)", contrafirmas)
	}

	// Una segunda contrafirma se aplica a las hojas: las contrafirmas previas.
	doble, _ := firmarCAdESParaPrueba(t, contrafirma, domain.ActionCounterSign, "Contrafirmante D", nil)
	if n := bytes.Count(doble, mustMarshalOID(t, oidCounterSignature)); n != 4 {
		t.Fatalf("atributos de contrafirma = %d, want 4", n)
	}

	if _, err := exec.LookPath("openssl"); err == nil {
		dir := t.TempDir()
		sig, data := filepath.Join(dir, "f.der"), filepath.Join(dir, "d.bin")
		_ = os.WriteFile(sig, doble, 0o600)
		_ = os.WriteFile(data, contenido, 0o600)
		out, err := exec.Command("openssl", "cms", "-verify", "-binary", "-inform", "DER", "-in", sig, "-content", data, "-noverify", "-out", os.DevNull).CombinedOutput()
		if err != nil {
			t.Fatalf("openssl rechaza la firma: %v\n%s", err, out)
		}
	}
}

func TestCAdES_CofirmaExigeDatosSiNingunFirmanteComparteAlgoritmo(t *testing.T) {
	firma, _ := firmarCAdESParaPrueba(t, []byte("datos"), domain.ActionSign, "A", nil)
	_, _ = firmarCAdESParaPrueba(t, firma, domain.ActionCoSign, "B", map[string]string{"algorithm": "SHA256withRSA"})
	priv, cert := certForTest(t, "C")
	doc, _ := domain.NewDocument("f.csig", firma, "application/octet-stream")
	_, err := NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionCoSign,
		Options: map[string]string{"algorithm": "SHA512withRSA"},
	}, &LocalSigningKey{ID: "c", Signer: priv, Certificate: cert})
	if err == nil {
		t.Fatal("una cofirma explícita con otro resumen no puede inventarse el messageDigest")
	}
}

func TestCAdES_ContrafirmaRechazaDatosNoCMS(t *testing.T) {
	priv, cert := certForTest(t, "X")
	doc, _ := domain.NewDocument("f.xml", []byte("<a/>"), "application/xml")
	_, err := NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionCounterSign,
	}, &LocalSigningKey{ID: "x", Signer: priv, Certificate: cert})
	if err == nil {
		t.Fatal("contrafirmar algo que no es CMS debe fallar")
	}
}

func mustMarshalOID(t *testing.T, oid asn1.ObjectIdentifier) []byte {
	t.Helper()
	der, err := asn1.Marshal(oid)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// mode=implicit (MITES, Murcia, VALIDe) produce CAdES con los datos dentro,
// verificable sin el documento; por defecto sigue siendo explícita.
func TestCAdES_ModoImplicitoIncluyeLosDatos(t *testing.T) {
	contenido := []byte("Cadena a firmar")
	implicita, _ := firmarCAdESParaPrueba(t, contenido, domain.ActionSign, "Implicita", map[string]string{"mode": "implicit"})
	explicita, _ := firmarCAdESParaPrueba(t, contenido, domain.ActionSign, "Explicita", nil)
	if !bytes.Contains(implicita, contenido) || bytes.Contains(explicita, contenido) {
		t.Fatal("solo la firma implícita debe contener los datos")
	}
	doc, _ := domain.NewDocument("f.csig", implicita, "application/octet-stream")
	vr, _, err := NewCAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la firma implícita no verifica sola: %v %+v", err, vr.Integrity)
	}
	cofirma, _ := firmarCAdESParaPrueba(t, implicita, domain.ActionCoSign, "Cofirma", map[string]string{"algorithm": "SHA512withRSA"})
	doc, _ = domain.NewDocument("f.csig", cofirma, "application/octet-stream")
	vr, firmantes, err := NewCAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid || len(firmantes) != 2 {
		t.Fatalf("la cofirma SHA-512 de una firma implícita no verifica: %v %+v %d", err, vr.Integrity, len(firmantes))
	}
	if _, err := exec.LookPath("openssl"); err == nil {
		dir := t.TempDir()
		sig := filepath.Join(dir, "f.der")
		_ = os.WriteFile(sig, cofirma, 0o600)
		out, err := exec.Command("openssl", "cms", "-verify", "-inform", "DER", "-in", sig, "-noverify", "-out", filepath.Join(dir, "o")).CombinedOutput()
		if err != nil {
			t.Fatalf("openssl rechaza la firma implícita cofirmada: %v\n%s", err, out)
		}
		if extraidos, _ := os.ReadFile(filepath.Join(dir, "o")); !bytes.Equal(extraidos, contenido) {
			t.Fatal("openssl no extrae los datos originales")
		}
	}
}
