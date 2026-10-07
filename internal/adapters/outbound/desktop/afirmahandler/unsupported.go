// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"

	"grxfirma/internal/ports"
)

// unsupported es el selector de las plataformas donde no hay elección.
type unsupported struct{}

func (unsupported) Estado(context.Context) (ports.EstadoProtocoloAfirma, error) {
	return ports.EstadoProtocoloAfirma{Soportado: false, Actual: ports.ProgramaAfirmaNinguno}, nil
}

func (unsupported) Elegir(context.Context, string) (ports.EstadoProtocoloAfirma, error) {
	return ports.EstadoProtocoloAfirma{}, ports.ErrProtocoloAfirmaNoSoportado
}
