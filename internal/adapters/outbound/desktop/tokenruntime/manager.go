// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

import (
	"sync"

	"grxfirma/internal/ports"
)

// Manager administers saved local settings, never a live Runtime. A successful
// change requires restarting the application; existing operations keep their
// original configuration. Construction performs no filesystem or module IO.
type Manager struct {
	configDir       string
	mu              sync.Mutex
	restartRequired bool
	ops             managerOperations // private per-instance test seams
}

var _ ports.LocalTokenSettings = (*Manager)(nil)

func NewManager(configDir string) *Manager { return &Manager{configDir: configDir} }
