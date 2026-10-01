// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestCAdESBESDetached_Sign_RSA(t *testing.T) {
	priv, cert := certForTest(t, "RSA")
	engine := NewCAdESBESDetached()
	doc, err := domain.NewDocument("contrato.txt", []byte("contenido de prueba"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-rsa",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if result.Format != domain.FormatCAdES {
		t.Fatalf("formato inesperado: %s", result.Format)
	}
	if result.Algorithm != "SHA256withRSA" {
		t.Fatalf("algoritmo inesperado: %s", result.Algorithm)
	}
	verifyCAdESStructureAndSignature(t, doc.Content, cert, result.Data)
}

func TestCAdESBESDetached_Sign_ECDSA(t *testing.T) {
	priv, cert := ecdsaCertForTest(t)
	engine := NewCAdESBESDetached()
	doc, err := domain.NewDocument("contrato.txt", []byte("contenido de prueba"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-ecdsa",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if result.Algorithm != "SHA256withECDSA" {
		t.Fatalf("algoritmo inesperado: %s", result.Algorithm)
	}
	verifyCAdESStructureAndSignature(t, doc.Content, cert, result.Data)
}

func TestCAdESBESDetached_RechazaFormatoNoCompatible(t *testing.T) {
	priv, cert := certForTest(t, "RSA")
	engine := NewCAdESBESDetached()
	doc, _ := domain.NewDocument("documento.xml", []byte("<x/>"), "application/xml")

	_, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-rsa",
		Signer:      priv,
		Certificate: cert,
	})
	if err == nil {
		t.Fatal("se esperaba error por formato no soportado")
	}
}

func TestCAdESBESDetached_RespetaCancelacionDeContexto(t *testing.T) {
	priv, cert := certForTest(t, "RSA")
	engine := NewCAdESBESDetached()
	doc, _ := domain.NewDocument("contrato.txt", []byte("contenido"), "text/plain")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.Sign(ctx, domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-rsa",
		Signer:      priv,
		Certificate: cert,
	})
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}

func verifyCAdESStructureAndSignature(t *testing.T, contenido []byte, cert *x509.Certificate, firma []byte) {
	t.Helper()

	var ci contentInfo
	rest, err := asn1.Unmarshal(firma, &ci)
	if err != nil {
		t.Fatalf("firma ASN.1 invalida: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("resto inesperado tras ContentInfo: %d bytes", len(rest))
	}
	if !ci.ContentType.Equal(oidSignedData) {
		t.Fatalf("contentType inesperado: %v", ci.ContentType)
	}

	var sd signedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		t.Fatalf("SignedData invalido: %v", err)
	}
	if sd.EncapContentInfo.EContentType.String() != oidData.String() {
		t.Fatalf("eContentType inesperado: %v", sd.EncapContentInfo.EContentType)
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("se esperaba un firmante, se obtuvieron %d", len(sd.SignerInfos))
	}

	si := sd.SignerInfos[0]
	signedAttrsDER := makeSetDER(si.SignedAttributes.Bytes)
	attrs := parseAttributes(t, signedAttrsDER)

	foundDigest := false
	foundContentType := false
	foundSigningTime := false
	foundSigningCertificate := false
	expectedDigest := sha256.Sum256(contenido)
	for _, attr := range attrs {
		switch {
		case attr.Type.Equal(oidMessageDigest):
			foundDigest = true
			if len(attr.Values) != 1 || string(attr.Values[0].Bytes) != string(expectedDigest[:]) {
				t.Fatal("messageDigest no coincide con el contenido firmado")
			}
		case attr.Type.Equal(oidContentType):
			foundContentType = true
		case attr.Type.Equal(oidSigningTime):
			foundSigningTime = true
		case attr.Type.Equal(oidSigningCertificateV2):
			foundSigningCertificate = true
		}
	}
	if !foundDigest || !foundContentType || !foundSigningTime || !foundSigningCertificate {
		t.Fatalf("faltan atributos obligatorios: digest=%t contentType=%t signingTime=%t signingCertificateV2=%t",
			foundDigest, foundContentType, foundSigningTime, foundSigningCertificate)
	}

	hashSignedAttrs := sha256.Sum256(signedAttrsDER)
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashSignedAttrs[:], si.Signature); err != nil {
			t.Fatalf("firma RSA invalida: %v", err)
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, hashSignedAttrs[:], si.Signature) {
			t.Fatal("firma ECDSA invalida")
		}
	default:
		t.Fatalf("clave publica no soportada en test: %T", pub)
	}
}

func parseAttributes(t *testing.T, der []byte) []attribute {
	t.Helper()
	var attrs []attribute
	if _, err := asn1.UnmarshalWithParams(der, &attrs, "set"); err != nil {
		t.Fatalf("no se pudieron parsear los atributos: %v", err)
	}
	return attrs
}

func makeSetDER(content []byte) []byte {
	header := []byte{0x31}
	header = append(header, encodeDERLength(len(content))...)
	header = append(header, content...)
	return header
}

func encodeDERLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var tmp [8]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte(n)
		n >>= 8
	}
	out := []byte{0x80 | byte(len(tmp)-i)}
	out = append(out, tmp[i:]...)
	return out
}

func certForTest(t *testing.T, name string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar clave RSA: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(20260318),
		Subject: pkix.Name{
			CommonName:   "GrxFirma " + name,
			Organization: []string{"Dipgra"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("no se pudo crear certificado RSA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("no se pudo parsear certificado RSA: %v", err)
	}
	return priv, cert
}

func ecdsaCertForTest(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("no se pudo generar clave ECDSA: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(20260319),
		Subject: pkix.Name{
			CommonName:   "GrxFirma ECDSA",
			Organization: []string{"Dipgra"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("no se pudo crear certificado ECDSA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("no se pudo parsear certificado ECDSA: %v", err)
	}
	return priv, cert
}
