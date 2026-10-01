// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !windows && !darwin

package localtlstrust

import (
	"context"
	"crypto/x509"
)

func ensureTrustedPlatform(_ context.Context, _, _ string, _ *x509.Certificate) error {
	return ErrSoporteNoDisponible
}
