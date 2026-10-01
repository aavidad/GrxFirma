// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin && !cgo

package certaccess

import (
	"context"
	"fmt"
)

func importarP12AMac(context.Context, string, string) error {
	return fmt.Errorf("importación P12 a Keychain requiere macOS con cgo habilitado")
}
