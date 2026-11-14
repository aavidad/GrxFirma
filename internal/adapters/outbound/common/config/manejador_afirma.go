// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"fmt"
	"log/slog"
	"slices"

	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
)

// validarManejadorAfirmaPolicy deja en ManejadorAfirma el valor canónico o,
// si no es grxfirma, autofirma ni preguntar, lo marca como inválido.
func validarManejadorAfirmaPolicy(raw *Policy, errs *[]error) {
	if raw.ManejadorAfirma == nil {
		return
	}
	valor, ok := ports.NormalizarProgramaAfirma(*raw.ManejadorAfirma)
	if !ok {
		raw.ManejadorAfirma = nil
		raw.invalidos = append(raw.invalidos, machinepolicy.ManejadorAfirma)
		*errs = append(*errs, fmt.Errorf("%s: valor no admitido", machinepolicy.ManejadorAfirma))
		return
	}
	raw.ManejadorAfirma = &valor
}

// PoliticaProgramaAfirma lee manejador_afirma de la política de máquina
// (HKLM\SOFTWARE\Policies\GrxFirma en Windows; policy.json en el resto).
// Si la política existe pero no se puede leer, o el valor no se entiende, se
// devuelve Ilegible: se mantiene lo de antes (la elección del usuario) y se
// avisa en el registro.
func PoliticaProgramaAfirma(policyDir string) ports.PoliticaProgramaAfirma {
	raw, err := loadSystemPolicy(policyDir)
	if raw.ilegible || slices.Contains(raw.invalidos, machinepolicy.ManejadorAfirma) {
		slog.Warn("la política de máquina no se pudo leer para manejador_afirma; se mantiene la elección del usuario",
			"error", err)
		return ports.PoliticaProgramaAfirma{Ilegible: true}
	}
	if raw.ManejadorAfirma != nil {
		return ports.PoliticaProgramaAfirma{Valor: *raw.ManejadorAfirma}
	}
	return ports.PoliticaProgramaAfirma{}
}

// LectorPoliticaProgramaAfirma lee la política de la ruta del sistema. Es lo
// que reciben los selectores de afirma:// en los binarios.
func LectorPoliticaProgramaAfirma() ports.PoliticaProgramaAfirma {
	return PoliticaProgramaAfirma(DirPolicyDefecto)
}
