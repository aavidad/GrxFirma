// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production

package tokenruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	"grxfirma/internal/ports"
)

func loadManager(t *testing.T, m *Manager) ports.TokenSettingsSnapshot {
	t.Helper()
	s, err := m.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func confirmedUpdate(s ports.TokenSettingsSnapshot) ports.TokenSettingsUpdate {
	return ports.TokenSettingsUpdate{Enabled: s.Enabled, Modules: s.Modules, Revision: s.Revision, Confirmed: true}
}

func TestManagerMissingSaveAndRestart(t *testing.T) {
	parent := privateDir(t)
	dir := filepath.Join(parent, "local-config")
	m := NewManager(dir)
	s := loadManager(t, m)
	if s.State != "missing" || s.Exists || !s.Available || !s.Editable || len(s.Revision) != 64 || s.RestartRequired {
		t.Fatalf("missing snapshot: %+v", s)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("constructor or Load created directory")
	}
	update := confirmedUpdate(s)
	s, err := m.Save(context.Background(), update)
	if err != nil || s.State != "disabled" || !s.Exists || !s.RestartRequired || s.Revision == update.Revision {
		t.Fatalf("save: %+v %v", s, err)
	}
	file, err := os.Stat(filepath.Join(dir, "tokens.json"))
	if err != nil || file.Mode().Perm() != 0600 {
		t.Fatal("saved settings not private")
	}
	directory, err := os.Stat(dir)
	if err != nil || directory.Mode().Perm() != 0700 {
		t.Fatal("created directory not private")
	}
	if !loadManager(t, m).RestartRequired || loadManager(t, NewManager(dir)).RestartRequired {
		t.Fatal("restart flag is not local sticky state")
	}
	if _, err := m.Save(context.Background(), update); !errors.Is(err, ports.ErrTokenSettingsConflict) {
		t.Fatal("stale missing revision accepted")
	}
}

func TestManagerEnablingDisablingAndLimits(t *testing.T) {
	dir := privateDir(t)
	module := filepath.Join(dir, "module.so")
	writeFixture(t, module, []byte("synthetic module; never loaded"), 0600)
	m := NewManager(dir)
	s := loadManager(t, m)
	update := confirmedUpdate(s)
	update.Enabled, update.Modules = true, []ports.TokenModuleSetting{{Path: module}}
	s, err := m.Save(context.Background(), update)
	if err != nil || s.State != "configured" || !s.Enabled || len(s.Modules) != 1 {
		t.Fatal("could not save controlled synthetic module", err)
	}
	if err := os.Remove(module); err != nil {
		t.Fatal(err)
	}
	update = confirmedUpdate(s)
	if _, err := m.Save(context.Background(), update); !errors.Is(err, ports.ErrTokenSettingsInvalid) {
		t.Fatal("enabled absent module accepted")
	}
	update.Enabled = false
	s, err = m.Save(context.Background(), update)
	if err != nil || s.Enabled || len(s.Modules) != 1 {
		t.Fatal("disable required removed driver", err)
	}
	update = confirmedUpdate(s)
	update.Modules = nil
	if _, err := m.Save(context.Background(), update); err != nil {
		t.Fatal("cannot remove missing module", err)
	}
	for _, modules := range [][]ports.TokenModuleSetting{
		{{Path: "relative"}}, {{Path: "/a\x00b"}}, {{Path: "/" + strings.Repeat("x", 4096)}}, {{Path: "/a"}, {Path: "/a"}}, make([]ports.TokenModuleSetting, 9),
	} {
		update = confirmedUpdate(loadManager(t, m))
		update.Modules = modules
		if _, err := m.Save(context.Background(), update); !errors.Is(err, ports.ErrTokenSettingsInvalid) {
			t.Fatal("invalid module list accepted", err)
		}
	}
	update = confirmedUpdate(loadManager(t, m))
	update.Enabled = true
	if _, err := m.Save(context.Background(), update); !errors.Is(err, ports.ErrTokenSettingsInvalid) {
		t.Fatal("enabled empty configuration accepted")
	}
}

func TestManagerInvalidRepairRequiresExplicitConfirmationAndRevision(t *testing.T) {
	dir := privateDir(t)
	path := filepath.Join(dir, "tokens.json")
	raw := []byte(`{"version":1,"pin":"must never be exposed"}`)
	writeFixture(t, path, raw, 0600)
	m := NewManager(dir)
	s := loadManager(t, m)
	if s.State != "invalid" || !s.Editable || len(s.Revision) != 64 || len(s.Modules) != 0 {
		t.Fatal("invalid safe file cannot be explicitly repaired")
	}
	for _, tc := range []struct {
		confirmed, repair bool
		revision          string
		want              error
	}{
		{false, true, s.Revision, ports.ErrTokenSettingsConfirmation},
		{true, false, s.Revision, ports.ErrTokenSettingsInvalid},
		{true, true, "", ports.ErrTokenSettingsConflict},
		{true, true, strings.Repeat("0", 64), ports.ErrTokenSettingsConflict},
		{true, true, strings.Repeat("z", 64), ports.ErrTokenSettingsConflict},
	} {
		_, err := m.Save(context.Background(), ports.TokenSettingsUpdate{Confirmed: tc.confirmed, ReplaceInvalid: tc.repair, Revision: tc.revision})
		if !errors.Is(err, tc.want) {
			t.Fatalf("repair guard: %v", err)
		}
		got, _ := os.ReadFile(path)
		if !bytes.Equal(got, raw) {
			t.Fatal("guard changed existing invalid file")
		}
	}
	if s, err := m.Save(context.Background(), ports.TokenSettingsUpdate{Confirmed: true, ReplaceInvalid: true, Revision: s.Revision}); err != nil || s.State != "disabled" {
		t.Fatal("explicit repair failed", err)
	}
}

func TestManagerRejectsUnsafeFilesystem(t *testing.T) {
	for _, scenario := range []string{"symlink", "ancestor_symlink", "fifo", "directory", "hardlink", "oversize", "writable_file", "writable_ancestor", "nonprivate_directory"} {
		t.Run(scenario, func(t *testing.T) {
			dir := privateDir(t)
			path := filepath.Join(dir, "tokens.json")
			targetDir := privateDir(t)
			target := filepath.Join(targetDir, "target.json")
			original := configBytes(t, false)
			writeFixture(t, target, original, 0600)
			switch scenario {
			case "symlink":
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "ancestor_symlink":
				alias := filepath.Join(dir, "alias")
				if err := os.Symlink(targetDir, alias); err != nil {
					t.Fatal(err)
				}
				dir = alias
			case "fifo":
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(target, path); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				writeFixture(t, path, bytes.Repeat([]byte{'x'}, maxConfigBytes+1), 0600)
			case "writable_file":
				writeFixture(t, path, original, 0660)
			case "writable_ancestor":
				if err := os.Chmod(dir, 0770); err != nil {
					t.Fatal(err)
				}
			case "nonprivate_directory":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			m := NewManager(dir)
			s := loadManager(t, m)
			if s.Editable {
				t.Fatal("unsafe configuration reported editable")
			}
			_, err := m.Save(context.Background(), ports.TokenSettingsUpdate{Confirmed: true, ReplaceInvalid: true, Revision: settingsRevision(nil, false)})
			if !errors.Is(err, ports.ErrTokenSettingsUnsafe) {
				t.Fatal("unsafe configuration save did not fail closed", err)
			}
			got, _ := os.ReadFile(target)
			if !bytes.Equal(got, original) {
				t.Fatal("unrelated target overwritten")
			}
		})
	}
	for _, dir := range []string{"relative", "/", "/a\x00b", "/" + strings.Repeat("x", 4096)} {
		if loadManager(t, NewManager(dir)).Editable {
			t.Fatal("invalid directory accepted")
		}
	}
}

func TestManagerRootOwnedConfigurationReadOnly(t *testing.T) {
	dir := privateDir(t)
	writeFixture(t, filepath.Join(dir, "tokens.json"), configBytes(t, false), 0644)
	m := NewManager(dir)
	// No chown or privileged host mutation. Override only the opened regular
	// file's owner evidence; all path operations use real temporary files.
	m.ops.fstat = func(fd int, stat *unix.Stat_t) error {
		err := unix.Fstat(fd, stat)
		if stat.Mode&unix.S_IFMT == unix.S_IFREG {
			stat.Uid = 0
		}
		return err
	}
	s := loadManager(t, m)
	if s.State != "disabled" || s.Editable || !s.Exists || s.Checks[0].Status != "read_only" {
		t.Fatal("root-owned settings not exposed read-only")
	}
	if _, err := m.Save(context.Background(), confirmedUpdate(s)); !errors.Is(err, ports.ErrTokenSettingsUnsafe) {
		t.Fatal("root-owned configuration replaced")
	}
}

func TestManagerConcurrentRevisionAndCancellation(t *testing.T) {
	dir := privateDir(t)
	a, b := NewManager(dir), NewManager(dir)
	update := confirmedUpdate(loadManager(t, a))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, manager := range []*Manager{a, b} {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := manager.Save(context.Background(), update); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, ports.ErrTokenSettingsConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent stale updates were not serialized")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, call := range []func(context.Context) (ports.TokenSettingsSnapshot, error){a.Load, a.Diagnose} {
		if _, err := call(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal("cancel ignored")
		}
	}
	if _, err := a.Save(ctx, confirmedUpdate(loadManager(t, a))); !errors.Is(err, context.Canceled) {
		t.Fatal("save cancel ignored")
	}
}

func TestManagerConfigByteBoundaryAndCanonicalModuleDuplicates(t *testing.T) {
	dir := privateDir(t)
	path := filepath.Join(dir, "tokens.json")
	valid := configBytes(t, false)
	boundary := append(valid, bytes.Repeat([]byte{' '}, maxConfigBytes-len(valid))...)
	writeFixture(t, path, boundary, 0600)
	m := NewManager(dir)
	s := loadManager(t, m)
	if !s.Editable || s.State != "disabled" {
		t.Fatal("exact bounded configuration rejected")
	}
	var modules []ports.TokenModuleSetting
	for i := 0; i < maxModules; i++ {
		module := filepath.Join(dir, "module"+strings.Repeat("x", i+1)+".so")
		writeFixture(t, module, []byte("synthetic; not loaded"), 0600)
		modules = append(modules, ports.TokenModuleSetting{Path: module})
	}
	update := confirmedUpdate(s)
	update.Enabled, update.Modules = true, modules
	s, err := m.Save(context.Background(), update)
	if err != nil || len(s.Modules) != maxModules {
		t.Fatal("eight valid modules rejected", err)
	}
	alias := filepath.Join(dir, "alias.so")
	if err := os.Symlink(modules[0].Path, alias); err != nil {
		t.Fatal(err)
	}
	update = confirmedUpdate(s)
	update.Modules = []ports.TokenModuleSetting{modules[0], {Path: alias}}
	if _, err := m.Save(context.Background(), update); !errors.Is(err, ports.ErrTokenSettingsInvalid) {
		t.Fatal("canonical duplicate modules accepted")
	}
}

func TestManagerSerializedByteLimitIncludesFinalNewline(t *testing.T) {
	for _, target := range []int{maxConfigBytes - 1, maxConfigBytes} {
		update := ports.TokenSettingsUpdate{Modules: []ports.TokenModuleSetting{
			{Path: "/a" + strings.Repeat("\x01", 4000)},
			{Path: "/b" + strings.Repeat("\x01", 4000)},
			{Path: "/c"},
		}}
		marshal := func() []byte {
			data, err := json.Marshal(struct {
				Version int                        `json:"version"`
				Enabled bool                       `json:"enabled"`
				Modules []ports.TokenModuleSetting `json:"modules"`
			}{1, false, update.Modules})
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		remaining := target - len(marshal())
		update.Modules[2].Path += strings.Repeat("\x01", remaining/6) + strings.Repeat("x", remaining%6)
		if len(marshal()) != target {
			t.Fatal("fixture did not reach serialized boundary")
		}
		for _, module := range update.Modules {
			if len(module.Path) > 4096 {
				t.Fatal("fixture exceeds path limit")
			}
		}
		data, err := validateSettingsUpdate(update)
		if target == maxConfigBytes-1 {
			if err != nil || len(data) != maxConfigBytes || data[len(data)-1] != '\n' {
				t.Fatal("exact byte limit including newline rejected", err)
			}
		} else if !errors.Is(err, ports.ErrTokenSettingsInvalid) || data != nil {
			t.Fatal("newline exceeded configuration byte limit")
		}
	}
}
