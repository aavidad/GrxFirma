// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

import "os/exec"

//lint:ignore U1000 usado en builds con -tags fyne_gui (load_picker_fyne.go)
func configureGUICommand(cmd *exec.Cmd) {
	_ = cmd
}
