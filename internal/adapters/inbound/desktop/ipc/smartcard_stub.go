//go:build !windows && (!linux || !cgo)

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"errors"
)

const smartcardAvailable = false

func detectSmartcards(context.Context) ([]smartcardReader, error) {
	return nil, errors.New("smartcard.pcsc_unavailable")
}
