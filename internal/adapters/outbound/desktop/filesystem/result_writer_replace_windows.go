// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package filesystem

import "golang.org/x/sys/windows"

func reemplazarFicheroAtomico(origen, destino string) error {
	return windows.Rename(origen, destino)
}
