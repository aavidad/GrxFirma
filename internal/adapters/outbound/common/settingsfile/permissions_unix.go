//go:build !windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import "os"

func protectSettingsDirectory(dir *os.File) error {
	return dir.Chmod(0700)
}

// KeyNoPermission explica que los ajustes no se pueden abrir por permisos y
// cómo devolvérselos al usuario en Linux y macOS.
const KeyNoPermission = "error.settings_sin_permiso_unix"
