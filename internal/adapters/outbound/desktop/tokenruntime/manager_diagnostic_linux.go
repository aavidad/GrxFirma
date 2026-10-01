// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production

package tokenruntime

import (
	"context"
	"os"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/ports"
)

// Diagnose is an explicit, noninteractive local check. It never starts a
// helper/dialog, loads a library, reads environment options or requests a PIN.
// Filesystem readiness and a momentary parent-memory reservation are NOT
// evidence of module loading, future capacity, reader/token access or signing.
func (m *Manager) Diagnose(ctx context.Context) (ports.TokenSettingsSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, err := m.load(ctx)
	if err != nil {
		return snapshot, err
	}
	checkPath := m.ops.checkPath
	if checkPath == nil {
		checkPath = pkcs11worker.ValidateModulePath
	}
	modules := "not_checked"
	if snapshot.State == "configured" {
		modules = "filesystem_checked"
		seen := make(map[string]bool)
		for _, module := range snapshot.Modules {
			if err := ctx.Err(); err != nil {
				return snapshot, err
			}
			resolved, err := checkPath(module.Path)
			if err != nil || seen[resolved] {
				modules = "invalid"
			}
			seen[resolved] = true
		}
	}
	snapshot.Checks = append(snapshot.Checks, ports.TokenSettingCheck{ID: "modules", Status: modules})
	executable := m.ops.executable
	if executable == nil {
		executable = os.Executable
	}
	helper := "unavailable"
	if path, err := executable(); err == nil {
		if _, err := resolveHelper(path); err == nil {
			helper = "filesystem_checked"
		}
	}
	snapshot.Checks = append(snapshot.Checks, ports.TokenSettingCheck{ID: "helper", Status: helper})
	pinentry := "unavailable"
	// Same fixed candidates and managed-path policy as tokenpin.requestLocal.
	// This does not check DISPLAY, D-Bus or run the executable to probe its UI.
	for _, candidate := range []string{"/usr/bin/pinentry-qt", "/usr/bin/pinentry-gnome3", "/usr/bin/pinentry-gtk-2"} {
		path, err := checkPath(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && info.Mode().Perm()&0111 != 0 {
			pinentry = "filesystem_checked"
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	memory := "unavailable"
	if m.checkPINMemory() {
		memory = "reservation_checked"
	}
	snapshot.Checks = append(snapshot.Checks,
		ports.TokenSettingCheck{ID: "pinentry", Status: pinentry},
		ports.TokenSettingCheck{ID: "pin_memory", Status: memory},
		ports.TokenSettingCheck{ID: "hardware", Status: "not_checked"})
	return snapshot, ctx.Err()
}

func (m *Manager) checkPINMemory() bool {
	allocate := m.ops.allocate
	if allocate == nil {
		allocate = secmem.NewLockedSize
	}
	var owners []*secmem.Blob
	defer func() {
		for _, owner := range owners {
			owner.Destroy()
		}
	}()
	// Parent transport and Assuan owners coexist while prompting. Reserve
	// both, initially zero, not two sequential probes that overstate capacity.
	for _, size := range []int{pkcs11worker.MaxPINBytes + 1, 4096 + 3*pkcs11worker.MaxPINBytes + 16 + pkcs11worker.MaxPINBytes} {
		owner, err := allocate(size)
		if owner != nil {
			owners = append(owners, owner)
		}
		if err != nil || owner == nil || !owner.Locked() || owner.Len() != size {
			return false
		}
	}
	return true
}
