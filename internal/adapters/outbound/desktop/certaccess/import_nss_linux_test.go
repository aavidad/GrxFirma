// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package certaccess

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// F-03: la contraseña del P12 llega a pk12util por descriptor heredado y no
// queda nada en el TMPDIR compartido ni en el directorio privado.
func TestImportarP12ANSS_ContrasenaPorDescriptor(t *testing.T) {
	for _, herramienta := range []string{"certutil", "pk12util", "openssl"} {
		if _, err := exec.LookPath(herramienta); err != nil {
			t.Skipf("%s no está instalado: %v", herramienta, err)
		}
	}
	raiz := t.TempDir()
	home := filepath.Join(raiz, "home")
	tmp := filepath.Join(raiz, "tmp")
	db := filepath.Join(raiz, "nssdb")
	material := filepath.Join(raiz, "material")
	for _, dir := range []string{home, tmp, db, material} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	correr := func(nombre string, args ...string) string {
		t.Helper()
		// #nosec G204 -- herramientas del sistema con argumentos fijos de la prueba.
		out, err := exec.Command(nombre, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v (%s)", nombre, args, err, out)
		}
		return string(out)
	}
	keyPath := filepath.Join(material, "clave.pem")
	crtPath := filepath.Join(material, "cert.pem")
	p12Path := filepath.Join(material, "identidad.p12")
	correr("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", keyPath, "-out", crtPath, "-subj", "/CN=Importada de prueba", "-days", "1")
	correr("openssl", "pkcs12", "-export", "-inkey", keyPath, "-in", crtPath, "-name", "Importada de prueba", "-out", p12Path, "-passout", "pass:clave con espacios")
	correr("certutil", "-N", "-d", "sql:"+db, "--empty-password")

	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", tmp)
	if err := importarP12ANSS(context.Background(), db, p12Path, "clave con espacios"); err != nil {
		t.Fatalf("importarP12ANSS() error = %v", err)
	}
	if lista := correr("certutil", "-L", "-d", "sql:"+db); !strings.Contains(lista, "Importada de prueba") {
		t.Fatalf("el certificado no quedó en NSS:\n%s", lista)
	}
	if restos, _ := os.ReadDir(tmp); len(restos) != 0 {
		t.Fatalf("quedaron restos en TMPDIR: %v", restos)
	}
	if restos, _ := filepath.Glob(filepath.Join(home, ".cache", "grxfirma", "temporal", "*")); len(restos) != 0 {
		t.Fatalf("quedaron restos en el directorio privado: %v", restos)
	}
}
