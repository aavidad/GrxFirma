//go:build !linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"path/filepath"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
)

func readFilePinned(path string, max int64) ([]byte, error) {
	return securefile.ReadFileLimit(path, max)
}

// writeFile comprueba la ruta y escribe en la ruta pedida, no en la física
// que devuelve Check. El escritor recorre esa ruta con Lstat y rechaza un
// enlace simbólico o una unión en cualquier componente; con la ruta ya
// resuelta por EvalSymlinks no quedaría ningún enlace que rechazar y se
// guardaría fuera de la carpeta elegida.
func writeFile(path string, data []byte, force bool, check func(string) (string, error)) (string, error) {
	if _, err := check(path); err != nil {
		return "", err
	}
	policy := filesystem.PoliticaRenombrar
	if force {
		policy = filesystem.PoliticaForzar
	}
	return filesystem.NuevoEscritorResultado(policy).Escribir(filepath.Clean(path), data)
}
