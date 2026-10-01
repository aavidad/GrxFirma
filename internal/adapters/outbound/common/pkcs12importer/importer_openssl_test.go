// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs12importer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/testsupport/exttools"
)

func TestImportarDesdePKCS12ConOpenSSL(t *testing.T) {
	exttools.Require(t, "openssl")

	priv, cert := generarMaterialOpenSSL(t, "P12 OpenSSL")
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "clave.pem")
	certPath := filepath.Join(dir, "cert.pem")
	p12Path := filepath.Join(dir, "cert.p12")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}

	cmd := exec.Command("openssl", "pkcs12", "-export", "-in", certPath, "-inkey", keyPath, "-out", p12Path, "-passout", "pass:valor-prueba")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("openssl pkcs12 -export error = %v (%s)", err, string(out))
	}
	data, err := os.ReadFile(p12Path)
	if err != nil {
		t.Fatalf("ReadFile(p12) error = %v", err)
	}

	identidad, err := importarDesdePKCS12ConOpenSSL(context.Background(), data, "valor-prueba")
	if err != nil {
		t.Fatalf("importarDesdePKCS12ConOpenSSL() error = %v", err)
	}
	if identidad.Reference.Subject != "P12 OpenSSL" {
		t.Fatalf("Subject = %q, want %q", identidad.Reference.Subject, "P12 OpenSSL")
	}
	if identidad.Signer == nil || identidad.Certificate == nil {
		t.Fatalf("identidad incompleta: %+v", identidad)
	}
}

func TestImportarDesdePKCS12ConOpenSSL_AdmiteContenedorLegacy(t *testing.T) {
	exttools.Require(t, "openssl")

	priv, cert := generarMaterialOpenSSL(t, "P12 OpenSSL Legacy")
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "clave.pem")
	certPath := filepath.Join(dir, "cert.pem")
	p12Path := filepath.Join(dir, "cert-legacy.p12")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}

	cmd := exec.Command(
		"openssl",
		"pkcs12",
		"-export",
		"-legacy",
		"-in", certPath,
		"-inkey", keyPath,
		"-out", p12Path,
		"-passout", "pass:valor-prueba",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "Unknown option") ||
			strings.Contains(string(out), "unknown option") {
			t.Skipf("OpenSSL sin soporte para -legacy: %v", err)
		}
		t.Fatalf("openssl pkcs12 -export -legacy error = %v (%s)", err, string(out))
	}
	data, err := os.ReadFile(p12Path)
	if err != nil {
		t.Fatalf("ReadFile(p12) error = %v", err)
	}

	identidad, err := importarDesdePKCS12ConOpenSSL(
		context.Background(),
		data,
		"valor-prueba",
	)
	if err != nil {
		t.Fatalf("importarDesdePKCS12ConOpenSSL() error = %v", err)
	}
	if identidad.Reference.Subject != "P12 OpenSSL Legacy" {
		t.Fatalf(
			"Subject = %q, want %q",
			identidad.Reference.Subject,
			"P12 OpenSSL Legacy",
		)
	}
	if identidad.Signer == nil || identidad.Certificate == nil {
		t.Fatalf("identidad incompleta: %+v", identidad)
	}
}

func TestImportarDesdePKCS12ConOpenSSL_RespetaContextoCancelado(t *testing.T) {
	exttools.Require(t, "openssl")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := importarDesdePKCS12ConOpenSSL(ctx, []byte("p12-invalido"), "secreto")
	if err == nil {
		t.Fatal("se esperaba error con el contexto cancelado")
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("error = %v, se esperaba context canceled", err)
	}
}

func TestImportIdentity_RecurreAOpenSSLSiDecodeChainFalla(t *testing.T) {
	exttools.Require(t, "openssl")

	priv, cert := generarMaterialOpenSSL(t, "Fallback OpenSSL")
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "clave.pem")
	certPath := filepath.Join(dir, "cert.pem")
	p12Path := filepath.Join(dir, "cert.p12")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}

	cmd := exec.Command("openssl", "pkcs12", "-export", "-in", certPath, "-inkey", keyPath, "-out", p12Path, "-passout", "pass:valor-prueba")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("openssl pkcs12 -export error = %v (%s)", err, string(out))
	}
	data, err := os.ReadFile(p12Path)
	if err != nil {
		t.Fatalf("ReadFile(p12) error = %v", err)
	}

	importador := New()
	identidad, err := importador.ImportIdentity(context.Background(), data, "valor-prueba")
	if err != nil {
		t.Fatalf("ImportIdentity() error = %v", err)
	}
	if identidad.Reference.Subject != "Fallback OpenSSL" {
		t.Fatalf("Subject = %q, want %q", identidad.Reference.Subject, "Fallback OpenSSL")
	}
}

func generarMaterialOpenSSL(t *testing.T, commonName string) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	return priv, cert
}
