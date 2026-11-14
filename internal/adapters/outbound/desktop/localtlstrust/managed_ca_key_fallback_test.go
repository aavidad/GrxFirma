// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package localtlstrust

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// registroPrueba captura el registro por defecto mientras dura la prueba.
type registroPrueba struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *registroPrueba) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *registroPrueba) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func capturarRegistro(t *testing.T) *registroPrueba {
	t.Helper()
	registro := &registroPrueba{}
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(registro, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })
	return registro
}

func dirClaveCA(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tls")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// No usa t.Parallel: cambia el almacén y el registro del paquete.
func TestManagedCAKey_LinuxSinAlmacenGuardaEnFicheroPrivadoYAvisaUnaVez(t *testing.T) {
	almacen := &almacenSecretosPrueba{datos: map[string][]byte{}, falla: true}
	UseManagedCAKeyStorage(almacen)
	t.Cleanup(func() { UseManagedCAKeyStorage(nil) })
	reiniciarAvisoClaveCAEnFichero(t)
	registro := capturarRegistro(t)

	dir := dirClaveCA(t)
	path := filepath.Join(dir, "websocket-localhost-root.key.pem")
	clave := []byte("clave CA de prueba sin almacén")
	if err := SaveManagedCAKey(path, clave); err != nil {
		t.Fatalf("sin almacén de secretos la CA local debe poder crearse en Linux: %v", err)
	}
	if got, err := LoadManagedCAKey(path); err != nil || !bytes.Equal(got, clave) {
		t.Fatalf("LoadManagedCAKey() = %q, %v", got, err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directorio de la clave sin 0700: %v, %v", dirInfo, err)
	}
	fileInfo, err := os.Lstat(path)
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("fichero de la clave sin 0600: %v, %v", fileInfo, err)
	}
	if err := SaveManagedCAKey(path, []byte("otra clave CA sin almacén")); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(registro.String(), eventoClaveCAEnFichero); n != 1 {
		t.Fatalf("avisos de clave sin almacén = %d; want 1\n%s", n, registro.String())
	}
}

func TestManagedCAKey_LinuxSinAlmacenNoSigueEnlaces(t *testing.T) {
	almacen := &almacenSecretosPrueba{datos: map[string][]byte{}, falla: true}
	UseManagedCAKeyStorage(almacen)
	t.Cleanup(func() { UseManagedCAKeyStorage(nil) })
	capturarRegistro(t)

	dir := dirClaveCA(t)
	victima := filepath.Join(t.TempDir(), "victima")
	if err := os.WriteFile(victima, []byte("intacta"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "websocket-localhost-root.key.pem")
	if err := os.Symlink(victima, path); err != nil {
		t.Skipf("sin enlaces simbólicos: %v", err)
	}
	if err := SaveManagedCAKey(path, []byte("clave CA")); err == nil {
		t.Fatal("SaveManagedCAKey() escribió a través de un enlace simbólico")
	}
	if data, err := os.ReadFile(victima); err != nil || string(data) != "intacta" {
		t.Fatalf("el destino del enlace cambió: %q, %v", data, err)
	}
}

func TestManagedCAKey_LinuxMigraAlAlmacenCuandoAparece(t *testing.T) {
	almacen := &almacenSecretosPrueba{datos: map[string][]byte{}, falla: true}
	UseManagedCAKeyStorage(almacen)
	t.Cleanup(func() { UseManagedCAKeyStorage(nil) })
	capturarRegistro(t)

	dir := dirClaveCA(t)
	path := filepath.Join(dir, "websocket-localhost-root.key.pem")
	clave := []byte("clave CA guardada sin almacén")
	if err := SaveManagedCAKey(path, clave); err != nil {
		t.Fatal(err)
	}

	almacen.mu.Lock()
	almacen.falla = false
	almacen.mu.Unlock()
	if got, err := LoadManagedCAKey(path); err != nil || !bytes.Equal(got, clave) {
		t.Fatalf("LoadManagedCAKey() al migrar = %q, %v", got, err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, clave) || !bytes.HasPrefix(stored, managedCAKeyringHeader) {
		t.Fatal("la clave de la CA sigue en claro tras aparecer el almacén")
	}
	if len(almacen.datos) != 1 {
		t.Fatalf("claves de cifrado en el almacén = %d; want 1", len(almacen.datos))
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("restos en el directorio tras migrar: %v, %v", entries, err)
	}
	if got, err := LoadManagedCAKey(path); err != nil || !bytes.Equal(got, clave) {
		t.Fatalf("LoadManagedCAKey() tras migrar = %q, %v", got, err)
	}
}

func TestManagedCAKey_FueraDeLinuxSinAlmacenFallaCerrado(t *testing.T) {
	almacen := &almacenSecretosPrueba{datos: map[string][]byte{}, falla: true}
	UseManagedCAKeyStorage(almacen)
	anterior := managedCAKeyFileFallback
	managedCAKeyFileFallback = false
	t.Cleanup(func() {
		UseManagedCAKeyStorage(nil)
		managedCAKeyFileFallback = anterior
	})

	path := filepath.Join(dirClaveCA(t), "websocket-localhost-root.key.pem")
	if err := SaveManagedCAKey(path, []byte("clave CA")); err == nil {
		t.Fatal("en macOS la clave de la CA no debe guardarse sin almacén de secretos")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("quedó un fichero de clave: %v", err)
	}
}

func reiniciarAvisoClaveCAEnFichero(t *testing.T) {
	t.Helper()
	managedCAKeyFileWarning = sync.Once{}
	t.Cleanup(func() { managedCAKeyFileWarning = sync.Once{} })
}
