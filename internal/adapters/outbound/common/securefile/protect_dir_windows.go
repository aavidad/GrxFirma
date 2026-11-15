// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// protectOpenedDirectory deja la carpeta para su propietario, el usuario del
// proceso, SYSTEM y Administradores. OWNER RIGHTS (OW) solo no basta: si un
// proceso elevado crea un fichero dentro, su propietario es Administradores y
// la entrada heredada OW deja fuera al mismo usuario cuando no está elevado.
// La entrada explícita del usuario del token no abre la carpeta a nadie más:
// es el mismo usuario que ya la está protegiendo.
func protectOpenedDirectory(dir *os.File, _ os.FileMode) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf(
		"D:P(A;OICI;FA;;;OW)(A;OICI;FA;;;%s)(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
		user.String(),
	))
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(
		windows.Handle(dir.Fd()),
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
