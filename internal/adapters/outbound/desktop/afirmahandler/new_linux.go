// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package afirmahandler

import (
	"os"
	"path/filepath"

	"grxfirma/internal/ports"
)

// New devuelve el selector XDG del usuario actual.
func New() ports.ProtocoloAfirma {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return unsupported{}
	}
	return NewXDGSelector(XDGEnvFrom(home, os.Getenv))
}
