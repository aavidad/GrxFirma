// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

	"grxfirma/internal/adapters/outbound/desktop/nssstore"
)

// herramientaRegistrada crea en bin un envoltorio que anota sus argumentos en
// registro y ejecuta la herramienta real, conservando los descriptores
// heredados.
func herramientaRegistrada(t *testing.T, bin, nombre, registro string) {
	t.Helper()
	real, err := exec.LookPath(nombre)
	if err != nil {
		t.Skipf("%s no está instalado: %v", nombre, err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"" + nombre + " $*\" >> '" + registro + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, nombre), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func ejecutar(t *testing.T, nombre string, args ...string) {
	t.Helper()
	// #nosec G204 -- herramientas del sistema con argumentos fijos de la prueba.
	if out, err := exec.Command(nombre, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v (%s)", nombre, args, err, out)
	}
}

// F-03: la exportación desde NSS no deja en el TMPDIR compartido ni el P12, ni
// la contraseña, ni el clon de key4.db, y las contraseñas llegan a pk12util y
// openssl por descriptor heredado, no por un fichero.
func TestKeyFor_NoDejaSecretosEnTemporales(t *testing.T) {
	for _, herramienta := range []string{"certutil", "pk12util", "openssl"} {
		if _, err := exec.LookPath(herramienta); err != nil {
			t.Skipf("%s no está instalado: %v", herramienta, err)
		}
	}
	raiz := t.TempDir()
	home := filepath.Join(raiz, "home")
	tmp := filepath.Join(raiz, "tmp")
	bin := filepath.Join(raiz, "bin")
	db := filepath.Join(raiz, "nssdb")
	for _, dir := range []string{home, tmp, bin, db} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	// Identidad sintética importada en una base NSS real sin contraseña.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: "Firmante de prueba"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	material := filepath.Join(raiz, "material")
	if err := os.Mkdir(material, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(material, "clave.pem")
	crtPath := filepath.Join(material, "cert.pem")
	p12Path := filepath.Join(material, "identidad.p12")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	ejecutar(t, "certutil", "-N", "-d", "sql:"+db, "--empty-password")
	ejecutar(t, "openssl", "pkcs12", "-export", "-inkey", keyPath, "-in", crtPath, "-name", "Firmante de prueba", "-out", p12Path, "-passout", "pass:x")
	ejecutar(t, "pk12util", "-i", p12Path, "-d", "sql:"+db, "-W", "x", "-K", "")

	registro := filepath.Join(raiz, "ordenes.log")
	herramientaRegistrada(t, bin, "pk12util", registro)
	herramientaRegistrada(t, bin, "openssl", registro)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("GRXFIRMA_NSS_SLOT_PASSWORD", "")
	t.Setenv("GRXFIRMA_NSS_EXPORT_PASSWORD", "")

	almacen := nssstore.NewConHerramientas("certutil", "pk12util", []string{db})
	refs, err := almacen.List(context.Background())
	if err != nil || len(refs) != 1 {
		t.Fatalf("List() = %d refs, %v", len(refs), err)
	}
	clave, err := almacen.KeyFor(context.Background(), refs[0])
	if err != nil {
		t.Fatalf("KeyFor() error = %v", err)
	}
	if closer, ok := clave.(interface{ Close() error }); ok {
		_ = closer.Close()
	}

	ordenes, err := os.ReadFile(registro)
	if err != nil {
		t.Fatal(err)
	}
	for _, linea := range strings.Split(strings.TrimSpace(string(ordenes)), "\n") {
		campos := strings.Fields(linea)
		for i, campo := range campos {
			if i == 0 || (campos[i-1] != "-k" && campos[i-1] != "-w" && campos[i-1] != "-passin") {
				continue
			}
			if !strings.HasPrefix(campo, "/dev/fd/") && !strings.HasPrefix(campo, "fd:") {
				t.Fatalf("contraseña pasada por fichero (%s %s): %s", campos[i-1], campo, linea)
			}
		}
	}
	restos, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, resto := range restos {
		t.Errorf("quedó en TMPDIR: %s", resto.Name())
	}
	trabajo, err := filepath.Glob(filepath.Join(home, ".cache", "grxfirma", "temporal", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(trabajo) != 0 {
		t.Errorf("quedó trabajo NSS sin borrar: %v", trabajo)
	}
}
