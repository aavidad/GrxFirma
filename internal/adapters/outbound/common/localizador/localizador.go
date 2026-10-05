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
	"regexp"
	"strings"
	"time"
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

// EtiquetaHTML devuelve la etiqueta BCP 47 del idioma para el atributo lang
// de un documento (el valenciano es una variante registrada del catalán).
func (l *Localizador) EtiquetaHTML() string {
	if l.locale == "va" {
		return "ca-ES-valencia"
	}
	return l.locale
}

// Idioma normaliza un idioma pedido a uno de los catálogos disponibles.
// Devuelve "" si la petición está vacía, para que quien llama aplique su
// propio valor por defecto.
func Idioma(pedido string) string {
	if strings.TrimSpace(pedido) == "" {
		return ""
	}
	return localeBase(pedido)
}

var nombreZonaIANA = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+\-]*(/[A-Za-z0-9_+\-]+){0,2}$`)

// Zona carga una zona horaria IANA («Europe/Madrid»). Rechaza nombres con
// forma de ruta y devuelve nil si el nombre está vacío o no se reconoce, de
// modo que quien llama siga con la zona local del sistema.
func Zona(nombre string) *time.Location {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" || len(nombre) > 64 || !nombreZonaIANA.MatchString(nombre) {
		return nil
	}
	zona, err := time.LoadLocation(nombre)
	if err != nil {
		return nil
	}
	return zona
}

// FechaHora escribe un instante en la zona indicada (la local si es nil) con
// el formato de fecha del catálogo y la zona entre paréntesis: la abreviatura
// cuando la hay («08:25 (CEST)») y, si no, el desfase («08:25 (UTC+03:00)»).
// Un documento firmado se lee en otros husos: la hora sin zona sería ambigua.
func (l *Localizador) FechaHora(t time.Time, zona *time.Location, segundos bool) string {
	if zona == nil {
		zona = time.Local
	}
	t = t.In(zona)
	clave, porDefecto := "format.datetime_minutes", "02/01/2006 15:04"
	if segundos {
		clave, porDefecto = "format.datetime_seconds", "02/01/2006 15:04:05"
	}
	formato := l.T(clave)
	if formato == clave {
		formato = porDefecto
	}
	return t.Format(formato) + " (" + nombreZona(t) + ")"
}

func nombreZona(t time.Time) string {
	abreviatura, desfase := t.Zone()
	if desfase == 0 && (abreviatura == "" || abreviatura == "UTC" || abreviatura == "GMT") {
		return "UTC"
	}
	if abreviatura != "" && abreviatura[0] != '+' && abreviatura[0] != '-' {
		return abreviatura
	}
	return "UTC" + t.Format("-07:00")
}

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
	case strings.HasPrefix(v, "ca") && strings.Contains(v, "valencia"), strings.HasPrefix(v, "val"), strings.HasPrefix(v, "va"):
		return "va"
	case strings.HasPrefix(v, "ca"):
		return "ca"
	default:
		return "es"
	}
}
