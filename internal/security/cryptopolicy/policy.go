// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package cryptopolicy centraliza opt-ins para algoritmos de firma heredados.
package cryptopolicy

import (
	"errors"

	"grxfirma/internal/security/machinepolicy"
)

const EnvEnableLegacySHA1 = "GRXFIRMA_ENABLE_LEGACY_SHA1"

var ErrSHA1Disabled = errors.New(
	"firma bloqueada: el portal solicita SHA-1, un algoritmo obsoleto e inseguro; " +
		"no se ha generado ninguna firma. La entidad responsable del portal debe " +
		"actualizarlo a SHA-256 o superior")

// LegacySHA1Enabled solo admite nuevas firmas SHA-1 si un administrador lo
// habilita en la política de máquina; EnvEnableLegacySHA1 ya no basta.
func LegacySHA1Enabled() bool {
	return machinepolicy.OptIn(machinepolicy.PermitirSHA1Legacy)
}

// LegacySHA1Prohibited indica que un administrador ha fijado
// permitir_sha1_legacy a 0 (directiva deshabilitada) o a false en la
// política de máquina. Sin la directiva, una firma SHA-1 antigua que
// corresponde a su documento se sigue admitiendo con aviso al envolverla en
// un ENI o al cofirmar con el original (retrocompatibilidad, lote 16c); con
// ella deshabilitada, se rechaza.
func LegacySHA1Prohibited() bool {
	habilitado, fijado := machinepolicy.OptInFijado(machinepolicy.PermitirSHA1Legacy)
	return fijado && !habilitado
}

// RequireLegacySHA1 protege todos los puntos que generan una firma SHA-1.
func RequireLegacySHA1() error {
	if !LegacySHA1Enabled() {
		return ErrSHA1Disabled
	}
	return nil
}
