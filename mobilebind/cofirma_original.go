// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"errors"
	"log/slog"
	"strings"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

// Claves cerradas del catálogo (EngineKeys en Android las deja pasar).
const (
	mobileErrorCofirmaSinOriginal    = "firma.cofirma_sin_original.pedir_original"
	mobileErrorOriginalNoCorresponde = "firma.cofirma_sin_original.original_no_corresponde"
	mobileAvisoCofirmaSinOriginal    = "firma.cofirma_sin_original.aviso"
	mobileErrorSHA1Prohibido         = "firma.sha1_corresponde.prohibido"
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
		return resolverCotejoMovil(commonsigner.CotejarCAdESExplicita(content, original), "cofirma", mobileErrorOriginalNoCorresponde)
	}
	if !confirmado {
		return nil, newFacadeError(mobileErrorCofirmaSinOriginal)
	}
	slog.Warn("cofirma_sin_original_confirmada", "plataforma", "movil")
	return []string{mobileAvisoCofirmaSinOriginal}, nil
}

// resolverCotejoMovil traduce un cotejo a las claves cerradas del móvil. Una
// firma SHA-1 que corresponde sigue adelante con su aviso (lote 16c), salvo
// que la política de máquina prohíba SHA-1 legado.
func resolverCotejoMovil(cotejo error, operacion, errorNoCorresponde string) ([]string, error) {
	aviso, err := commonsigner.AdmitirCotejoSHA1(cotejo, operacion)
	switch {
	case errors.Is(err, commonsigner.ErrCotejoSHA1Prohibido):
		return nil, newFacadeError(mobileErrorSHA1Prohibido)
	case err != nil:
		return nil, newFacadeError(errorNoCorresponde)
	case aviso:
		return []string{commonsigner.AvisoFirmaSHA1Corresponde}, nil
	}
	return nil, nil
}
