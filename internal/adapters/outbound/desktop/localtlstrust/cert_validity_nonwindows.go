// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux || darwin

package localtlstrust

import (
	"crypto/x509"
	"time"
)

func certVigente(cert *x509.Certificate, now time.Time) bool {
	return !now.Before(cert.NotBefore) && !now.After(cert.NotAfter)
}
