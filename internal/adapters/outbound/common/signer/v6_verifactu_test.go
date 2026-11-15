// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"testing"
	"time"
)

func TestV6VeriFactuC14NBloqueadaNoAceptaFirma(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_xml_c14n_antigua")
	excepciones.SetForTest(t, "xml_c14n_antigua", false)
	priv, cert := certForTest(t, "VeriFactu histórico")
	data := vfTestXML("RegistroAlta", "12345678/G33", "", time.Now().UTC().Format(time.RFC3339))
	data = bytes.Replace(data, []byte(" xmlns="), []byte(" xmlns:unused=\"urn:historico\" xmlns="), 1)
	// El productor histórico firmaba SignedInfo aislado sin los espacios de nombres heredados.
	canon, err := exclusiveC14N(string(data))
	if err != nil {
		t.Fatal(err)
	}
	alg := xadesAlgorithmOptions{Label: "SHA256withRSA", Hash: crypto.SHA256, SignatureMethod: algRSASHA256, DigestMethod: algSHA256}
	opts := resolveXAdESBuildOptions(map[string]string{"expPolicy": "firmaage19"}, alg)
	opts.CanonicalizationAlg = algC14N
	opts.DocumentTransforms = []string{algEnveloped, algExcC14N}
	digest, err := digestBytes(alg.Hash, canon)
	if err != nil {
		t.Fatal(err)
	}
	ids := idsXAdESNuevos()
	info, value, keyinfo, props, err := firmarPartesXAdES(&LocalSigningKey{ID: "key", Signer: priv, Certificate: cert}, alg, opts, ids, "", base64.StdEncoding.EncodeToString(digest), "text/xml")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := insertarFirmaEnRaiz(data, ensamblarFirmaXAdES(ids, opts.Namespace, info, value, keyinfo, "", props))
	if err != nil {
		t.Fatal(err)
	}
	node, err := vfParse(signed)
	if err != nil {
		t.Fatal(err)
	}
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	problems := vfCheckSignatureConContexto(ctx, signed, node, time.Now())
	if clavesVeriFactu(problems)["excepciones.xml_c14n_antigua.bloqueo"] != "error" && clavesVeriFactu(problems)["verifactu.signature"] != "error" {
		t.Fatalf("firma histórica sin ajuste aceptada: %+v; avisos=%v", problems, warnings.Claves())
	}
}
