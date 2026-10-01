// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux

package tokenpin

import (
	"context"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
)

func requestLocal(context.Context, string, string, pkcs11worker.PINMode) ([]byte, error) {
	return nil, ErrUnavailable
}
