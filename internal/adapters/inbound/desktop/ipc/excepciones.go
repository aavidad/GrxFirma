// SPDX-License-Identifier: EUPL-1.2
package ipc

import (
	"context"
	"encoding/json"
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
			if key != "id" && key != "accepted" && key != "acceptanceVersion" && key != "retryId" {
				return respuesta{OK: false, Action: action, Error: m.t("error.formato_invalido")}
			}
		}
		var p struct {
			ID                string `json:"id"`
			Accepted          *bool  `json:"accepted"`
			AcceptanceVersion string `json:"acceptanceVersion"`
			RetryID           string `json:"retryId"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Accepted == nil {
			return respuesta{OK: false, Action: action, Error: m.t("excepciones.aceptacion_requerida")}
		}
		if p.RetryID != "" && !m.retryMatches(ctx, p.RetryID, p.ID) {
			return respuesta{OK: false, Action: action, Error: m.t("excepciones.reintento_caducado")}
		}
		m.settingsMu.Lock()
		receipt, err := excepciones.ActivateLocal(ctx, m.configDirSeguro(), excepciones.ActivationRequest{ID: p.ID, Accepted: *p.Accepted, AcceptanceVersion: p.AcceptanceVersion})
		m.settingsMu.Unlock()
		if err != nil {
			return respuesta{OK: false, Action: action, Error: localUnsafeError(m, err)}
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
			return respuesta{OK: false, Action: action, Error: localUnsafeError(m, err)}
		}
	}
	return respuesta{OK: true, Action: action, Data: excepciones.ListAt(m.configDirSeguro())}
}
func localUnsafeError(m *Manejador, err error) string {
	if excepciones.FromText(err.Error()) != nil {
		return err.Error()
	}
	switch err {
	case excepciones.ErrNonLocal, excepciones.ErrAcceptanceRequired, excepciones.ErrAcceptanceVersion:
		return m.t(err.Error())
	}
	return err.Error()
}
