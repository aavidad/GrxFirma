// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"errors"

	"grxfirma/internal/adapters/outbound/common/settingsfile"
	"grxfirma/internal/application"
)

// Códigos de fallo de ajustes que la persona puede arreglar (lote 39g). Las
// interfaces los reconocen y pintan el texto de su propio catálogo, que lleva
// una orden para copiar; por eso no pasa por el filtro de rutas del texto
// libre del motor.
const (
	ipcErrorSettingsNoPermission = "settings_no_permission"
	ipcErrorSettingsOtherUser    = "settings_other_user"
)

// falloAjustesExplicado convierte un *settingsfile.Error en la respuesta que
// enseñan Qt y WinUI: el texto del catálogo en el idioma del manejador como
// error y como diagnostic.userMessage, en vez del diagnóstico genérico de
// «aplicación local». El detalle técnico es solo la clave: la causa lleva la
// ruta del perfil y no sale de aquí. Devuelve false si err es otro fallo.
func (m *Manejador) falloAjustesExplicado(action string, err error) (respuesta, bool) {
	var ajustes *settingsfile.Error
	if !errors.As(err, &ajustes) {
		return respuesta{}, false
	}
	var code string
	switch ajustes.Key {
	case settingsfile.KeyNoPermission:
		code = ipcErrorSettingsNoPermission
	case settingsfile.KeyOtherUser:
		code = ipcErrorSettingsOtherUser
	default:
		return respuesta{}, false
	}
	message := m.t(ajustes.Key)
	return respuesta{
		OK:        false,
		Action:    action,
		ErrorCode: code,
		Error:     message,
		Diagnostic: &resultadoDiagnosticoGuiado{
			Category:               string(application.GuidedDiagnosticAppLocal),
			FailureCode:            "SETTINGS_ACCESS",
			UserMessage:            message,
			ExpertMessage:          ajustes.Key,
			LikelyOwner:            string(application.GuidedDiagnosticAppLocal),
			UserCanResolveDirectly: true,
		},
	}, true
}
