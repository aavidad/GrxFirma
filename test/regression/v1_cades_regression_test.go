// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package regression_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/exttools"
)

type fixtureManifest struct {
	Version     int      `json:"version"`
	Descripcion string   `json:"descripcion"`
	Fixtures    []string `json:"fixtures"`
}

func TestFixturesV1_ManifestMinimo(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("fixtures", "v1", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("no se pudo leer el manifest de fixtures: %v", err)
	}

	var manifest fixtureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("no se pudo parsear el manifest de fixtures: %v", err)
	}

	if len(manifest.Fixtures) < 8 {
		t.Fatalf("se esperaban al menos 8 fixtures, obtenidos %d", len(manifest.Fixtures))
	}

	for _, relative := range manifest.Fixtures {
		path := filepath.Join("fixtures", "v1", relative)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("fixture ausente %s: %v", relative, err)
		}
		if info.Size() == 0 {
			t.Fatalf("fixture vacío %s", relative)
		}
	}
}

func TestCAdES_QA_IntegridadVerificablePorV2(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		filepath.Join("fixtures", "v1", "samples", "2_txt_signed.csig"),
		filepath.Join("fixtures", "v1", "samples", "2_txt_signed_signed.csig"),
	}

	verifier := signer.NewMultiVerifierOffline()

	for _, fixturePath := range fixtures {
		fixturePath := fixturePath
		t.Run(filepath.Base(fixturePath), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("no se pudo leer el fixture %s: %v", fixturePath, err)
			}

			doc, err := domain.NewDocument(filepath.Base(fixturePath), data, "application/pkcs7-signature")
			if err != nil {
				t.Fatalf("no se pudo construir el documento de prueba: %v", err)
			}

			result, signers, err := verifier.Verify(context.Background(), doc, domain.CertificateChain{})
			if err != nil {
				t.Fatalf("la verificación V2 falló para %s: %v", fixturePath, err)
			}
			assertQAFixtureIntegrity(t, fixturePath, result, signers)
			if len(result.Details) == 0 {
				t.Fatalf("no se obtuvieron detalles de verificación para %s", fixturePath)
			}
		})
	}
}

func TestPAdES_QA_ValidaConPdfsig(t *testing.T) {
	t.Parallel()

	fixturePath := requireSignedPDFFixture(t)
	exttools.Require(t, "pdfsig")
	salida, err := exec.Command("pdfsig", fixturePath).CombinedOutput()
	if err != nil {
		t.Fatalf("pdfsig devolvió error para el fixture QA: %v\n%s", err, salida)
	}

	out := string(salida)
	if !strings.Contains(out, "Signature #1:") {
		t.Fatalf("pdfsig no detectó firma en el fixture QA:\n%s", out)
	}
	if !strings.Contains(out, "Signature Type: ETSI.CAdES.detached") {
		t.Fatalf("pdfsig no reconoció ETSI.CAdES.detached en el fixture QA:\n%s", out)
	}
	if !strings.Contains(out, "Total document signed") {
		t.Fatalf("pdfsig no confirmó que el PDF QA esté completamente firmado:\n%s", out)
	}
	if !strings.Contains(out, "Signature Validation: Signature is Valid.") {
		t.Fatalf("pdfsig no validó la firma del fixture QA:\n%s", out)
	}
}

func TestPAdES_QA_EsEstructuralmenteValidoConQPDF(t *testing.T) {
	t.Parallel()

	fixturePath := requireSignedPDFFixture(t)
	exttools.Require(t, "qpdf")
	salida, err := exec.Command("qpdf", "--check", fixturePath).CombinedOutput()
	if err != nil {
		t.Fatalf("qpdf devolvió error para el fixture V1: %v\n%s", err, salida)
	}

	out := string(salida)
	if !strings.Contains(out, "checking") && !strings.Contains(out, "No syntax or stream encoding errors found") {
		t.Fatalf("salida inesperada de qpdf para el fixture V1:\n%s", out)
	}
}

func TestPAdES_QA_IntegridadVerificablePorV2(t *testing.T) {
	t.Parallel()

	fixturePath := requireSignedPDFFixture(t)
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("no se pudo leer el fixture PAdES QA: %v", err)
	}

	doc, err := domain.NewDocument(filepath.Base(fixturePath), data, "application/pdf")
	if err != nil {
		t.Fatalf("no se pudo construir el documento PAdES de prueba: %v", err)
	}

	verifier := signer.NewMultiVerifierOffline()
	result, signers, err := verifier.Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la verificación PAdES QA falló en V2: %v", err)
	}
	if result.Valid {
		t.Fatalf("la firma PAdES QA no debe considerarse válida sin revocación acreditada: %s", result.Reason)
	}
	if result.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la integridad criptográfica del fixture QA debería ser válida: %+v", result.Integrity)
	}
	if result.Certificate.Status != domain.VerificationStatusUnknown && result.Certificate.Status != domain.VerificationStatusWarning {
		t.Fatalf("el certificado oficial de pruebas debería quedar sin revocación acreditada: %+v", result.Certificate)
	}
	if len(signers) == 0 {
		t.Fatal("no se extrajo ningún firmante del fixture PAdES QA")
	}
}

func requireSignedPDFFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join("fixtures", "v1", "samples", "2_signed.pdf")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("falta el PAdES 2_signed.pdf del PDF sintético; el responsable debe firmar 2.pdf con el certificado FNMT de pruebas y añadirlo")
	} else if err != nil {
		t.Fatalf("no se pudo comprobar el fixture PAdES %s: %v", path, err)
	}
	return path
}

func TestXAdES_QA_IntegridadVerificablePorV2(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		filepath.Join("fixtures", "v1", "samples", "2_xml_signed_signed.xsig"),
	}

	verifier := signer.NewMultiVerifierOffline()

	for _, fixturePath := range fixtures {
		fixturePath := fixturePath
		t.Run(filepath.Base(fixturePath), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("no se pudo leer el fixture %s: %v", fixturePath, err)
			}

			doc, err := domain.NewDocument(filepath.Base(fixturePath), data, "application/xml")
			if err != nil {
				t.Fatalf("no se pudo construir el documento de prueba: %v", err)
			}

			result, signers, err := verifier.Verify(context.Background(), doc, domain.CertificateChain{})
			if err != nil {
				t.Fatalf("la verificación XAdES QA falló para %s: %v", fixturePath, err)
			}
			assertQAFixtureIntegrity(t, fixturePath, result, signers)
		})
	}
}

// 2_xml_signed.xsig lo generó en julio de 2026 una versión anterior de V2 con
// una canonicalización no estándar de la referencia externa "2.xml": la
// canonicalización exclusiva estándar (libxml2) da otro resumen, así que
// @firma o VALIDe la rechazarían. Debe verificarse como compatibilidad
// histórica, nunca como íntegra, y sin dejar de identificar al firmante.
func TestXAdES_QA_FirmaHistoricaNoEstandarSeMarca(t *testing.T) {
	t.Parallel()
	fixturePath := filepath.Join("fixtures", "v1", "samples", "2_xml_signed.xsig")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("no se pudo leer el fixture %s: %v", fixturePath, err)
	}
	doc, err := domain.NewDocument(filepath.Base(fixturePath), data, "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	result, signers, err := signer.NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("la verificación no debe fallar: %v", err)
	}
	if result.Valid || result.Integrity.Status != domain.VerificationStatusWarning || !strings.Contains(result.Integrity.Reason, "Compatibilidad histórica") {
		t.Fatalf("se esperaba compatibilidad histórica, obtenido valid=%v integridad=%+v", result.Valid, result.Integrity)
	}
	if len(signers) == 0 {
		t.Fatal("se esperaba identificar al firmante")
	}
}

// Los fixtures firmados se regeneran con el certificado oficial QA. La cadena
// puede no estar anclada en el sistema local, pero tanto la integridad como la
// vigencia del certificado deben evaluarse de forma independiente.
func assertQAFixtureIntegrity(
	t *testing.T,
	fixturePath string,
	result domain.VerificationResult,
	signers []domain.CertificateRef,
) {
	t.Helper()

	if len(signers) == 0 {
		t.Fatalf("no se extrajo ningún firmante de %s", fixturePath)
	}
	if result.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la integridad criptográfica de %s debería ser válida, estado=%q motivo=%q",
			fixturePath, result.Integrity.Status, result.Integrity.Reason)
	}
	if result.Certificate.Status != domain.VerificationStatusUnknown && result.Certificate.Status != domain.VerificationStatusWarning {
		t.Fatalf("el certificado QA de %s debería quedar sin revocación acreditada, estado=%q motivo=%q",
			fixturePath, result.Certificate.Status, result.Certificate.Reason)
	}
	if result.Valid {
		t.Fatalf("la firma QA de %s no debe considerarse válida sin revocación acreditada", fixturePath)
	}
	for _, signerRef := range signers {
		if signerRef.NotAfter.IsZero() {
			t.Fatalf("el firmante de %s no expone NotAfter", fixturePath)
		}
		if signerRef.IsExpired(time.Now()) {
			t.Fatalf("el certificado QA de %s no debería estar caducado", fixturePath)
		}
	}
}
