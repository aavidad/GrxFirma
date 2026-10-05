// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package signer

import "grxfirma/internal/domain"

// InspectSignature reutiliza los parsers del motor para proponer una cofirma.
// Es una inspección estructural, no una verificación ni una decisión de confianza.
// Funciona con CAdES separado sin exigir el original ni consultar servicios.
func InspectSignature(doc domain.Document) (bool, string) {
	_, format, err := (&MultiVerifier{}).pickEngine(doc)
	if err != nil {
		return false, ""
	}
	switch format {
	case "CAdES":
		parts, err := parseSignedDataParts(doc.Content)
		return err == nil && len(parts.signerInfos) > 0 && len(parts.signerInfos) <= 64, format
	case "PAdES":
		signatures, err := extractPDFEmbeddedSignatures(doc.Content)
		return err == nil && len(signatures) > 0, format
	case "XAdES", "XMLdSig":
		parsed, err := analizarDocumentoXAdES(doc.Content)
		if err != nil {
			return false, format
		}
		count := len(parsed.firmasPrincipales())
		return count > 0 && count <= 64, format
	default:
		return false, format
	}
}
