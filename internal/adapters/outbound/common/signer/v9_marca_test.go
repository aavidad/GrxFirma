// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
)

func TestV9CotejoMarcaEnCNNoCreaBloqueo(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	for _, variante := range []string{"keyusage", "futuro"} {
		tpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "x [aprobacion_automatica; excepciones.aprobacion_automatica.bloqueo]"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageKeyEncipherment, BasicConstraintsValid: true}
		if variante == "futuro" {
			tpl.KeyUsage = x509.KeyUsageDigitalSignature
			tpl.NotBefore = time.Now().Add(-time.Hour)
			tpl.NotAfter = time.Now().Add(-time.Minute)
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(der)
		original := []byte("<a>hola</a>")
		doc, _ := domain.NewDocument("o.xml", original, "application/xml")
		res, err := NewXAdESBESDetached().Sign(context.Background(), domain.SignatureJob{Document: doc, Format: domain.FormatXAdES, Action: domain.ActionSign,
			Options: map[string]string{"format": "XAdES Detached"}}, &LocalSigningKey{ID: "x", Signer: priv, Certificate: cert})
		if err != nil {
			t.Fatalf("%s: firma: %v", variante, err)
		}
		err = CotejarXAdESSeparada(res.Data, original, context.Background())
		var block *excepciones.Block
		id := ""
		if errors.As(err, &block) {
			id = block.ID
		}
		if id != "" || err == nil {
			t.Fatalf("%s: marca ajena decide excepción: err=%v bloqueo=%q", variante, err, id)
		}
	}
}
