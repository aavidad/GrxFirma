// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certselectionstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func simularSesion(t *testing.T, id string) {
	t.Helper()
	previo := sessionIDActual
	sessionIDActual = func() (string, error) { return id, nil }
	t.Cleanup(func() { sessionIDActual = previo })
}

func TestSessionIDDelSistemaEsEstable(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("sin identificador de arranque en este sistema")
	}
	a, err := sessionID()
	if err != nil || a == "" {
		t.Fatalf("sessionID = %q, %v", a, err)
	}
	if b, err := sessionID(); err != nil || b != a {
		t.Fatalf("sessionID cambia dentro de la misma sesión: %q / %q, %v", a, b, err)
	}
}

// «Solo esta sesión» no sobrevive a un reinicio ni a un nuevo inicio de
// sesión, aunque la carpeta de reserva sí sobreviva.
func TestSessionNoSobreviveAOtroArranque(t *testing.T) {
	perfilSinXDG(t)
	simularSesion(t, "arranque-1")
	store := New(t.TempDir())
	if err := store.SaveSession(context.Background(), "https://portal.example", "cert-1"); err != nil {
		t.Fatal(err)
	}
	anterior, _ := SessionPath()

	simularSesion(t, "arranque-2")
	store = New(t.TempDir())
	if id, ok, err := store.LoadSession(context.Background(), "https://portal.example"); err != nil || ok || id != "" {
		t.Fatalf("preferencia de otro arranque usada: %q, %v, %v", id, ok, err)
	}
	if err := store.SaveSession(context.Background(), "https://otro.example", "cert-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(anterior); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el fichero de la sesión anterior sigue ahí: %v", err)
	}
}

// El fichero de versiones anteriores, sin identificador, no se lee.
func TestSessionIgnoraFicheroSinIdentificador(t *testing.T) {
	perfilSinXDG(t)
	simularSesion(t, "arranque-1")
	ruta, err := SessionPath()
	if err != nil {
		t.Fatal(err)
	}
	viejo := filepath.Join(filepath.Dir(ruta), "preferred-certificates-session.json")
	if err := WriteSessionFile(ruta, map[string]string{}); err != nil { // deja las carpetas privadas
		t.Fatal(err)
	}
	if err := os.WriteFile(viejo, []byte(`{"https://portal.example":"cert-viejo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(t.TempDir())
	if id, ok, err := store.LoadSession(context.Background(), "https://portal.example"); err != nil || ok || id != "" {
		t.Fatalf("fichero sin identificador usado: %q, %v, %v", id, ok, err)
	}
	if borrado, err := RemoveSessionFile(ruta); err != nil || borrado {
		t.Fatalf("RemoveSessionFile = %v, %v", borrado, err)
	}
	if _, err := os.Lstat(viejo); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no se limpió el fichero sin identificador: %v", err)
	}
}

// Sin identificador de sesión no se guardan preferencias de sesión.
func TestSessionSinIdentificadorFallaCerrado(t *testing.T) {
	perfilSinXDG(t)
	previo := sessionIDActual
	sessionIDActual = func() (string, error) { return "", errors.New("sin id") }
	t.Cleanup(func() { sessionIDActual = previo })
	if _, err := SessionPath(); !errors.Is(err, ErrSessionIDNoDisponible) {
		t.Fatalf("SessionPath sin id: %v", err)
	}
	store := New(t.TempDir())
	if err := store.SaveSession(context.Background(), "https://portal.example", "cert"); err == nil {
		t.Fatal("se guardó sin identificador de sesión")
	}
	if err := WriteSessionFile("", map[string]string{"a": "b"}); !errors.Is(err, ErrSessionIDNoDisponible) {
		t.Fatalf("WriteSessionFile sin ruta: %v", err)
	}
}
