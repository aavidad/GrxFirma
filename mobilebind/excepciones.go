// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/json"
	"errors"
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
		return "", errorTipado(err)
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
		return "", errorTipado(err)
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

// bloqueoTipado lleva a la app el contrato de un bloqueo con el mismo JSON que
// el IPC de escritorio (exceptionId, activable, blockKey, settingKey...). La app
// lo lee del mensaje de la excepción de gomobile (lote 33) y pinta el bloqueo
// con su propio catálogo. Solo se crea desde un *excepciones.Block que emitió
// el núcleo; nunca desde el texto de un documento o de otro error.
type bloqueoTipado struct{ block *excepciones.Block }

func (e *bloqueoTipado) Error() string {
	b := e.block
	data, err := json.Marshal(struct {
		ErrorCode         string `json:"errorCode"`
		ExceptionID       string `json:"exceptionId"`
		Activable         bool   `json:"activable"`
		BlockKey          string `json:"blockKey"`
		SettingKey        string `json:"settingKey"`
		TitleKey          string `json:"titleKey"`
		RiskKey           string `json:"riskKey"`
		AcceptanceKey     string `json:"acceptanceKey"`
		HelpKey           string `json:"helpKey"`
		OrganizationKey   string `json:"organizationKey"`
		AcceptanceVersion string `json:"acceptanceVersion"`
		ConfigUnavailable bool   `json:"configUnavailable,omitempty"`
	}{"unsafe_exception_blocked", b.ID, b.Activable, b.BlockKey, b.SettingKey, b.TitleKey, b.RiskKey,
		b.AcceptanceKey, b.HelpKey, b.OrganizationKey, b.AcceptanceVersion, b.ConfigUnavailable})
	if err != nil {
		return b.BlockKey
	}
	return string(data)
}

// Unwrap conserva errors.As(err, *excepciones.Block) para el resto del núcleo.
func (e *bloqueoTipado) Unwrap() error { return e.block }

// errorTipado sustituye un error que contiene un bloqueo por su contrato JSON
// y deja los demás errores como estaban.
func errorTipado(err error) error {
	if err == nil {
		return nil
	}
	var typed *bloqueoTipado
	if errors.As(err, &typed) {
		return typed
	}
	var block *excepciones.Block
	if !errors.As(err, &block) || block == nil {
		return err
	}
	return &bloqueoTipado{block: block}
}
