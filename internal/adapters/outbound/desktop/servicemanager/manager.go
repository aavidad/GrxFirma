// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package servicemanager implementa ports.GestorServicio para gestionar el
// servicio de usuario de GrxFirma en el sistema operativo subyacente.
//
// En Linux usa systemd --user. En otros sistemas devuelve ErrNoSoportado.
package servicemanager

import "errors"

// ErrNoSoportado se devuelve cuando la plataforma no soporta gestion de servicios.
var ErrNoSoportado = errors.New("gestion de servicio no soportada en esta plataforma")
