// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package rest

import "os"

func replaceIdempotencyFile(source, destination string) error {
	return os.Rename(source, destination)
}
