// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/exttools"
)

func generarCertRSAPrueba(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	return generarCertRSAPruebaConCN(t, "Certificado PAdES GrxFirma")
}

func generarCertRSAPruebaConCN(t *testing.T, cn string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar clave RSA de prueba: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("no se pudo generar serie RSA de prueba: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("no se pudo crear el certificado de prueba: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("no se pudo parsear el certificado de prueba: %v", err)
	}
	return priv, cert
}

func TestPAdESVerifier_VerificaPDFGeneradoPorV2(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)

	doc := documentoPDFRealPrueba(t)
	job := domain.SignatureJob{Document: doc, Format: domain.FormatPAdES, Action: domain.ActionSign}
	firmado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("no se pudo firmar el PDF de prueba: %v", err)
	}

	verifier := commonsigner.NewPAdESVerifier()
	signedDoc, _ := domain.NewDocument("firmado.pdf", firmado.Data, "application/pdf")
	result, signers, err := verifier.Verify(context.Background(), signedDoc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la verificación PAdES falló: %v", err)
	}
	if !result.Valid {
		t.Fatalf("la verificación PAdES debería ser válida: %s", result.Reason)
	}
	if result.Reason != "firma PAdES válida" {
		t.Fatalf("reason inesperado: %q", result.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante, obtenidos %d", len(signers))
	}
}

func TestPAdESVerifier_PropagaConfianzaCAdES(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)

	firmado, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: documentoPDFRealPrueba(t),
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
	}, clave)
	if err != nil {
		t.Fatalf("no se pudo firmar el PDF de prueba: %v", err)
	}
	signedDoc, err := domain.NewDocument("firmado-confianza.pdf", firmado.Data, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := commonsigner.NewPAdESVerifier().Verify(context.Background(), signedDoc, domain.CertificateChain{
		DERCertificates: [][]byte{cert.Raw},
	})
	if err != nil {
		t.Fatalf("Verify() con ancla confiable devolvió error: %v", err)
	}
	if !result.Valid || result.Trust.Status != domain.VerificationStatusValid {
		t.Fatalf("PAdES no propagó la confianza válida: valid=%v trust=%+v", result.Valid, result.Trust)
	}

	_, otherCert := generarCertRSAPruebaConCN(t, "Raíz no confiable")
	result, _, err = commonsigner.NewPAdESVerifier().Verify(context.Background(), signedDoc, domain.CertificateChain{
		DERCertificates: [][]byte{otherCert.Raw},
	})
	if err != nil {
		t.Fatalf("Verify() con ancla no confiable devolvió error: %v", err)
	}
	if result.Valid {
		t.Fatal("PAdES no puede ser globalmente válido con una cadena no confiable")
	}
	if result.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la cadena inválida no debe contaminar la integridad: %q", result.Integrity.Status)
	}
	if result.Trust.Status != domain.VerificationStatusInvalid {
		t.Fatalf("Trust.Status=%q, want invalid", result.Trust.Status)
	}
}

func TestPAdESVerifier_VerificaPDFAdobeCompatibleGeneradoPorV2(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)

	doc := documentoPDFRealPrueba(t)
	job := domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"subfilter": "adobe"},
	}
	firmado, err := motor.Sign(context.Background(), job, clave)
	if err != nil {
		t.Fatalf("no se pudo firmar el PDF Adobe-compatible de prueba: %v", err)
	}

	verifier := commonsigner.NewPAdESVerifier()
	signedDoc, _ := domain.NewDocument("firmado-adobe.pdf", firmado.Data, "application/pdf")
	result, signers, err := verifier.Verify(context.Background(), signedDoc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la verificación PAdES Adobe-compatible falló: %v", err)
	}
	if !result.Valid {
		t.Fatalf("la verificación PAdES Adobe-compatible debería ser válida: %s", result.Reason)
	}
	if result.Reason != "firma PAdES válida" {
		t.Fatalf("reason inesperado: %q", result.Reason)
	}
	if len(signers) != 1 {
		t.Fatalf("se esperaba un firmante Adobe-compatible, obtenidos %d", len(signers))
	}
	if !containsDetail(result.Details, "subfilter=adbe.pkcs7.detached") {
		t.Fatalf("no se registró el subfiltro Adobe-compatible en detalles: %v", result.Details)
	}
}

func TestPAdESVerifier_VerificaTodasLasFirmasEmbebidasDelPDF(t *testing.T) {
	priv1, cert1 := generarCertRSAPruebaConCN(t, "Firmante Principal")
	clave1 := desktopsigner.NuevaClaveLocal(priv1, cert1)
	priv2, cert2 := generarCertRSAPruebaConCN(t, "Firmante Secundario")
	clave2 := desktopsigner.NuevaClaveLocal(priv2, cert2)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)

	doc := documentoPDFRealPrueba(t)
	firmado1, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
	}, clave1)
	if err != nil {
		t.Fatalf("no se pudo crear la firma principal: %v", err)
	}

	docFirmado1, err := domain.NewDocument("firmado-1.pdf", firmado1.Data, "application/pdf")
	if err != nil {
		t.Fatalf("no se pudo crear el documento para cofirma: %v", err)
	}
	firmado2, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: docFirmado1,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionCoSign,
	}, clave2)
	if err != nil {
		t.Fatalf("no se pudo crear la cofirma: %v", err)
	}
	if got := strings.Count(string(firmado2.Data), "/ByteRange"); got != 2 {
		t.Fatalf("el PDF cofirmado contiene %d ByteRange, want 2", got)
	}

	verifier := commonsigner.NewPAdESVerifier()
	signedDoc, _ := domain.NewDocument("firmado-multiple.pdf", firmado2.Data, "application/pdf")
	result, signers, err := verifier.Verify(context.Background(), signedDoc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la verificación PAdES múltiple falló: %v", err)
	}
	if !result.Valid {
		t.Fatalf("la verificación PAdES múltiple debería ser válida: %s", result.Reason)
	}
	if len(signers) != 2 {
		t.Fatalf("se esperaban 2 firmantes, obtenidos %d", len(signers))
	}
	if signers[0].Subject != cert1.Subject.String() {
		t.Fatalf("primer firmante inesperado: %q", signers[0].Subject)
	}
	if signers[1].Subject != cert2.Subject.String() {
		t.Fatalf("segundo firmante inesperado: %q", signers[1].Subject)
	}
	if result.Coverage != "full" {
		t.Fatalf("la última firma debe cubrir el PDF completo: coverage=%q", result.Coverage)
	}
	if !containsDetail(result.Details, "firmas_pdf=2") {
		t.Fatalf("no se registró el número de firmas PDF: %v", result.Details)
	}
	if !containsDetail(result.Details, "cobertura_firma_pdf_1=revision_hasta_") {
		t.Fatalf("no se registró la cobertura de la primera revisión: %v", result.Details)
	}
}

func TestPAdESVerifier_ContenidoPosteriorNoSeDeclaraCoberturaCompleta(t *testing.T) {
	priv, cert := generarCertRSAPrueba(t)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)
	firmado, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: documentoPDFRealPrueba(t),
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
	}, desktopsigner.NuevaClaveLocal(priv, cert))
	if err != nil {
		t.Fatalf("no se pudo firmar el PDF: %v", err)
	}
	conAppend := append(append([]byte(nil), firmado.Data...), []byte("\n% bytes posteriores sin firma\n")...)
	doc, err := domain.NewDocument("firmado-con-append.pdf", conAppend, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}

	result, signers, err := commonsigner.NewPAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify() devolvió error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("la firma criptográfica debe seguir siendo válida: %s", result.Reason)
	}
	if result.Coverage != "partial" {
		t.Fatalf("coverage=%q, want partial", result.Coverage)
	}
	if result.Integrity.Status != domain.VerificationStatusWarning {
		t.Fatalf("integrity.status=%q, want warning", result.Integrity.Status)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("se esperaba un warning por los bytes posteriores no cubiertos")
	}
	if len(signers) != 1 {
		t.Fatalf("firmantes=%d, want 1", len(signers))
	}
}

func TestPAdESFixtureQA_ValidaConPdfsig(t *testing.T) {
	exttools.Require(t, "pdfsig")

	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_signed.pdf")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("fixture 2_signed.pdf pendiente: firmar el 2.pdf sintético con el certificado FNMT de pruebas")
	}
	salida, err := exec.Command("pdfsig", fixturePath).CombinedOutput()
	if err != nil {
		t.Fatalf("pdfsig devolvió error para el fixture PAdES QA: %v\n%s", err, salida)
	}
	out := string(salida)
	if !strings.Contains(out, "Signature #1:") {
		t.Fatalf("pdfsig no detectó firma en el fixture PAdES QA:\n%s", out)
	}
	if !strings.Contains(out, "Signature Validation: Signature is Valid.") {
		t.Fatalf("pdfsig no validó el fixture PAdES QA:\n%s", out)
	}
}

func TestPAdESVerifier_VerificaFixtureQAConRutaGo(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_signed.pdf")
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("fixture 2_signed.pdf pendiente: firmar el 2.pdf sintético con el certificado FNMT de pruebas")
	}
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("no se pudo leer el fixture PAdES QA: %v", err)
	}
	doc, err := domain.NewDocument("2_signed.pdf", data, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}

	verifier := commonsigner.NewPAdESVerifier()
	result, signers, err := verifier.Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la verificación PAdES Go falló para el fixture QA: %v", err)
	}
	if !result.Valid {
		t.Fatalf("el fixture PAdES QA debería ser válido: %s", result.Reason)
	}
	if result.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la integridad criptográfica del fixture debería ser válida: %+v", result.Integrity)
	}
	if !(result.Certificate.Status == domain.VerificationStatusValid || (result.Certificate.Status == domain.VerificationStatusWarning && strings.Contains(result.Certificate.Reason, "revocación"))) {
		t.Fatalf("el certificado oficial de pruebas debería estar vigente: %+v", result.Certificate)
	}
	if len(signers) == 0 {
		t.Fatal("se esperaba al menos un firmante en el fixture PAdES QA")
	}
}

func containsDetail(details []string, needle string) bool {
	for _, detail := range details {
		if strings.Contains(detail, needle) {
			return true
		}
	}
	return false
}

func documentoPDFRealPrueba(t *testing.T) domain.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "test", "prueba1.pdf"))
	if err != nil {
		t.Fatalf("no se pudo leer el PDF de prueba: %v", err)
	}
	doc, err := domain.NewDocument("prueba1.pdf", data, "application/pdf")
	if err != nil {
		t.Fatalf("no se pudo crear el documento PDF de prueba: %v", err)
	}
	return doc
}
