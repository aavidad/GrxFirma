// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !darwin && !windows

package certselectionstore

import "errors"

// En los demás sistemas no se sabe cuándo empieza la sesión: no se guardan
// preferencias de sesión.
func sessionID() (string, error) {
	return "", errors.New("sin identificador de arranque en este sistema")
}
