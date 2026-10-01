// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
)

var trustI18n struct {
	mu  sync.RWMutex
	loc *localizador.Localizador
}

var trustFallbacks = map[string]map[string]string{
	"en": {
		"Portal u origen desconocido": "Unknown portal or origin",
		"Portal/origen:":              "Portal/origin:",
		"Este portal quiere iniciar una operación de firma electrónica con GrxFirma, pero todavía no está marcado como origen confiable.":                               "This portal wants to start an electronic signing operation with GrxFirma, but it is not yet marked as a trusted origin.",
		"Si continúas, este sitio podrá pedir operaciones de firma o acceso al flujo local en esta sesión.":                                                             "If you continue, this site will be able to request signing operations or access the local flow in this session.",
		"Si GrxFirma está residente, aceptar sin revisar el origen reduce la barrera entre el navegador y el agente local. Verifica bien el portal antes de continuar.": "If GrxFirma is resident, accepting without reviewing the origin lowers the barrier between the browser and the local agent. Check the portal carefully before continuing.",
		"¿Qué quieres hacer con este portal?": "What do you want to do with this portal?",
		"Qué está intentando hacer":           "What it is trying to do",
		"Riesgo si continúas":                 "Risk if you continue",
		"Confiar siempre":                     "Always trust",
		"Confiar esta vez":                    "Trust this time",
		"Rechazar":                            "Reject",
		"Autorización de firma":               "Signing authorisation",
		"Cerrar":                              "Close",
	},
}

func SetConfigDir(configDir string) {
	loc := resolveLocalizer(configDir)
	trustI18n.mu.Lock()
	defer trustI18n.mu.Unlock()
	trustI18n.loc = loc
}

func resolveLocalizer(configDir string) *localizador.Localizador {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "grxfirma")
	}
	doc, err := usersettings.CargarDocumentoCompat(context.Background(), configDir)
	if err == nil && doc.General.Idioma != nil && strings.TrimSpace(*doc.General.Idioma) != "" {
		return localizador.Para(*doc.General.Idioma)
	}
	return localizador.Detectar()
}

func tt(id string, args ...any) string {
	trustI18n.mu.RLock()
	loc := trustI18n.loc
	trustI18n.mu.RUnlock()
	if loc == nil {
		loc = localizador.Detectar()
	}
	msg := loc.T(id, args...)
	if msg == id {
		if fallback := trustFallbackText(loc.Locale(), id); fallback != "" {
			if len(args) == 0 {
				return fallback
			}
			return fmt.Sprintf(fallback, args...)
		}
	}
	return msg
}

func trustFallbackText(locale, id string) string {
	base := strings.TrimSpace(strings.ToLower(locale))
	if base == "" {
		base = "es"
	}
	if items, ok := trustFallbacks[base]; ok {
		return items[id]
	}
	return ""
}
