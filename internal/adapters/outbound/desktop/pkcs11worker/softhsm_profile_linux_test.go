// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSoftHSMFileProfileParsing(t *testing.T) {
	t.Parallel()
	valid := "# Synthetic configuration only\ndirectories.tokendir = /qa/tokens\nobjectstore.backend = file\nlog.level = ERROR\nslots.removable = false\n"
	if got, err := parseSoftHSMStoreDirectory([]byte(valid)); err != nil || got != "/qa/tokens" {
		t.Fatalf("valid: %q %v", got, err)
	}
	for name, raw := range map[string]string{
		"empty": "", "no_store": "log.level=ERROR", "relative": "directories.tokendir=relative", "dirty": "directories.tokendir=/qa/../tokens",
		"environment": "directories.tokendir=$HOME/tokens", "quotes": "directories.tokendir=\"/qa/tokens\"", "nul": "directories.tokendir=/qa/\x00tokens",
		"duplicate": valid + "directories.tokendir=/other\n", "duplicate_backend": valid + "objectstore.backend=file\n", "database": strings.Replace(valid, "backend = file", "backend = db", 1),
		"invalid_line": valid + "malformed", "large": strings.Repeat("#", 64*1024+1),
		"chunked_line": "directories.tokendir=/qa/" + strings.Repeat("a", 1024),
		"extra_equals": "directories.tokendir=/qa/tokens=/qa/other", "invalid_utf8": "directories.tokendir=/qa/\xff",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSoftHSMStoreDirectory([]byte(raw)); !errors.Is(err, ErrSandboxPolicy) {
				t.Fatalf("accepted invalid profile: %v", err)
			}
		})
	}
}

func TestSoftHSMFileProfileUsesOnlyExplicitSyntheticLocations(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "softhsm.conf")
	data := []byte("directories.tokendir=" + filepath.Join(dir, "tokens") + "\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	before := SandboxResources{ReadOnlyFiles: []string{"/qa/explicit"}}
	got, err := softHSMFileResources([]string{filepath.Join(dir, "missing"), path}, before)
	if err != nil || len(got.ReadOnlyFiles) != 2 || len(got.ReadWriteDirs) != 1 || len(before.ReadOnlyFiles) != 1 {
		t.Fatalf("profile: %+v %v", got, err)
	}
	other := filepath.Join(dir, "different.conf")
	if err := os.WriteFile(other, []byte("directories.tokendir=/qa/different\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := softHSMFileResources([]string{path, other}, SandboxResources{}); !errors.Is(err, ErrSandboxPolicy) {
		t.Fatal("ambiguous configuration accepted")
	}
	if _, err := softHSMFileResources([]string{filepath.Join(dir, "missing")}, SandboxResources{}); !errors.Is(err, ErrSandboxPolicy) {
		t.Fatal("missing profile accepted")
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := softHSMFileResources([]string{path}, SandboxResources{}); !errors.Is(err, ErrSandboxPolicy) {
		t.Fatal("unsafe profile accepted")
	}
	// A different module must not inspect SoftHSM configuration or environment.
	t.Setenv("SOFTHSM2_CONF", other)
	if got, err := moduleSandboxResources("/qa/another-driver.so", before); err != nil || len(got.ReadOnlyFiles) != 1 || len(got.ReadWriteDirs) != 0 {
		t.Fatalf("unexpected automatic grants: %+v %v", got, err)
	}
}
