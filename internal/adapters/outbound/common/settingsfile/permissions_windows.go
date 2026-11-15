//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"golang.org/x/sys/windows"
	"os"
)

// El directorio de ajustes pertenece al usuario del token, también cuando
// otro componente ya aplicó su DACL. El descriptor fue abierto sin reanálisis.
func protectSettingsDirectory(dir *os.File) error {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	userSID, err := tokenUser.User.Sid.Copy()
	if err != nil {
		return err
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	adminsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	entry := func(sid *windows.SID, kind windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
		return windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
			Trustee:           windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: kind, TrusteeValue: windows.TrusteeValueFromSID(sid)},
		}
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		entry(userSID, windows.TRUSTEE_IS_USER), entry(systemSID, windows.TRUSTEE_IS_USER), entry(adminsSID, windows.TRUSTEE_IS_WELL_KNOWN_GROUP),
	}, nil)
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(windows.Handle(dir.Fd()), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

// KeyNoPermission explica que los ajustes no se pueden abrir por permisos y
// cómo devolvérselos al usuario en Windows.
const KeyNoPermission = "error.settings_sin_permiso"
