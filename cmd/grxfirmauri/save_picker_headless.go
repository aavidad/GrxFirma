// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
)

// Sin interfaz no hay diálogo que pregunte antes de reemplazar.
const legacySavePickerConfirmsOverwrite = false

func selectLegacySaveTargetPath(_ context.Context, defaultPath string, _ string) (string, error) {
	return defaultPath, nil
}
