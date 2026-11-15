// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/settingsfile"
)

// rutaPerfilSensible es la ruta que lleva la causa técnica. No debe llegar a
// la interfaz ni en el texto ni en el diagnóstico.
const rutaPerfilSensible = "/home/maria.perez/.config/grxfirma/settings.json"

func errorAjustesSinPermiso() error {
	return settingsfile.ExplainAccess(&fs.PathError{Op: "open", Path: rutaPerfilSensible, Err: fs.ErrPermission})
}

// ajustesQueNoSeGuardan lee bien y falla al guardar, como un proceso elevado
// que encuentra la carpeta de ajustes a nombre de otra cuenta.
type ajustesQueNoSeGuardan struct{ err error }

func (a *ajustesQueNoSeGuardan) Cargar(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}
func (a *ajustesQueNoSeGuardan) Guardar(context.Context, map[string]any) error { return a.err }

func comprobarFalloAjustes(t *testing.T, resp respuesta, idioma, action, code, key string) {
	t.Helper()
	want := localizador.Para(idioma).T(key)
	if want == key {
		t.Fatalf("el catálogo %s no tiene %q", idioma, key)
	}
	if resp.OK || resp.Action != action || resp.Outcome != ipcOutcomeFailure {
		t.Fatalf("%s: esperaba fallo de %s, resp=%+v", idioma, action, resp)
	}
	if resp.ErrorCode != code {
		t.Fatalf("%s: errorCode=%q, quiero %q", idioma, resp.ErrorCode, code)
	}
	if resp.Error != want {
		t.Fatalf("%s: error=%q, quiero el texto del catálogo %q", idioma, resp.Error, want)
	}
	d := resp.Diagnostic
	if d == nil {
		t.Fatalf("%s: sin diagnóstico; WinUI enseña diagnostic.userMessage", idioma)
	}
	if d.UserMessage != want {
		t.Fatalf("%s: diagnostic.userMessage=%q, quiero %q", idioma, d.UserMessage, want)
	}
	if d.Category != "app_local" || d.FailureCode != "SETTINGS_ACCESS" || !d.UserCanResolveDirectly || d.ExpertMessage != key {
		t.Fatalf("%s: diagnóstico inesperado %+v", idioma, d)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibido := range []string{"maria.perez", "Posible problema", "permission denied"} {
		if strings.Contains(string(raw), prohibido) {
			t.Fatalf("%s: la respuesta lleva %q: %s", idioma, prohibido, raw)
		}
	}
}

// TestAjustesIlegiblesLleganConElTextoDelCatalogo (lote 39g): leer o guardar
// unos ajustes sin permiso da el texto del catálogo en el idioma del
// manejador, como error y como diagnostic.userMessage, y no el diagnóstico
// genérico de «aplicación local» ni la ruta del perfil.
func TestAjustesIlegiblesLleganConElTextoDelCatalogo(t *testing.T) {
	t.Parallel()
	for _, idioma := range []string{"es", "en", "de", "zh"} {
		t.Run("get_settings/"+idioma, func(t *testing.T) {
			t.Parallel()
			m := &Manejador{Loc: localizador.Para(idioma), Settings: &stubSettings{err: errorAjustesSinPermiso()}}
			resp := m.despachar(context.Background(), peticion{Action: "get_settings"})
			comprobarFalloAjustes(t, resp, idioma, "get_settings", "settings_no_permission", settingsfile.KeyNoPermission)
		})
		t.Run("get_settings_tipado/"+idioma, func(t *testing.T) {
			t.Parallel()
			m := &Manejador{Loc: localizador.Para(idioma), Settings: &stubTypedSettings{err: errorAjustesSinPermiso()}}
			resp := m.despachar(context.Background(), peticion{Action: "get_settings"})
			comprobarFalloAjustes(t, resp, idioma, "get_settings", "settings_no_permission", settingsfile.KeyNoPermission)
		})
		t.Run("save_settings_al_leer/"+idioma, func(t *testing.T) {
			t.Parallel()
			m := &Manejador{Loc: localizador.Para(idioma), Settings: &stubSettings{err: errorAjustesSinPermiso()}}
			resp := m.despachar(context.Background(), peticion{Action: "save_settings", Params: json.RawMessage(`{"tema":"oscuro"}`)})
			comprobarFalloAjustes(t, resp, idioma, "save_settings", "settings_no_permission", settingsfile.KeyNoPermission)
		})
		t.Run("save_settings_otra_cuenta/"+idioma, func(t *testing.T) {
			t.Parallel()
			otra := &settingsfile.Error{Key: settingsfile.KeyOtherUser, Err: fs.ErrPermission}
			m := &Manejador{Loc: localizador.Para(idioma), Settings: &ajustesQueNoSeGuardan{err: otra}}
			resp := m.despachar(context.Background(), peticion{Action: "save_settings", Params: json.RawMessage(`{"tema":"oscuro"}`)})
			comprobarFalloAjustes(t, resp, idioma, "save_settings", "settings_other_user", settingsfile.KeyOtherUser)
		})
	}
}

// Un fallo de ajustes que no tiene explicación sigue como antes.
func TestFalloAjustesSinExplicacionNoCambia(t *testing.T) {
	t.Parallel()
	m := &Manejador{Loc: localizador.Para("es"), Settings: &ajustesQueNoSeGuardan{err: fs.ErrPermission}}
	resp := m.despachar(context.Background(), peticion{Action: "save_settings", Params: json.RawMessage(`{"tema":"oscuro"}`)})
	if resp.OK || resp.ErrorCode == "settings_no_permission" || resp.Error != m.t("error.settings_no_configurado") {
		t.Fatalf("respuesta inesperada: %+v", resp)
	}
	desconocida := &settingsfile.Error{Key: "error.otra_cosa", Err: fs.ErrPermission}
	if _, ok := m.falloAjustesExplicado("get_settings", desconocida); ok {
		t.Fatal("solo se explican las claves conocidas")
	}
}
