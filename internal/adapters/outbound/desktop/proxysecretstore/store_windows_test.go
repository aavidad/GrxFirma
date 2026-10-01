// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package proxysecretstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/ports"
)

// fakeDPAPI sustituye a CryptProtectData/CryptUnprotectData en los tests.
type fakeDPAPI struct {
	output []byte
	err    error
	inputs [][]byte
}

func (f *fakeDPAPI) protect(plaintext []byte) ([]byte, error) {
	f.inputs = append(f.inputs, append([]byte(nil), plaintext...))
	return append([]byte(nil), f.output...), f.err
}

func (f *fakeDPAPI) unprotect(ciphertext []byte) ([]byte, error) {
	f.inputs = append(f.inputs, append([]byte(nil), ciphertext...))
	return append([]byte(nil), f.output...), f.err
}

func newWindowsTestStore(codec *fakeDPAPI) *Store {
	return &Store{
		dpapi: codec,
		idgen: func() (string, error) {
			return "proxy-secret-fixed-id", nil
		},
		configDir: func() (string, error) {
			return `C:\Users\usuario-prueba\AppData\Roaming`, nil
		},
		readFile: func(string, int64) ([]byte, error) {
			return nil, os.ErrNotExist
		},
		writeFile:   func(string, []byte, os.FileMode) error { return nil },
		mkdirAll:    func(string, os.FileMode) error { return nil },
		protectDir:  func(string, os.FileMode) error { return nil },
		protectFile: func(string, os.FileMode) error { return nil },
		removeFile:  func(string) error { return nil },
	}
}

func TestStoreWindows_GuardaSecretoProtegido(t *testing.T) {
	t.Parallel()

	runner := &fakeDPAPI{output: []byte("ciphertext")}
	var wrotePath string
	var wroteBody []byte
	var protectedDir string
	var protectedFile string
	store := newWindowsTestStore(runner)
	store.writeFile = func(path string, body []byte, _ os.FileMode) error {
		wrotePath = path
		wroteBody = append([]byte(nil), body...)
		return nil
	}
	store.protectDir = func(path string, mode os.FileMode) error {
		protectedDir = path
		if mode != 0o700 {
			t.Fatalf("protectDir mode = %o, want 700", mode)
		}
		return nil
	}
	store.protectFile = func(path string, mode os.FileMode) error {
		protectedFile = path
		if mode != 0o600 {
			t.Fatalf("protectFile mode = %o, want 600", mode)
		}
		return nil
	}

	desc, err := store.Store(context.Background(), "corp-proxy", ports.ProxySecretMaterial{
		Username: "alberto",
		Password: []byte("secreto"),
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if desc.ID != "proxy-secret-fixed-id" || desc.Realm != "corp-proxy" || desc.Username != "alberto" {
		t.Fatalf("descriptor inesperado: %#v", desc)
	}
	if wrotePath != filepath.Join(`C:\Users\usuario-prueba\AppData\Roaming`, "GrxFirma", "proxy-secrets", "proxy-secret-fixed-id.dpapi") {
		t.Fatalf("write path = %q", wrotePath)
	}
	if string(wroteBody) != "ciphertext" {
		t.Fatalf("write body = %q, want ciphertext", string(wroteBody))
	}
	if protectedDir != filepath.Dir(wrotePath) {
		t.Fatalf("protected dir = %q, want %q", protectedDir, filepath.Dir(wrotePath))
	}
	if protectedFile != wrotePath {
		t.Fatalf("protected file = %q, want %q", protectedFile, wrotePath)
	}
	if len(runner.inputs) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.inputs))
	}
	var env secretEnvelope
	if err := json.Unmarshal(runner.inputs[0], &env); err != nil {
		t.Fatalf("entrada DPAPI json invalida: %v", err)
	}
	if env.Realm != "corp-proxy" || env.Username != "alberto" {
		t.Fatalf("envelope inesperado: %#v", env)
	}
}

func TestStoreWindows_LoadRecuperaYDesprotegeSecreto(t *testing.T) {
	t.Parallel()

	raw, _ := json.Marshal(secretEnvelope{
		Realm:       "corp-proxy",
		Username:    "alberto",
		PasswordB64: base64.StdEncoding.EncodeToString([]byte("secreto")),
	})
	runner := &fakeDPAPI{output: raw}
	store := newWindowsTestStore(runner)
	var readLimit int64
	store.readFile = func(_ string, maxBytes int64) ([]byte, error) {
		readLimit = maxBytes
		return []byte("ciphertext"), nil
	}

	got, err := store.Load(context.Background(), "proxy-secret-fixed-id")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Realm != "corp-proxy" || got.Username != "alberto" || string(got.Password) != "secreto" {
		t.Fatalf("material inesperado: %#v", got)
	}
	if readLimit != maxProxySecretBlobBytes {
		t.Fatalf("read limit = %d, want %d", readLimit, maxProxySecretBlobBytes)
	}
}

func TestStoreWindows_RechazaMaterialSobredimensionadoAntesDeDPAPI(t *testing.T) {
	t.Parallel()

	runner := &fakeDPAPI{}
	store := newWindowsTestStore(runner)
	_, err := store.Store(context.Background(), "corp-proxy", ports.ProxySecretMaterial{
		Username: "alberto",
		Password: make([]byte, maxProxySecretPasswordBytes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "password supera el limite") {
		t.Fatalf("Store() error = %v, want limite de password", err)
	}
	if len(runner.inputs) != 0 {
		t.Fatalf("DPAPI calls = %d, want 0", len(runner.inputs))
	}
}

func TestStoreWindows_LimpiaBlobSiFallaLaProteccionDelFichero(t *testing.T) {
	t.Parallel()

	runner := &fakeDPAPI{output: []byte("ciphertext")}
	store := newWindowsTestStore(runner)
	protectErr := errors.New("DACL rechazada")
	store.protectFile = func(string, os.FileMode) error {
		return protectErr
	}
	var removedPath string
	store.removeFile = func(path string) error {
		removedPath = path
		return nil
	}

	_, err := store.Store(context.Background(), "corp-proxy", ports.ProxySecretMaterial{
		Username: "alberto",
		Password: []byte("secreto"),
	})
	if !errors.Is(err, protectErr) {
		t.Fatalf("Store() error = %v, want %v", err, protectErr)
	}
	if removedPath == "" || !strings.HasSuffix(removedPath, "proxy-secret-fixed-id.dpapi") {
		t.Fatalf("cleanup path = %q", removedPath)
	}
}

func TestLoadWindows_RechazaBlobSobredimensionadoAntesDeDPAPI(t *testing.T) {
	t.Parallel()

	runner := &fakeDPAPI{}
	store := newWindowsTestStore(runner)
	store.readFile = func(string, int64) ([]byte, error) {
		return make([]byte, maxProxySecretBlobBytes+1), nil
	}

	_, err := store.Load(context.Background(), "proxy-secret-fixed-id")
	if err == nil || !strings.Contains(err.Error(), "blob DPAPI fuera del limite") {
		t.Fatalf("Load() error = %v, want limite de blob", err)
	}
	if len(runner.inputs) != 0 {
		t.Fatalf("DPAPI calls = %d, want 0", len(runner.inputs))
	}
}

func TestDeleteWindows_UsaBorradoSeguroEIdempotente(t *testing.T) {
	t.Parallel()

	store := newWindowsTestStore(&fakeDPAPI{})
	var removedPath string
	store.removeFile = func(path string) error {
		removedPath = path
		return os.ErrNotExist
	}
	if err := store.Delete(context.Background(), "proxy-secret-fixed-id"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !strings.HasSuffix(removedPath, "proxy-secret-fixed-id.dpapi") {
		t.Fatalf("remove path = %q", removedPath)
	}
}

func TestProtectWithDPAPI_RechazaSalidaSobredimensionada(t *testing.T) {
	t.Parallel()

	codec := &fakeDPAPI{output: make([]byte, maxProxySecretBlobBytes+1)}
	if _, err := protectWithDPAPI(codec, []byte("{}")); err == nil {
		t.Fatal("protectWithDPAPI accepted oversized DPAPI output")
	}
}

// Prueba real contra DPAPI de Windows (sin procesos externos).
func TestNativeDPAPI_IdaYVuelta(t *testing.T) {
	codec := nativeDPAPI{}
	protegido, err := codec.protect([]byte("secreto-proxy"))
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	plano, err := codec.unprotect(protegido)
	if err != nil {
		t.Fatalf("unprotect: %v", err)
	}
	if string(plano) != "secreto-proxy" {
		t.Fatalf("ida y vuelta = %q", plano)
	}
}

func TestStoreWindows_StatusDisponibleCuandoDPAPIFunciona(t *testing.T) {
	t.Parallel()

	runner := &fakeDPAPI{output: []byte("ciphertext")}
	store := newWindowsTestStore(runner)
	status, err := store.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Available || status.Platform != "windows" || status.Backend != windowsBackendName {
		t.Fatalf("status inesperado: %+v", status)
	}
}

func TestClassifyWindowsStatusReason(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		want string
	}{
		{err: errors.New("APPDATA no disponible"), want: "proxysecretstore: directorio de configuracion de usuario no disponible"},
	}
	for _, tc := range cases {
		if got := classifyWindowsStatusReason(tc.err); got != tc.want {
			t.Fatalf("classifyWindowsStatusReason(%q) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
