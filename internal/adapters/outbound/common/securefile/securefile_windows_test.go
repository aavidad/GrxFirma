// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile_test

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/common/securefile"
)

func TestProtectFileAplicaDACLPrivadaWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rest-auth.curl")
	if err := os.WriteFile(path, []byte("header privado"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		t.Fatalf("ProtectFile() error = %v", err)
	}
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatal(err)
	}
	dacl, defaulted, err := sd.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("DACL ausente: dacl=%v err=%v", dacl, err)
	}
	if defaulted {
		t.Fatal("la DACL no debe ser la DACL predeterminada")
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("la DACL debe estar protegida de herencia")
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{tokenUser.User.Sid.String(): false}
	for _, kind := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid,
		windows.WinBuiltinAdministratorsSid,
	} {
		sid, sidErr := windows.CreateWellKnownSid(kind)
		if sidErr != nil {
			t.Fatal(sidErr)
		}
		allowed[sid.String()] = false
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if aceErr := windows.GetAce(dacl, index, &ace); aceErr != nil {
			t.Fatal(aceErr)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("ACE inesperada: %#v", ace)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		sidText := aceSID.String()
		if _, ok := allowed[sidText]; !ok {
			t.Fatalf("la DACL concede acceso a %s", aceSID)
		}
		allowed[sidText] = true
	}
	for sid, seen := range allowed {
		if !seen {
			t.Fatalf("la DACL no concede acceso al principal requerido %s", sid)
		}
	}
}
