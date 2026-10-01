// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows && !linux

package localtlstrust

import (
	"context"
	"crypto/x509"
)

func ensureManagedTrustedPlatform(
	ctx context.Context,
	certFile string,
	validatedCertFile string,
	cert *x509.Certificate,
) error {
	// El inventario de propiedad introducido aquí es específico del almacén
	// CurrentUser/ROOT. Linux y macOS conservan por ahora su ciclo existente.
	return ensureTrustedPlatform(ctx, certFile, validatedCertFile, cert)
}

func removeManagedTrustedPlatform(context.Context, string) error {
	return ErrSoporteNoDisponible
}

func managedTrustLifecycleSupportedPlatform() bool {
	return false
}

func managedTrustChangeTokenPlatform(string) (string, error) { return "", nil }
