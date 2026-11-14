// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows && !linux && !darwin

package localtlstrust

import (
	"context"
	"crypto/x509"
)

// En las plataformas sin inventario de propiedad no se instala confianza
// gestionada: no se podría retirar después (B-07).
func ensureManagedTrustedPlatform(context.Context, string, string, *x509.Certificate) error {
	return ErrSoporteNoDisponible
}

func removeManagedTrustedPlatform(context.Context, string) error {
	return ErrSoporteNoDisponible
}

func managedTrustLifecycleSupportedPlatform() bool {
	return false
}

func managedTrustChangeTokenPlatform(string) (string, error) { return "", nil }
