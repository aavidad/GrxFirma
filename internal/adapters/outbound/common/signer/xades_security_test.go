// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"errors"
	"testing"

	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
)

func TestResolveXAdESAlgorithmOptions_RechazaSHA1SinOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	_, err := resolveXAdESAlgorithmOptions(map[string]string{"algorithm": "SHA1withRSA"})
	if !errors.Is(err, cryptopolicy.ErrSHA1Disabled) {
		t.Fatalf("resolveXAdESAlgorithmOptions() error = %v", err)
	}
}

func TestResolveXAdESAlgorithmOptions_AdmiteSHA1ConOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, true)
	options, err := resolveXAdESAlgorithmOptions(map[string]string{"algorithm": "SHA1withRSA"})
	if err != nil {
		t.Fatalf("resolveXAdESAlgorithmOptions() error = %v", err)
	}
	if options.Hash != crypto.SHA1 {
		t.Fatalf("hash = %v", options.Hash)
	}
}
