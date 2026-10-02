// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows && fyne_gui

package main

import "os/exec"

func configureGUICommand(cmd *exec.Cmd) {
	_ = cmd
}
