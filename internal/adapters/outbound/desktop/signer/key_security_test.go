// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
)

func TestSignPreData_RechazaSHA1SinOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key := NuevaClaveLocal(privateKey, signingPolicyCertificate(t, privateKey, time.Now()))

	if _, err := key.SignPreData([]byte("PRE"), "SHA1withRSA", nil); !errors.Is(err, cryptopolicy.ErrSHA1Disabled) {
		t.Fatalf("SignPreData() error = %v", err)
	}
}

func TestSignPreData_AdmiteSHA1ConOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, true)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key := NuevaClaveLocal(privateKey, signingPolicyCertificate(t, privateKey, time.Now()))

	if _, err := key.SignPreData([]byte("PRE"), "SHA1withRSA", nil); err != nil {
		t.Fatalf("SignPreData() error = %v", err)
	}
}

func TestSignDigest_RechazaSHA1SinOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key := NuevaClaveLocal(privateKey, signingPolicyCertificate(t, privateKey, time.Now()))

	if _, err := key.SignDigest(make([]byte, crypto.SHA1.Size()), crypto.SHA1); !errors.Is(err, cryptopolicy.ErrSHA1Disabled) {
		t.Fatalf("SignDigest() error = %v", err)
	}
}
