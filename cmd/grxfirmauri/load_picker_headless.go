// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
)

func selectLegacyLoadPaths(ctx context.Context, _ string, _ string, _ bool) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, errLegacyLoadCanceled
	default:
		return nil, errLegacyLoadPickerUnavailable
	}
}
