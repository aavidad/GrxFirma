// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
)

func selectLegacySaveTargetPath(_ context.Context, defaultPath string, _ string) (string, error) {
	return defaultPath, nil
}
