// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && cgo && nss_cgo

package nssstore

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"grxfirma/internal/testsupport/exttools"
)

// crearBaseNSSConCertificado construye una base NSS real (cert9.db) con un
// certificado y su clave importados vía pk12util. Las passwords van siempre
// por fichero, nunca por argv (política T111).
func crearBaseNSSConCertificado(t *testing.T) (dir string, fingerprint string) {
	t.Helper()
	if !exttools.Available(t, "certutil") || !exttools.Available(t, "pk12util") {
		t.Skip("certutil/pk12util no disponibles para preparar la base NSS de prueba")
	}

	dir = t.TempDir()

	// Base NSS con password vacía.
	dbPassFile := filepath.Join(dir, "db-pass.txt")
	if err := os.WriteFile(dbPassFile, []byte("\n"), 0o600); err != nil {
		t.Fatalf("no se pudo escribir el fichero de password de la base: %v", err)
	}
	if out, err := exec.Command("certutil", "-N", "-d", "sql:"+dir, "-f", dbPassFile).CombinedOutput(); err != nil {
		t.Fatalf("certutil -N falló: %v\n%s", err, out)
	}

	// Certificado autofirmado + clave, empaquetados como P12.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generando clave RSA: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "Prueba NSS CGo"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creando certificado: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parseando certificado: %v", err)
	}

	p12, err := pkcs12.Modern.Encode(key, cert, nil, "p12-pass-prueba")
	if err != nil {
		t.Fatalf("codificando P12: %v", err)
	}
	p12Path := filepath.Join(dir, "prueba.p12")
	if err := os.WriteFile(p12Path, p12, 0o600); err != nil {
		t.Fatalf("escribiendo P12: %v", err)
	}
	p12PassFile := filepath.Join(dir, "p12-pass.txt")
	if err := os.WriteFile(p12PassFile, []byte("p12-pass-prueba"), 0o600); err != nil {
		t.Fatalf("escribiendo password del P12: %v", err)
	}

	if out, err := exec.Command("pk12util",
		"-i", p12Path,
		"-d", "sql:"+dir,
		"-k", dbPassFile,
		"-w", p12PassFile,
	).CombinedOutput(); err != nil {
		t.Fatalf("pk12util -i falló: %v\n%s", err, out)
	}

	huella := sha256.Sum256(cert.Raw)
	return dir, hex.EncodeToString(huella[:])
}

// TestListarEnRutaNativo_BaseReal verifica que el listado CGo con libnss3
// encuentra el certificado importado en una base NSS real, con el mismo
// fingerprint SHA-256 que calcula el resto del catálogo.
func TestListarEnRutaNativo_BaseReal(t *testing.T) {
	dir, fingerprint := crearBaseNSSConCertificado(t)

	refs, err := listarEnRutaNativo(context.Background(), dir)
	if err != nil {
		t.Fatalf("listarEnRutaNativo() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("se esperaba 1 certificado, obtenidos %d", len(refs))
	}
	if refs[0].Fingerprint != fingerprint {
		t.Errorf("fingerprint = %s, quiere %s", refs[0].Fingerprint, fingerprint)
	}
	if refs[0].Subject != "Prueba NSS CGo" {
		t.Errorf("subject = %q, quiere %q", refs[0].Subject, "Prueba NSS CGo")
	}
}

// TestListarEnRutaNativo_RutaInexistente verifica que una ruta sin base NSS
// produce error (y por tanto el dispatcher degradaría a certutil).
func TestListarEnRutaNativo_RutaInexistente(t *testing.T) {
	_, err := listarEnRutaNativo(context.Background(), filepath.Join(t.TempDir(), "no-existe"))
	if err == nil {
		t.Fatal("se esperaba error al abrir una ruta sin base NSS")
	}
}

// TestAlmacenList_CGoContraBaseReal verifica el flujo completo del catálogo
// (Almacen.List) usando el listado nativo sobre la base real.
func TestAlmacenList_CGoContraBaseReal(t *testing.T) {
	dir, fingerprint := crearBaseNSSConCertificado(t)

	a := NewConRutas("certutil", []string{dir})
	refs, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(refs) != 1 || refs[0].Fingerprint != fingerprint {
		t.Fatalf("List() = %+v, se esperaba solo el certificado %s", refs, fingerprint)
	}
}
