// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// La CLI no lista ni exporta el fichero de sesión si su carpeta no es
// privada, y al guardar la deja privada antes de escribir.
func TestAutoseleccionSesionCarpetaNoPrivadaNoSeLeeYSeCorrigeAlGuardar(t *testing.T) {
	tmp := t.TempDir()
	sesion := prepararSesionAutoseleccionPrueba(t, tmp)
	if err := os.WriteFile(sesion, []byte(`{"https://plantado":"cert-ajeno"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(sesion)
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	ejecutar := func(args ...string) string {
		t.Helper()
		a := New(nil, nil)
		a.ConfigDir = tmp
		var stdout, stderr strings.Builder
		a.Stdout, a.Stderr = &stdout, &stderr
		if rc := a.Run(context.Background(), append([]string{"-modo-cli"}, args...)); rc != 0 {
			t.Fatalf("%v: rc=%d stderr=%s", args, rc, stderr.String())
		}
		return stdout.String()
	}
	if salida := ejecutar("-operacion", "listar-autoseleccion-cert"); strings.Contains(salida, "plantado") {
		t.Fatalf("se listó el fichero de una carpeta no privada: %s", salida)
	}
	export := filepath.Join(tmp, "export.json")
	ejecutar("-operacion", "exportar-autoseleccion-cert", "-fichero-autoseleccion", export)
	if data, err := os.ReadFile(export); err != nil || strings.Contains(string(data), "plantado") {
		t.Fatalf("se exportó el fichero de una carpeta no privada: %s %v", data, err)
	}
	if err := os.WriteFile(export, []byte(`{"persistente":{},"sesion":{"https://nuevo":"cert-nuevo"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ejecutar("-operacion", "importar-autoseleccion-cert", "-fichero-autoseleccion", export)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("la carpeta de sesión quedó con permisos %o", info.Mode().Perm())
	}
	if salida := ejecutar("-operacion", "listar-autoseleccion-cert"); !strings.Contains(salida, "https://nuevo -> cert-nuevo") {
		t.Fatalf("no se lee lo importado: %s", salida)
	}
}
