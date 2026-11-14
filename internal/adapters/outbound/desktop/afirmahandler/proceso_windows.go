// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package afirmahandler

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// prepararProcesoIndependiente lanza AutoFirma en su propio grupo de
// procesos para que siga abierto cuando GrxFirma termine.
func prepararProcesoIndependiente(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}
