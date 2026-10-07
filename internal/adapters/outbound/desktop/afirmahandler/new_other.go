// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package afirmahandler

import "grxfirma/internal/ports"

// New devuelve un selector sin elección: fuera de Windows el protocolo se
// registra con los mecanismos propios de cada sistema.
func New() ports.ProtocoloAfirma {
	return unsupported{}
}
