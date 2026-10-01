// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"errors"

	"grxfirma/internal/domain"
)

// HeadlessSelector es una implementación de CertSelector sin GUI.
// Selecciona automáticamente el primer certificado de la lista.
// Es útil en modo CLI, en tests y en entornos sin display gráfico.
type HeadlessSelector struct{}

// NewHeadless crea un HeadlessSelector.
func NewHeadless() *HeadlessSelector {
	return &HeadlessSelector{}
}

// Select retorna el primer certificado de la lista, o error si la lista está vacía.
// Si el contexto ya está cancelado antes de llamar, retorna ctx.Err().
func (h *HeadlessSelector) Select(ctx context.Context, certs []domain.CertificateRef) (ResultadoSeleccion, error) {
	// Comprobar cancelación del contexto primero.
	select {
	case <-ctx.Done():
		return ResultadoSeleccion{}, ctx.Err()
	default:
	}

	if len(certs) == 0 {
		return ResultadoSeleccion{}, errors.New(tp("no hay certificados disponibles"))
	}
	// A non-interactive selector is not fresh consent to unlock a device.
	// Do not silently substitute another identity either.
	if certs[0].SigningKeyNeedsUnlock {
		return ResultadoSeleccion{}, ErrSeleccionInteractivaNecesaria
	}

	return ResultadoSeleccion{
		Certificado: certs[0],
		Recuerdo:    NoRecordar,
	}, nil
}
