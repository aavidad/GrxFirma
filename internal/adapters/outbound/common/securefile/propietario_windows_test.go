// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile_test

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/common/securefile"
)

func usuarioDelProceso(t *testing.T) *windows.SID {
	t.Helper()
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return tokenUser.User.Sid
}

func descriptor(t *testing.T, path string) *windows.SECURITY_DESCRIPTOR {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd
}

// En un proceso elevado el propietario por defecto es Administradores; los
// ficheros nuevos tienen que quedar a nombre del usuario y con DACL propia,
// no con la heredada de la carpeta. Sin elevar también se exige la DACL
// protegida.
func TestFicherosNuevosSonDelUsuarioConDACLPrivadaWindows(t *testing.T) {
	user := usuarioDelProceso(t)
	dir := t.TempDir()
	paths := map[string]func(string) error{
		"atomico.json": func(path string) error { return securefile.WriteFileAtomic(path, []byte("{}"), 0o600) },
		"registro.jsonl": func(path string) error {
			f, err := securefile.OpenAppend(path, 0o600)
			if err == nil {
				err = f.Close()
			}
			return err
		},
		".cerrojo": func(path string) error {
			f, err := securefile.OpenLockFile(path, 0o600)
			if err == nil {
				err = f.Close()
			}
			return err
		},
	}
	for name, create := range paths {
		path := filepath.Join(dir, name)
		if err := create(path); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sd := descriptor(t, path)
		owner, _, err := sd.Owner()
		if err != nil || owner == nil || !owner.Equals(user) {
			t.Errorf("%s: propietario %v; se esperaba %s (%s)", name, owner, user, sd)
		}
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Errorf("%s: DACL sin protección de herencia: %s", name, sd)
		}
		if !strings.Contains(sd.String(), ";;;"+user.String()+")") {
			t.Errorf("%s: la DACL no da acceso al usuario: %s", name, sd)
		}
	}
}

// La carpeta protegida tiene que dar acceso explícito al usuario del proceso:
// con solo OWNER RIGHTS, lo que crea dentro un proceso elevado hereda acceso
// para Administradores y no para el usuario.
func TestProtectDirectoryDaAccesoHeredableAlUsuarioWindows(t *testing.T) {
	user := usuarioDelProceso(t)
	dir := t.TempDir()
	if err := securefile.ProtectDirectory(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sddl := descriptor(t, dir).String()
	if !strings.Contains(sddl, "(A;OICI;FA;;;"+user.String()+")") {
		t.Fatalf("la carpeta no da acceso heredable al usuario %s: %s", user, sddl)
	}
	for _, sid := range []string{"(A;OICI;FA;;;SY)", "(A;OICI;FA;;;BA)", "(A;OICI;FA;;;OW)"} {
		if !strings.Contains(sddl, sid) {
			t.Fatalf("falta %s en %s", sid, sddl)
		}
	}
	if strings.Count(sddl, "(A;") != 4 {
		t.Fatalf("la carpeta da acceso a alguien más: %s", sddl)
	}
}
