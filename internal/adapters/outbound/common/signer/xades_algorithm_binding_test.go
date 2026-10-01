// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestXMLSignatureMethodMustMatchRSAKey(t *testing.T) {
	private, cert := certForTest(t, "QA algorithm binding")
	for _, tc := range []struct {
		name, method string
		hash         crypto.Hash
		accept       bool
	}{
		{"rsa_sha1_legacy_verification", algRSASHA1, crypto.SHA1, true},
		{"rsa_sha256", algRSASHA256, crypto.SHA256, true},
		{"rsa_sha384", algRSASHA384, crypto.SHA384, true},
		{"rsa_sha512", algRSASHA512, crypto.SHA512, true},
		{"rsa_declares_ecdsa_sha1", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha1", crypto.SHA1, false},
		{"rsa_declares_ecdsa_sha256", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256", crypto.SHA256, false},
		{"rsa_declares_ecdsa_sha384", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384", crypto.SHA384, false},
		{"rsa_declares_ecdsa_sha512", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512", crypto.SHA512, false},
		{"empty_algorithm", "", crypto.SHA256, false},
		{"whitespace_algorithm", " ", crypto.SHA256, false},
		{"unknown_algorithm", "urn:grxfirma:qa:unknown", crypto.SHA256, false},
		{"absent_signature_method", "ABSENT", crypto.SHA256, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			methodXML := fmt.Sprintf(`<ds:SignatureMethod Algorithm="%s"/>`, tc.method)
			if tc.method == "ABSENT" {
				methodXML = ""
			}
			signedInfo := []byte(fmt.Sprintf(`<ds:SignedInfo xmlns:ds="%s"><ds:CanonicalizationMethod Algorithm="%s"/>%s</ds:SignedInfo>`, nsXMLDSig, algExcC14N, methodXML))
			canonical, err := canonicalizeSignedInfo(signedInfo)
			if err != nil {
				t.Fatal(err)
			}
			hasher := tc.hash.New()
			_, _ = hasher.Write(canonical)
			digest := hasher.Sum(nil)
			// Firmar la declaración incoherente, no alterar una firma existente:
			// la prueba debe fallar por algoritmo, no por un digest modificado.
			signature, err := rsa.SignPKCS1v15(rand.Reader, private, tc.hash, digest)
			if err != nil {
				t.Fatal(err)
			}
			if err := rsa.VerifyPKCS1v15(&private.PublicKey, tc.hash, digest, signature); err != nil {
				t.Fatal(err)
			}
			err = verifyXAdESSignatureValue(signedInfo, signedInfo, base64.StdEncoding.EncodeToString(signature), cert)
			if (err == nil) != tc.accept {
				t.Fatalf("accept=%v err=%v", tc.accept, err)
			}
		})
	}
}
