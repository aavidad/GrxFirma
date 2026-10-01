// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

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

var pickerI18n struct {
	mu  sync.RWMutex
	loc *localizador.Localizador
}

var pickerFallbacks = map[string]map[string]string{
	"en": {
		"Otras formas de firmar: certificado del sistema o P12/PFX. Cl@ve Firma requiere que la sede ofrezca esa integración. DNIe y tokens dependen del almacén y del dispositivo; aún no están validados aquí.": "Other signing methods: system certificate or P12/PFX. Cl@ve Firma requires integration provided by the portal. DNIe and tokens depend on the certificate store and device; they are not yet validated here.",
		"Usar un archivo P12/PFX…":                              "Use a P12/PFX file…",
		"Actualizar certificados":                               "Refresh certificates",
		"No hay certificados. Puedes abrir un archivo P12/PFX.": "No certificates. You can open a P12/PFX file.",
		"Seleccionar certificado":                               "Select certificate",
		"Recordar esta selección":                               "Remember this selection",
		"Solo esta sesión":                                      "Only this session",
		"Siempre":                                               "Always",
		"CERTIFICADOS":                                          "CERTIFICATES",
		"Válido hasta %s":                                       "Valid until %s",
		"Caducado":                                              "Expired",
		"Activo":                                                "Active",
		"Requiere autorización":                                 "Authorization required",
		"Detalles del certificado":                              "Certificate details",
		"Titular":                                               "Holder",
		"Selecciona un certificado de la lista.":                "Select a certificate from the list.",
		"Emisor":                                                "Issuer",
		"Identificador":                                         "Identifier",
		"Válido hasta":                                          "Valid until",
		"Huella":                                                "Fingerprint",
		"Filtrar por nombre o emisor...":                        "Filter by holder or issuer...",
		"Selecciona un certificado y pulsa “Usar certificado”.": "Select a certificate and click “Use certificate”.",
		"Usar certificado":                   "Use certificate",
		"Cancelar":                           "Cancel",
		"no hay certificados disponibles":    "no certificates available",
		"selección cancelada por el usuario": "selection cancelled by the user",
	},
}

func SetConfigDir(configDir string) {
	loc := resolveLocalizer(configDir)
	pickerI18n.mu.Lock()
	defer pickerI18n.mu.Unlock()
	pickerI18n.loc = loc
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

func tp(id string, args ...any) string {
	pickerI18n.mu.RLock()
	loc := pickerI18n.loc
	pickerI18n.mu.RUnlock()
	if loc == nil {
		loc = localizador.Detectar()
	}
	msg := loc.T(id, args...)
	if msg == id {
		if fallback := pickerFallbackText(loc.Locale(), id); fallback != "" {
			if len(args) == 0 {
				return fallback
			}
			return fmt.Sprintf(fallback, args...)
		}
	}
	return msg
}

func pickerFallbackText(locale, id string) string {
	base := strings.TrimSpace(strings.ToLower(locale))
	if base == "" {
		base = "es"
	}
	if items, ok := pickerFallbacks[base]; ok {
		return items[id]
	}
	return ""
}
