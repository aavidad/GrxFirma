// SPDX-License-Identifier: EUPL-1.2
package mobilebind

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
	cryptobinx509 "github.com/deatil/go-cryptobin/x509"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"slices"
	"testing"
)

// F11 ya estaba corregido en V2: esta regresión acredita el contrato público
// real de apertura, que antes del lote devolvía un error genérico.
func TestV3MobileUnprotectPreservesBlockAndAllowedWarning(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_cms_rsa_v15_leer")
	f := newAndroidFacadeForTest(t)
	p12, cert := encipheringPKCS12(t, "pw")
	importPKCS12(t, f, p12, "pw")
	compat, err := cryptobinx509.ParseCertificate(cert.Raw)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cryptobinpkcs7.Encrypt(rand.Reader, []byte("antiguo"), []*cryptobinx509.Certificate{compat}, cryptobinpkcs7.Opts{Cipher: cryptobinpkcs7.AES256GCM, KeyEncrypt: cryptobinpkcs7.KeyEncryptRSA, Mode: cryptobinpkcs7.DefaultMode})
	if err != nil {
		t.Fatal(err)
	}
	req := mustJSON(t, unprotectRequest{Name: "old.cms", MIMEType: "application/pkcs7-mime", ContentBase64: base64.StdEncoding.EncodeToString(encrypted)})
	_, err = f.UnprotectJSON(req, nil)
	var block *excepciones.Block
	if !errors.As(err, &block) || block.ID != "cms_rsa_v15_leer" || !block.Activable {
		t.Fatalf("bloqueo perdido: %v", err)
	}
	_, err = excepciones.ActivateLocal(excepciones.ForLocalAndroid(context.Background()), f.configDir, excepciones.ActivationRequest{ID: block.ID, Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := f.UnprotectJSON(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out unprotectResponse
	decodeResponse(t, raw, &out)
	if !slices.Contains(out.Warnings, excepciones.Warning(block.ID)) {
		t.Fatalf("aviso perdido: %+v", out)
	}
	machinepolicy.SetForTest(t, "permitir_cms_rsa_v15_leer", false)
	_, err = f.UnprotectJSON(req, nil)
	if !errors.As(err, &block) || block.Activable {
		t.Fatalf("política no bloquea: %v", err)
	}
}

func TestV3MobileC14NUsesOwnWarning(t *testing.T) {
	warnings, err := resolverCotejoMovil(commonsigner.ErrFirmaCorrespondeConC14NAntigua, "eni", eniErrorMismatch)
	if err != nil || !slices.Contains(warnings, excepciones.Warning("xml_c14n_antigua")) {
		t.Fatalf("aviso C14N perdido: %v %v", warnings, err)
	}
}
