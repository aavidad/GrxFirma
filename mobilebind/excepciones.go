// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package mobilebind

import (
	"encoding/json"
	"grxfirma/internal/security/excepciones"
)

// UnsafeExceptionsJSON expone el mismo catálogo que el IPC de escritorio.
// La fachada vincula la consulta al directorio privado entregado por Android.
func (f *Facade) UnsafeExceptionsJSON() (string, error) {
	if f == nil || f.configDir == "" {
		return "", newFacadeError("error.settings_no_configurado")
	}
	data, err := json.Marshal(excepciones.ListAt(f.configDir))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SetUnsafeExceptionJSON guarda una elección expresa sin aceptar ajustes
// desconocidos ni permitir sobrescribir una directiva de máquina.
func (f *Facade) SetUnsafeExceptionJSON(request string) (string, error) {
	if f == nil || f.configDir == "" {
		return "", newFacadeError("error.settings_no_configurado")
	}
	if len(request) > 4096 {
		return "", newFacadeError("error.formato_invalido")
	}
	var p struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}
	if json.Unmarshal([]byte(request), &p) != nil || p.Enabled == nil {
		return "", newFacadeError("error.formato_invalido")
	}
	if _, ok := excepciones.Find(p.ID); !ok {
		return "", newFacadeError("error.formato_invalido")
	}
	if err := excepciones.SetUserAt(f.configDir, p.ID, *p.Enabled); err != nil {
		return "", err
	}
	return f.UnsafeExceptionsJSON()
}
