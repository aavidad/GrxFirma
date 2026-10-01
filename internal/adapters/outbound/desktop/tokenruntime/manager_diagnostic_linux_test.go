// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production

package tokenruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/ports"
)

func TestManagerDiagnosticsOnlyChecksFilesystemAndParentReservation(t *testing.T) {
	dir := privateDir(t)
	module, executable := filepath.Join(dir, "module.so"), filepath.Join(dir, "app")
	for _, path := range []string{module, executable, filepath.Join(dir, "grxfirma-pkcs11-worker")} {
		writeFixture(t, path, []byte("not an executable: never run this fixture"), 0700)
	}
	writeFixture(t, filepath.Join(dir, "tokens.json"), configBytes(t, true, module), 0600)
	m := NewManager(dir)
	m.ops.executable = func() (string, error) { return executable, nil }
	m.ops.checkPath = func(path string) (string, error) {
		if path == "/usr/bin/pinentry-qt" {
			return executable, nil
		}
		return pkcs11worker.ValidateModulePath(path)
	}
	var owners []*secmem.Blob
	var views [][]byte
	var sizes []int
	m.ops.allocate = func(size int) (*secmem.Blob, error) {
		for _, owner := range owners {
			if !owner.Locked() {
				t.Fatal("reservation probes did not coexist")
			}
		}
		owner, err := secmem.NewLockedSize(size)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(owner.Bytes(), make([]byte, size)) {
			t.Fatal("probe contains secret data")
		}
		sizes = append(sizes, size)
		owners, views = append(owners, owner), append(views, owner.Bytes())
		return owner, nil
	}
	s, err := m.Diagnose(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []ports.TokenSettingCheck{{ID: "config", Status: "valid"}, {ID: "modules", Status: "filesystem_checked"}, {ID: "helper", Status: "filesystem_checked"}, {ID: "pinentry", Status: "filesystem_checked"}, {ID: "pin_memory", Status: "reservation_checked"}, {ID: "hardware", Status: "not_checked"}}
	if !reflect.DeepEqual(s.Checks, want) || !reflect.DeepEqual(sizes, []int{4097, 20496}) {
		t.Fatalf("unexpected diagnostic: %+v sizes=%v", s.Checks, sizes)
	}
	for i, owner := range owners {
		if owner.Locked() || owner.Len() != 0 || !bytes.Equal(views[i], make([]byte, len(views[i]))) {
			t.Fatal("diagnostic owner retained")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 4 {
		t.Fatal("diagnostic changed filesystem")
	}
}

func TestManagerDiagnosticFailureAndDisabledModules(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		dir := privateDir(t)
		writeFixture(t, filepath.Join(dir, "tokens.json"), configBytes(t, enabled, "/missing/local/module.so"), 0600)
		m := NewManager(dir)
		m.ops.executable = func() (string, error) { return "", errors.New("private path detail") }
		m.ops.checkPath = func(path string) (string, error) {
			if !enabled && path == "/missing/local/module.so" {
				t.Fatal("disabled module inspected")
			}
			return "", errors.New("private driver detail")
		}
		m.ops.allocate = func(int) (*secmem.Blob, error) { return nil, secmem.ErrLockUnavailable }
		s, err := m.Diagnose(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range s.Checks {
			switch check.ID {
			case "modules":
				want := "not_checked"
				if enabled {
					want = "invalid"
				}
				if check.Status != want {
					t.Fatal(check)
				}
			case "helper", "pinentry", "pin_memory":
				if check.Status != "unavailable" {
					t.Fatal(check)
				}
			case "hardware":
				if check.Status != "not_checked" {
					t.Fatal(check)
				}
			}
		}
	}
}

func TestManagerMemoryProbeReleasesEarlierOwnerOnSecondFailure(t *testing.T) {
	m := NewManager("/not-used")
	var first *secmem.Blob
	calls := 0
	m.ops.allocate = func(size int) (*secmem.Blob, error) {
		calls++
		if calls == 2 {
			return nil, secmem.ErrLockUnavailable
		}
		var err error
		first, err = secmem.NewLockedSize(size)
		return first, err
	}
	if m.checkPINMemory() || calls != 2 || first == nil || first.Locked() || first.Len() != 0 {
		t.Fatal("failed probe retained first owner")
	}
}
