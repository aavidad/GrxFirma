// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build unix

package afirmahandler

import (
	"os/exec"
	"syscall"
)

// prepararProcesoIndependiente separa AutoFirma de la sesión de GrxFirma
// para que siga abierto cuando GrxFirma termine.
func prepararProcesoIndependiente(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
