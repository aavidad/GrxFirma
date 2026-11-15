// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer_test

import (
	"errors"

	"grxfirma/internal/security/excepciones"
)

// esBloqueoV38 reconoce el bloqueo por el error tipado (lote 38), no por el texto.
func esBloqueoV38(err error, id string) bool {
	var block *excepciones.Block
	return errors.As(err, &block) && block.ID == id
}
