// SPDX-License-Identifier: EUPL-1.2
package signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"grxfirma/internal/domain"
	"math/big"
	"testing"
	"time"
)

func TestV9VeriFactuMarcaEnCNNoCreaBloqueo(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "caducado [aprobacion_automatica; excepciones.aprobacion_automatica.bloqueo]"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(-time.Minute), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	data := vfTestXML("RegistroAlta", "12345678/G33", "", time.Now().UTC().Format(time.RFC3339))
	canon, err := canonicalizeXML(data, algExcC14N)
	if err != nil {
		t.Fatal(err)
	}
	alg := xadesAlgorithmOptions{Label: "SHA256withRSA", Hash: crypto.SHA256, SignatureMethod: algRSASHA256, DigestMethod: algSHA256}
	opts := resolveXAdESBuildOptions(map[string]string{"expPolicy": "firmaage19"}, alg)
	opts.DocumentTransforms = []string{algEnveloped, algExcC14N}
	digest, err := digestBytes(alg.Hash, canon)
	if err != nil {
		t.Fatal(err)
	}
	ids := idsXAdESNuevos()
	info, value, ki, props, err := firmarPartesXAdES(&LocalSigningKey{ID: "bad", Signer: key, Certificate: cert}, alg, opts, ids, "", base64.StdEncoding.EncodeToString(digest), "text/xml")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := insertarFirmaEnRaiz(data, ensamblarFirmaXAdES(ids, opts.Namespace, info, value, ki, "", props))
	if err != nil {
		t.Fatal(err)
	}
	node, err := vfParse(signed)
	if err != nil {
		t.Fatal(err)
	}
	problems := clavesVeriFactu(vfCheckSignatureConContexto(context.Background(), signed, node, time.Now()))
	if problems["excepciones.aprobacion_automatica.bloqueo"] != "" || (problems["verifactu.signature"] != "error" && problems["verifactu.certificate"] != "error") {
		t.Fatalf("marca del CN elige excepción: %v", problems)
	}
}

func TestV9PAdESMarcaAjenaNoRebajaIntegridad(t *testing.T) {
	base := domain.NewVerificationFailure("PAdES", "documento alterado", nil)
	next := domain.NewVerificationFailure("CAdES", "certificado caducado: x [sha1_leer; excepciones.sha1_leer.bloqueo]", nil)
	next.Integrity.Status = domain.VerificationStatusValid
	merged := mergePAdESVerificationResult(base, next)
	if merged.Valid || merged.Integrity.Status != domain.VerificationStatusInvalid {
		t.Fatalf("marca rebaja integridad: %+v", merged.Integrity)
	}
}
