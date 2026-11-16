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

// checkOpenedDirOwner exige que la carpeta, ya abierta sin seguir uniones ni
// enlaces, sea del usuario del proceso. Un proceso elevado del mismo usuario
// crea carpetas a nombre de Administradores, y SYSTEM puede con todo: ambos
// se admiten porque ya tienen poder sobre cualquier carpeta del usuario. El
// propietario de una carpeta conserva el derecho a cambiar su DACL, así que
// una carpeta de otra persona nunca se da por privada aunque se pudiera
// reescribir su DACL.
func checkOpenedDirOwner(dir *os.File) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(dir.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%w: owner unavailable: %v", ErrNotPrivateDir, err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("%w: owner unavailable", ErrNotPrivateDir)
	}
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	if owner.Equals(user) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) || owner.IsWellKnown(windows.WinLocalSystemSid) {
		return nil
	}
	return fmt.Errorf("%w: owned by %s", ErrNotPrivateDir, owner.String())
}

// La DACL no se compara con bits POSIX: EnsurePrivateDir la deja protegida al
// escribir, y la carpeta vive en el perfil del usuario.
func checkOpenedDirPrivate(*os.File) error { return nil }
