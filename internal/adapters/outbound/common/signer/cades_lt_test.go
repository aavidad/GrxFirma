// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type tsaLTMock struct {
	token []byte
}

func (m *tsaLTMock) RequestTimestamp(context.Context, []byte, crypto.Hash) ([]byte, error) {
	return append([]byte(nil), m.token...), nil
}

type revocationProviderMock struct {
	evidence ports.RevocationEvidence
}

func (m *revocationProviderMock) Fetch(context.Context, *x509.Certificate, *x509.Certificate) (ports.RevocationEvidence, error) {
	return m.evidence, nil
}

func TestSignerCAdESLT_EmbebeRevocacionYVerificaOffline(t *testing.T) {
	leaf, ca, leafKey, caKey := generarCadenaLTPrueba(t)
	doc, _ := domain.NewDocument("doc.txt", []byte("hola mundo"), "text/plain")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}

	crlDER := crearCRLPruebaLT(t, ca, caKey)
	key := &LocalSigningKey{
		ID:          "lt-test",
		Signer:      leafKey,
		Certificate: leaf,
		Chain:       []*x509.Certificate{ca},
	}

	signer := NewSignerCAdESLT(
		NewSignerCAdEST(NewCAdESBESDetached(), &tsaLTMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}),
		&revocationProviderMock{evidence: ports.RevocationEvidence{
			CRLs: [][]byte{crlDER},
		}},
	)

	result, err := signer.Sign(context.Background(), job, key)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if result.Algorithm != "CAdES-B-LT" {
		t.Fatalf("algoritmo inesperado: %s", result.Algorithm)
	}

	verification, _, err := NewCAdESVerifier().VerifyDetachedCMS(context.Background(), result.Data, doc.Content)
	if err != nil {
		t.Fatalf("VerifyDetachedCMS devolvio error: %v", err)
	}
	if !verification.Valid {
		t.Fatalf("la firma LT deberia verificar offline: %s", verification.Reason)
	}
	if !containsDetail(verification.Details, "crl-embebida") {
		t.Fatalf("se esperaba evidencia crl-embebida en detalles: %v", verification.Details)
	}
}

func TestCAdESLT_NoMarcaComoBuenaUnaCRLEmbebidaNoAutenticada(t *testing.T) {
	leaf, ca, leafKey, caKey := generarCadenaLTPrueba(t)
	doc, _ := domain.NewDocument("doc.txt", []byte("evidencia alterada"), "text/plain")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}
	key := &LocalSigningKey{
		ID:          "lt-invalid-revocation",
		Signer:      leafKey,
		Certificate: leaf,
		Chain:       []*x509.Certificate{ca},
	}
	tamperedCRL := crearCRLPruebaLT(t, ca, caKey)
	tamperedCRL = append([]byte(nil), tamperedCRL...)
	tamperedCRL[len(tamperedCRL)-1] ^= 0x01

	signer := NewSignerCAdESLT(
		NewSignerCAdEST(NewCAdESBESDetached(), &tsaLTMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}),
		&revocationProviderMock{evidence: ports.RevocationEvidence{
			CRLs: [][]byte{tamperedCRL},
		}},
	)
	signed, err := signer.Sign(context.Background(), job, key)
	if err != nil {
		t.Fatalf("Sign devolvió error inesperado: %v", err)
	}
	verification, _, err := NewCAdESVerifier().VerifyDetachedCMS(
		context.Background(),
		signed.Data,
		doc.Content,
	)
	if err != nil {
		t.Fatalf("VerifyDetachedCMS devolvió error: %v", err)
	}
	if verification.Certificate.Status == domain.VerificationStatusValid {
		t.Fatal("una CRL con firma alterada marcó la revocación como válida")
	}
	if verification.Trust.Status == domain.VerificationStatusValid {
		t.Fatal("una CRL con firma alterada marcó la evidencia como autenticada")
	}
	if !containsDetail(verification.Details, "sin evidencias embebidas utilizables") {
		t.Fatalf("falta el aviso de evidencia no utilizable: %v", verification.Details)
	}
}

func TestRevocacionEmbebida_RevokedPrevaleceSobreGood(t *testing.T) {
	leaf, ca, _, caKey := generarCadenaLTPrueba(t)
	goodCRL := crearCRLPruebaLT(t, ca, caKey)
	now := time.Now()
	revokedCRL, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		RevokedCertificateEntries: []x509.RevocationListEntry{{
			SerialNumber:   leaf.SerialNumber,
			RevocationTime: now.Add(-time.Minute),
		}},
		Number:     big.NewInt(78),
		ThisUpdate: now.Add(-time.Hour),
		NextUpdate: now.Add(time.Hour),
	}, ca, caKey)
	if err != nil {
		t.Fatal(err)
	}

	verification, err := verifyEmbeddedRevocation(
		[]*x509.Certificate{leaf, ca},
		ports.RevocationEvidence{CRLs: [][]byte{goodCRL, revokedCRL}},
	)
	if err != nil {
		t.Fatalf("verifyEmbeddedRevocation() error: %v", err)
	}
	if verification.Valid {
		t.Fatal("una CRL Good ocultó otra CRL autenticada que revocaba el certificado")
	}
	if verification.Certificate.Status != domain.VerificationStatusInvalid {
		t.Fatalf("estado certificado=%s, se esperaba inválido", verification.Certificate.Status)
	}
	if !containsDetail(verification.Details, "revocado") {
		t.Fatalf("falta detalle de revocación: %v", verification.Details)
	}
}

func TestSignerCAdESLTA_EmbebeArchiveTimestamp(t *testing.T) {
	leaf, ca, leafKey, caKey := generarCadenaLTPrueba(t)
	doc, _ := domain.NewDocument("doc.txt", []byte("hola mundo"), "text/plain")
	job := domain.SignatureJob{Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign}
	key := &LocalSigningKey{
		ID:          "lta-test",
		Signer:      leafKey,
		Certificate: leaf,
		Chain:       []*x509.Certificate{ca},
	}

	ltSigner := NewSignerCAdESLT(
		NewSignerCAdEST(NewCAdESBESDetached(), &tsaLTMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x01}}),
		&revocationProviderMock{evidence: ports.RevocationEvidence{
			CRLs: [][]byte{crearCRLPruebaLT(t, ca, caKey)},
		}},
	)
	ltaSigner := NewSignerCAdESLTA(ltSigner, &tsaLTMock{token: []byte{0x30, 0x03, 0x02, 0x01, 0x02}})

	result, err := ltaSigner.Sign(context.Background(), job, key)
	if err != nil {
		t.Fatalf("Sign devolvio error inesperado: %v", err)
	}
	if result.Algorithm != "CAdES-B-LTA" {
		t.Fatalf("algoritmo inesperado: %s", result.Algorithm)
	}
	if !cmsContieneOID(t, result.Data, oidArchiveTimeStamp) {
		t.Fatal("la firma CAdES-LTA debe contener archiveTimestamp")
	}
}

func cmsContieneOID(t *testing.T, cmsDER []byte, oid asn1.ObjectIdentifier) bool {
	t.Helper()
	ci, sd, err := parseCMSForUnsignedMutation(cmsDER)
	if err != nil {
		t.Fatalf("no se pudo parsear CMS: %v", err)
	}
	_ = ci
	attrs, err := parseUnsignedAttributes(sd.SignerInfos[0].UnsignedAttributes)
	if err != nil {
		t.Fatalf("no se pudieron parsear atributos no firmados: %v", err)
	}
	for _, attr := range attrs {
		if attr.Type.Equal(oid) {
			return true
		}
	}
	return false
}

func containsDetail(details []string, needle string) bool {
	for _, detail := range details {
		if strings.Contains(detail, needle) {
			return true
		}
	}
	return false
}

func generarCadenaLTPrueba(t *testing.T) (leaf, ca *x509.Certificate, leafKey *ecdsa.PrivateKey, caKey *ecdsa.PrivateKey) {
	t.Helper()

	var err error
	caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave CA: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA Test LT"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("crear CA: %v", err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parsear CA: %v", err)
	}

	leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generar clave hoja: %v", err)
	}
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Hoja Test LT"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("crear hoja: %v", err)
	}
	leaf, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("parsear hoja: %v", err)
	}
	return leaf, ca, leafKey, caKey
}

func crearCRLPruebaLT(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey) []byte {
	t.Helper()
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		SignatureAlgorithm:        x509.ECDSAWithSHA256,
		RevokedCertificateEntries: []x509.RevocationListEntry{},
		Number:                    big.NewInt(77),
		ThisUpdate:                time.Now().Add(-time.Hour),
		NextUpdate:                time.Now().Add(time.Hour),
	}, ca, caKey)
	if err != nil {
		t.Fatalf("no se pudo crear CRL de prueba: %v", err)
	}
	return crlDER
}
