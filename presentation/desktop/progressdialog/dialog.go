// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package progressdialog proporciona un diálogo de progreso para operaciones de firma.
// La implementación Fyne solo existe en fyne_dialog.go; el resto del paquete
// no importa fyne.io y puede compilarse sin display gráfico.
package progressdialog

import "context"

// Reporter permite actualizar el progreso de una operación en curso.
type Reporter interface {
	// SetMensaje actualiza el texto descriptivo visible al usuario.
	SetMensaje(msg string)
	// SetProgreso actualiza la fracción completada (0.0–1.0). -1 = indeterminado.
	SetProgreso(fraccion float64)
	// Cerrar cierra el diálogo de progreso.
	Cerrar()
}

// ProgressProvider crea un diálogo de progreso modal con el título dado.
// Bloquea la UI hasta que se llame a Reporter.Cerrar() o el contexto se cancele.
type ProgressProvider interface {
	MostrarProgreso(ctx context.Context, titulo string) Reporter
}
