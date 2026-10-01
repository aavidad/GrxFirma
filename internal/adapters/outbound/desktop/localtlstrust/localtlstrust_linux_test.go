// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package localtlstrust

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstaladorNSS_InstalaYNoReinstala(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	nssDir := filepath.Join(dir, "perfil")
	if err := os.MkdirAll(nssDir, 0o700); err != nil {
		t.Fatalf("mkdir nss: %v", err)
	}

	certFile := filepath.Join(dir, "localhost.pem")
	cert := generarCertificadoPrueba(t, certFile)

	scriptPath := filepath.Join(dir, "certutil")
	if err := os.WriteFile(scriptPath, []byte(scriptMockCertutil()), 0o755); err != nil {
		t.Fatalf("script certutil: %v", err)
	}

	instalador := &instaladorNSS{
		certutil: scriptPath,
		rutas:    []string{nssDir},
	}
	ctx := context.Background()

	if err := instalador.EnsureTrusted(ctx, certFile, certFile, cert); err != nil {
		t.Fatalf("EnsureTrusted instalacion: %v", err)
	}
	instalado, err := instalador.certificadoInstalado(ctx, nssDir, nicknameParaCertificado(cert), cert)
	if err != nil {
		t.Fatalf("certificadoInstalado: %v", err)
	}
	if !instalado {
		t.Fatal("el certificado debería quedar instalado en NSS")
	}

	contador := filepath.Join(nssDir, "cert-added-count")
	valorAntes, err := os.ReadFile(contador)
	if err != nil {
		t.Fatalf("leer contador: %v", err)
	}

	if err := instalador.EnsureTrusted(ctx, certFile, certFile, cert); err != nil {
		t.Fatalf("EnsureTrusted idempotente: %v", err)
	}

	valorDespues, err := os.ReadFile(contador)
	if err != nil {
		t.Fatalf("leer contador despues: %v", err)
	}
	if string(valorAntes) != string(valorDespues) {
		t.Fatal("no debería reinstalar el certificado si ya está vigente")
	}
}

func TestManagedNSS_RotacionRetiraSoloHuellaInventariada(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nssDir := filepath.Join(dir, "perfil")
	if err := os.Mkdir(nssDir, 0o700); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "certutil")
	if err := os.WriteFile(scriptPath, []byte(scriptMockCertutil()), 0o755); err != nil {
		t.Fatal(err)
	}
	i := &instaladorNSS{certutil: scriptPath, rutas: []string{nssDir}}
	certFile := filepath.Join(dir, "root.crt.pem")
	oldCert := newManagedLocalCATestCertificate(t, 101, true)
	newCert := newManagedLocalCATestCertificate(t, 102, true)
	writeManagedNSSCertFixture(t, certFile, oldCert)
	if err := i.ensureManagedTrusted(context.Background(), certFile, certFile, oldCert); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(nssDir, managedNSSNickname(fingerprintSHA256(oldCert))+".pem")
	writeManagedNSSCertFixture(t, certFile, newCert)
	if err := i.ensureManagedTrusted(context.Background(), certFile, certFile, newCert); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("la CA anterior no se retiró: %v", err)
	}
	newPath := filepath.Join(nssDir, managedNSSNickname(fingerprintSHA256(newCert))+".pem")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("la CA nueva no está instalada: %v", err)
	}
	inventory, err := loadManagedNSSTrustInventory(certFile + managedNSSTrustInventorySuffix)
	if err != nil || len(inventory.Entries) != 1 || inventory.Entries[0].Fingerprint != fingerprintSHA256(newCert) {
		t.Fatalf("inventario NSS tras rotación inválido: %#v, %v", inventory, err)
	}
}

func TestManagedNSS_ConservaAliasLegacySinInventario(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nssDir := filepath.Join(dir, "perfil")
	if err := os.Mkdir(nssDir, 0o700); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "certutil")
	if err := os.WriteFile(scriptPath, []byte(scriptMockCertutil()), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := newManagedLocalCATestCertificate(t, 103, true)
	legacyPath := filepath.Join(nssDir, nicknameLocalhostRoot+".pem")
	writeManagedNSSCertFixture(t, legacyPath, legacy)
	current := newManagedLocalCATestCertificate(t, 104, true)
	certFile := filepath.Join(dir, "root.crt.pem")
	writeManagedNSSCertFixture(t, certFile, current)
	i := &instaladorNSS{certutil: scriptPath, rutas: []string{nssDir}}
	if err := i.ensureManagedTrusted(context.Background(), certFile, certFile, current); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("se borró CA legacy sin inventario: %v", err)
	}
}

func TestManagedNSS_SinCertutilDevuelveSentinelDeCompatibilidad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nssDir := filepath.Join(dir, "perfil")
	if err := os.Mkdir(nssDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cert := newManagedLocalCATestCertificate(t, 105, true)
	certFile := filepath.Join(dir, "root.crt.pem")
	writeManagedNSSCertFixture(t, certFile, cert)
	i := &instaladorNSS{certutil: filepath.Join(dir, "certutil-ausente"), rutas: []string{nssDir}}
	if err := i.ensureManagedTrusted(context.Background(), certFile, certFile, cert); err != ErrHerramientaNoDisponible {
		t.Fatalf("sin certutil: %v", err)
	}
}

func writeManagedNSSCertFixture(t *testing.T, path string, cert *x509.Certificate) {
	t.Helper()
	raw := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func generarCertificadoPrueba(t *testing.T, certFile string) *x509.Certificate {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost"},
		IPAddresses: nil,
		IsCA:        false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certFile, pemData, 0o600); err != nil {
		t.Fatalf("WriteFile cert: %v", err)
	}
	return cert
}

func scriptMockCertutil() string {
	return `#!/bin/sh
set -eu

db=""
nick=""
input=""
modo=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -L|-A|-D) modo="$1"; shift ;;
    -d) db="$2"; shift 2 ;;
    -n) nick="$2"; shift 2 ;;
    -i) input="$2"; shift 2 ;;
    -a|-t) shift ;;
    "P,,") shift ;;
    *) shift ;;
  esac
done

certfile="$db/${nick}.pem"
countfile="$db/cert-added-count"

case "$modo" in
  -L)
    if [ -f "$certfile" ]; then
      cat "$certfile"
      exit 0
    fi
    exit 255
    ;;
  -D)
    rm -f "$certfile"
    exit 0
    ;;
  -A)
    cp "$input" "$certfile"
    if [ ! -f "$countfile" ]; then
      echo 1 > "$countfile"
    else
      value="$(cat "$countfile")"
      expr "$value" + 1 > "$countfile"
    fi
    exit 0
    ;;
esac

exit 1
`
}
