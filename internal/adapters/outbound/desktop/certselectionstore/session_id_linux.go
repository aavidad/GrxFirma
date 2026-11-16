// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package certselectionstore

import (
	"errors"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

// sessionID es el identificador del arranque: el núcleo lo genera al
// arrancar y cambia en cada reinicio.
func sessionID() (string, error) {
	raw, err := securefile.ReadFileLimit("/proc/sys/kernel/random/boot_id", 64)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if len(id) < 16 {
		return "", errors.New("boot_id vacío o inválido")
	}
	return "linux-boot:" + id, nil
}
