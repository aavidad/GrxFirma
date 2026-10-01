// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package identityevidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func assertPermisosRegistro(t *testing.T, path string) {
	t.Helper()
	token, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, ruta := range []string{path, filepath.Dir(path)} {
		sd, err := windows.GetNamedSecurityInfo(ruta, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		dacl, _, err := sd.DACL()
		if err != nil || dacl == nil {
			t.Fatalf("DACL ausente en %s: %v", ruta, err)
		}
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatalf("DACL no protegida en %s: %v", ruta, err)
		}
		user := token.User.Sid.String()
		allowed := map[string]bool{user: false, "S-1-5-18": false, "S-1-5-32-544": false}
		all := windows.ACCESS_MASK(windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff)
		for i := uint32(0); i < uint32(dacl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(dacl, i, &ace); err != nil {
				t.Fatal(err)
			}
			if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
				t.Fatalf("ACE inesperada en %s", ruta)
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
			if _, ok := allowed[sid]; !ok {
				t.Fatalf("principal inesperado %s en %s", sid, ruta)
			}
			if ace.Mask != windows.GENERIC_ALL && ace.Mask&all != all {
				t.Fatalf("ACE sin acceso completo en %s", ruta)
			}
			if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 {
				allowed[sid] = true
			}
		}
		for sid, seen := range allowed {
			if !seen {
				t.Fatalf("falta ACE efectiva %s en %s", sid, ruta)
			}
		}
	}
}

func TestRegistroWindowsRechazaACLDebilitadaSinModificarla(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "fichero", true: "directorio"}[directory], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "privado", "registro.log")
			config := Configuracion{Ruta: path, Clave: make([]byte, 32), VersionClave: "v1"}
			registro, err := Nuevo(config)
			if err != nil {
				t.Fatal(err)
			}
			ruta := path
			if directory {
				ruta = filepath.Dir(path)
			}
			ampliarDACLFixture(t, ruta)
			before := descriptorFixture(t, ruta)
			if _, err := registro.RegistrarIdentidad(context.Background(), evidenciaPrueba()); err == nil {
				t.Fatal("aceptó DACL debilitada")
			}
			if _, err := Nuevo(config); err == nil {
				t.Fatal("reabrió registro con DACL debilitada")
			}
			if after := descriptorFixture(t, ruta); before != after {
				t.Fatalf("modificó DACL/propietario: antes=%s después=%s", before, after)
			}
			if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
				t.Fatalf("escribió tras debilitar DACL: longitud=%d error=%v", len(data), err)
			}
		})
	}
}

func ampliarDACLFixture(t *testing.T, ruta string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(ruta, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

func descriptorFixture(t *testing.T, ruta string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(ruta, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

func TestRegistroWindowsNoModificaPadrePreexistente(t *testing.T) {
	parent := t.TempDir()
	ampliarDACLFixture(t, parent)
	before := descriptorFixture(t, parent)
	path := filepath.Join(parent, "registro.log")
	config := Configuracion{Ruta: path, Clave: make([]byte, 32), VersionClave: "v1"}
	if _, err := Nuevo(config); err == nil {
		t.Fatal("aceptó padre preexistente amplio")
	}
	if before != descriptorFixture(t, parent) {
		t.Fatal("modificó DACL/propietario del padre preexistente")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("creó fichero pese a padre inseguro: %v", err)
	}
	// La misma raíz amplia sí puede albergar un nuevo subdirectorio privado.
	config.Ruta = filepath.Join(parent, "nuevo-privado", "registro.log")
	if _, err := Nuevo(config); err != nil {
		t.Fatal(err)
	}
	assertPermisosRegistro(t, config.Ruta)
	if before != descriptorFixture(t, parent) {
		t.Fatal("modificó el padre al crear un subdirectorio privado")
	}
}
