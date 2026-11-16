// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package certselectionstore

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// sessionID es la hora de arranque del sistema, que cambia en cada reinicio.
func sessionID() (string, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("darwin-boot:%d.%06d", tv.Sec, tv.Usec), nil
}
