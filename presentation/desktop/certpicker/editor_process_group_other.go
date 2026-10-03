// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !windows

package certpicker

import "os/exec"

func prepararGrupoEditorSello(_ *exec.Cmd) {}
func terminarGrupoEditorSello(_ *exec.Cmd) {}
