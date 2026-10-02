// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"errors"
	"io"
	"path/filepath"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

const maxTemporaryCredentialBytes = 2 * 1024 * 1024
const maxTemporaryPasswordBytes = 4096

// readTemporaryCredential limita la lectura al fichero regular elegido localmente.
// No propaga rutas ni contenido de la credencial en errores.
func readTemporaryCredential(path string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".p12" && ext != ".pfx" {
		return nil, errors.New("seleccione un archivo P12 o PFX")
	}
	file, err := securefile.OpenRead(path)
	if err != nil {
		return nil, errors.New("no se pudo abrir el archivo P12/PFX seleccionado")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTemporaryCredentialBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxTemporaryCredentialBytes {
		clear(data)
		return nil, errors.New("el archivo P12/PFX debe ser legible, no estar vacío y ocupar como máximo 2 MiB")
	}
	return data, nil
}
