// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package conformance_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/tsaclient"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

const expectedDSSConformanceResult = "TOTAL_PASSED"

func TestConformidadDSS_MuestrasV2(t *testing.T) {
	runner := requireDSSConformanceRunner(t)

	priv, cert := generarCertRSAConformance(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)

	casos := []struct {
		name     string
		format   domain.SignatureFormat
		filename string
		content  []byte
		mime     string
	}{
		{name: "cades", format: domain.FormatCAdES, filename: "muestra.txt", content: []byte("muestra cades"), mime: "text/plain"},
		{name: "xades", format: domain.FormatXAdES, filename: "muestra.xml", content: []byte("<doc>muestra xades</doc>"), mime: "application/xml"},
		{name: "pades", format: domain.FormatPAdES, filename: "muestra.pdf", content: pdffixture.Minimal(), mime: "application/pdf"},
	}

	for _, tc := range casos {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			doc, err := domain.NewDocument(tc.filename, tc.content, tc.mime)
			if err != nil {
				t.Fatal(err)
			}
			result, err := motor.Sign(context.Background(), domain.SignatureJob{
				Document: doc,
				Format:   tc.format,
				Action:   domain.ActionSign,
			}, clave)
			if err != nil {
				t.Fatalf("firmando %s: %v", tc.name, err)
			}

			tempDir := t.TempDir()
			signedPath := filepath.Join(tempDir, "signed.bin")
			if err := os.WriteFile(signedPath, result.Data, 0o600); err != nil {
				t.Fatalf("escribiendo firmado: %v", err)
			}
			originalPath := filepath.Join(tempDir, tc.filename)
			if err := os.WriteFile(originalPath, tc.content, 0o600); err != nil {
				t.Fatalf("escribiendo original: %v", err)
			}

			args := []string{
				"--format", tc.name,
				"--signed", signedPath,
				"--expect", expectedDSSConformanceResult,
			}
			if tc.format == domain.FormatCAdES || tc.format == domain.FormatPAdES || tc.format == domain.FormatXAdES {
				args = append(args, "--original", originalPath)
			}

			runDSSConformance(t, runner, args...)
		})
	}
}

func TestConformidadDSS_RunnerAusenteHaceSkip(t *testing.T) {
	if os.Getenv("GRXFIRMA_DSS_RUNNER") != "" {
		t.Skip("solo aplica cuando no hay runner configurado")
	}
	t.Skip("GRXFIRMA_DSS_RUNNER no configurado")
}

func TestValidateDSSConformanceOutput_ExigeEvidenciaEstricta(t *testing.T) {
	t.Parallel()

	valid := "DSS TOTAL_PASSED: formato=PAdES-BASELINE-B indicacion=TOTAL_PASSED subindicacion=N/A\n"
	if err := validateDSSConformanceOutput(valid); err != nil {
		t.Fatalf("validateDSSConformanceOutput() error = %v", err)
	}

	for _, output := range []string{
		"",
		"DSS OK: formato=PAdES-BASELINE-B indicacion=INDETERMINATE subindicacion=NO_CERTIFICATE_CHAIN_FOUND",
		"DSS INTEGRITY_FORMAT_RECOGNIZED: formato=PAdES-BASELINE-B indicacion=INDETERMINATE subindicacion=NO_CERTIFICATE_CHAIN_FOUND",
		"DSS TOTAL_PASSED: formato=PAdES-BASELINE-B indicacion=TOTAL_PASSED",
		"DSS TOTAL_PASSED: formato=PAdES-BASELINE-B indicacion=TOTAL_PASSED subindicacion=N/A\ntexto adicional",
	} {
		if err := validateDSSConformanceOutput(output); err == nil {
			t.Errorf("validateDSSConformanceOutput(%q) debía fallar", output)
		}
	}
}

func TestConformidadDSS_XAdEST(t *testing.T) {
	runner := requireDSSConformanceRunner(t)
	tsaURL := os.Getenv("GRXFIRMA_TSA_URL")
	if tsaURL == "" {
		t.Skip("GRXFIRMA_TSA_URL no configurado")
	}

	priv, cert := generarCertRSAConformance(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil).WithTimestampAuthority(tsaclient.New(tsaURL))

	contenido := []byte("<doc>muestra xades-t</doc>")
	doc, err := domain.NewDocument("muestra-xades-t.xml", contenido, "application/xml")
	if err != nil {
		t.Fatal(err)
	}
	result, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"level": "T"},
	}, clave)
	if err != nil {
		t.Fatalf("firmando XAdES-T: %v", err)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "signed-xades-t.xml")
	if err := os.WriteFile(signedPath, result.Data, 0o600); err != nil {
		t.Fatalf("escribiendo firmado: %v", err)
	}
	originalPath := filepath.Join(tempDir, "muestra-xades-t.xml")
	if err := os.WriteFile(originalPath, contenido, 0o600); err != nil {
		t.Fatalf("escribiendo original: %v", err)
	}

	runDSSConformance(
		t,
		runner,
		"--format", "xades",
		"--signed", signedPath,
		"--original", originalPath,
		"--expect", expectedDSSConformanceResult,
	)
}

func TestConformidadDSS_PAdEST(t *testing.T) {
	runner := requireDSSConformanceRunner(t)
	tsaURL := os.Getenv("GRXFIRMA_TSA_URL")
	if tsaURL == "" {
		t.Skip("GRXFIRMA_TSA_URL no configurado")
	}

	priv, cert := generarCertRSAConformance(t)
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil).WithTimestampAuthority(tsaclient.New(tsaURL))

	contenido := pdffixture.Minimal()
	doc, err := domain.NewDocument("muestra-pades-t.pdf", contenido, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	result, err := motor.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatPAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"level": "T"},
	}, clave)
	if err != nil {
		t.Fatalf("firmando PAdES-T: %v", err)
	}

	tempDir := t.TempDir()
	signedPath := filepath.Join(tempDir, "signed-pades-t.pdf")
	if err := os.WriteFile(signedPath, result.Data, 0o600); err != nil {
		t.Fatalf("escribiendo firmado: %v", err)
	}
	originalPath := filepath.Join(tempDir, "muestra-pades-t.pdf")
	if err := os.WriteFile(originalPath, contenido, 0o600); err != nil {
		t.Fatalf("escribiendo original: %v", err)
	}

	runDSSConformance(
		t,
		runner,
		"--format", "pades",
		"--signed", signedPath,
		"--original", originalPath,
		"--expect", expectedDSSConformanceResult,
	)
}

func requireDSSConformanceRunner(t *testing.T) string {
	t.Helper()

	runner := os.Getenv("GRXFIRMA_DSS_RUNNER")
	if runner == "" {
		t.Skip("GRXFIRMA_DSS_RUNNER no configurado; gate DSS externo no ejecutado")
	}
	expected := os.Getenv("GRXFIRMA_DSS_EXPECTED_RESULT")
	if expected != expectedDSSConformanceResult {
		t.Fatalf(
			"GRXFIRMA_DSS_EXPECTED_RESULT debe ser exactamente %q para ejecutar el gate de conformidad (valor recibido: %q)",
			expectedDSSConformanceResult,
			expected,
		)
	}
	return runner
}

func runDSSConformance(t *testing.T, runner string, args ...string) {
	t.Helper()

	cmd := exec.Command(runner, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf(
			"runner DSS falló: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	if err := validateDSSConformanceOutput(stdout.String()); err != nil {
		t.Fatalf(
			"runner DSS terminó sin evidencia TOTAL_PASSED verificable: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
}

func validateDSSConformanceOutput(output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return errors.New("salida estándar vacía")
	}
	if strings.Contains(output, "\n") {
		return errors.New("se esperaba una única línea de resultado")
	}
	fields := strings.Fields(output)
	if len(fields) != 5 ||
		fields[0] != "DSS" ||
		fields[1] != "TOTAL_PASSED:" ||
		!strings.HasPrefix(fields[2], "formato=") ||
		strings.TrimPrefix(fields[2], "formato=") == "" ||
		fields[3] != "indicacion=TOTAL_PASSED" ||
		fields[4] != "subindicacion=N/A" {
		return fmt.Errorf("resultado no estricto: %q", output)
	}
	return nil
}

func generarCertRSAConformance(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generando clave RSA: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          newSerialConformance(),
		Subject:               pkix.Name{CommonName: "GrxFirma DSS Conformance"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("creando certificado: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parseando certificado: %v", err)
	}
	return priv, cert
}

func newSerialConformance() *big.Int {
	return big.NewInt(time.Now().UnixNano())
}
