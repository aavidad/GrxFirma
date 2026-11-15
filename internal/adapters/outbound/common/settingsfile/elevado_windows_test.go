//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/perfil"
)

// guardarComoCLIElevada repite lo que dejó el recorrido de escritorio sin
// Configuración: una preferencia general y la aceptación de tsa_http por la
// línea de órdenes. La aceptación pasa por la auditoría, que protege la
// carpeta, y luego reescribe settings.json.
func guardarComoCLIElevada(t *testing.T, dir string) {
	t.Helper()
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	if err := usersettings.New(dir).Guardar(ctx, map[string]any{"idioma": "es"}); err != nil {
		t.Fatal("guardar preferencias elevado:", err)
	}
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), dir, excepciones.ActivationRequest{
		ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion,
	}); err != nil {
		t.Fatal("aceptar tsa_http elevado:", err)
	}
}

// usarComoUsuario es lo que hace la aplicación sin elevar al arrancar y al
// cambiar un ajuste: leer, comprobar la excepción, guardar y desactivar.
func usarComoUsuario(t *testing.T, dir string) {
	t.Helper()
	ctx := excepciones.WithConfigDir(context.Background(), dir)
	datos, err := usersettings.New(dir).Cargar(ctx)
	if err != nil {
		t.Fatal("leer los ajustes sin elevar:", err)
	}
	if datos["idioma"] != "es" {
		t.Fatalf("idioma = %v; se esperaba es", datos["idioma"])
	}
	if err := excepciones.Check("tsa_http", ctx); err != nil {
		t.Fatal("la excepción aceptada no se ve sin elevar:", err)
	}
	if err := usersettings.New(dir).Guardar(ctx, map[string]any{"idioma": "en"}); err != nil {
		t.Fatal("guardar sin elevar:", err)
	}
	if err := excepciones.SetUserAt(dir, "tsa_http", false); err != nil {
		t.Fatal("desactivar sin elevar:", err)
	}
	if excepciones.Check("tsa_http", ctx) == nil {
		t.Fatal("la excepción sigue activa tras desactivarla")
	}
}

func TestWindowsAjustesGuardadosElevadoLosAbreElUsuarioSinElevar(t *testing.T) {
	exigirElevado(t)
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	home := perfil.Nuevo(t)
	dir := perfil.DirConfig(home)
	guardarComoCLIElevada(t, dir)
	// Lo escrito elevado queda a nombre del usuario, no de Administradores.
	for _, path := range []string{dir, filepath.Join(dir, "settings.json"),
		filepath.Join(dir, "unsafe-exceptions-audit.jsonl"), filepath.Join(dir, ".settings.lock")} {
		comprobarPropietarioUsuario(t, path)
	}
	comoUsuarioSinElevar(t, func() { usarComoUsuario(t, dir) })
}

// TestWindowsAjustesElevadoEntreProcesos es la versión con dos procesos de
// verdad para la máquina virtual: la fase «escribir» se lanza elevada (SSH) y
// la fase «leer», con el mismo perfil, desde una tarea programada del mismo
// usuario sin elevar. Sin las variables no hace nada.
func TestWindowsAjustesElevadoEntreProcesos(t *testing.T) {
	fase, home := os.Getenv("GRXFIRMA_L39C_FASE"), os.Getenv("GRXFIRMA_L39C_PERFIL")
	if fase == "" || home == "" {
		t.Skip("solo en la máquina virtual, con GRXFIRMA_L39C_FASE y GRXFIRMA_L39C_PERFIL")
	}
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := perfil.DirConfig(perfil.Aislar(t, home))
	elevado := windows.GetCurrentProcessToken().IsElevated()
	t.Logf("fase=%s elevado=%v perfil=%s", fase, elevado, home)
	switch fase {
	case "escribir":
		if !elevado {
			t.Fatal("la fase escribir tiene que ir elevada")
		}
		guardarComoCLIElevada(t, dir)
	case "leer":
		if elevado {
			t.Fatal("la fase leer tiene que ir sin elevar")
		}
		usarComoUsuario(t, dir)
	default:
		t.Fatalf("fase desconocida %q", fase)
	}
}

func comprobarPropietarioUsuario(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatal(err)
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if !owner.Equals(tokenUser.User.Sid) {
		t.Errorf("propietario de %s = %s; se esperaba el usuario %s", path, owner, tokenUser.User.Sid)
	}
}
