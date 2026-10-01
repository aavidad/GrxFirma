// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package localtlstrust

func protectManagedCAKey(key []byte) ([]byte, error) {
	return append([]byte(nil), key...), nil
}

func unprotectManagedCAKey(stored []byte) ([]byte, error) {
	return append([]byte(nil), stored...), nil
}
