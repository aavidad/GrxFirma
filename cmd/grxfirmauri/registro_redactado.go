// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"io"
	"log/slog"

	"grxfirma/internal/adapters/outbound/common/logging"
)

// configurarRegistroPorDefecto hace que slog por defecto y el paquete log
// (log.Printf del orquestador de sesión) escriban a través del logger
// redactado. slog.SetDefault redirige también el paquete log. En Linux esta
// salida acaba en el diario del usuario, que pueden leer además los grupos
// adm y systemd-journal (A-10).
func configurarRegistroPorDefecto(w io.Writer) {
	slog.SetDefault(logging.New("cmd/grxfirma-afirmauri", w))
}
