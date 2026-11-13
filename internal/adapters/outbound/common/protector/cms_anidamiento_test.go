// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/anidamientoasn1"
	"grxfirma/internal/domain"
)

// E2-05: un CMS cuyo contenido son 4 MiB de "30 80" (dos millones de niveles
// de longitud indefinida) dentro de un ContentInfo bien formado. La envoltura
// en DER pasa detectCMSContentType y llega a los analizadores recursivos
// (Ber2der de go-cryptobin y ber2der de digitorus/pkcs7). Ejecutar con
// -timeout 60s y GOMEMLIMIT=2GiB.

func tlvLargo(tag byte, contenido []byte) []byte {
	n := len(contenido)
	var longitud []byte
	switch {
	case n < 0x80:
		longitud = []byte{byte(n)}
	default:
		var digitos []byte
		for v := n; v > 0; v >>= 8 {
			digitos = append([]byte{byte(v)}, digitos...)
		}
		longitud = append([]byte{0x80 | byte(len(digitos))}, digitos...)
	}
	out := make([]byte, 0, 1+len(longitud)+n)
	out = append(out, tag)
	out = append(out, longitud...)
	return append(out, contenido...)
}

func cmsAnidadoProfundo(t *testing.T, oid asn1.ObjectIdentifier) []byte {
	t.Helper()
	oidDER, err := asn1.Marshal(oid)
	if err != nil {
		t.Fatal(err)
	}
	profundo := tlvLargo(0x30, bytes.Repeat([]byte{0x30, 0x80}, 2<<20))
	contenido := tlvLargo(0xa0, profundo)
	return tlvLargo(0x30, append(oidDER, contenido...))
}

func exigirRechazoPorProfundidad(t *testing.T, punto string, err error) {
	t.Helper()
	if !errors.Is(err, anidamientoasn1.ErrDemasiadoProfundo) {
		t.Fatalf("%s: el anidamiento profundo no se cortó antes del analizador recursivo: %v", punto, err)
	}
}

func claveCMSPrueba(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "destinatario E2-05"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment,
	}, &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "destinatario E2-05"}}, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, priv
}

func TestCMSAnidamientoProfundo_ValidarAlgoritmos(t *testing.T) {
	data := cmsAnidadoProfundo(t, oidCMSEnvelopedData)
	if ct, err := detectCMSContentType(data); err != nil || ct != cmsContentTypeEnvelopedData {
		t.Fatalf("el CMS de prueba debe pasar la detección: %v, %v", ct, err)
	}
	_, err := validarAlgoritmosCMS(data)
	exigirRechazoPorProfundidad(t, "validarAlgoritmosCMS", err)
}

func TestCMSAnidamientoProfundo_DescifrarEnvelopedData(t *testing.T) {
	cert, priv := claveCMSPrueba(t)
	_, err := decryptCMSEnvelopedData(cmsAnidadoProfundo(t, oidCMSEnvelopedData), cert, priv)
	exigirRechazoPorProfundidad(t, "decryptCMSEnvelopedData", err)
}

func TestCMSAnidamientoProfundo_DescifrarEncryptedData(t *testing.T) {
	_, err := decryptCMSEncryptedData(cmsAnidadoProfundo(t, oidCMSEncryptedData), bytes.Repeat([]byte{1}, 32))
	exigirRechazoPorProfundidad(t, "decryptCMSEncryptedData", err)
}

func TestCMSAnidamientoProfundo_SignedAndEnvelopedData(t *testing.T) {
	doc := domain.Document{Name: "x.p7m", Content: cmsAnidadoProfundo(t, oidCMSSignedEnvelopedData)}
	_, err := decryptSignedEnvelopedCMS(context.Background(), doc, nil, nil)
	exigirRechazoPorProfundidad(t, "decryptSignedEnvelopedCMS", err)
}

func TestCMSAnidamientoProfundo_UnprotectDeExtremoAExtremo(t *testing.T) {
	cert, priv := claveCMSPrueba(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	claves := []domain.ProtectionKeyMaterial{{
		RecipientID:               "destinatario",
		CertificateDER:            cert.Raw,
		RSAOAEP256PrivateKeyPKCS8: pkcs8,
		SymmetricKey:              bytes.Repeat([]byte{2}, 32),
	}}
	for nombre, oid := range map[string]asn1.ObjectIdentifier{
		"EnvelopedData":          oidCMSEnvelopedData,
		"EncryptedData":          oidCMSEncryptedData,
		"SignedAndEnvelopedData": oidCMSSignedEnvelopedData,
	} {
		doc := domain.Document{Name: "x.p7m", Content: cmsAnidadoProfundo(t, oid)}
		_, err := NuevoCMSProtector().Unprotect(context.Background(), doc, claves)
		exigirRechazoPorProfundidad(t, "Unprotect "+nombre, err)
	}
}
