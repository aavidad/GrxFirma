// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"grxfirma/internal/security/excepciones"
)

func unsafeLocalRequired(action string) respuesta {
	return respuesta{OK: false, Action: action, ErrorCode: "local_interface_required", Error: "excepciones.canal_no_local"}
}
func (m *Manejador) handleUnsafeExceptions(ctx context.Context, action string, raw json.RawMessage) respuesta {
	if !esInterfazVerificada(ctx) {
		r := unsafeLocalRequired(action)
		r.Error = m.t(r.Error)
		return r
	}
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	ctx = excepciones.ForLocalDesktop(ctx)
	if action == "unsafe_exceptions_activate" {
		obj, err := strictTokenSettingsObject(raw)
		if err != nil {
			return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
		}
		for key := range obj {
			if key != "id" && key != "accepted" && key != "acceptanceVersion" && key != "retryId" && key != "origin" {
				return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
			}
		}
		var p struct {
			ID                string `json:"id"`
			Accepted          *bool  `json:"accepted"`
			AcceptanceVersion string `json:"acceptanceVersion"`
			RetryID           string `json:"retryId"`
			Origin            string `json:"origin"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Accepted == nil {
			return respuesta{OK: false, Action: action, Error: m.t("excepciones.aceptacion_requerida")}
		}
		// La interfaz dice dónde aceptó la persona: en el aviso de un bloqueo
		// o en el apartado Seguridad. Un reintento solo nace de un bloqueo.
		if !excepciones.ValidInterfaceOrigin(p.Origin) || (p.RetryID != "" && p.Origin != excepciones.OriginWarning) {
			return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
		}
		if p.RetryID != "" && !m.retryMatches(ctx, p.RetryID, p.ID) {
			return respuesta{OK: false, Action: action, Error: m.t("excepciones.reintento_caducado")}
		}
		m.settingsMu.Lock()
		receipt, err := excepciones.ActivateLocal(ctx, m.configDirSeguro(), excepciones.ActivationRequest{ID: p.ID, Accepted: *p.Accepted, AcceptanceVersion: p.AcceptanceVersion, Origin: p.Origin})
		m.settingsMu.Unlock()
		if err != nil {
			return respuesta{OK: false, Action: action, Error: localUnsafeError(m, err), cause: err}
		}
		if p.RetryID != "" {
			m.activateUnsafeRetry(ctx, p.RetryID)
		}
		return respuesta{OK: true, Action: action, RetryID: p.RetryID, Data: receipt}
	}
	if action == "unsafe_exceptions_set" {
		var p struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		}
		obj, err := strictTokenSettingsObject(raw)
		if err != nil {
			return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
		}
		for key := range obj {
			if key != "id" && key != "enabled" {
				return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
			}
		}
		if json.Unmarshal(raw, &p) != nil || p.Enabled == nil {
			return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
		}
		if *p.Enabled {
			return respuesta{OK: false, Action: action, ErrorCode: "unsafe_acceptance_required", Error: m.t("excepciones.aceptacion_requerida")}
		}
		m.settingsMu.Lock()
		err = excepciones.SetUserAt(m.configDirSeguro(), p.ID, false)
		m.settingsMu.Unlock()
		if err != nil {
			return respuesta{OK: false, Action: action, Error: localUnsafeError(m, err), cause: err}
		}
	}
	return respuesta{OK: true, Action: action, Data: excepciones.ListAt(m.configDirSeguro())}
}
func localUnsafeError(m *Manejador, err error) string {
	var block *excepciones.Block
	if errors.As(err, &block) {
		return err.Error()
	}
	switch err {
	case excepciones.ErrNonLocal, excepciones.ErrAcceptanceRequired, excepciones.ErrAcceptanceVersion, excepciones.ErrAcceptanceOrigin:
		return m.t(err.Error())
	}
	return err.Error()
}
