// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func raizRepositorio(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", "..", "..", ".."))
}

func leerSuperficie(t *testing.T, ruta string) string {
	t.Helper()
	raw, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", ruta, err)
	}
	return string(raw)
}

func contienePalabraVisible(texto string) bool {
	consecutivas := 0
	for _, r := range texto {
		if unicode.IsLetter(r) {
			consecutivas++
			if consecutivas >= 3 {
				return true
			}
			continue
		}
		consecutivas = 0
	}
	return false
}

func TestI18nQMLNoIntroduceTextoVisibleDirecto(t *testing.T) {
	raiz := filepath.Join(raizRepositorio(t), "cmd", "gui-qml", "qml")
	asignacionVisible := regexp.MustCompile(
		`(?m)\b(?:text|title|placeholderText|toolTip|Accessible\.name|Accessible\.description|statusMessage|message|summary|label|description)\s*:\s*("(?:\\.|[^"\\])*")`,
	)

	err := filepath.Walk(raiz, func(ruta string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(ruta) != ".qml" {
			return nil
		}
		fuente := leerSuperficie(t, ruta)
		for _, match := range asignacionVisible.FindAllStringSubmatchIndex(fuente, -1) {
			literal := fuente[match[2]:match[3]]
			texto, err := strconv.Unquote(literal)
			if err != nil {
				t.Fatalf("%s: literal QML inválido %s: %v", ruta, literal, err)
			}
			if !contienePalabraVisible(texto) {
				continue
			}
			linea := 1 + strings.Count(fuente[:match[0]], "\n")
			t.Errorf("%s:%d: texto visible hardcodeado %q; usar tr(...)", ruta, linea, texto)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk(%s): %v", raiz, err)
	}
}

func TestI18nBridgesQtLocalizanLiteralesVisibles(t *testing.T) {
	raiz := raizRepositorio(t)
	// Cubrir tanto QStringLiteral("...") como QString("..."). El segundo
	// constructor dejó escapar anteriormente un estado visible sin localizar.
	literalQString := regexp.MustCompile(`(?:QStringLiteral|QString)\("((?:\\.|[^"\\])*)"\)`)
	envuelto := regexp.MustCompile(`\b(?:bt|it)\s*\(\s*$`)
	// Son patrones internos para clasificar diagnósticos, nunca se muestran.
	clasificadoresInternos := map[string]bool{
		"sin conexión con el motor": true,
		"firma no soportada":        true,
	}

	for _, nombre := range []string{"backendbridge.cpp", "ipcbridge.cpp"} {
		ruta := filepath.Join(raiz, "cmd", "gui-qml", nombre)
		fuente := leerSuperficie(t, ruta)
		for _, match := range literalQString.FindAllStringSubmatchIndex(fuente, -1) {
			texto, err := strconv.Unquote(`"` + fuente[match[2]:match[3]] + `"`)
			if err != nil {
				t.Fatalf("%s: QStringLiteral inválido: %v", ruta, err)
			}
			if clasificadoresInternos[texto] ||
				!strings.ContainsRune(texto, ' ') ||
				!contienePalabraVisible(texto) {
				continue
			}
			inicio := match[0] - 96
			if inicio < 0 {
				inicio = 0
			}
			if envuelto.MatchString(fuente[inicio:match[0]]) {
				continue
			}
			linea := 1 + strings.Count(fuente[:match[0]], "\n")
			t.Errorf("%s:%d: literal visible %q fuera de bt/it", ruta, linea, texto)
		}
	}
}

func TestI18nAyudaQtTieneContratoLocalizadoYFallback(t *testing.T) {
	claves := []string{
		"help.title",
		"help.subtitle",
		"help.sign.title",
		"help.sign.body",
		"help.verify.title",
		"help.verify.body",
		"help.certs.title",
		"help.certs.body",
		"help.web.title",
		"help.web.body",
		"help.security.title",
		"help.security.body",
		"help.more.title",
		"help.more.body",
	}
	for _, idioma := range append([]string{"es"}, idiomasI18n...) {
		catalogo := cargarLocale(t, idioma)
		for _, clave := range claves {
			if strings.TrimSpace(catalogo[clave]) == "" {
				t.Errorf("%s.json: falta contenido para %q", idioma, clave)
			}
		}
	}

	raiz := raizRepositorio(t)
	translator := leerSuperficie(t, filepath.Join(raiz, "cmd", "gui-qml", "translatorbridge.cpp"))
	for _, clave := range claves {
		if !strings.Contains(translator, `QStringLiteral("`+clave+`")`) {
			t.Errorf("translatorbridge.cpp no proyecta la clave de ayuda %q", clave)
		}
	}

	for _, contrato := range []struct {
		archivo  string
		resolver string
		fallback string
	}{
		{"backendbridge.cpp", "resolveLocalizedHelpManual", "openLocalizedHelpFallback"},
		{"ipcbridge.cpp", "resolveLocalizedHelpManualIPC", "openLocalizedHelpFallbackIPC"},
	} {
		fuente := leerSuperficie(t, filepath.Join(raiz, "cmd", "gui-qml", contrato.archivo))
		for _, esperado := range []string{
			"TranslatorBridge::shared()->locale()",
			contrato.resolver,
			contrato.fallback,
		} {
			if !strings.Contains(fuente, esperado) {
				t.Errorf("%s: falta el contrato de ayuda localizada %q", contrato.archivo, esperado)
			}
		}
	}
}
