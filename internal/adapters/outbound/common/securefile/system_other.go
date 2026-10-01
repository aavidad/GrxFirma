//go:build !linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile

// Fuera de Linux se conserva la lectura sin seguir enlaces.
func ReadTrustedSystemFileLimit(path string, maxBytes int64) ([]byte, error) {
	return ReadFileLimit(path, maxBytes)
}
