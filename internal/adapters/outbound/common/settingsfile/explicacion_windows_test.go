//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/settingsfile"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/perfil"
)

// estropearComoAntes deja la carpeta y settings.json como los dejó la CLI
// elevada antes del arreglo (lo visto en la máquina virtual el 15/11):
// carpeta del usuario con OW, SYSTEM y Administradores, y settings.json con
// propietario Administradores y esas mismas entradas heredadas.
func estropearComoAntes(t *testing.T, dir string) {
	t.Helper()
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	aplicarSDDL(t, dir, "O:"+tokenUser.User.Sid.String()+"D:PAI(A;OICI;FA;;;OW)(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	aplicarSDDL(t, filepath.Join(dir, "settings.json"), "O:BAD:AI(A;ID;FA;;;OW)(A;ID;FA;;;SY)(A;ID;FA;;;BA)")
}

func aplicarSDDL(t *testing.T, path, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	info := windows.SECURITY_INFORMATION(windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION)
	if strings.Contains(sddl, "D:P") {
		info |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		info |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, info, owner, nil, dacl, nil); err != nil {
		t.Fatalf("aplicar %s a %s: %v", sddl, path, err)
	}
}

func exigirExplicacion(t *testing.T, err error, key string) {
	t.Helper()
	var explained *settingsfile.Error
	if !errors.As(err, &explained) {
		t.Fatalf("error sin explicación del catálogo: %v", err)
	}
	if explained.Key != key {
		t.Fatalf("clave = %q; se esperaba %q", explained.Key, key)
	}
	if got, want := err.Error(), localizador.Detectar().T(key); got != want {
		t.Fatalf("texto = %q; se esperaba el del catálogo %q", got, want)
	}
}

func TestWindowsAjustesIlegiblesDicenQuePasaYComoArreglarlo(t *testing.T) {
	exigirElevado(t)
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := perfil.DirConfig(perfil.Nuevo(t))
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if err := usersettings.New(dir).Guardar(ctx, map[string]any{"idioma": "es"}); err != nil {
		t.Fatal(err)
	}
	estropearComoAntes(t, dir)

	comoUsuarioSinElevar(t, func() {
		_, err := usersettings.New(dir).Cargar(ctx)
		exigirExplicacion(t, err, settingsfile.KeyNoPermission)
		err = usersettings.New(dir).Guardar(ctx, map[string]any{"idioma": "en"})
		exigirExplicacion(t, err, settingsfile.KeyNoPermission)
		_, err = excepciones.ActivateLocal(excepciones.ForLocalDesktop(ctx), dir, excepciones.ActivationRequest{
			ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion, Origin: excepciones.OriginSettings,
		})
		exigirExplicacion(t, err, settingsfile.KeyNoPermission)
	})

	// El remedio del mensaje: guardar una vez como administrador devuelve el
	// fichero al usuario, sin perder lo que había.
	datos, err := usersettings.New(dir).Cargar(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := usersettings.New(dir).Guardar(ctx, datos); err != nil {
		t.Fatal("guardar como administrador:", err)
	}
	comoUsuarioSinElevar(t, func() {
		datos, err := usersettings.New(dir).Cargar(ctx)
		if err != nil {
			t.Fatal("sigue sin poder leer tras guardar como administrador:", err)
		}
		if datos["idioma"] != "es" {
			t.Fatalf("idioma = %v; se esperaba es", datos["idioma"])
		}
	})
}

// habilitarPrivilegio activa un privilegio del token elevado mientras dura la
// preparación y lo desactiva después, para que no tape otros fallos.
func habilitarPrivilegio(t *testing.T, name string) func() {
	t.Helper()
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
		token.Close()
		t.Fatal(err)
	}
	set := func(attributes uint32) error {
		privileges := windows.Tokenprivileges{PrivilegeCount: 1}
		privileges.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: attributes}
		// Si el token no tiene el privilegio, el fallo sale al usarlo.
		return windows.AdjustTokenPrivileges(token, false, &privileges, 0, nil, nil)
	}
	if err := set(windows.SE_PRIVILEGE_ENABLED); err != nil {
		token.Close()
		t.Skip("sin ", name, ": ", err)
	}
	return func() {
		_ = set(0)
		token.Close()
	}
}

// cuentaInvitado devuelve el SID de la cuenta Invitado (RID 501) del mismo
// dominio o equipo que el usuario: existe siempre, aunque esté desactivada.
func cuentaInvitado(t *testing.T) *windows.SID {
	t.Helper()
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	text := tokenUser.User.Sid.String()
	cut := strings.LastIndexByte(text, '-')
	if !strings.HasPrefix(text, "S-1-5-21-") || cut < 0 {
		t.Skip("el usuario no es una cuenta local ni de dominio:", text)
	}
	guest, err := windows.StringToSid(text[:cut] + "-501")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, kind, err := guest.LookupAccount(""); err != nil || kind != windows.SidTypeUser {
		t.Skip("no se encuentra la cuenta Invitado:", err)
	}
	return guest
}

func TestWindowsElevadoNoEscribeLosAjustesDeOtraPersona(t *testing.T) {
	exigirElevado(t)
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := perfil.DirConfig(perfil.Nuevo(t))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	guest := cuentaInvitado(t)
	restore := habilitarPrivilegio(t, "SeRestorePrivilege")
	err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, guest, nil, nil, nil)
	restore()
	if err != nil {
		t.Fatal("dar la carpeta a Invitado:", err)
	}
	before, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}

	ctx := excepciones.WithConfigDir(context.Background(), dir)
	err = usersettings.New(dir).Guardar(ctx, map[string]any{"idioma": "es"})
	exigirExplicacion(t, err, settingsfile.KeyOtherUser)
	_, err = excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), dir, excepciones.ActivationRequest{
		ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion,
	})
	exigirExplicacion(t, err, settingsfile.KeyOtherUser)

	after, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if before.String() != after.String() {
		t.Fatalf("se cambiaron los permisos de la carpeta ajena:\nantes   %s\ndespués %s", before, after)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se escribió settings.json en la carpeta ajena: %v", err)
	}
}
