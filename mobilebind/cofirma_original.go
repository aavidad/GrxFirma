// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/security/excepciones"
)

// Claves cerradas del catálogo (EngineKeys en Android las deja pasar).
const (
	mobileErrorCofirmaSinOriginal    = "firma.cofirma_sin_original.pedir_original"
	mobileErrorOriginalNoCorresponde = "firma.cofirma_sin_original.original_no_corresponde"
	mobileErrorSHA1Prohibido         = "firma.sha1_corresponde.prohibido"
)

var mobileAvisoCofirmaSinOriginal = excepciones.Warning("cofirma_sin_original")

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
func comprobarOriginalCofirma(content, original []byte, confirmado bool, contexts ...context.Context) (avisos []string, err error) {
	if len(original) > 0 {
		return resolverCotejoMovil(commonsigner.CotejarCAdESExplicita(content, original), "cofirma", mobileErrorOriginalNoCorresponde, contexts...)
	}
	if !confirmado {
		return nil, newFacadeError(mobileErrorCofirmaSinOriginal)
	}
	if err := excepciones.Check("cofirma_sin_original", contexts...); err != nil {
		return nil, err
	}
	slog.Warn("cofirma_sin_original_confirmada", "plataforma", "movil")
	return []string{excepciones.Warning("cofirma_sin_original")}, nil
}

// resolverCotejoMovil traduce un cotejo a las claves cerradas del móvil. Una
// firma SHA-1 que corresponde sigue adelante con su aviso (lote 16c), salvo
// que la política de máquina prohíba SHA-1 legado.
func resolverCotejoMovil(cotejo error, operacion, errorNoCorresponde string, contexts ...context.Context) ([]string, error) {
	var avisos []string
	if errors.Is(cotejo, commonsigner.ErrFirmaCorrespondeConC14NAntigua) {
		avisos = append(avisos, excepciones.Warning("xml_c14n_antigua"))
		if !errors.Is(cotejo, commonsigner.ErrFirmaCorrespondeConSHA1) {
			return avisos, nil
		}
		cotejo = commonsigner.ErrFirmaCorrespondeConSHA1
	}
	aviso, err := commonsigner.AdmitirCotejoSHA1(cotejo, operacion, contexts...)
	var block *excepciones.Block
	if errors.As(err, &block) {
		return nil, err
	}
	switch {
	case errors.Is(err, commonsigner.ErrCotejoSHA1Prohibido):
		return nil, err
	case err != nil:
		return nil, newFacadeError(errorNoCorresponde)
	case aviso:
		return append(avisos, excepciones.Warning("sha1_conservar")), nil
	}
	return nil, nil
}
