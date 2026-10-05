// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
)

// SetRegion fija el idioma de la interfaz y la zona horaria del dispositivo
// para el sello visible y el informe de verificación. La app la llama al
// arrancar y cuando cambian (Kotlin: idioma de la app y
// TimeZone.getDefault().id). En Android Go no conoce la zona del móvil y
// escribiría la hora en UTC como si fuera local.
//
// Un idioma vacío vuelve al castellano; una zona vacía, a la del proceso.
// Devuelve error si la zona no es un identificador IANA reconocido.
func (f *Facade) SetRegion(language, timeZone string) error {
	if f == nil {
		return newFacadeError("fachada no inicializada")
	}
	if err := validateBoundedText("language", language, 35, true); err != nil {
		return err
	}
	if err := validateBoundedText("time_zone", timeZone, 64, true); err != nil {
		return err
	}
	var zona *time.Location
	if nombre := strings.TrimSpace(timeZone); nombre != "" {
		if zona = localizador.Zona(nombre); zona == nil {
			return newFacadeError("zona horaria no reconocida")
		}
	}
	f.regionMu.Lock()
	defer f.regionMu.Unlock()
	f.language = localizador.Idioma(language)
	f.timeZone = zona
	return nil
}

func (f *Facade) region() (string, *time.Location) {
	f.regionMu.RLock()
	defer f.regionMu.RUnlock()
	return f.language, f.timeZone
}

// withRegionOptions completa las opciones del sello con el idioma y la zona
// de la app, salvo que la petición ya los traiga.
func (f *Facade) withRegionOptions(options map[string]string) map[string]string {
	idioma, zona := f.region()
	out := make(map[string]string, len(options)+2)
	for k, v := range options {
		out[k] = v
	}
	if _, ok := out[desktopsigner.OpcionIdiomaSello]; !ok && idioma != "" {
		out[desktopsigner.OpcionIdiomaSello] = idioma
	}
	if _, ok := out[desktopsigner.OpcionZonaSello]; !ok && zona != nil {
		out[desktopsigner.OpcionZonaSello] = zona.String()
	}
	return out
}
