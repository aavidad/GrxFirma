// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import "os"

// marcarDescargaWeb escribe el flujo alternativo Zone.Identifier. Si el sistema
// de ficheros no lo admite (FAT, algunas unidades de red) se ignora el error:
// el fichero ya está guardado y la marca es una defensa añadida.
func marcarDescargaWeb(path, origen string) {
	_ = os.WriteFile(path+":Zone.Identifier", []byte(contenidoZoneIdentifier(origen)), 0o600) // #nosec G306 -- flujo de metadatos del propio fichero
}
