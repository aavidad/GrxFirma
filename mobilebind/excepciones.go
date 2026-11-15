// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package mobilebind

import (
	"encoding/json"
	"io"
	"strings"

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

// SetUnsafeExceptionJSON conserva la desactivación local de un ajuste. La
// activación requiere la aceptación versionada del método explícito.
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
	if decodeUnsafeExceptionRequest(request, &p) != nil || p.Enabled == nil {
		return "", newFacadeError("error.formato_invalido")
	}
	if _, ok := excepciones.Find(p.ID); !ok {
		return "", newFacadeError("error.formato_invalido")
	}
	if *p.Enabled {
		return "", excepciones.ErrAcceptanceRequired
	}
	if err := excepciones.SetUserAt(f.configDir, p.ID, false); err != nil {
		return "", err
	}
	return f.UnsafeExceptionsJSON()
}

// ActivateUnsafeExceptionJSON solo la app Android puede llamar a este punto
// local. La solicitud no contiene ni elige el origen de la autorización.
func (f *Facade) ActivateUnsafeExceptionJSON(request string) (string, error) {
	if f == nil || f.configDir == "" || f.platform != "android" {
		return "", newFacadeError("error.settings_no_configurado")
	}
	if len(request) > 4096 {
		return "", newFacadeError("error.formato_invalido")
	}
	var p struct {
		ID                string `json:"id"`
		Accepted          *bool  `json:"accepted"`
		AcceptanceVersion string `json:"acceptanceVersion"`
	}
	if decodeUnsafeExceptionRequest(request, &p) != nil {
		return "", newFacadeError("error.formato_invalido")
	}
	if p.Accepted == nil || !*p.Accepted {
		return "", excepciones.ErrAcceptanceRequired
	}
	if _, ok := excepciones.Find(p.ID); !ok {
		return "", newFacadeError("error.formato_invalido")
	}
	ctx := excepciones.ForLocalAndroid(f.operationContext())
	accepted, err := excepciones.ActivateLocal(ctx, f.configDir, excepciones.ActivationRequest{
		ID: p.ID, Accepted: *p.Accepted, AcceptanceVersion: p.AcceptanceVersion,
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(accepted)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnsafeExceptionBlockJSON da a la app las claves y el alcance de un bloqueo.
// La solicitud original se vuelve a enviar después de activar el ajuste.
func (f *Facade) UnsafeExceptionBlockJSON(id string) (string, error) {
	if f == nil || f.configDir == "" {
		return "", newFacadeError("error.settings_no_configurado")
	}
	if _, ok := excepciones.Find(id); !ok {
		return "", newFacadeError("error.formato_invalido")
	}
	block := excepciones.DescribeAt(f.configDir, id)
	if block == nil {
		return "", newFacadeError("error.formato_invalido")
	}
	data, err := json.Marshal(struct {
		*excepciones.Block
		RetryRequiresResubmit bool `json:"retry_requires_resubmit"`
	}{Block: block, RetryRequiresResubmit: true})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeUnsafeExceptionRequest(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return newFacadeError("error.formato_invalido")
	}
	return nil
}
