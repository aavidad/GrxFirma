// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs12importer_test

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
	"path/filepath"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"software.sslmate.com/src/go-pkcs12"
)

func TestImport_DesdeP12ConContrasena(t *testing.T) {
	t.Parallel()

	data := generarP12(t, "Prueba P12", "valor-prueba", false)
	importador := pkcs12importer.New()
	ref, err := importador.Import(context.Background(), data, "valor-prueba")
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if ref.Subject != "Prueba P12" {
		t.Fatalf("Subject = %q, want %q", ref.Subject, "Prueba P12")
	}
	if ref.ID == "" || ref.Fingerprint == "" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestImport_DesdeP12SinContrasena(t *testing.T) {
	t.Parallel()

	data := generarP12(t, "Prueba P12", "", true)
	importador := pkcs12importer.New()
	ref, err := importador.Import(context.Background(), data, "")
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if ref.Subject != "Prueba P12" {
		t.Fatalf("Subject = %q, want %q", ref.Subject, "Prueba P12")
	}
}

func TestImportPEMFiles_Exito(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certPEM, keyPEM := generarPEM(t)
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}

	importador := pkcs12importer.New()
	identidad, err := importador.ImportPEMFiles(context.Background(), certPath, keyPath)
	if err != nil {
		t.Fatalf("ImportPEMFiles() error = %v", err)
	}
	if identidad.Reference.Subject != "PEM Prueba" {
		t.Fatalf("Subject = %q, want %q", identidad.Reference.Subject, "PEM Prueba")
	}
	if identidad.Signer == nil || identidad.Certificate == nil {
		t.Fatalf("identidad incompleta: %+v", identidad)
	}
}

func TestImportP12File_RechazaSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.p12")
	link := filepath.Join(dir, "credential.p12")
	if err := os.WriteFile(target, []byte("not-a-p12"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := pkcs12importer.New().ImportP12File(context.Background(), link, "password"); err == nil {
		t.Fatal("ImportP12File accepted a final-component symlink")
	}
}

func TestImportP12File_RechazaFicheroDemasiadoGrande(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.p12")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := file.Truncate(16*1024*1024 + 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := pkcs12importer.New().ImportP12File(context.Background(), path, "password"); err == nil {
		t.Fatal("ImportP12File accepted an oversized credential")
	}
}

func TestImport_DesdePEMBundle(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generarPEM(t)
	bundle := append(append([]byte(nil), certPEM...), keyPEM...)

	importador := pkcs12importer.New()
	ref, err := importador.Import(context.Background(), bundle, "")
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if ref.Subject != "PEM Prueba" {
		t.Fatalf("Subject = %q, want %q", ref.Subject, "PEM Prueba")
	}
}

func generarPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()

	priv, cert := generarMaterialCertificado(t, "PEM Prueba")
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey() error = %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

func generarP12(t *testing.T, commonName, password string, passwordless bool) []byte {
	t.Helper()

	priv, cert := generarMaterialCertificado(t, commonName)
	var (
		data []byte
		err  error
	)
	if passwordless {
		data, err = pkcs12.Passwordless.Encode(priv, cert, nil, "")
	} else {
		data, err = pkcs12.Modern.Encode(priv, cert, nil, password)
	}
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	return data
}

func generarMaterialCertificado(t *testing.T, commonName string) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          bigInt(2),
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

func bigInt(v int64) *big.Int {
	return big.NewInt(v)
}
