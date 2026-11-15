// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
)

// Criterio único para firmas hechas con un algoritmo antiguo (SHA-1) en XAdES,
// XMLdSig, FacturaE, ODF, OOXML, ASiC, CAdES y PAdES: si el resumen y la firma
// cuadran, la integridad criptográfica se da por comprobada, pero la firma no
// sale válida y el aviso queda a la vista y en la evidencia. Nunca se usa para
// firmar.
// detalleAlgoritmoSHA1 identifica el algoritmo en detalles y evidencia.
const detalleAlgoritmoSHA1 = "algoritmo_antiguo=sha1"

var motivoAlgoritmoAntiguo = excepciones.Warning("sha1_leer")

// marcarAlgoritmoAntiguo deja la integridad como estaba (comprobada), quita la
// validez global y añade el aviso, el detalle y la evidencia. Si el usuario
// aceptó SHA-1 en su equipo (lote 14), la validez no se toca y el aviso dice
// que se da por válida porque lo aceptó.
func marcarAlgoritmoAntiguo(registro *domain.RegistroConfianzaForzada, result domain.VerificationResult, algoritmos []string, contexts ...context.Context) domain.VerificationResult {
	if len(algoritmos) == 0 {
		return result
	}
	if err := excepciones.Check("sha1_leer", contexts...); err != nil {
		result = result.WithPolicyError(err)
		result.Valid = false
		result.Coverage = domain.CoberturaNoAcreditada
		result.Reason = err.Error()
		result.Integrity.Status = domain.VerificationStatusUnknown
		result.Integrity.Reason = err.Error()
		result.Trust.Status = domain.VerificationStatusUnknown
		// El texto va en el motivo y en Errors (Normalize lo exige para un
		// resultado no válido); repetido como advertencia salía tres veces.
		result.Errors = []string{err.Error()}
		return anotarAlgoritmoAntiguo(result, algoritmos, contexts...)
	}
	result.Warnings = appendUniqueString(result.Warnings, excepciones.Warning("sha1_leer"))
	obstaculo := domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1}
	if registro.Decidir(obstaculo) {
		result = marcarConfianzaForzada(result, obstaculo)
	} else {
		if result.Valid {
			result.Valid = false
			result.Reason = motivoAlgoritmoAntiguo
		}
		result.Warnings = appendUniqueString(result.Warnings, motivoAlgoritmoAntiguo)
	}
	return anotarAlgoritmoAntiguo(result, algoritmos, contexts...)
}

// anotarAlgoritmoAntiguo deja el detalle y la evidencia sin tocar la validez
// ni añadir el aviso. Sirve cuando la integridad no se ha podido comprobar
// (CAdES explícita sin el original): el aviso dice que el documento no ha
// cambiado, y eso no se sabe.
func anotarAlgoritmoAntiguo(result domain.VerificationResult, algoritmos []string, contexts ...context.Context) domain.VerificationResult {
	if len(algoritmos) > 0 {
		if err := excepciones.Check("sha1_leer", contexts...); err != nil {
			result = result.WithPolicyError(err)
			result.Valid = false
			result.Coverage = domain.CoberturaNoAcreditada
			result.Reason = err.Error()
			result.Integrity.Status = domain.VerificationStatusUnknown
			result.Integrity.Reason = err.Error()
			result.Trust.Status = domain.VerificationStatusUnknown
			result.Errors = []string{err.Error()}
		} else {
			result.Warnings = appendUniqueString(result.Warnings, excepciones.Warning("sha1_leer"))
		}
	}
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

// bloqueoSHA1Previo evita comprobar el resumen y la firma cuando el lector
// histórico está desactivado.
func bloqueoSHA1Previo(ctx context.Context, formato string) (domain.VerificationResult, bool) {
	err := excepciones.Check("sha1_leer", ctx)
	if err == nil {
		return domain.VerificationResult{}, false
	}
	result := domain.NewVerificationFailure(formato, err.Error(), nil).WithPolicyError(err)
	result.Coverage = domain.CoberturaNoAcreditada
	result.Integrity.Status = domain.VerificationStatusUnknown
	result.Integrity.Reason = err.Error()
	return result.Normalize(), true
}
