// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// Sin carpeta personal válida, la CLI no guarda ni lee material de claves en
// la carpeta temporal ni en la de trabajo: cada orden termina con el texto
// del catálogo y las dos carpetas quedan vacías.
func TestSinCarpetaPersonalNoSeGuardanClavesEnTemporalNiTrabajo(t *testing.T) {
	raiz := t.TempDir()
	temporal := filepath.Join(raiz, "tmp")
	trabajo := filepath.Join(raiz, "trabajo")
	for _, dir := range []string{temporal, trabajo} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	p12Path := filepath.Join(raiz, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12CLI(t, "CLI Sin Perfil", "valor-prueba"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, variable := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(variable, "")
	}
	t.Setenv("TMPDIR", temporal)
	t.Setenv("TMP", temporal)
	t.Setenv("TEMP", temporal)
	t.Chdir(trabajo)

	loc := localizador.Para("es")
	mensaje := loc.T(claveSinCarpetaPersonal)
	if mensaje == claveSinCarpetaPersonal {
		t.Fatalf("falta %s en el catálogo", claveSinCarpetaPersonal)
	}
	ordenes := [][]string{
		{"-operacion", "importar-p12", "-fichero-p12", p12Path, "-contrasena-p12-stdin"},
		{"-operacion", "generar-certificados-tls"},
		{"-operacion", "instalar-confianza-tls"},
		{"-operacion", "estado-almacen-tls"},
		{"-operacion", "limpiar-almacen-tls"},
		{"-operacion", "estado-confianza-tls"},
		{"-operacion", "listar-autoseleccion-cert"},
		{"-operacion", "limpiar-autoseleccion-cert"},
	}
	for _, args := range ordenes {
		adaptador := New(nil, nil).WithLocalizador(loc)
		var stderr strings.Builder
		adaptador.Stdout = io.Discard
		adaptador.Stderr = &stderr
		adaptador.Stdin = strings.NewReader("valor-prueba\n")
		if rc := adaptador.Run(context.Background(), args); rc != 1 {
			t.Fatalf("%v: código=%d stderr=%q", args, rc, stderr.String())
		}
		if !strings.Contains(stderr.String(), mensaje) {
			t.Fatalf("%v: no explica qué falta: %q", args, stderr.String())
		}
	}
	for _, dir := range []string{temporal, trabajo} {
		entradas, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entradas) != 0 {
			t.Fatalf("se escribió en %s: %v", dir, entradas)
		}
	}
}

// Con la carpeta indicada de forma explícita se respeta, como siempre.
func TestCarpetaExplicitaSeRespetaSinCarpetaPersonal(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("APPDATA", "")
	configDir := t.TempDir()
	adaptador := New(nil, nil).WithConfigDir(configDir)
	dir, err := adaptador.directorioTLS()
	if err != nil || dir != filepath.Join(configDir, "tls") {
		t.Fatalf("directorioTLS explícito = %q, %v", dir, err)
	}
	dir, err = adaptador.directorioP12()
	if err != nil || dir != filepath.Join(configDir, "pkcs12") {
		t.Fatalf("directorioP12 explícito = %q, %v", dir, err)
	}
}
