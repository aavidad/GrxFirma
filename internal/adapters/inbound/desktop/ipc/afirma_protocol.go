// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"

	"grxfirma/internal/ports"
)

const (
	accionProtocoloAfirmaEstado = "afirma_handler_status"
	accionProtocoloAfirmaElegir = "afirma_handler_select"
)

// resultadoProtocoloAfirma dice qué programa abre las firmas de los portales
// (afirma://) y qué opciones se pueden elegir.
type resultadoProtocoloAfirma struct {
	Supported          bool   `json:"supported"`
	Current            string `json:"current"`
	CurrentPath        string `json:"currentPath,omitempty"`
	GrxFirmaInstalled  bool   `json:"grxfirmaInstalled"`
	AutoFirmaInstalled bool   `json:"autofirmaInstalled"`
	AutoFirmaPath      string `json:"autofirmaPath,omitempty"`
	Preference         string `json:"preference,omitempty"`
}

type paramsProtocoloAfirma struct {
	Handler string `json:"handler"`
}

// WithProtocoloAfirma inyecta el selector del programa que atiende afirma://.
func (s *Servidor) WithProtocoloAfirma(p ports.ProtocoloAfirma) *Servidor {
	s.manejador.ProtocoloAfirma = p
	return s
}

func aResultadoProtocoloAfirma(e ports.EstadoProtocoloAfirma) resultadoProtocoloAfirma {
	return resultadoProtocoloAfirma{
		Supported:          e.Soportado,
		Current:            e.Actual,
		CurrentPath:        e.RutaActual,
		GrxFirmaInstalled:  e.GrxFirmaInstalada,
		AutoFirmaInstalled: e.AutoFirmaInstalada,
		AutoFirmaPath:      e.RutaAutoFirma,
		Preference:         e.Preferencia,
	}
}

func (m *Manejador) handleProtocoloAfirmaEstado(ctx context.Context) respuesta {
	if m.ProtocoloAfirma == nil {
		return respuesta{OK: true, Action: accionProtocoloAfirmaEstado, Data: resultadoProtocoloAfirma{
			Supported: false, Current: ports.ProgramaAfirmaNinguno,
		}}
	}
	estado, err := m.ProtocoloAfirma.Estado(ctx)
	if err != nil {
		return m.errorProtocoloAfirma(accionProtocoloAfirmaEstado, err)
	}
	return respuesta{OK: true, Action: accionProtocoloAfirmaEstado, Data: aResultadoProtocoloAfirma(estado)}
}

func (m *Manejador) handleProtocoloAfirmaElegir(ctx context.Context, raw json.RawMessage) respuesta {
	var params paramsProtocoloAfirma
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return m.errorProtocoloAfirma(accionProtocoloAfirmaElegir, ports.ErrProgramaAfirmaDesconocido)
		}
	}
	if m.ProtocoloAfirma == nil {
		return m.errorProtocoloAfirma(accionProtocoloAfirmaElegir, ports.ErrProtocoloAfirmaNoSoportado)
	}
	estado, err := m.ProtocoloAfirma.Elegir(ctx, params.Handler)
	if err != nil {
		return m.errorProtocoloAfirma(accionProtocoloAfirmaElegir, err)
	}
	return respuesta{OK: true, Action: accionProtocoloAfirmaElegir, Data: aResultadoProtocoloAfirma(estado)}
}

// errorProtocoloAfirma traduce el fallo a un código estable y un mensaje
// claro; los detalles técnicos (rutas del registro) no salen del motor.
func (m *Manejador) errorProtocoloAfirma(accion string, err error) respuesta {
	code, key := "afirma_handler_failed", "protocolo.error.generico"
	switch {
	case errors.Is(err, ports.ErrProtocoloAfirmaNoSoportado):
		code, key = "afirma_handler_unsupported", "protocolo.error.no_disponible"
	case errors.Is(err, ports.ErrProtocoloAfirmaAjeno):
		code, key = "afirma_handler_foreign", "protocolo.error.ajeno"
	case errors.Is(err, ports.ErrAutoFirmaNoInstalada):
		code, key = "autofirma_not_installed", "protocolo.autofirma_no_instalada"
	case errors.Is(err, ports.ErrGrxFirmaAfirmaNoInstalada):
		code, key = "grxfirma_afirma_missing", "protocolo.error.grxfirma_no_instalada"
	case errors.Is(err, ports.ErrProgramaAfirmaDesconocido):
		code, key = "afirma_handler_invalid", "protocolo.error.generico"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = "cancelled"
	}
	if accion == accionProtocoloAfirmaEstado && key == "protocolo.error.generico" {
		key = "protocolo.estado.error"
	}
	return respuesta{OK: false, Action: accion, ErrorCode: code, Error: m.t(key)}
}
