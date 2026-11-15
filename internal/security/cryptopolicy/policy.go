// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package cryptopolicy centraliza opt-ins para algoritmos de firma heredados.
package cryptopolicy

import (
	"context"
	"encoding/asn1"
	"errors"
	"github.com/digitorus/pkcs7"

	"grxfirma/internal/security/excepciones"
)

const EnvEnableLegacySHA1 = "GRXFIRMA_ENABLE_LEGACY_SHA1"

var ErrSHA1Disabled = errors.New("excepciones.sha1_crear.bloqueo")

// LegacySHA1Enabled solo admite nuevas firmas SHA-1 si un administrador lo
// habilita en la política de máquina; EnvEnableLegacySHA1 ya no basta.
func LegacySHA1Enabled() bool {
	return excepciones.Check("sha1_crear") == nil
}

// LegacySHA1Prohibited indica que un administrador ha fijado
// permitir_sha1_legacy a 0 (directiva deshabilitada) o a false en la
// política de máquina. Sin la directiva, una firma SHA-1 antigua que
// corresponde a su documento se sigue admitiendo con aviso al envolverla en
// un ENI o al cofirmar con el original (retrocompatibilidad, lote 16c); con
// ella deshabilitada, se rechaza.
func LegacySHA1Prohibited() bool {
	return excepciones.Check("sha1_conservar") != nil
}

// RequireLegacySHA1 protege todos los puntos que generan una firma SHA-1.
func RequireLegacySHA1(ctxs ...context.Context) error {
	if len(ctxs) > 0 && ctxs[0] != nil {
		if err := excepciones.Require(ctxs[0], "sha1_crear"); err != nil {
			return sha1DisabledError{err}
		}
		return nil
	}
	if err := excepciones.Check("sha1_crear"); err != nil {
		return sha1DisabledError{err}
	}
	return nil
}

// El centinela conserva errors.Is para los protocolos que identifican SHA-1,
// mientras que el texto visible procede del catálogo de excepciones.
type sha1DisabledError struct{ cause error }

func (e sha1DisabledError) Error() string        { return e.cause.Error() }
func (e sha1DisabledError) Unwrap() error        { return e.cause }
func (e sha1DisabledError) Is(target error) bool { return target == ErrSHA1Disabled }

// CMSUsesSHA1 incluye el resumen y los identificadores de firma que la librería
// interpreta como SHA-1 aunque el resumen declarado diga SHA-256.
func CMSUsesSHA1(digest, signature asn1.ObjectIdentifier) bool {
	return digest.Equal(pkcs7.OIDDigestAlgorithmSHA1) ||
		signature.Equal(pkcs7.OIDDigestAlgorithmECDSASHA1) ||
		signature.Equal(pkcs7.OIDDigestAlgorithmDSASHA1) ||
		signature.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA1)
}
