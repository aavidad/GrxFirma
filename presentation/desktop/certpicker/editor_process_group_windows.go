// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import "os/exec"

// WinUI no inicia un backend hijo para previsualizar el PDF.
func prepararGrupoEditorSello(_ *exec.Cmd) {}
func terminarGrupoEditorSello(_ *exec.Cmd) {}
