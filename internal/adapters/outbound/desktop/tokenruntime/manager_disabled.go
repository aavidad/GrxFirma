// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux || !pkcs11_preview || production

package tokenruntime

import (
	"context"

	"grxfirma/internal/ports"
)

type managerOperations struct{}

func (*Manager) Load(context.Context) (ports.TokenSettingsSnapshot, error) {
	return ports.TokenSettingsSnapshot{State: "unavailable", Modules: []ports.TokenModuleSetting{}, Checks: []ports.TokenSettingCheck{{ID: "config", Status: "unavailable"}}}, nil
}

func (m *Manager) Diagnose(ctx context.Context) (ports.TokenSettingsSnapshot, error) {
	return m.Load(ctx)
}

func (*Manager) Save(context.Context, ports.TokenSettingsUpdate) (ports.TokenSettingsSnapshot, error) {
	return ports.TokenSettingsSnapshot{State: "unavailable"}, ports.ErrTokenSettingsUnavailable
}
