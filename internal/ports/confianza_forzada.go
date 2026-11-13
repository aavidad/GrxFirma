// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"

	"grxfirma/internal/domain"
)

// AlmacenConfianzaForzada guarda lo que el usuario ha aceptado al verificar
// (lote 14) en su configuración y dice si la política de máquina lo prohíbe.
// Solo lo componen la CLI y la interfaz de escritorio; ningún canal web.
type AlmacenConfianzaForzada interface {
	// CargarConfianzaForzada devuelve lo aceptado y si la política lo
	// prohíbe. Con la política ilegible o inválida, Prohibida es true.
	CargarConfianzaForzada(ctx context.Context) (domain.ConfianzaForzada, error)
	// AceptarConfianzaForzada añade (o renueva) una entrada. Falla con
	// domain.ErrConfianzaProhibida si la política lo impide.
	AceptarConfianzaForzada(ctx context.Context, aceptacion domain.AceptacionConfianza) error
	// QuitarConfianzaForzada borra la entrada del tipo y huella indicados.
	QuitarConfianzaForzada(ctx context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error)
}
