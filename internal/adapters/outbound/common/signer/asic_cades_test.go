// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"grxfirma/internal/domain"
)

// CAdES-ASiC-S de AutoFirma Java: contenedor con el dato y
// META-INF/signature.p7s, verificable por el verificador múltiple.
func TestASiCCAdES_FirmaYVerifica(t *testing.T) {
	priv, cert := certForTest(t, "ASiC-CAdES")
	datos := []byte("contrato firmado en contenedor")
	doc, _ := domain.NewDocument("contrato.txt", datos, "text/plain")
	res, err := NewASiCCAdESSigner().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: formatASiCCAdES, Action: domain.ActionSign,
		Options: map[string]string{"algorithm": "SHA512withRSA"},
	}, &LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(res.Data), int64(len(res.Data)))
	if err != nil {
		t.Fatal(err)
	}
	entradas := map[string]bool{}
	for _, f := range zr.File {
		entradas[f.Name] = true
	}
	for _, e := range []string{"mimetype", "contrato.txt", "META-INF/signature.p7s"} {
		if !entradas[e] {
			t.Fatalf("falta %s en el contenedor: %v", e, entradas)
		}
	}
	firmado, _ := domain.NewDocument("contrato.asics", res.Data, "application/vnd.etsi.asic-s+zip")
	vr, firmantes, err := NewMultiVerifier().Verify(context.Background(), firmado, domain.CertificateChain{})
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid || len(firmantes) != 1 {
		t.Fatalf("verificación: %v %+v %d", err, vr.Integrity, len(firmantes))
	}
	alterado := bytes.Replace(res.Data, datos, []byte("contrato alterado en contenedor"), 1)
	if !bytes.Equal(alterado, res.Data) {
		doc, _ := domain.NewDocument("x.asics", alterado, "application/vnd.etsi.asic-s+zip")
		if vr, _, err := NewMultiVerifier().Verify(context.Background(), doc, domain.CertificateChain{}); err == nil && vr.Integrity.Status == domain.VerificationStatusValid {
			t.Fatal("un dato alterado no puede verificar")
		}
	}
}
