// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package localizador implementa el puerto ports.Localizador mediante
// ficheros JSON embebidos. No importa librerias externas de i18n: usa
// solo encoding/json y embed de la stdlib.
//
// Uso:
//
//	loc := localizador.Detectar()    // auto-detecta LANG/LANGUAGE del entorno
//	loc.T("error.archivo_no_encontrado", "/ruta/al/fichero")
package localizador

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

// Localizador implementa ports.Localizador con un mapa de cadenas embebido.
type Localizador struct {
	mensajes map[string]string
	locale   string
}

// Detectar construye un Localizador detectando el locale del entorno
// (LANGUAGE → LC_ALL → LC_MESSAGES → LANG). Cae a "es" si no se puede
// determinar.
func Detectar() *Localizador {
	return Para(detectarLocale())
}

// Para construye un Localizador para un locale concreto.
// Soporta variantes de idioma comunes y cae a "es" cuando no encuentra
// el paquete solicitado.
func Para(locale string) *Localizador {
	base := localeBase(locale)
	cargar := func(locale string) map[string]string {
		datos, err := localesFS.ReadFile("locales/" + locale + ".json")
		if err != nil {
			return map[string]string{}
		}
		var mensajes map[string]string
		if err := json.Unmarshal(datos, &mensajes); err != nil {
			return map[string]string{}
		}
		return mensajes
	}

	mensajes := cargar("es")
	if base != "es" {
		for k, v := range cargar(base) {
			mensajes[k] = v
		}
	}
	return &Localizador{mensajes: mensajes, locale: base}
}

// T devuelve la cadena localizada para id, aplicando args con fmt.Sprintf
// si se proporcionan. Si la clave no existe devuelve el propio id.
func (l *Localizador) T(id string, args ...any) string {
	msg, ok := l.mensajes[id]
	if !ok {
		return id
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// Locale devuelve el locale detectado.
func (l *Localizador) Locale() string { return l.locale }

func detectarLocale() string {
	for _, env := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			// Normalizar: "es_ES.UTF-8" → "es"
			v = strings.SplitN(v, "_", 2)[0]
			v = strings.SplitN(v, ".", 2)[0]
			return v
		}
	}
	return "es"
}

func localeBase(locale string) string {
	v := strings.TrimSpace(strings.ToLower(locale))
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.SplitN(v, ".", 2)[0]
	switch {
	case strings.HasPrefix(v, "en"):
		return "en"
	case strings.HasPrefix(v, "fr"):
		return "fr"
	case strings.HasPrefix(v, "de"):
		return "de"
	case strings.HasPrefix(v, "it"):
		return "it"
	case strings.HasPrefix(v, "pt"):
		return "pt"
	case strings.HasPrefix(v, "zh"):
		return "zh"
	case strings.HasPrefix(v, "gl"):
		return "gl"
	case strings.HasPrefix(v, "eu"):
		return "eu"
	case strings.HasPrefix(v, "ca-valencia"), strings.HasPrefix(v, "val"), strings.HasPrefix(v, "va"):
		return "va"
	case strings.HasPrefix(v, "ca"):
		return "ca"
	default:
		return "es"
	}
}
