// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

func firmaCAdESExplicitaPrueba(t *testing.T, priv *rsa.PrivateKey, cert *x509.Certificate, datos []byte) []byte {
	t.Helper()
	doc, err := domain.NewDocument("datos.txt", datos, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign,
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return res.Data
}

// E2-07: otro certificado con la misma clave (y la misma serie, para que el
// SID lo localice) no puede ocupar el lugar del firmante: el atributo firmado
// signingCertificateV2 identifica el certificado original.
func TestCAdESVerifier_RechazaCertificadoSustituidoConMismaClave(t *testing.T) {
	priv, original := generarCertRSAPruebaConCN(t, "Firmante legítimo")
	plantilla := &x509.Certificate{
		SerialNumber: original.SerialNumber,
		Subject:      pkix.Name{CommonName: "Otra identidad con la misma clave"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	datos := []byte("contrato")
	cms := firmaCAdESExplicitaPrueba(t, priv, original, datos)
	verifier := commonsigner.NewCAdESVerifierWithChecker(commonsigner.NewRevocationCheckerOffline())
	if _, _, err := verifier.VerifyDetachedCMS(context.Background(), cms, datos); err != nil {
		t.Fatalf("la firma original debe verificar: %v", err)
	}
	sustituido, err := commonsigner.CMSConCertificadosParaPrueba(cms, der)
	if err != nil {
		t.Fatal(err)
	}
	vr, firmantes, err := verifier.VerifyDetachedCMS(context.Background(), sustituido, datos)
	if err == nil && vr.Valid {
		t.Fatalf("un certificado distinto del firmado no puede atribuirse la firma: %+v", firmantes)
	}
}

// E2-07: el atributo firmado contentType tiene que coincidir con eContentType.
func TestCAdESVerifier_RechazaContentTypeDistintoDeEContentType(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	datos := []byte("contrato")
	cms := firmaCAdESExplicitaPrueba(t, priv, cert, datos)
	alterado, err := commonsigner.CMSConEContentTypeParaPrueba(cms, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4})
	if err != nil {
		t.Fatal(err)
	}
	verifier := commonsigner.NewCAdESVerifierWithChecker(commonsigner.NewRevocationCheckerOffline())
	if vr, _, err := verifier.VerifyDetachedCMS(context.Background(), alterado, datos); err == nil && vr.Valid {
		t.Fatal("un eContentType distinto del atributo contentType firmado no puede verificar")
	}
}

// E2-07: ETSI EN 319 142-1 exige exactamente un SignerInfo en el /Contents.
// En el hueco de un PDF firmado se pone un CAdES explícito válido sobre su
// ByteRange (clave ECDSA, para que quepa) con el SignerInfo repetido.
func TestPAdESVerifier_RechazaVariosSignerInfo(t *testing.T) {
	firmado := firmarPDFParaAtaque(t)
	m := reByteRangePrueba.FindSubmatch(firmado)
	if m == nil {
		t.Fatal("PDF sin ByteRange")
	}
	var br [4]int
	for i := range br {
		br[i], _ = strconv.Atoi(string(m[i+1]))
	}
	contenido := append(append([]byte(nil), firmado[br[0]:br[0]+br[1]]...), firmado[br[2]:br[2]+br[3]]...)
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Firmante ECDSA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	doc, _ := domain.NewDocument("contenido.bin", contenido, "application/octet-stream")
	res, err := commonsigner.NewCAdESBESDetached().Sign(context.Background(), domain.SignatureJob{
		Document: doc, Format: domain.FormatCAdES, Action: domain.ActionSign,
	}, &commonsigner.LocalSigningKey{ID: "k", Signer: clave, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	if vr := verificarPDF(t, sustituirContentsPDF(t, firmado, res.Data)); !vr.Valid {
		t.Fatalf("con un único SignerInfo el PDF debe verificar: %s", vr.Reason)
	}
	doble, err := commonsigner.CMSConSignerInfoDuplicadoParaPrueba(res.Data)
	if err != nil {
		t.Fatal(err)
	}
	pdfDoble, _ := domain.NewDocument("doble.pdf", sustituirContentsPDF(t, firmado, doble), "application/pdf")
	vr, _, err := commonsigner.NewPAdESVerifier().Verify(context.Background(), pdfDoble, domain.CertificateChain{})
	if err == nil && vr.Valid {
		t.Fatal("un /Contents con dos SignerInfo no es una firma PAdES válida")
	}
}

// E2-07: una firma PAdES ETSI sin signingCertificateV2 no identifica su
// certificado; la integridad queda con aviso.
func TestPAdESVerifier_AvisaSinSigningCertificate(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	firmado := firmarPDFParaAtaque(t)
	m := reByteRangePrueba.FindSubmatch(firmado)
	if m == nil {
		t.Fatal("PDF sin ByteRange")
	}
	var br [4]int
	for i := range br {
		br[i], _ = strconv.Atoi(string(m[i+1]))
	}
	contenido := append(append([]byte(nil), firmado[br[0]:br[0]+br[1]]...), firmado[br[2]:br[2]+br[3]]...)
	cms := firmaCAdESExplicitaPrueba(t, priv, cert, contenido)
	sinAtributo, err := commonsigner.CMSSinSigningCertificateParaPrueba(cms, priv)
	if err != nil {
		t.Fatal(err)
	}
	vr := verificarPDF(t, sustituirContentsPDF(t, firmado, sinAtributo))
	if vr.Integrity.Status != domain.VerificationStatusWarning {
		t.Fatalf("integrity=%q, want warning: %v", vr.Integrity.Status, vr.Integrity.Details)
	}
	encontrado := false
	for _, aviso := range vr.Warnings {
		encontrado = encontrado || strings.Contains(aviso, "certificado")
	}
	if !encontrado {
		t.Fatalf("falta el aviso de signingCertificate: %v", vr.Warnings)
	}
}
