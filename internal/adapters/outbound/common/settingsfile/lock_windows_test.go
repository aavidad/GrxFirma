//go:build windows

// SPDX-License-Identifier: EUPL-1.2
package settingsfile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/settingsfile"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestWindowsSettingsLockPermiteCrearGuardarYDesactivar(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "GrxFirma")
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if _, err := config.LoadWithPolicyDir(dir, t.TempDir()); err != nil {
		t.Fatal("crear configuración:", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	if err := usersettings.New(dir).Guardar(ctx, map[string]any{"idioma": "es"}); err != nil {
		t.Fatal("guardar preferencias:", err)
	}
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalDesktop(ctx), dir, excepciones.ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal("activar:", err)
	}
	if err := excepciones.Check("tsa_http", ctx); err != nil {
		t.Fatal(err)
	}
	if err := excepciones.SetUserAt(dir, "tsa_http", false); err != nil {
		t.Fatal("desactivar:", err)
	}
	if excepciones.Check("tsa_http", ctx) == nil {
		t.Fatal("no se desactivó")
	}
	if err := settingsfile.WithLock(ctx, dir, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		t.Fatalf("DACL ausente: %v", err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL sin protección de herencia: %v", err)
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{tokenUser.User.Sid.String(): false}
	for _, kind := range []windows.WELL_KNOWN_SID_TYPE{windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid} {
		sid, err := windows.CreateWellKnownSid(kind)
		if err != nil {
			t.Fatal(err)
		}
		allowed[sid.String()] = false
	}
	effective := map[string]bool{}
	inheritable := map[string]bool{}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("ACE inesperada: %#v", ace)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if _, ok := allowed[sid]; !ok {
			t.Fatalf("directorio concede acceso a %s", sid)
		}
		// Windows puede partir una entrada GENERIC_ALL heredable en dos: una
		// efectiva sobre la carpeta y otra solo de herencia con los derechos
		// genéricos. Se exige que cada principal tenga las dos cosas.
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
			effective[sid] = true
		}
		if ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) == (windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE) {
			inheritable[sid] = true
		}
		allowed[sid] = true
	}
	for sid, found := range allowed {
		if !found {
			t.Fatalf("DACL no incluye %s", sid)
		}
		if !effective[sid] {
			t.Fatalf("ACE sin acceso efectivo: %s", sid)
		}
		if !inheritable[sid] {
			t.Fatalf("ACE no heredable por ficheros y carpetas: %s", sid)
		}
	}

}
