// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector_test

import (
	"errors"

	"grxfirma/internal/security/excepciones"
)

// bloqueoTipadoV38 sustituye a excepciones.FromText (lote 38): el bloqueo se
// reconoce por el error tipado o por la causa del resultado, nunca por el texto.
func bloqueoTipadoV38(err error) *excepciones.Block {
	var block *excepciones.Block
	if errors.As(err, &block) {
		return block
	}
	return nil
}
