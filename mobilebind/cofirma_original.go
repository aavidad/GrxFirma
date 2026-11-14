// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"log/slog"
	"strings"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

// Claves cerradas del catálogo (EngineKeys en Android las deja pasar).
const (
	mobileErrorCofirmaSinOriginal    = "firma.cofirma_sin_original.pedir_original"
	mobileErrorOriginalNoCorresponde = "firma.cofirma_sin_original.original_no_corresponde"
	mobileAvisoCofirmaSinOriginal    = "firma.cofirma_sin_original.aviso"
)

// esCofirmaCAdESExplicita indica si la operación añade una firma a una CAdES
// que no lleva el documento dentro. En ese caso el motor firmaría el resumen
// que declara la firma recibida, sin que nadie haya visto el documento.
func esCofirmaCAdESExplicita(format, action string, content []byte) bool {
	if format != "cades" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "cosign", "countersign":
	default:
		return false
	}
	_, implicita, err := commonsigner.ContenidoCAdESImplicito(content)
	// Si no se puede leer, el motor la rechazará igual.
	return err == nil && !implicita
}

// comprobarOriginalCofirma exige el original de una CAdES explícita y que
// corresponda a la firma. Sin original, solo sigue con la confirmación
// expresa de quien firma, y entonces devuelve el aviso que debe acompañar al
// resultado.
func comprobarOriginalCofirma(content, original []byte, confirmado bool) (avisos []string, err error) {
	if len(original) > 0 {
		if commonsigner.CotejarCAdESExplicita(content, original) != nil {
			return nil, newFacadeError(mobileErrorOriginalNoCorresponde)
		}
		return nil, nil
	}
	if !confirmado {
		return nil, newFacadeError(mobileErrorCofirmaSinOriginal)
	}
	slog.Warn("cofirma_sin_original_confirmada", "plataforma", "movil")
	return []string{mobileAvisoCofirmaSinOriginal}, nil
}
