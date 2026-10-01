// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"errors"
)

// Token configuration is a local desktop administrative operation. Never
// expose this service through browser, REST, URI or Native Messaging options.
var (
	ErrTokenSettingsUnavailable  = errors.New("configuración de tarjetas no disponible en esta compilación")
	ErrTokenSettingsInvalid      = errors.New("configuración de tarjetas no válida")
	ErrTokenSettingsUnsafe       = errors.New("archivo o directorio de tarjetas no seguro o no modificable")
	ErrTokenSettingsConflict     = errors.New("la configuración de tarjetas cambió; vuelva a cargarla antes de guardar")
	ErrTokenSettingsConfirmation = errors.New("confirme los cambios de controladores antes de guardar")
	ErrTokenSettingsWrite        = errors.New("no se pudo guardar la configuración de tarjetas")
)

type TokenModuleSetting struct {
	Path string `json:"path"`
}

// ID and Status are fixed codes; no driver output, PIN or identity appears in
// checks. A filesystem check is not a successful hardware/signing operation.
type TokenSettingCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Modules are shown only to the bound local frontend, never in exported
// diagnostics. State describes the saved configuration, not a live session.
type TokenSettingsSnapshot struct {
	Available       bool                 `json:"available"`
	Editable        bool                 `json:"editable"`
	Exists          bool                 `json:"exists"`
	State           string               `json:"state"`
	Enabled         bool                 `json:"enabled"`
	Modules         []TokenModuleSetting `json:"modules"`
	Revision        string               `json:"revision"`
	RestartRequired bool                 `json:"restartRequired"`
	Checks          []TokenSettingCheck  `json:"checks"`
	// Fixed warning "durability_unconfirmed" means rename succeeded but the
	// directory fsync failed. It is a saved change, not a failed/absent write.
	Warning string `json:"warning,omitempty"`
}

type TokenSettingsUpdate struct {
	Enabled        bool                 `json:"enabled"`
	Modules        []TokenModuleSetting `json:"modules"`
	Revision       string               `json:"revision"`
	Confirmed      bool                 `json:"confirmed"`
	ReplaceInvalid bool                 `json:"replaceInvalid"`
}

type LocalTokenSettings interface {
	Load(context.Context) (TokenSettingsSnapshot, error)
	Save(context.Context, TokenSettingsUpdate) (TokenSettingsSnapshot, error)
	Diagnose(context.Context) (TokenSettingsSnapshot, error)
}
