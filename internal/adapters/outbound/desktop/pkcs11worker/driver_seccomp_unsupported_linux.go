// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && !amd64 && !arm64

package pkcs11worker

import "errors"

func RestrictDriverSyscalls() error {
	return errors.New("arquitectura no admitida para aislar el controlador")
}
