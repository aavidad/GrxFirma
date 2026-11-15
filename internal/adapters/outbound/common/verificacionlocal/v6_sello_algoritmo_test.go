// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"github.com/digitorus/pkcs7"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"math/big"
	"slices"
	"testing"
	"time"
)

func TestV6SelloSHA1EnAlgoritmoDeFirmaExigeAjuste(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pkcs7.NewSignedData([]byte("TSTInfo de prueba"))
	if err != nil {
		t.Fatal(err)
	}
	data.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := data.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	raw, err := data.Finish()
	if err != nil {
		t.Fatal(err)
	}
	p7, err := pkcs7.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, oid := range []asn1.ObjectIdentifier{pkcs7.OIDDigestAlgorithmECDSASHA1, pkcs7.OIDDigestAlgorithmDSASHA1} {
		p7.Signers[0].DigestEncryptionAlgorithm.Algorithm = oid
		excepciones.SetForTest(t, "sha1_leer", false)
		ctx, warnings := domain.ConAvisosOperacion(context.Background())
		if control := controlFirmaSello(ctx, p7, cert); control.Estado != domain.SelloNoComprobado || control.Motivo != "excepciones.sha1_leer.bloqueo" {
			t.Fatalf("SHA-1 oculto bajo SHA-256: %s %+v", oid, control)
		}
		excepciones.SetForTest(t, "sha1_leer", true)
		if control := controlFirmaSello(ctx, p7, cert); control.Estado != "" {
			t.Fatal(control)
		}
		if !slices.Contains(warnings.Claves(), excepciones.Warning("sha1_leer")) {
			t.Fatal(warnings.Claves())
		}
	}
}
