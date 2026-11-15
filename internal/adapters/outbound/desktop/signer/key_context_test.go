// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/security/signingpolicy"
	"testing"
	"time"
)

func privateSigningContext(t *testing.T, id string) context.Context {
	t.Helper()
	e, ok := excepciones.Find(id)
	if !ok {
		t.Fatal(id)
	}
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	dir := t.TempDir()
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), dir, excepciones.ActivationRequest{ID: id, Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestClaveLocalTrifasicaContextualAceptaRSA1024(t *testing.T) {
	ctx := privateSigningContext(t, "rsa_corta")
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	cert := signingPolicyCertificate(t, key, time.Now())
	local := NuevaClaveLocal(key, cert)
	ctx, warnings := domain.ConAvisosOperacion(ctx)
	if err := signingpolicy.ValidateSigningCertificateDER(ctx, cert.Raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	sig, err := local.SignDigestWithContext(ctx, make([]byte, crypto.SHA256.Size()), crypto.SHA256)
	if err != nil || len(sig) == 0 {
		t.Fatalf("clave corta permitida por directorio privado: %v", err)
	}
	if len(warnings.Claves()) != 1 || warnings.Claves()[0] != excepciones.Warning("rsa_corta") {
		t.Fatalf("aviso RSA: %v", warnings.Claves())
	}
}
func TestClaveLocalTrifasicaContextualAceptaSHA1(t *testing.T) {
	ctx := privateSigningContext(t, "sha1_crear")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	local := NuevaClaveLocal(key, signingPolicyCertificate(t, key, time.Now()))
	ctx, warnings := domain.ConAvisosOperacion(ctx)
	sig, err := local.SignPreDataWithContext(ctx, []byte("PRE"), "SHA1withRSA", nil)
	if err != nil {
		t.Fatalf("SHA1 autorizado por directorio privado: %v", err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(sig); err != nil || len(decoded) == 0 {
		t.Fatal("firma vacía", err)
	}
	if len(warnings.Claves()) != 1 || warnings.Claves()[0] != excepciones.Warning("sha1_crear") {
		t.Fatalf("aviso SHA1: %v", warnings.Claves())
	}
	machinepolicy.SetForTest(t, "permitir_sha1_legacy", false)
	if _, err := local.SignPreDataWithContext(ctx, []byte("PRE"), "SHA1withRSA", nil); err == nil {
		t.Fatal("soslaya política de máquina")
	} else {
		var b *excepciones.Block
		if !errors.As(err, &b) || b.ID != "sha1_crear" {
			t.Fatal(err)
		}
	}
}
