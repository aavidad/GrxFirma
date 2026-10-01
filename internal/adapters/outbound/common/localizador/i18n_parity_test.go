// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

var (
	idiomasI18n = []string{"ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"}

	// Los catálogos comunes se consumen desde dos runtimes distintos:
	// QString::arg usa %1, %2..., mientras que Localizador.T usa fmt.Sprintf.
	// También conservamos los marcadores con nombre usados por algunas
	// superficies web, aunque no sean argumentos de ninguno de esos runtimes.
	placeholderNombre = regexp.MustCompile(`^%[A-Za-z_][A-Za-z0-9_.-]*%`)
	placeholderQt     = regexp.MustCompile(`^%[1-9][0-9]*`)
	placeholderPrintf = regexp.MustCompile(
		`^%(?:\[[1-9][0-9]*\])?[-+# 0]*(?:\*|[0-9]+)?(?:\.(?:\[[1-9][0-9]*\])?(?:\*|[0-9]+))?(?:\[[1-9][0-9]*\])?[vTtbcdoOqxXUeEfFgGspw]`,
	)
)

type firmaPlaceholders struct {
	qt     map[string]int
	printf map[string]int
	nombre map[string]int
}

func cargarLocale(t *testing.T, idioma string) map[string]string {
	t.Helper()
	ruta := filepath.Join("locales", idioma+".json")
	catalogo, err := cargarCatalogoEstricto(ruta)
	if err != nil {
		t.Fatalf("%s: %v", ruta, err)
	}
	return catalogo
}

func cargarCatalogoEstricto(ruta string) (map[string]string, error) {
	data, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errors.New("el fichero no es UTF-8 válido")
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	inicio, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON inválido: %w", err)
	}
	if delimitador, ok := inicio.(json.Delim); !ok || delimitador != '{' {
		return nil, errors.New("la raíz JSON debe ser un objeto")
	}

	catalogo := make(map[string]string)
	for dec.More() {
		tokenClave, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("clave JSON inválida: %w", err)
		}
		clave, ok := tokenClave.(string)
		if !ok {
			return nil, errors.New("una clave del catálogo no es texto")
		}
		if _, repetida := catalogo[clave]; repetida {
			return nil, fmt.Errorf("clave duplicada %q", clave)
		}

		var valor string
		if err := dec.Decode(&valor); err != nil {
			return nil, fmt.Errorf("valor inválido para %q: %w", clave, err)
		}
		catalogo[clave] = valor
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("objeto JSON sin cerrar: %w", err)
	}
	if err := comprobarFinJSON(dec); err != nil {
		return nil, err
	}
	return catalogo, nil
}

func comprobarFinJSON(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("contenido posterior al objeto JSON: %w", err)
	}
	return errors.New("el fichero contiene más de un valor JSON")
}

func extraerFirmaPlaceholders(valor string) firmaPlaceholders {
	firma := firmaPlaceholders{
		qt:     make(map[string]int),
		printf: make(map[string]int),
		nombre: make(map[string]int),
	}

	for pos := 0; pos < len(valor); {
		relativa := strings.IndexByte(valor[pos:], '%')
		if relativa < 0 {
			break
		}
		pos += relativa
		resto := valor[pos:]

		if strings.HasPrefix(resto, "%%") {
			pos += 2
			continue
		}
		if token := placeholderNombre.FindString(resto); token != "" {
			firma.nombre[token]++
			pos += len(token)
			continue
		}
		if token := placeholderPrintf.FindString(resto); token != "" {
			// Índices, flags y anchuras constantes no cambian el tipo del
			// argumento. Los asteriscos sí consumen argumentos adicionales.
			normalizado := fmt.Sprintf("%d*%%%c", strings.Count(token, "*"), token[len(token)-1])
			firma.printf[normalizado]++
			pos += len(token)
			continue
		}
		if token := placeholderQt.FindString(resto); token != "" {
			firma.qt[token]++
			pos += len(token)
			continue
		}
		pos++
	}
	return firma
}

func (f firmaPlaceholders) String() string {
	return fmt.Sprintf("Qt=%s, printf=%s, nombre=%s",
		formatearConteos(f.qt), formatearConteos(f.printf), formatearConteos(f.nombre))
}

func formatearConteos(conteos map[string]int) string {
	claves := make([]string, 0, len(conteos))
	for clave := range conteos {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	partes := make([]string, 0, len(claves))
	for _, clave := range claves {
		partes = append(partes, fmt.Sprintf("%s×%d", clave, conteos[clave]))
	}
	return "[" + strings.Join(partes, ", ") + "]"
}

func firmasIguales(a, b firmaPlaceholders) bool {
	return mapasIguales(a.qt, b.qt) &&
		mapasIguales(a.printf, b.printf) &&
		mapasIguales(a.nombre, b.nombre)
}

func mapasIguales(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for clave, cantidad := range a {
		if b[clave] != cantidad {
			return false
		}
	}
	return true
}

// TestI18nCatalogosComunes es el gate reproducible de T093. Un único test
// impide que cualquiera de los 11 catálogos deje de ser JSON/UTF-8 estricto,
// se desincronice de es.json, publique valores vacíos o altere los argumentos
// que aplican QString::arg, fmt.Sprintf y las superficies con marcadores.
func TestI18nCatalogosComunes(t *testing.T) {
	esperados := make(map[string]struct{}, len(idiomasI18n))
	for _, idioma := range idiomasI18n {
		esperados[idioma+".json"] = struct{}{}
	}

	entradas, err := filepath.Glob(filepath.Join("locales", "*.json"))
	if err != nil {
		t.Fatalf("Glob(locales): %v", err)
	}
	encontrados := make(map[string]struct{}, len(entradas))
	for _, ruta := range entradas {
		encontrados[filepath.Base(ruta)] = struct{}{}
	}
	if !mismosConjuntos(esperados, encontrados) {
		t.Fatalf("catálogos distintos de los 11 soportados: esperados=%v encontrados=%v",
			clavesOrdenadas(esperados), clavesOrdenadas(encontrados))
	}

	catalogos := make(map[string]map[string]string, len(idiomasI18n))
	for _, idioma := range idiomasI18n {
		ruta := filepath.Join("locales", idioma+".json")
		catalogo, err := cargarCatalogoEstricto(ruta)
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		if len(catalogo) == 0 {
			t.Fatalf("%s está vacío", ruta)
		}
		for clave, valor := range catalogo {
			if strings.TrimSpace(clave) == "" {
				t.Errorf("%s: contiene una clave vacía o en blanco", ruta)
			}
			if contieneMojibakeUTF8(clave) {
				t.Errorf("%s: contiene una clave con mojibake UTF-8: %q", ruta, clave)
			}
			if strings.TrimSpace(valor) == "" {
				t.Errorf("%s: valor vacío o en blanco para %q", ruta, clave)
			}
			if contieneMojibakeUTF8(valor) {
				t.Errorf("%s: contiene un valor con mojibake UTF-8 para %q: %q", ruta, clave, valor)
			}
		}
		catalogos[idioma] = catalogo
	}

	referencia := catalogos["es"]
	for _, idioma := range idiomasI18n {
		if idioma == "es" {
			continue
		}
		t.Run(idioma, func(t *testing.T) {
			catalogo := catalogos[idioma]
			faltan, sobran := diferenciasClaves(referencia, catalogo)
			if len(faltan) > 0 || len(sobran) > 0 {
				t.Fatalf("%s.json no tiene paridad con es.json: faltan=%v sobran=%v",
					idioma, faltan, sobran)
			}
			for clave, valorReferencia := range referencia {
				firmaReferencia := extraerFirmaPlaceholders(valorReferencia)
				firmaTraducida := extraerFirmaPlaceholders(catalogo[clave])
				if !firmasIguales(firmaReferencia, firmaTraducida) {
					t.Errorf("%s.json: placeholders incompatibles para %q: es={%s} %s={%s}",
						idioma, clave, firmaReferencia, idioma, firmaTraducida)
				}
			}
		})
	}
}

func TestDetectorMojibakeUTF8(t *testing.T) {
	t.Parallel()

	for _, texto := range []string{
		"AtenciÃ³n",
		"Elegir imagenâ\u0080¦",
		"comillas â€™incorrectasâ€™",
		"emoji ðŸ”‘",
		"carácter de reemplazo �",
	} {
		if !contieneMojibakeUTF8(texto) {
			t.Errorf("no detectó mojibake en %q", texto)
		}
	}
	for _, texto := range []string{
		"CONFIGURAÇÃO",
		"Âmbito de resolução",
		"instância local",
		"français",
		"中文",
	} {
		if contieneMojibakeUTF8(texto) {
			t.Errorf("marcó texto válido como mojibake: %q", texto)
		}
	}
}

func contieneMojibakeUTF8(texto string) bool {
	runas := []rune(texto)
	for indice, runa := range runas {
		if runa == '\uFFFD' || (runa >= '\u0080' && runa <= '\u009F') {
			return true
		}
		switch runa {
		case 'Ã', 'Â':
			if indice+1 < len(runas) && esContinuacionMojibake(runas[indice+1]) {
				return true
			}
		case 'â', 'ï':
			if indice+2 < len(runas) &&
				esContinuacionMojibake(runas[indice+1]) &&
				esContinuacionMojibake(runas[indice+2]) {
				return true
			}
		case 'ð':
			if indice+3 < len(runas) &&
				esContinuacionMojibake(runas[indice+1]) &&
				esContinuacionMojibake(runas[indice+2]) &&
				esContinuacionMojibake(runas[indice+3]) {
				return true
			}
		}
	}
	return false
}

func esContinuacionMojibake(runa rune) bool {
	if runa >= '\u0080' && runa <= '\u00BF' {
		return true
	}
	switch runa {
	case '€', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ',
		'Ž', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›',
		'œ', 'ž', 'Ÿ':
		return true
	default:
		return false
	}
}

func TestExtraerFirmaPlaceholders(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		valor  string
		qt     map[string]int
		printf map[string]int
		named  map[string]int
	}{
		{
			nombre: "Qt conserva posición y repetición",
			valor:  "PDF %1/%2: %1",
			qt:     map[string]int{"%1": 2, "%2": 1},
		},
		{
			nombre: "printf conserva tipo y argumentos de anchura",
			valor:  "%[2]*.[1]*[3]f / %02d / %1s / %s",
			printf: map[string]int{"2*%f": 1, "0*%d": 1, "0*%s": 2},
		},
		{
			nombre: "porcentaje escapado no es argumento",
			valor:  "100%% completado",
		},
		{
			nombre: "marcadores con nombre no se confunden con printf",
			valor:  "Mostrando %visible% de %total%",
			named:  map[string]int{"%visible%": 1, "%total%": 1},
		},
	}

	for _, caso := range casos {
		caso := caso
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			esperada := firmaPlaceholders{
				qt:     mapaNoNil(caso.qt),
				printf: mapaNoNil(caso.printf),
				nombre: mapaNoNil(caso.named),
			}
			obtenida := extraerFirmaPlaceholders(caso.valor)
			if !firmasIguales(esperada, obtenida) {
				t.Fatalf("firma de %q = {%s}; esperada {%s}", caso.valor, obtenida, esperada)
			}
		})
	}
}

func TestFirmasPlaceholdersDetectanIncompatibilidades(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre     string
		referencia string
		traduccion string
	}{
		{
			nombre:     "falta argumento Qt",
			referencia: "PDF %1/%2",
			traduccion: "PDF %1",
		},
		{
			nombre:     "cambia tipo printf",
			referencia: "Código %d",
			traduccion: "Code %s",
		},
		{
			nombre:     "pierde marcador con nombre",
			referencia: "%visible% de %total%",
			traduccion: "%visible%",
		},
	}

	for _, caso := range casos {
		caso := caso
		t.Run(caso.nombre, func(t *testing.T) {
			t.Parallel()
			if firmasIguales(
				extraerFirmaPlaceholders(caso.referencia),
				extraerFirmaPlaceholders(caso.traduccion),
			) {
				t.Fatalf("no se detectó la incompatibilidad entre %q y %q",
					caso.referencia, caso.traduccion)
			}
		})
	}
}

func TestOpacidadLogoTraducidaEnTodosLosIdiomas(t *testing.T) {
	t.Parallel()
	const clave = "Opacidad del sello"
	for _, idioma := range idiomasI18n {
		idioma := idioma
		t.Run(idioma, func(t *testing.T) {
			t.Parallel()
			catalogo := cargarLocale(t, idioma)
			if strings.TrimSpace(catalogo[clave]) == "" {
				t.Fatalf("falta la traducción de %q", clave)
			}
			if catalogo["sign.seal.logo_opacity"] != catalogo[clave] {
				t.Fatal("el rótulo de WinUI y Qt debe coincidir")
			}
			if strings.TrimSpace(catalogo["error.opacidad_logo_sello"]) == "" {
				t.Fatal("falta el error localizado de opacidad")
			}
			if strings.TrimSpace(catalogo["sign.seal.opacity_help"]) == "" {
				t.Fatal("falta la ayuda localizada de opacidad")
			}
		})
	}
}

func mapaNoNil(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

func diferenciasClaves(referencia, catalogo map[string]string) (faltan, sobran []string) {
	for clave := range referencia {
		if _, ok := catalogo[clave]; !ok {
			faltan = append(faltan, clave)
		}
	}
	for clave := range catalogo {
		if _, ok := referencia[clave]; !ok {
			sobran = append(sobran, clave)
		}
	}
	sort.Strings(faltan)
	sort.Strings(sobran)
	return faltan, sobran
}

func mismosConjuntos(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for elemento := range a {
		if _, ok := b[elemento]; !ok {
			return false
		}
	}
	return true
}

func clavesOrdenadas(m map[string]struct{}) []string {
	claves := make([]string, 0, len(m))
	for clave := range m {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	return claves
}
