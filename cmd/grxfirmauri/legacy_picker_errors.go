// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import "errors"

// Estos errores forman parte del control de flujo y no deben depender del
// idioma activo. La traduccion se aplica solo al presentar el resultado.
var (
	errLegacyLoadCanceled          = errors.New("legacy load cancelled")
	errLegacyLoadPickerUnavailable = errors.New("legacy load picker unavailable")
	errLegacySaveCanceled          = errors.New("legacy save cancelled")
)
