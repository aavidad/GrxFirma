//go:build linux && pkcs11_preview && !production

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
func writeFixture(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
func configBytes(t *testing.T, enabled bool, paths ...string) []byte {
	t.Helper()
	modules := make([]map[string]string, 0, len(paths))
	for _, path := range paths {
		modules = append(modules, map[string]string{"path": path})
	}
	data, err := json.Marshal(map[string]any{"version": 1, "enabled": enabled, "modules": modules})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestStrictVersionedConfigRejectsHostileJSON(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `{}`, `{"version":1,"enabled":true,"modules":[]}`,
		`{"version":2,"enabled":false}`, `{"version":1}`, `{"version":1,"enabled":null}`,
		`{"version":1,"enabled":false,"pin":"do-not-accept"}`, `{"Version":1,"enabled":false}`,
		`{"version":1,"version":1,"enabled":false}`, `{"version":1,"enabled":false,"modules":[{"path":"/one","path":"/two"}]}`,
		`{"version":1,"enabled":false,"modules":[{"Path":"/one"}]}`, `{"version":1,"enabled":false,"modules":[{"path":"relative"}]}`,
		`{"version":1,"enabled":false,"modules":[{"path":"/one","pin":"bad"}]}`, `{"version":1,"enabled":false} {}`,
		`{"version":1,"enabled":false,"modules":[{"path":"/one"},{"path":"/one"}]}`, `{"version":1,"enabled":false,"modules":[null]}`,
		`{"version":1,"enabled":false,"modules":[{"path":"/bad\u0000path"}]}`,
	} {
		if _, err := decodeConfig([]byte(raw)); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("accepted hostile config %q: %v", raw, err)
		}
	}
	paths := make([]string, 9)
	for i := range paths {
		paths[i] = "/module/" + strings.Repeat("a", i+1)
	}
	for _, raw := range [][]byte{configBytes(t, true, paths...), []byte(strings.Repeat(" ", maxConfigBytes+1)), configBytes(t, false, "/"+strings.Repeat("a", 4096))} {
		if _, err := decodeConfig(raw); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal("oversize config accepted")
		}
	}
	if _, err := decodeConfig(configBytes(t, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeConfig(configBytes(t, true, paths[:8]...)); err != nil {
		t.Fatal(err)
	}
}

func TestConfigPermissionsBoundsAndCanonicalModules(t *testing.T) {
	dir := privateDir(t)
	module := filepath.Join(dir, "module.so")
	configPath := filepath.Join(dir, "tokens.json")
	writeFixture(t, module, []byte("synthetic module; not loaded"), 0600)
	alias := filepath.Join(dir, "module-link.so")
	if err := os.Symlink(module, alias); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, configPath, configBytes(t, true, alias), 0600)
	modules, err := readConfig(dir)
	if err != nil || !reflect.DeepEqual(modules, []string{module}) {
		t.Fatalf("modules=%v err=%v", modules, err)
	}
	writeFixture(t, configPath, configBytes(t, true, module, alias), 0600)
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("duplicate canonical module accepted")
	}
	writeFixture(t, configPath, configBytes(t, true, module), 0620)
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("writable config accepted")
	}
	writeFixture(t, configPath, configBytes(t, true, module), 0600)
	if err := os.Chmod(module, 0620); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("writable module accepted")
	}
	if err := os.Chmod(module, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0770); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("writable ancestor accepted")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, configPath, []byte(strings.Repeat("x", maxConfigBytes+1)), 0600)
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("oversize file accepted")
	}
	writeFixture(t, configPath, nil, 0600)
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("empty file accepted")
	}
	for _, path := range []string{"relative", "\x00", strings.Repeat("a", 4097)} {
		if _, err := readConfig(path); !errors.Is(err, ErrInvalidConfig) {
			t.Fatal("invalid configuration directory accepted")
		}
	}
}

func TestConfigNonregularAndSymlinkFiles(t *testing.T) {
	dir := privateDir(t)
	if err := unix.Mkfifo(filepath.Join(dir, "tokens.json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(dir); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("FIFO config accepted")
	}
	other := privateDir(t)
	target := filepath.Join(other, "managed.json")
	writeFixture(t, target, configBytes(t, false), 0600)
	linked := privateDir(t)
	if err := os.Symlink(target, filepath.Join(linked, "tokens.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(linked); err != nil {
		t.Fatal("controlled symlink rejected", err)
	}
	if err := os.Chmod(target, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(linked); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("symlink to untrusted file accepted")
	}
}

func TestPreviewOptInAndDisabledConfigNeverResolveModules(t *testing.T) {
	if !EnabledInBuild() {
		t.Fatal("preview build gate missing")
	}
	dir := privateDir(t)
	r := New(dir, nil)
	refs, err := r.List(context.Background())
	if err != nil || len(refs) != 0 || r.Diagnostico() != nil {
		t.Fatal("missing config should be inactive")
	}
	writeFixture(t, filepath.Join(dir, "tokens.json"), configBytes(t, false, "/definitely-not-present/module.so"), 0600)
	r = New(dir, nil)
	if _, err := r.List(context.Background()); err != nil || r.Diagnostico() != nil {
		t.Fatal("disabled config resolved absent module")
	}
	writeFixture(t, filepath.Join(dir, "tokens.json"), []byte(`{"version":1,"enabled":true,"module":"web-selected"}`), 0600)
	r = New(dir, nil)
	if _, err := r.List(context.Background()); !errors.Is(err, ErrInvalidConfig) || !errors.Is(r.Diagnostico(), ErrInvalidConfig) {
		t.Fatal("invalid configuration hidden")
	}
	module := filepath.Join(dir, "fake.so")
	writeFixture(t, module, []byte("not loaded"), 0600)
	writeFixture(t, filepath.Join(dir, "tokens.json"), configBytes(t, true, module), 0600)
	r = New(dir, nil)
	if !errors.Is(r.Diagnostico(), ErrHelperUnavailable) {
		t.Fatal("enabled preview did not require installation-relative helper")
	}
}

func TestInstallationRelativeHelperResolution(t *testing.T) {
	for executable, want := range map[string][]string{
		"/usr/bin/grxfirma":                 {"/usr/bin/grxfirma-pkcs11-worker", "/usr/lib/grxfirma/bin/grxfirma-pkcs11-worker"},
		"/user/.local/bin/grxfirma":         {"/user/.local/bin/grxfirma-pkcs11-worker", "/user/.local/lib/grxfirma/bin/grxfirma-pkcs11-worker"},
		"/usr/lib/grxfirma/bin/native-host": {"/usr/lib/grxfirma/bin/grxfirma-pkcs11-worker"},
		"/stage/bin/grxfirma":               {"/stage/bin/grxfirma-pkcs11-worker"},
	} {
		if got := helperCandidates(executable); !reflect.DeepEqual(got, want) {
			t.Fatalf("candidates=%v want=%v", got, want)
		}
	}
	dir := privateDir(t)
	bin := filepath.Join(dir, ".local", "bin")
	lib := filepath.Join(dir, ".local", "lib", "grxfirma", "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lib, 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bin, "grxfirma")
	writeFixture(t, executable, []byte("synthetic executable"), 0700)
	helper := filepath.Join(lib, "grxfirma-pkcs11-worker")
	writeFixture(t, helper, []byte("synthetic helper; never executed"), 0700)
	if got, err := resolveHelper(executable); err != nil || got != helper {
		t.Fatalf("helper=%v err=%v", got, err)
	}
	sibling := filepath.Join(bin, "grxfirma-pkcs11-worker")
	writeFixture(t, sibling, []byte("staged helper"), 0700)
	if got, err := resolveHelper(executable); err != nil || got != sibling {
		t.Fatalf("sibling=%v err=%v", got, err)
	}
	for _, mode := range []os.FileMode{0600, 0720} {
		if err := os.Chmod(sibling, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveHelper(executable); !errors.Is(err, ErrHelperUnavailable) {
			t.Fatal("untrusted sibling fell through to another helper")
		}
	}
	if _, err := resolveHelper("/missing/executable"); !errors.Is(err, ErrHelperUnavailable) {
		t.Fatal(err)
	}
}
