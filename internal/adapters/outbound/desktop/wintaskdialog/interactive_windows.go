// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wintaskdialog

import "grxfirma/internal/adapters/outbound/desktop/escritorio"

// Interactive dice si el proceso tiene un escritorio que alguien ve. En una
// sesión de servicio, por SSH o en un agente de CI, la estación de ventanas
// no es visible: un diálogo se abriría donde nadie puede cerrarlo y la
// operación esperaría para siempre. La detección es la común del paquete
// escritorio.
func Interactive() bool { return interactive() }

// interactive se puede sustituir en las pruebas.
var interactive = escritorio.Hay
