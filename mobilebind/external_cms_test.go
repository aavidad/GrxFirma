// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
	cryptobinx509 "github.com/deatil/go-cryptobin/x509"
	"software.sslmate.com/src/go-pkcs12"
)

func TestProtectAndSignWithExternalSigner(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	recipientP12, recipient := encipheringPKCS12(t, "destino")
	cardID, card := installSimulatedDNIe(t, facade)
	original := []byte("firmado con DNIe y cifrado")
	raw, err := facade.ProtectJSON(mustJSON(t, protectRequest{Name: "f.txt", MIMEType: "text/plain",
		ContentBase64: base64.StdEncoding.EncodeToString(original), Sign: true, CertificateID: cardID,
		Recipients: []protectionRecipientRequest{{CertificateBase64: base64.StdEncoding.EncodeToString(recipient.Raw)}}}), nil)
	if err != nil {
		t.Fatal(err)
	}
	var protected protectResponse
	decodeResponse(t, raw, &protected)
	if protected.Container != "signedandenvelopeddata" || protected.CertificateID != cardID || card.calls != 1 {
		t.Fatalf("proteger y firmar con DNIe: %+v, llamadas %d", protected, card.calls)
	}
	content, err := base64.StdEncoding.DecodeString(protected.ContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, _, err := pkcs12.Decode(recipientP12, "destino")
	if err != nil {
		t.Fatal(err)
	}
	p7, err := cryptobinpkcs7.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := cryptobinx509.ParseCertificate(recipient.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := p7.Decrypt(cert, privateKey.(*rsa.PrivateKey)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(p7.Content, original) {
		t.Fatal("el contenido descifrado no coincide")
	}
	if err := p7.Verify(); err != nil {
		t.Fatalf("la firma del DNIe no verifica: %v", err)
	}
}

func TestExternalCMSKeySignOnlyAcceptsExternalSigner(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	adapter := externalCMSKeySign{}
	if adapter.Check(private) || adapter.Check(&private.PublicKey) || adapter.Check(nil) {
		t.Fatal("el adaptador no debe sustituir al firmador RSA de la biblioteca")
	}
	signer := &externalRSASigner{publicKey: &private.PublicKey, delegate: &simulatedCard{private: private}}
	if !adapter.Check(signer) {
		t.Fatal("el adaptador debe aceptar el firmador externo")
	}
	data := []byte("contenido")
	digest, signature, err := adapter.Sign(signer, data)
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(data)
	if !bytes.Equal(digest, expected[:]) {
		t.Fatal("resumen inesperado")
	}
	if ok, err := adapter.Verify(&private.PublicKey, data, signature); !ok || err != nil {
		t.Fatalf("verificación: %v", err)
	}
	if !adapter.OID().Equal(cryptobinpkcs7.KeySignWithRSASHA256.OID()) {
		t.Fatal("el OID de firma debe ser el de RSA con SHA-256")
	}
}
