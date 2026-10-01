// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package usersettings_test

import (
	"context"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
)

func TestGuardar_PermisosRestringidos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	almacen := usersettings.New(dir)
	if err := almacen.Guardar(context.Background(), map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(LocalSystem): %v", err)
	}
	administratorsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(Administrators): %v", err)
	}
	allowedSIDs := []*windows.SID{tokenUser.User.Sid, systemSID, administratorsSID}
	for _, path := range []string{dir, filepath.Join(dir, "settings.json")} {
		assertDACLPrivada(t, path, allowedSIDs)
	}
}

func assertDACLPrivada(t *testing.T, path string, allowedSIDs []*windows.SID) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%s): %v", path, err)
	}
	if sd == nil {
		t.Fatalf("%s no tiene descriptor de seguridad", path)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("%s no tiene DACL: %v", path, err)
	}
	if dacl == nil {
		t.Fatalf("%s no tiene una DACL presente", path)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatalf("Control(%s): %v", path, err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("la DACL de %s debe estar protegida de herencia", path)
	}

	seenEffective := make([]bool, len(allowedSIDs))
	fileAllAccess := windows.ACCESS_MASK(windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff)
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			t.Fatalf("GetAce(%s, %d): %v", path, index, err)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("la DACL de %s contiene una ACE que no es allow: %#v", path, ace)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		allowedIndex := -1
		for candidateIndex, allowedSID := range allowedSIDs {
			if allowedSID.Equals(aceSID) {
				allowedIndex = candidateIndex
				// LocalSystem es a la vez usuario del proceso y principal SYSTEM.
				if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
					seenEffective[candidateIndex] = true
				}
			}
		}
		if allowedIndex < 0 {
			t.Fatalf("la DACL de %s concede acceso a un principal inesperado %s", path, aceSID.String())
		}
		if ace.Mask != windows.GENERIC_ALL && ace.Mask&fileAllAccess != fileAllAccess {
			t.Fatalf("la ACE de %s para %s no concede acceso total: mask=%#x", path, aceSID.String(), ace.Mask)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
			seenEffective[allowedIndex] = true
		}
	}
	for index, seen := range seenEffective {
		if !seen {
			t.Fatalf("la DACL de %s no contiene una ACE efectiva para %s", path, allowedSIDs[index].String())
		}
	}
}
