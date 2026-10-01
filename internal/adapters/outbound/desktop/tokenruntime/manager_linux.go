// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production

package tokenruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/ports"
)

func (m *Manager) Load(ctx context.Context) (ports.TokenSettingsSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.load(ctx)
}

func (m *Manager) load(ctx context.Context) (ports.TokenSettingsSnapshot, error) {
	snapshot := m.initialSnapshot()
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	dir, err := m.openDirectory(false)
	if err != nil {
		return snapshot, nil // fixed unsafe state, never raw filesystem text
	}
	defer dir.close()
	if dir.missing {
		return m.missingSnapshot(dir.writable), nil
	}
	snapshot, _ = m.readSnapshot(dir.fd, dir.writable)
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (m *Manager) initialSnapshot() ports.TokenSettingsSnapshot {
	return ports.TokenSettingsSnapshot{Available: true, State: "invalid", RestartRequired: m.restartRequired,
		Modules: []ports.TokenModuleSetting{}, Checks: []ports.TokenSettingCheck{{ID: "config", Status: "unsafe"}}}
}

func (m *Manager) missingSnapshot(editable bool) ports.TokenSettingsSnapshot {
	s := m.initialSnapshot()
	s.State, s.Editable, s.Revision = "missing", editable, settingsRevision(nil, false)
	s.Checks[0].Status = "missing"
	return s
}

func settingsRevision(data []byte, exists bool) string {
	h := sha256.New()
	if exists {
		_, _ = h.Write([]byte("tokens.json:v1:present\x00"))
	} else {
		_, _ = h.Write([]byte("tokens.json:v1:missing\x00"))
	}
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func (m *Manager) Save(ctx context.Context, update ports.TokenSettingsUpdate) (ports.TokenSettingsSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := m.initialSnapshot()
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	if !update.Confirmed {
		return snapshot, ports.ErrTokenSettingsConfirmation
	}
	if len(update.Revision) != 64 {
		return snapshot, ports.ErrTokenSettingsConflict
	}
	if _, err := hex.DecodeString(update.Revision); err != nil {
		return snapshot, ports.ErrTokenSettingsConflict
	}
	data, err := validateSettingsUpdate(update)
	if err != nil {
		return snapshot, err
	}
	dir, err := m.openDirectory(true)
	if err != nil {
		if errors.Is(err, ports.ErrTokenSettingsWrite) {
			return snapshot, ports.ErrTokenSettingsWrite
		}
		return snapshot, ports.ErrTokenSettingsUnsafe
	}
	defer dir.close()
	if !dir.writable {
		return snapshot, ports.ErrTokenSettingsUnsafe
	}
	if err := lockSettingsDirectory(ctx, dir.fd); err != nil {
		return snapshot, err
	}
	defer unlockSettingsDirectory(dir.fd)
	snapshot, err = m.readSnapshot(dir.fd, true)
	if err != nil || !snapshot.Editable {
		return snapshot, ports.ErrTokenSettingsUnsafe
	}
	if snapshot.Revision != update.Revision {
		return snapshot, ports.ErrTokenSettingsConflict
	}
	if snapshot.State == "invalid" && !update.ReplaceInvalid {
		return snapshot, ports.ErrTokenSettingsInvalid
	}
	warning, err := m.writeSnapshot(ctx, dir.fd, data, snapshot.Revision)
	if err != nil {
		return snapshot, err
	}
	m.restartRequired = true
	snapshot, err = m.readSnapshot(dir.fd, true)
	if err != nil {
		// The rename succeeded: report what was saved rather than an absent
		// write. A later load diagnoses subsequent external filesystem changes.
		snapshot = m.snapshotFromData(data, true)
	}
	snapshot.Warning = warning
	snapshot.RestartRequired = true
	return snapshot, nil
}

func validateSettingsUpdate(update ports.TokenSettingsUpdate) ([]byte, error) {
	if len(update.Modules) > maxModules || (update.Enabled && len(update.Modules) == 0) {
		return nil, ports.ErrTokenSettingsInvalid
	}
	seen := make(map[string]bool)
	for _, module := range update.Modules {
		if !filepath.IsAbs(module.Path) || len(module.Path) > 4096 || strings.ContainsRune(module.Path, 0) || seen[module.Path] {
			return nil, ports.ErrTokenSettingsInvalid
		}
		seen[module.Path] = true
	}
	if update.Enabled {
		resolved := make(map[string]bool)
		for _, module := range update.Modules {
			path, err := pkcs11worker.ValidateModulePath(module.Path)
			if err != nil || resolved[path] {
				return nil, ports.ErrTokenSettingsInvalid
			}
			resolved[path] = true
		}
	}
	modules := update.Modules
	if modules == nil {
		modules = []ports.TokenModuleSetting{}
	}
	data, err := json.Marshal(struct {
		Version int                        `json:"version"`
		Enabled bool                       `json:"enabled"`
		Modules []ports.TokenModuleSetting `json:"modules"`
	}{1, update.Enabled, modules})
	if err != nil || len(data)+1 > maxConfigBytes {
		return nil, ports.ErrTokenSettingsInvalid
	}
	return append(data, '\n'), nil
}

func (m *Manager) snapshotFromData(data []byte, editable bool) ports.TokenSettingsSnapshot {
	s := m.initialSnapshot()
	s.Exists, s.Editable, s.Revision = true, editable, settingsRevision(data, true)
	cfg, err := decodeConfig(data)
	if err != nil {
		s.Checks[0].Status = "invalid"
		return s
	}
	s.Enabled = *cfg.Enabled
	s.State = "disabled"
	if s.Enabled {
		s.State = "configured"
	}
	for _, module := range cfg.Modules {
		s.Modules = append(s.Modules, ports.TokenModuleSetting{Path: module.Path})
	}
	s.Checks[0].Status = "valid"
	if !editable {
		s.Checks[0].Status = "read_only"
	}
	return s
}

// Kept here to avoid accidentally returning PathError text through the DTO.
func fixedSettingsIOError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, os.ErrPermission) {
		return ports.ErrTokenSettingsUnsafe
	}
	return ports.ErrTokenSettingsWrite
}
