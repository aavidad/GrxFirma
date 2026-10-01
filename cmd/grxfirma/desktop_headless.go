// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
	"fmt"
	"io"
)

func maybeRunDesktopApp(_ context.Context, stderr io.Writer, frontend, rutaP12, password, _, _ string) (bool, int) {
	if frontend == "qt" {
		return maybeRunQtDesktopApp(stderr, rutaP12, password)
	}
	_, _ = fmt.Fprintln(stderr, "esta compilación no incluye interfaz gráfica; recompila con -tags fyne_gui")
	return true, 1
}
