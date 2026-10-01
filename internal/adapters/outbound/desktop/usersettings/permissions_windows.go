// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package usersettings

import "golang.org/x/sys/windows"

func protegerDirectorioConfiguracion(path string) error {
	return aplicarDACLPrivada(path, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
}

func protegerFicheroConfiguracion(path string) error {
	return aplicarDACLPrivada(path, windows.NO_INHERITANCE)
}

func aplicarDACLPrivada(path string, inheritance uint32) error {
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
	administratorsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}

	entries := []windows.EXPLICIT_ACCESS{
		nuevaEntradaDACL(userSID, windows.TRUSTEE_IS_USER, inheritance),
		nuevaEntradaDACL(systemSID, windows.TRUSTEE_IS_USER, inheritance),
		nuevaEntradaDACL(administratorsSID, windows.TRUSTEE_IS_WELL_KNOWN_GROUP, inheritance),
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func nuevaEntradaDACL(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}
