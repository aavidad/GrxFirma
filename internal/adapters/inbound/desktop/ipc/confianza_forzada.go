// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Confianza que el usuario fuerza al verificar (lote 14). Solo la atiende el
// IPC del escritorio, que ya solo responde al frontend que lanzó el backend.
// Para aceptar, el obstáculo tiene que haber salido en una verificación de
// esta sesión: el frontend no puede inventar huellas.

// ConfianzaForzadaUseCase gestiona lo aceptado por el usuario.
type ConfianzaForzadaUseCase interface {
	Listar(ctx context.Context) (domain.ConfianzaForzada, error)
	Aceptar(ctx context.Context, obstaculo domain.ObstaculoConfianza) (domain.AceptacionConfianza, error)
	Quitar(ctx context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error)
}

// contextoConfianzaForzada adjunta al contexto lo que el usuario aceptó en
// su equipo, para comprobaciones locales que no pasan por la verificación de
// firmas (Veri*Factu). Si no se puede leer, cuenta como prohibido.
func (m *Manejador) contextoConfianzaForzada(ctx context.Context) (context.Context, *domain.RegistroConfianzaForzada) {
	if m.ConfianzaForzada == nil {
		return ctx, nil
	}
	confianza, err := m.ConfianzaForzada.Listar(ctx)
	if err != nil {
		confianza = domain.ConfianzaForzada{Prohibida: true}
	}
	registro := domain.NuevoRegistroConfianzaForzada(confianza)
	return domain.ConRegistroConfianzaForzada(ctx, registro), registro
}

// auditarConfianzaForzadaAplicada registra en la auditoría lo aceptado que se
// aplicó, si el caso de uso lo admite.
func (m *Manejador) auditarConfianzaForzadaAplicada(ctx context.Context, operacion string, registro *domain.RegistroConfianzaForzada) {
	auditor, ok := m.ConfianzaForzada.(interface {
		RegistrarAplicadas(context.Context, string, []domain.ObstaculoConfianza)
	})
	if ok && registro != nil {
		auditor.RegistrarAplicadas(ctx, operacion, registro.Aplicadas())
	}
}

// maxConfianzaForzableSesion limita los obstáculos recordados por sesión.
const maxConfianzaForzableSesion = 64

// WithConfianzaForzada habilita las acciones forced_trust_*.
func (s *Servidor) WithConfianzaForzada(gestion ConfianzaForzadaUseCase) *Servidor {
	s.manejador.ConfianzaForzada = gestion
	return s
}

type obstaculoConfianzaJSON struct {
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Subject     string `json:"subject,omitempty"`
	// Label y Notice son claves de catálogo: el frontend las traduce.
	Label  string `json:"label"`
	Notice string `json:"notice"`
}

type resultadoConfianzaForzadaVerificacion struct {
	Available  bool                     `json:"available"`
	Forbidden  bool                     `json:"forbidden"`
	Applied    []obstaculoConfianzaJSON `json:"applied"`
	Acceptable []obstaculoConfianzaJSON `json:"acceptable"`
}

type entradaConfianzaForzadaJSON struct {
	obstaculoConfianzaJSON
	AcceptedAt string `json:"acceptedAt"`
}

type resultadoListaConfianzaForzada struct {
	Forbidden bool                          `json:"forbidden"`
	Entries   []entradaConfianzaForzadaJSON `json:"entries"`
}

func (m *Manejador) obstaculoJSON(o domain.ObstaculoConfianza) obstaculoConfianzaJSON {
	etiqueta := "verify.forced_trust.label." + string(o.Tipo)
	return obstaculoConfianzaJSON{
		Type:        string(o.Tipo),
		Fingerprint: o.Huella,
		Subject:     o.Sujeto,
		Label:       etiqueta,
		Notice:      o.Tipo.ClaveAviso(),
	}
}

func (m *Manejador) obstaculosJSON(lista []domain.ObstaculoConfianza) []obstaculoConfianzaJSON {
	out := make([]obstaculoConfianzaJSON, 0, len(lista))
	for _, o := range lista {
		out = append(out, m.obstaculoJSON(o))
	}
	return out
}

// textoConfianzaForzada traduce los avisos de confianza forzada, que el
// verificador deja como claves de catálogo; el resto pasa tal cual.
func (m *Manejador) textoConfianzaForzada(texto string) string {
	if strings.HasPrefix(texto, "verify.forced_trust.") {
		return m.t(texto)
	}
	return texto
}

func (m *Manejador) textosConfianzaForzada(lista []string) []string {
	out := make([]string, 0, len(lista))
	for _, texto := range lista {
		out = append(out, m.textoConfianzaForzada(texto))
	}
	return out
}

func claveObstaculoIPC(tipo domain.TipoConfianzaForzada, huella string) string {
	return string(tipo) + "|" + strings.ToLower(strings.TrimSpace(huella))
}

// confianzaForzadaVerificacion prepara la parte de la respuesta de verify y
// recuerda los obstáculos que el usuario podrá aceptar.
func (m *Manejador) confianzaForzadaVerificacion(r application.ResultadoConfianzaForzada) *resultadoConfianzaForzadaVerificacion {
	if m.ConfianzaForzada == nil {
		return nil
	}
	m.confianzaMu.Lock()
	if m.confianzaForzable == nil {
		m.confianzaForzable = map[string]domain.ObstaculoConfianza{}
	}
	for _, o := range r.Forzable {
		if len(m.confianzaForzable) >= maxConfianzaForzableSesion {
			break
		}
		m.confianzaForzable[claveObstaculoIPC(o.Tipo, o.Huella)] = o
	}
	m.confianzaMu.Unlock()
	return &resultadoConfianzaForzadaVerificacion{
		Available:  !r.Prohibida && len(r.Forzable) > 0,
		Forbidden:  r.Prohibida,
		Applied:    m.obstaculosJSON(r.Aplicada),
		Acceptable: m.obstaculosJSON(r.Forzable),
	}
}

func (m *Manejador) errorConfianzaForzada(accion string, err error) respuesta {
	mensaje := err.Error()
	if strings.HasPrefix(mensaje, "verify.forced_trust.") {
		mensaje = m.t(mensaje)
	}
	codigo := ""
	if errors.Is(err, domain.ErrConfianzaProhibida) {
		codigo = "forced_trust_forbidden"
	}
	return respuesta{OK: false, Action: accion, ErrorCode: codigo, Error: mensaje}
}

func (m *Manejador) handleConfianzaForzadaListar(ctx context.Context) respuesta {
	const accion = "forced_trust_list"
	if m.ConfianzaForzada == nil {
		return m.errorConfianzaForzada(accion, application.ErrConfianzaForzadaNoConfigurada)
	}
	confianza, err := m.ConfianzaForzada.Listar(ctx)
	if err != nil {
		return m.errorConfianzaForzada(accion, err)
	}
	entradas := make([]entradaConfianzaForzadaJSON, 0, len(confianza.Aceptaciones))
	for _, e := range confianza.Aceptaciones {
		entradas = append(entradas, entradaConfianzaForzadaJSON{
			obstaculoConfianzaJSON: m.obstaculoJSON(domain.ObstaculoConfianza{Tipo: e.Tipo, Huella: e.Huella, Sujeto: e.Sujeto}),
			AcceptedAt:             e.AceptadaEn.UTC().Format(time.RFC3339),
		})
	}
	return respuesta{OK: true, Action: accion, Data: resultadoListaConfianzaForzada{Forbidden: confianza.Prohibida, Entries: entradas}}
}

type paramsConfianzaForzada struct {
	Items []struct {
		Type        string `json:"type"`
		Fingerprint string `json:"fingerprint"`
	} `json:"items"`
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
}

// handleConfianzaForzadaAceptar guarda lo que el usuario aceptó tras ver el
// aviso. Cada elemento tiene que ser un obstáculo que la verificación de
// esta sesión ofreció aceptar.
func (m *Manejador) handleConfianzaForzadaAceptar(ctx context.Context, raw json.RawMessage) respuesta {
	const accion = "forced_trust_accept"
	if m.ConfianzaForzada == nil {
		return m.errorConfianzaForzada(accion, application.ErrConfianzaForzadaNoConfigurada)
	}
	var params paramsConfianzaForzada
	if err := json.Unmarshal(raw, &params); err != nil || len(params.Items) == 0 || len(params.Items) > maxConfianzaForzableSesion {
		return respuesta{OK: false, Action: accion, Error: m.t("error.formato_invalido")}
	}
	var pendientes []domain.ObstaculoConfianza
	m.confianzaMu.Lock()
	for _, item := range params.Items {
		o, ok := m.confianzaForzable[claveObstaculoIPC(domain.TipoConfianzaForzada(item.Type), item.Fingerprint)]
		if !ok {
			m.confianzaMu.Unlock()
			return m.errorConfianzaForzada(accion, errors.New("verify.forced_trust.error.not_offered"))
		}
		pendientes = append(pendientes, o)
	}
	m.confianzaMu.Unlock()
	for _, o := range pendientes {
		if _, err := m.ConfianzaForzada.Aceptar(ctx, o); err != nil {
			return m.errorConfianzaForzada(accion, err)
		}
	}
	return m.handleConfianzaForzadaListarComo(ctx, accion)
}

func (m *Manejador) handleConfianzaForzadaListarComo(ctx context.Context, accion string) respuesta {
	resp := m.handleConfianzaForzadaListar(ctx)
	resp.Action = accion
	return resp
}

func (m *Manejador) handleConfianzaForzadaQuitar(ctx context.Context, raw json.RawMessage) respuesta {
	const accion = "forced_trust_remove"
	if m.ConfianzaForzada == nil {
		return m.errorConfianzaForzada(accion, application.ErrConfianzaForzadaNoConfigurada)
	}
	var params paramsConfianzaForzada
	if err := json.Unmarshal(raw, &params); err != nil {
		return respuesta{OK: false, Action: accion, Error: m.t("error.formato_invalido")}
	}
	if _, err := m.ConfianzaForzada.Quitar(ctx, domain.TipoConfianzaForzada(strings.TrimSpace(params.Type)), params.Fingerprint); err != nil {
		return m.errorConfianzaForzada(accion, err)
	}
	return m.handleConfianzaForzadaListarComo(ctx, accion)
}
