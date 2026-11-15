//go:build !linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package portalpaths

import (
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
)

func readFilePinned(path string, max int64) ([]byte, error) {
	return securefile.ReadFileLimit(path, max)
}

func WriteFile(path string, data []byte, force bool) (string, error) {
	physical, err := Check(path)
	if err != nil {
		return "", err
	}
	policy := filesystem.PoliticaRenombrar
	if force {
		policy = filesystem.PoliticaForzar
	}
	return filesystem.NuevoEscritorResultado(policy).Escribir(physical, data)
}
