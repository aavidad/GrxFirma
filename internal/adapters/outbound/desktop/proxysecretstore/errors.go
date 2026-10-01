// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import "errors"

var ErrNoDisponibleEnEstaPlataforma = errors.New("proxysecretstore: sin backend seguro implementado en esta plataforma")
