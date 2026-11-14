// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package localtlstrust

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type almacenSecretosPrueba struct {
	mu    sync.Mutex
	datos map[string][]byte
	falla bool
}

func (a *almacenSecretosPrueba) Store(_ context.Context, key string, value []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.falla {
		return errors.New("sin Secret Service")
	}
	a.datos[key] = append([]byte(nil), value...)
	return nil
}

func (a *almacenSecretosPrueba) Load(_ context.Context, key string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	value, ok := a.datos[key]
	if !ok || a.falla {
		return nil, errors.New("no encontrado")
	}
	return append([]byte(nil), value...), nil
}

func (a *almacenSecretosPrueba) Delete(_ context.Context, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.datos, key)
	return nil
}

// No usa t.Parallel: cambia el almacén del paquete.
func TestManagedCAKey_CifradaConElAlmacenDelSistema(t *testing.T) {
	almacen := &almacenSecretosPrueba{datos: map[string][]byte{}}
	UseManagedCAKeyStorage(almacen)
	t.Cleanup(func() { UseManagedCAKeyStorage(nil) })

	dir := filepath.Join(t.TempDir(), "tls")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "websocket-localhost-root.key.pem")
	// Clave anterior en claro, como la dejaban las versiones previas.
	anterior := []byte("clave CA anterior en claro")
	if err := os.WriteFile(path, anterior, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadManagedCAKey(path); err != nil || !bytes.Equal(got, anterior) {
		t.Fatalf("la clave anterior debe poder leerse para rotarla: %q, %v", got, err)
	}

	clave := []byte("clave CA nueva de prueba")
	if err := SaveManagedCAKey(path, clave); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, clave) || bytes.Contains(stored, anterior) {
		t.Fatal("la clave de la CA quedó en claro en disco")
	}
	if got, err := LoadManagedCAKey(path); err != nil || !bytes.Equal(got, clave) {
		t.Fatalf("LoadManagedCAKey() = %q, %v", got, err)
	}
	if len(almacen.datos) != 1 {
		t.Fatalf("claves de cifrado en el almacén = %d; want 1", len(almacen.datos))
	}

	// Al sustituirla se olvida la clave de cifrado anterior.
	if err := SaveManagedCAKey(path, []byte("otra clave CA")); err != nil {
		t.Fatal(err)
	}
	if len(almacen.datos) != 1 {
		t.Fatalf("claves de cifrado tras rotar = %d; want 1", len(almacen.datos))
	}

	// Sin almacén seguro: en macOS no se guarda la clave; en Linux va a un
	// fichero privado (managed_ca_key_fallback_test.go).
	if runtime.GOOS != "linux" {
		almacen.falla = true
		if err := SaveManagedCAKey(path, clave); err == nil {
			t.Fatal("SaveManagedCAKey() guardó la clave sin almacén seguro")
		}
	}
}
