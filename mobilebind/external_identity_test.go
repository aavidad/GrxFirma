// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/testsupport/pdffixture"
)

type simulatedCard struct {
	private *rsa.PrivateKey
	calls   int
}

func (c *simulatedCard) SignDigest(digest []byte, name string) ([]byte, error) {
	c.calls++
	if name != "SHA-256" {
		return nil, errMobileSigningIdentityUnsupported
	}
	return rsa.SignPKCS1v15(rand.Reader, c.private, crypto.SHA256, digest)
}

func TestExternalIdentitySignsOnlyDigest(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Simulated DNIe signature"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Simulated DNIe signature"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	card := &simulatedCard{private: private}
	store := newSessionIdentityStore()
	ref, err := store.installExternalIdentity(der, nil, card)
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.KeyFor(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	local, ok := key.(*desktopsigner.ClaveLocal)
	if !ok {
		t.Fatalf("unexpected key type %T", key)
	}
	digest := sha256.Sum256([]byte("document"))
	signature, err := local.ToLocalSigningKey().Signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(&private.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatal(err)
	}
	if card.calls != 1 {
		t.Fatalf("expected one card call, got %d", card.calls)
	}
	store.clear()

	facade := newAndroidFacadeForTest(t)
	request, _ := json.Marshal(externalIdentityRequest{CertificateBase64: base64.StdEncoding.EncodeToString(der)})
	identityJSON, err := facade.InstallExternalIdentityJSON(string(request), card)
	if err != nil {
		t.Fatal(err)
	}
	var imported importCertificateResponse
	if err := json.Unmarshal([]byte(identityJSON), &imported); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		format, name, mime string
		data               []byte
	}{
		{"pades", "test.pdf", "application/pdf", pdffixture.Minimal()},
		{"cades", "test.txt", "text/plain", []byte("test")},
		{"xades", "test.xml", "application/xml", []byte("<test/>")},
	} {
		t.Run(tc.format, func(t *testing.T) {
			response, err := facade.SignJSON(mustJSON(t, signRequest{
				Name: tc.name, MIMEType: tc.mime, Format: tc.format, Action: "sign",
				CertificateID: imported.CertificateID,
				ContentBase64: base64.StdEncoding.EncodeToString(tc.data),
			}))
			if err != nil {
				command, commandErr := application.NewSignCommand(tc.name, tc.data, tc.mime, tc.format, "sign", imported.CertificateID, nil)
				if commandErr == nil {
					_, underlying := facade.signService.Execute(context.Background(), command)
					t.Fatalf("SignJSON: %v; underlying: %v; card calls: %d", err, underlying, card.calls)
				}
				t.Fatal(err)
			}
			var signed signResponse
			if err := json.Unmarshal([]byte(response), &signed); err != nil {
				t.Fatal(err)
			}
			if signed.SignedContentBase64 == "" {
				t.Fatal("empty signature")
			}
		})
	}
}

func TestExternalRSASignerRejectsWrongSignature(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	card := &simulatedCard{private: private}
	signer := &externalRSASigner{publicKey: &private.PublicKey, delegate: card}
	digest := sha256.Sum256([]byte("document"))
	signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(&private.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatal(err)
	}
	if card.calls != 1 {
		t.Fatalf("expected 1 call, got %d", card.calls)
	}
	if _, err := signer.Sign(rand.Reader, digest[:31], crypto.SHA256); err == nil {
		t.Fatal("accepted truncated digest")
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer.delegate = &simulatedCard{private: other}
	if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256); err == nil {
		t.Fatal("accepted signature from another key")
	}
}
