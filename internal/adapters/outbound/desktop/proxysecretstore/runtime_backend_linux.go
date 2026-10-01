// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package proxysecretstore

import "errors"

func isPlatformStoreUnavailable(err error) bool {
	return errors.Is(err, ErrSecretToolUnavailable)
}
