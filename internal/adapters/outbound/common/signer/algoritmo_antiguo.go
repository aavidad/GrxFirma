// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import "grxfirma/internal/domain"

// Criterio único para firmas hechas con un algoritmo antiguo (SHA-1) en XAdES,
// XMLdSig, FacturaE, ODF, OOXML, ASiC, CAdES y PAdES: si el resumen y la firma
// cuadran, la integridad criptográfica se da por comprobada, pero la firma no
// sale válida y el aviso queda a la vista y en la evidencia. Nunca se usa para
// firmar.
const (
	// Texto visible (clave de los catálogos de idiomas).
	motivoAlgoritmoAntiguo = "El documento no ha cambiado desde que se firmó, pero la firma usa SHA-1, un algoritmo antiguo que hoy no se considera seguro. Por eso no se da por válida: sirve como prueba de una firma hecha cuando SHA-1 se aceptaba."
	// detalleAlgoritmoSHA1 identifica el algoritmo en detalles y evidencia.
	detalleAlgoritmoSHA1 = "algoritmo_antiguo=sha1"
)

// marcarAlgoritmoAntiguo deja la integridad como estaba (comprobada), quita la
// validez global y añade el aviso, el detalle y la evidencia.
func marcarAlgoritmoAntiguo(result domain.VerificationResult, algoritmos []string) domain.VerificationResult {
	if len(algoritmos) == 0 {
		return result
	}
	if result.Valid {
		result.Valid = false
		result.Reason = motivoAlgoritmoAntiguo
	}
	result.Warnings = appendUniqueString(result.Warnings, motivoAlgoritmoAntiguo)
	vistos := map[string]bool{}
	for _, evidencia := range result.Evidence {
		if evidencia.Type == domain.EvidenciaAlgoritmoAntiguo {
			vistos[evidencia.Summary] = true
		}
	}
	for _, algoritmo := range algoritmos {
		if vistos[algoritmo] {
			continue
		}
		vistos[algoritmo] = true
		result.Details = append(result.Details, algoritmo)
		result.Integrity.Details = append(result.Integrity.Details, algoritmo)
		result.Evidence = append(result.Evidence, domain.VerificationEvidence{Type: domain.EvidenciaAlgoritmoAntiguo, Summary: algoritmo})
	}
	return result.Normalize()
}

// algoritmosAntiguos recoge lo marcado en un resultado parcial (una firma)
// para volver a marcarlo en el resultado agregado.
func algoritmosAntiguos(result domain.VerificationResult) []string {
	var out []string
	for _, evidencia := range result.Evidence {
		if evidencia.Type == domain.EvidenciaAlgoritmoAntiguo {
			out = append(out, evidencia.Summary)
		}
	}
	return out
}
