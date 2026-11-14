// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build unix

package afirmahandler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/ports"
)

// El enlace llega a AutoFirma como un único argumento y sin pasar por una
// shell: «$(...)», «;» y «&» no se interpretan.
func TestReenviarPasaElEnlaceComoUnSoloArgumentoSinShell(t *testing.T) {
	dir := t.TempDir()
	salida := filepath.Join(dir, "argumentos")
	auto := filepath.Join(dir, "autofirma")
	script := "#!/bin/sh\n{ printf '%s\\n' \"$#\"; printf '%s\\n' \"$1\"; printf '%s\\n' \"$" + EnvReenvioAfirma + "\"; } > '" + salida + ".tmp'\nmv '" + salida + ".tmp' '" + salida + "'\n"
	if err := os.WriteFile(auto, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	uri := "afirma://sign?a=1&b=$(touch%20" + filepath.Join(dir, "inyectado") + ");c=1|d&e>f"
	if err := Reenviar(context.Background(), auto, uri); err != nil {
		t.Fatal(err)
	}
	var datos []byte
	for i := 0; i < 100; i++ {
		var err error
		if datos, err = os.ReadFile(salida); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	lineas := strings.Split(strings.TrimSuffix(string(datos), "\n"), "\n")
	if len(lineas) != 3 || lineas[0] != "1" || lineas[1] != uri || lineas[2] != "1" {
		t.Fatalf("AutoFirma recibió %q", lineas)
	}
	if _, err := os.Stat(filepath.Join(dir, "inyectado")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("se interpretó el enlace con una shell")
	}
}

func TestParsearExecAutoFirma(t *testing.T) {
	dir := t.TempDir()
	auto := filepath.Join(dir, "autofirma")
	if err := os.WriteFile(auto, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, exec := range []string{auto + " %u", auto + " %U", "  " + auto + "\t%u "} {
		if got, ok := parsearExecAutoFirma(exec); !ok || got != auto {
			t.Errorf("%q: %q %v", exec, got, ok)
		}
	}
	for _, exec := range []string{
		auto,
		auto + " %u %f",
		"sh -c '" + auto + " %u'",
		"env JAVA_OPTS=-Dx=1 " + auto + " %u",
		auto + " --extra %u",
		"/usr/bin/firefox %u",
		"bin/autofirma %u",
		auto + "; rm %u",
		"$HOME/autofirma %u",
		"\"" + auto + "\" %u",
	} {
		if got, ok := parsearExecAutoFirma(exec); ok {
			t.Errorf("%q no debería aceptarse: %q", exec, got)
		}
	}
}

func TestEjecutableAutoFirmaXDG(t *testing.T) {
	f := newXDGFixture(t, nil)
	f.installGrxFirma()
	bin := f.installAutoFirma()
	sel := NewXDGSelector(f.env)
	got, err := sel.EjecutableAutoFirma(context.Background())
	if err != nil || got != bin {
		t.Fatalf("%q %v", got, err)
	}
	// Un lanzador que pasa por una shell no se usa para reenviar.
	f.write(filepath.Join(f.system, "applications", "afirma.desktop"), "[Desktop Entry]\nType=Application\n"+
		"Exec=sh -c '"+bin+" %u'\nMimeType=x-scheme-handler/afirma;\n")
	if _, err := sel.EjecutableAutoFirma(context.Background()); !errors.Is(err, ports.ErrAutoFirmaNoInstalada) {
		t.Fatalf("Exec con shell: %v", err)
	}
}
