// SPDX-License-Identifier: EUPL-1.2
package signer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"math/big"
	"slices"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestV6CofirmaXAdESConservaSHA1(t *testing.T) {
	for _, id := range []string{"sha1_crear", "sha1_conservar", "sha1_leer", "cofirma_sin_original"} {
		machinepolicy.UnsetForTest(t, "permitir_"+id)
	}
	machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
	excepciones.SetForTest(t, "sha1_crear", true)
	firmado := operarXAdES(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", domain.ActionSign,
		map[string]string{"format": "XAdES Enveloped", "algorithm": "SHA1withRSA"}, "Antiguo")
	excepciones.SetForTest(t, "sha1_crear", false)
	excepciones.SetForTest(t, "sha1_conservar", false)
	excepciones.SetForTest(t, "sha1_leer", false)
	for _, accion := range []domain.SignatureAction{domain.ActionCoSign, domain.ActionCounterSign} {
		priv, cert := certForTest(t, "Nuevo")
		doc, _ := domain.NewDocument("entrada.xml", firmado, "application/xml")
		ctx, avisos := domain.ConAvisosOperacion(context.Background())
		res, err := NewXAdESBESDetached().Sign(ctx, domain.SignatureJob{Document: doc, Format: domain.FormatXAdES, Action: accion,
			Options: map[string]string{"format": "XAdES Enveloped"}}, &LocalSigningKey{ID: "n", Signer: priv, Certificate: cert})

		var block *excepciones.Block
		if !errors.As(err, &block) || block.ID != "sha1_conservar" || len(res.Data) != 0 {
			t.Fatalf("%s conservó SHA-1 sin permiso: %v", accion, err)
		}
		excepciones.SetForTest(t, "sha1_conservar", true)
		ctx, avisos = domain.ConAvisosOperacion(context.Background())
		res, err = NewXAdESBESDetached().Sign(ctx, domain.SignatureJob{Document: doc, Format: domain.FormatXAdES, Action: accion, Options: map[string]string{"format": "XAdES Enveloped"}}, &LocalSigningKey{ID: "n", Signer: priv, Certificate: cert})
		if err != nil || !bytes.Contains(res.Data, []byte("rsa-sha1")) || !slices.Contains(avisos.Claves(), excepciones.Warning("sha1_conservar")) {
			t.Fatalf("%s con permiso: %v avisos=%v", accion, err, avisos.Claves())
		}
		machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
		if _, err := NewXAdESBESDetached().Sign(ctx, domain.SignatureJob{Document: doc, Format: domain.FormatXAdES, Action: accion}, &LocalSigningKey{ID: "n", Signer: priv, Certificate: cert}); !errors.As(err, &block) {
			t.Fatalf("%s soslaya máquina: %v", accion, err)
		}
		machinepolicy.ErrorForTest(t, machinepolicy.PermitirSHA1Legacy, errors.New("ilegible"))
		if _, err := NewXAdESBESDetached().Sign(ctx, domain.SignatureJob{Document: doc, Format: domain.FormatXAdES, Action: accion}, &LocalSigningKey{ID: "n", Signer: priv, Certificate: cert}); !errors.As(err, &block) {
			t.Fatalf("%s soslaya máquina ilegible: %v", accion, err)
		}
		machinepolicy.UnsetForTest(t, machinepolicy.PermitirSHA1Legacy)
		excepciones.SetForTest(t, "sha1_conservar", false)

	}
}

func TestV6XAdESRSA1024NoEsValida(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tpl := &x509.Certificate{SerialNumber: big.NewInt(11), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("original firmado")
	signed := assessmentSignature(t, key, cert, "<root>", "</root>", "corta", payload, false, false)
	result, err := assessDocument(t, signed, payload)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || !slices.Contains(result.Certificate.Details, "rsa_bits=1024") {
		t.Fatalf("RSA corta XML más permisiva: %+v", result)
	}
}
