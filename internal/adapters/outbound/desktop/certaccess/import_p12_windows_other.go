// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package certaccess

import (
	"context"
	"errors"
)

func importarP12AWindows(context.Context, string, string) error {
	return errors.New("el almacén de certificados de Windows solo está disponible en Windows")
}
