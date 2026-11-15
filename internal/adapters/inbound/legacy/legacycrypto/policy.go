// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package legacycrypto centraliza la política de primitivas criptográficas
// heredadas que todavía pueden ser necesarias para interoperar con V1.9.
package legacycrypto

import (
	"context"
	"errors"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/excepciones"
)

const EnvEnableLegacyDES = "GRXFIRMA_ENABLE_LEGACY_DES"

var ErrDESDisabled = errors.New("excepciones.des_intercambio.bloqueo")

// DESEnabled solo permite DES mediante una decisión explícita del operador.
func DESEnabled() bool {
	// Solo la política de máquina (administrador) puede habilitar DES; la
	// variable de entorno EnvEnableLegacyDES ya no rebaja la seguridad.
	return excepciones.Enabled("des_intercambio")
}

func RequireDES() error {
	if err := excepciones.Check("des_intercambio"); err != nil {
		return errors.Join(err, ErrDESDisabled)
	}
	return nil
}

// RequireDESEnIntercambio autoriza DES solo para el intercambio de AutoFirma
// 1.x tras un ajuste expreso. HTTPS no autoriza DES por sí solo.
func RequireDESEnIntercambio(_ ...string) error {
	return RequireDESEnIntercambioConContexto(context.Background())
}

// RequireDESEnIntercambioConContexto añade además el aviso al resultado de
// la operación cuando el adaptador dispone de contexto.
func RequireDESEnIntercambioConContexto(ctx context.Context, _ ...string) error {
	if err := excepciones.Require(ctx, "des_intercambio"); err != nil {
		return errors.Join(err, ErrDESDisabled)
	}
	avisos.Registrar(localizador.Detectar().T(excepciones.Warning("des_intercambio")), "")
	return nil
}
