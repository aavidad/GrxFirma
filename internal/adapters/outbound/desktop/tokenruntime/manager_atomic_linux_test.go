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
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"grxfirma/internal/ports"
)

func TestManagerAtomicFailureCleanupAndDurability(t *testing.T) {
	for _, scenario := range []string{"sync_file", "rename", "sync_directory", "cancel", "concurrent_edit", "symlink_swap", "directory_move"} {
		t.Run(scenario, func(t *testing.T) {
			parent := privateDir(t)
			dir := filepath.Join(parent, "config")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "tokens.json")
			original := configBytes(t, false)
			writeFixture(t, path, original, 0600)
			m := NewManager(dir)
			update := confirmedUpdate(loadManager(t, m))
			update.Modules = []ports.TokenModuleSetting{{Path: "/absent/module.so"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := ports.ErrTokenSettingsWrite
			resultDir := dir
			switch scenario {
			case "sync_file":
				m.ops.syncFile = func(*os.File) error { return unix.EIO }
			case "rename":
				m.ops.rename = func(int, string, string) error { return unix.EIO }
			case "sync_directory":
				m.ops.syncDir = func(int) error { return unix.EIO }
				want = nil
			case "cancel":
				m.ops.beforeCommit = cancel
				want = context.Canceled
			case "concurrent_edit":
				m.ops.beforeCommit = func() { writeFixture(t, path, configBytes(t, false, "/concurrent.so"), 0600) }
				want = ports.ErrTokenSettingsConflict
			case "symlink_swap":
				m.ops.beforeCommit = func() {
					if err := os.Rename(path, filepath.Join(dir, "old.json")); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink("old.json", path); err != nil {
						t.Fatal(err)
					}
				}
				want = ports.ErrTokenSettingsUnsafe
			case "directory_move":
				resultDir = filepath.Join(parent, "moved")
				m.ops.beforeCommit = func() {
					if err := os.Rename(dir, resultDir); err != nil {
						t.Fatal(err)
					}
				}
				want = ports.ErrTokenSettingsConflict
			}
			s, err := m.Save(ctx, update)
			if !errors.Is(err, want) {
				t.Fatalf("save error=%v wanted=%v", err, want)
			}
			if scenario == "sync_directory" {
				if s.Warning != "durability_unconfirmed" || !s.RestartRequired || !s.Exists || len(s.Modules) != 1 {
					t.Fatal("completed rename misreported as absent write")
				}
			} else if m.restartRequired {
				t.Fatal("failed save changed restart state")
			}
			entries, err := os.ReadDir(resultDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".tokens.json.tmp-") {
					t.Fatal("temporary file retained")
				}
			}
			if scenario == "sync_file" || scenario == "rename" || scenario == "cancel" || scenario == "directory_move" {
				got, _ := os.ReadFile(filepath.Join(resultDir, "tokens.json"))
				if !bytes.Equal(got, original) {
					t.Fatal("failed write changed original")
				}
			}
		})
	}
}

func TestManagerDirectoryLockIsNonblockingAndContextAware(t *testing.T) {
	dir := privateDir(t)
	m := NewManager(dir)
	update := confirmedUpdate(loadManager(t, m))
	held, err := m.openDirectory(false)
	if err != nil {
		t.Fatal(err)
	}
	defer held.close()
	if err := lockSettingsDirectory(context.Background(), held.fd); err != nil {
		t.Fatal(err)
	}
	defer unlockSettingsDirectory(held.fd)
	if _, err := m.Save(context.Background(), update); !errors.Is(err, ports.ErrTokenSettingsConflict) {
		t.Fatal("busy directory did not return bounded conflict", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lockSettingsDirectory(ctx, held.fd); !errors.Is(err, context.Canceled) {
		t.Fatal("lock cancellation ignored")
	}
	if err := lockSettingsDirectory(context.Background(), -1); !errors.Is(err, ports.ErrTokenSettingsUnsafe) {
		t.Fatal("lock IO error not sanitized")
	}
	if _, err := os.Stat(filepath.Join(dir, "tokens.json")); !os.IsNotExist(err) {
		t.Fatal("busy save created file")
	}
}

func TestManagerNewDirectorySyncsParentBeforeWritingSettings(t *testing.T) {
	for _, fail := range []bool{false, true} {
		parent := privateDir(t)
		dir := filepath.Join(parent, "new-config")
		m := NewManager(dir)
		var expected unix.Stat_t
		if err := unix.Stat(parent, &expected); err != nil {
			t.Fatal(err)
		}
		calls := 0
		m.ops.syncParent = func(fd int) error {
			calls++
			var got unix.Stat_t
			if err := unix.Fstat(fd, &got); err != nil || got.Dev != expected.Dev || got.Ino != expected.Ino {
				t.Fatal("wrong parent descriptor synchronized")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatal("settings written before parent directory synchronization")
			}
			if fail {
				return unix.EIO
			}
			return unix.Fsync(fd)
		}
		s, err := m.Save(context.Background(), confirmedUpdate(loadManager(t, m)))
		if calls != 1 {
			t.Fatal("new directory parent not synchronized exactly once")
		}
		if fail {
			if !errors.Is(err, ports.ErrTokenSettingsWrite) || s.RestartRequired || s.Warning != "" {
				t.Fatal("pre-write failure claimed a saved configuration", err)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 0 {
				t.Fatal("failed parent sync left configuration or temporary file")
			}
		} else if err != nil || !s.Exists || !s.RestartRequired {
			t.Fatal("synchronized new directory was not saved", err)
		}
	}
}
