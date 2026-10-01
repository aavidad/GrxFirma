// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import "context"

// LocalTLSStartupStatus resume el resultado del alta de la CA local por usuario.
// Qt traduce State y Changed con su catálogo; no transportamos rutas de perfiles
// ni detalles del error por el canal de presentación.
type LocalTLSStartupStatus struct {
	State   string `json:"state"` // unknown, ready o error
	Changed bool   `json:"changed"`
}

// SetLocalTLSStartupStatus publica el resultado antes de abrir el IPC. También
// permite actualizarlo si una reconciliación posterior acaba más tarde.
func (s *Servidor) SetLocalTLSStartupStatus(status LocalTLSStartupStatus) {
	if s == nil || s.manejador == nil {
		return
	}
	s.manejador.setLocalTLSStartupStatus(status)
}

func (m *Manejador) setLocalTLSStartupStatus(status LocalTLSStartupStatus) {
	m.localTLSStartupMu.Lock()
	defer m.localTLSStartupMu.Unlock()
	switch status.State {
	case "ready", "error":
		m.localTLSStartupStatus = status
	default:
		m.localTLSStartupStatus = LocalTLSStartupStatus{State: "unknown"}
	}
}

// SetLocalTLSStartupRefresh vincula la reconciliación idempotente del arranque
// al IPC. Una Qt iniciada directamente puede conectarse a un servidor anterior
// y crear perfiles Firefox nuevos entre ambos arranques.
func (s *Servidor) SetLocalTLSStartupRefresh(refresh func(context.Context) LocalTLSStartupStatus) {
	if s == nil || s.manejador == nil {
		return
	}
	s.manejador.localTLSStartupMu.Lock()
	defer s.manejador.localTLSStartupMu.Unlock()
	s.manejador.localTLSStartupRefresh = refresh
}

func (m *Manejador) refreshLocalTLSStartup(ctx context.Context) LocalTLSStartupStatus {
	m.localTLSRefreshMu.Lock()
	defer m.localTLSRefreshMu.Unlock()
	m.localTLSStartupMu.RLock()
	refresh := m.localTLSStartupRefresh
	m.localTLSStartupMu.RUnlock()
	if refresh != nil {
		previous := m.localTLSStartupSnapshot()
		status := refresh(ctx)
		if status.State == "ready" && previous.State == "ready" {
			status.Changed = status.Changed || previous.Changed
		}
		m.setLocalTLSStartupStatus(status)
	}
	// Changed anuncia el alta solo a la primera Qt que consulte el servidor.
	// Si el backend persiste, una Qt posterior no repite un aviso ya visto.
	m.localTLSStartupMu.Lock()
	defer m.localTLSStartupMu.Unlock()
	if m.localTLSStartupStatus.State == "" {
		return LocalTLSStartupStatus{State: "unknown"}
	}
	result := m.localTLSStartupStatus
	m.localTLSStartupStatus.Changed = false
	return result
}

func (m *Manejador) localTLSStartupSnapshot() LocalTLSStartupStatus {
	m.localTLSStartupMu.RLock()
	defer m.localTLSStartupMu.RUnlock()
	if m.localTLSStartupStatus.State == "" {
		return LocalTLSStartupStatus{State: "unknown"}
	}
	return m.localTLSStartupStatus
}
