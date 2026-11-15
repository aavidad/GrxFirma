// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"strings"
	"testing"
	"unicode/utf16"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// Órdenes que la persona copia tal cual para recuperar sus ajustes. Son las
// mismas en todos los idiomas: lo que se escribe en la consola no se traduce.
var ordenesAjustes = map[string]string{
	"error.settings_sin_permiso":      `icacls "%APPDATA%\GrxFirma" /grant "%USERNAME%:(OI)(CI)F" /T`,
	"error.settings_sin_permiso_unix": `sudo chown -R "$USER" ~/.config/grxfirma`,
	"error.settings_otro_usuario":     "",
}

// maxDiagnosticoWinUI es lo que WinUI deja pasar de diagnostic.userMessage
// (SafeIpcText.Clean, 512 unidades UTF-16). Un texto más largo se cortaría
// justo por la orden, que va al final.
const maxDiagnosticoWinUI = 512

// TestAjustesSinPermisoLlevanLaOrdenLiteral (lote 39g): las tres claves existen
// en los 11 catálogos, la orden va sola en la última línea y sin traducir, el
// texto cabe en el diagnóstico de WinUI y T lo devuelve sin formatear.
func TestAjustesSinPermisoLlevanLaOrdenLiteral(t *testing.T) {
	t.Parallel()
	for _, idioma := range idiomasI18n {
		catalogo := cargarLocale(t, idioma)
		loc := localizador.Para(idioma)
		for clave, orden := range ordenesAjustes {
			valor, ok := catalogo[clave]
			if !ok {
				t.Errorf("%s.json: falta %q", idioma, clave)
				continue
			}
			if orden != "" {
				lineas := strings.Split(valor, "\n")
				if len(lineas) != 2 || lineas[1] != orden {
					t.Errorf("%s.json: %q debe acabar con la orden sola en su línea %q; está %q", idioma, clave, orden, valor)
				}
				if strings.Count(valor, orden) != 1 {
					t.Errorf("%s.json: %q repite o cambia la orden", idioma, clave)
				}
			} else if strings.Contains(valor, "\n") {
				t.Errorf("%s.json: %q no lleva orden y no debe partirse en líneas", idioma, clave)
			}
			if n := len(utf16.Encode([]rune(valor))); n > maxDiagnosticoWinUI {
				t.Errorf("%s.json: %q mide %d y WinUI corta en %d", idioma, clave, n, maxDiagnosticoWinUI)
			}
			if got := loc.T(clave); got != valor {
				t.Errorf("%s: T(%q) cambia el texto del catálogo: %q", idioma, clave, got)
			}
		}
	}
}

// TestMarcadoresNoConfundenVariablesDeEntorno: %APPDATA% y %USERNAME% cuentan
// como marcadores con nombre, que se comparan entre idiomas, y no como verbos
// de printf (%A, %U) ni de Qt. (OI)(CI)F y $USER no son marcadores.
func TestMarcadoresNoConfundenVariablesDeEntorno(t *testing.T) {
	t.Parallel()
	firma := extraerFirmaPlaceholders("ejecute:\n" + ordenesAjustes["error.settings_sin_permiso"])
	if len(firma.printf) != 0 || len(firma.qt) != 0 {
		t.Fatalf("la orden de icacls no debe tener marcadores printf ni Qt: %s", firma)
	}
	if firma.nombre["%APPDATA%"] != 1 || firma.nombre["%USERNAME%"] != 1 || len(firma.nombre) != 2 {
		t.Fatalf("esperaba %%APPDATA%% y %%USERNAME%% como marcadores con nombre: %s", firma)
	}
	unix := extraerFirmaPlaceholders(ordenesAjustes["error.settings_sin_permiso_unix"])
	if len(unix.printf)+len(unix.qt)+len(unix.nombre) != 0 {
		t.Fatalf("la orden de chown no tiene marcadores: %s", unix)
	}
	// Si una traducción perdiera una variable, la prueba de paridad lo ve.
	roto := extraerFirmaPlaceholders(`icacls "%APPDATA%\GrxFirma" /grant "USERNAME:(OI)(CI)F" /T`)
	if firmasIguales(firma, roto) {
		t.Fatal("perder la variable USERNAME debe cambiar la firma de marcadores")
	}
}
