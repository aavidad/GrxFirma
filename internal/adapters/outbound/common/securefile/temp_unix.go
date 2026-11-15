// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"fmt"
	"os"
)

func createPrivateTemp(dir, prefix string, perm os.FileMode) (*os.File, error) {
	tmp, err := os.CreateTemp(dir, prefix+"*")
	if err != nil {
		return nil, fmt.Errorf("securefile: create temporary file: %w", err)
	}
	if err := tmp.Chmod(perm.Perm()); err != nil {
		name := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(name)
		return nil, fmt.Errorf("securefile: restrict temporary file: %w", err)
	}
	return tmp, nil
}
