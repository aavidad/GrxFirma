// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package macoskeychain

import "errors"

// ErrNoDisponibleEnEstaPlataforma indica que el Keychain nativo no existe en
// la plataforma o configuración de compilación actual.
var ErrNoDisponibleEnEstaPlataforma = errors.New("macoskeychain: no disponible en esta plataforma")
