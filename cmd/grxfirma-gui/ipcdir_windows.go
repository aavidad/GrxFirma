// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import "errors"

// En Windows el canal es un named pipe con DACL propia; no hay directorio.
func comprobarDirectorioPrivado(string, bool) error {
	return errors.New("Windows usa named pipes, no un directorio para el canal IPC")
}
