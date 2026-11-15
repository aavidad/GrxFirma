// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package ipc

import (
	"context"
	"encoding/json"
	"grxfirma/internal/security/excepciones"
)

func (m *Manejador) handleUnsafeExceptions(ctx context.Context, action string, raw json.RawMessage) respuesta {
	if err := ctx.Err(); err != nil {
		return respuesta{OK: false, Action: action, Error: err.Error()}
	}
	if action == "unsafe_exceptions_set" {
		var p struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Enabled == nil {
			return respuesta{OK: false, Action: action, ErrorCode: "invalid_params", Error: m.t("error.formato_invalido")}
		}
		if _, ok := excepciones.Find(p.ID); !ok {
			return respuesta{OK: false, Action: action, ErrorCode: "invalid_params", Error: m.t("error.formato_invalido")}
		}
		if err := excepciones.SetUserAt(m.configDirSeguro(), p.ID, *p.Enabled); err != nil {
			return respuesta{OK: false, Action: action, Error: err.Error()}
		}
	}
	return respuesta{OK: true, Action: action, Data: excepciones.ListAt(m.configDirSeguro())}
}
