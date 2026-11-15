// SPDX-License-Identifier: EUPL-1.2
package triphase

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	desktopSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"math/big"
	"testing"
	"time"
)

func localTriphaseKey(t *testing.T, bits int) *desktopSigner.ClaveLocal {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tpl := &x509.Certificate{SerialNumber: big.NewInt(4242), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return desktopSigner.NuevaClaveLocal(private, cert)
}
func contextWithPrivateException(t *testing.T, id string) context.Context {
	t.Helper()
	e, _ := excepciones.Find(id)
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	dir := t.TempDir()
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), dir, excepciones.ActivationRequest{ID: id, Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestLegacyBatchUsesPrivateContextForShortRSA(t *testing.T) {
	ctx := contextWithPrivateException(t, "rsa_corta")
	ctx, warnings := domain.ConAvisosOperacion(ctx)
	key := localTriphaseKey(t, 1024)
	if err := requireLegacyBatchSigningPreflight(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := signLegacyBatchPreData([]byte("PRE"), key, "SHA256withRSA", crypto.SHA256, nil, ctx); err != nil {
		t.Fatalf("firma RSA corta aprobada: %v", err)
	}
	if len(warnings.Claves()) != 1 || warnings.Claves()[0] != excepciones.Warning("rsa_corta") {
		t.Fatalf("aviso perdido: %v", warnings.Claves())
	}
}
func TestLegacyBatchUsesPrivateContextForSHA1(t *testing.T) {
	ctx := contextWithPrivateException(t, "sha1_crear")
	ctx, warnings := domain.ConAvisosOperacion(ctx)
	key := localTriphaseKey(t, 2048)
	if err := requireLegacyBatchSHA1(ctx, "SHA1withRSA"); err != nil {
		t.Fatal(err)
	}
	if _, err := signLegacyBatchPreData([]byte("PRE"), key, "SHA1withRSA", crypto.SHA1, nil, ctx); err != nil {
		t.Fatalf("firma SHA1 aprobada: %v", err)
	}
	if len(warnings.Claves()) != 1 || warnings.Claves()[0] != excepciones.Warning("sha1_crear") {
		t.Fatalf("aviso perdido: %v", warnings.Claves())
	}
}
