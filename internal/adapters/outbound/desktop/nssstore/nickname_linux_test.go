// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/nssstore"
)

// F-10 con NSS real: dos certificados del mismo sujeto comparten nickname y
// `pk12util -n` exporta los dos pares. Cada certificado listado debe dar su
// propia clave.
func TestKeyFor_MismoNicknameDaLaClaveDelCertificado(t *testing.T) {
	for _, herramienta := range []string{"certutil", "pk12util", "openssl"} {
		if _, err := exec.LookPath(herramienta); err != nil {
			t.Skipf("%s no está instalado: %v", herramienta, err)
		}
	}
	raiz := t.TempDir()
	db := filepath.Join(raiz, "nssdb")
	material := filepath.Join(raiz, "material")
	for _, dir := range []string{db, material, filepath.Join(raiz, "home")} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ejecutar(t, "certutil", "-N", "-d", "sql:"+db, "--empty-password")
	for _, par := range []struct{ nombre, serie, dias string }{{"viejo", "1", "10"}, {"nuevo", "2", "30"}} {
		key := filepath.Join(material, par.nombre+".key")
		crt := filepath.Join(material, par.nombre+".crt")
		p12 := filepath.Join(material, par.nombre+".p12")
		ejecutar(t, "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", crt,
			"-subj", "/C=ES/O=FNMT/CN=APELLIDO NOMBRE - 00000000T", "-days", par.dias, "-set_serial", par.serie)
		ejecutar(t, "openssl", "pkcs12", "-export", "-inkey", key, "-in", crt, "-out", p12, "-passout", "pass:x")
		ejecutar(t, "pk12util", "-i", p12, "-d", "sql:"+db, "-W", "x", "-K", "")
	}
	t.Setenv("HOME", filepath.Join(raiz, "home"))
	t.Setenv("GRXFIRMA_NSS_SLOT_PASSWORD", "")
	t.Setenv("GRXFIRMA_NSS_EXPORT_PASSWORD", "")

	almacen := nssstore.NewConHerramientas("certutil", "pk12util", []string{db})
	refs, err := almacen.List(context.Background())
	if err != nil || len(refs) == 0 {
		t.Fatalf("List() = %d refs, %v", len(refs), err)
	}
	t.Logf("certificados listados: %d", len(refs))
	for _, ref := range refs {
		clave, err := almacen.KeyFor(context.Background(), ref)
		if err != nil {
			t.Fatalf("KeyFor(%s) error = %v", ref.Fingerprint, err)
		}
		chain := clave.(interface{ CertificateChainDER() [][]byte }).CertificateChainDER()
		if len(chain) == 0 || !mismaHuella(chain[0], ref.Fingerprint) {
			t.Fatalf("KeyFor(%s) devolvió otro certificado", ref.Fingerprint)
		}
	}
}

func mismaHuella(der []byte, huella string) bool {
	sum := sha256.Sum256(der)
	return strings.EqualFold(hex.EncodeToString(sum[:]), huella)
}
