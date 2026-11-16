// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import (
	"fmt"
	"os"
	"unsafe"

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

// checkOpenedDirPrivate exige la DACL que deja EnsurePrivateDir: protegida
// (sin herencia de la carpeta madre) y solo con entradas de permiso para
// OWNER RIGHTS, el usuario del proceso, SYSTEM y Administradores. Una DACL
// nula, heredada o con cualquier otra entrada no se da por privada.
func checkOpenedDirPrivate(dir *os.File) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(dir.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%w: dacl unavailable: %v", ErrNotPrivateDir, err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%w: dacl not protected", ErrNotPrivateDir)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("%w: null dacl", ErrNotPrivateDir)
	}
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil || ace == nil {
			return fmt.Errorf("%w: unreadable ace", ErrNotPrivateDir)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("%w: unexpected ace type %d", ErrNotPrivateDir, ace.Header.AceType)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) // #nosec G103 -- SidStart es el inicio del SID dentro del ACE que devuelve GetAce.
		if !sid.Equals(user) && !sid.IsWellKnown(windows.WinCreatorOwnerRightsSid) &&
			!sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			return fmt.Errorf("%w: access granted to %s", ErrNotPrivateDir, sid.String())
		}
	}
	return nil
}
