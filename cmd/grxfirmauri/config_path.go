// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"grxfirma/internal/appdirs"
	"os"
)

func defaultProtocolConfigDir() string {
	home, _ := os.UserHomeDir()
	return appdirs.Config(home)
}
