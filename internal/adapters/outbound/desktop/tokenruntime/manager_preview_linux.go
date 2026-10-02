// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production

package tokenruntime

import "sync"

// Manager administers saved local settings, never a live Runtime. Changes
// require a restart; existing operations keep their original configuration.
type Manager struct {
	configDir       string
	mu              sync.Mutex
	restartRequired bool
	ops             managerOperations // private per-instance test seams
}

func NewManager(configDir string) *Manager { return &Manager{configDir: configDir} }
