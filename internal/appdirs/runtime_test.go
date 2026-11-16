// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package appdirs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func reservaEsperada(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "GrxFirma", "run")
	}
	return filepath.Join(home, ".local", "run")
}

func prepararPerfil(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	return home
}

func TestRuntimeUsuarioSinXDGUsaCarpetaPersonalNuncaTemporal(t *testing.T) {
	home := prepararPerfil(t)
	t.Setenv("XDG_RUNTIME_DIR", "")
	dir, err := RuntimeUsuario()
	if err != nil {
		t.Fatal(err)
	}
	if dir != reservaEsperada(home) {
		t.Fatalf("carpeta %q, se esperaba %q", dir, reservaEsperada(home))
	}
	if !strings.HasPrefix(dir, home+string(filepath.Separator)) {
		t.Fatalf("la reserva no está dentro de la carpeta personal: %q", dir)
	}
}

func TestRuntimeUsuarioSinCarpetaPersonalFallaCerrado(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if dir, err := RuntimeUsuario(); !errors.Is(err, ErrRuntimeNoDisponible) {
		t.Fatalf("sin carpeta personal devolvió %q, %v", dir, err)
	}
}

func TestRuntimeUsuarioRechazaXDGRelativo(t *testing.T) {
	home := prepararPerfil(t)
	t.Setenv("XDG_RUNTIME_DIR", "relativo")
	dir, err := RuntimeUsuario()
	if err != nil || dir != reservaEsperada(home) {
		t.Fatalf("XDG relativo admitido: %q, %v", dir, err)
	}
}
