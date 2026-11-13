// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"path/filepath"
	"testing"
)

// D-02: el nombre del documento viene del sobre cifrado, que elige el
// remitente. Su extensión solo se conserva si es de un tipo documental; si
// no, la salida es .bin y "Abrir" no puede lanzar un ejecutable.
func TestConstruirSalidaDesprotegida_NoAceptaExtensionEjecutable(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(string(filepath.Separator)+"datos", "recibidos")
	entrada := filepath.Join(dir, "nomina.afp")
	casos := []struct {
		documento string
		want      string
	}{
		{"x.exe", "nomina_desprotegido.bin"},
		{"x.lnk", "nomina_desprotegido.bin"},
		{"x.hta", "nomina_desprotegido.bin"},
		{"x.js", "nomina_desprotegido.bin"},
		{"x.scr", "nomina_desprotegido.bin"},
		{"x.bat", "nomina_desprotegido.bin"},
		{"x.desktop", "nomina_desprotegido.bin"},
		{"x.txt:evil", "nomina_desprotegido.bin"},
		{"x.txt\x00.exe", "nomina_desprotegido.bin"},
		{"x.pdf\n", "nomina_desprotegido.bin"},
		{"sin_extension", "nomina_desprotegido.bin"},
		{"x.PDF", "nomina_desprotegido.PDF"},
		{"informe.docx", "nomina_desprotegido.docx"},
		{"datos.xml", "nomina_desprotegido.xml"},
	}
	for _, c := range casos {
		got := construirSalidaDesprotegidaRutaIPC(entrada, c.documento)
		if want := filepath.Join(dir, c.want); got != want {
			t.Errorf("documento %q: salida %q; se esperaba %q", c.documento, got, want)
		}
	}
}

// D-02: si la entrada no da un nombre base, el del sobre no puede sacar la
// salida de la carpeta de la entrada ni colar una extensión ejecutable.
func TestConstruirSalidaDesprotegida_NombreDelSobreNoSaleDeLaCarpeta(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(string(filepath.Separator)+"datos", "recibidos")
	entrada := filepath.Join(dir, ".afp")
	got := construirSalidaDesprotegidaRutaIPC(entrada, "../../evil.exe.pdf")
	if filepath.Dir(got) != dir {
		t.Fatalf("la salida %q salió de %q", got, dir)
	}
	if filepath.Ext(got) != ".pdf" {
		t.Fatalf("extensión inesperada en %q", got)
	}
	got = construirSalidaDesprotegidaRutaIPC(filepath.Join(dir, "nomina.exe.afp"), "documento")
	if filepath.Ext(got) != ".bin" {
		t.Fatalf("la extensión del nombre de entrada se coló en %q", got)
	}
}
