// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"strconv"
	"strings"
)

// BitsMinimosRSA es el tamaño mínimo de una clave RSA del firmante. Por
// debajo, la firma no es válida salvo que el usuario acepte la excepción
// clave_rsa_corta (orden de Alberto, 16/11/2026).
const BitsMinimosRSA = 2048

// DetalleBitsRSA es el prefijo del detalle del certificado con el tamaño de
// la clave RSA del firmante: «rsa_bits=1024».
const DetalleBitsRSA = "rsa_bits="

// ClaveRSACortaSinExcepcion dice si el certificado del firmante trae una
// clave RSA por debajo del mínimo (detalle rsa_bits=N) y el usuario no ha
// aceptado la excepción clave_rsa_corta (evidencia trust.user_forced). Se
// decide con datos estructurados, nunca con el texto del motivo.
func (r VerificationResult) ClaveRSACortaSinExcepcion() bool {
	for _, e := range r.Evidence {
		if e.Type == EvidenciaConfianzaForzada && e.Summary == string(ConfianzaClaveRSACorta) {
			return false
		}
	}
	for _, detalle := range r.Certificate.Details {
		bits, ok := strings.CutPrefix(strings.TrimSpace(detalle), DetalleBitsRSA)
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(bits); err == nil && n > 0 && n < BitsMinimosRSA {
			return true
		}
	}
	return false
}
