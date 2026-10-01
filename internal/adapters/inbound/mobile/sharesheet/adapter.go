// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package sharesheet

import (
	"context"

	mobileinbound "grxfirma/internal/adapters/inbound/mobile"
)

// Adaptador representa la entrada móvil desde la hoja de compartición.
type Adaptador struct{}

// Handle traduce un documento compartido a una operación interna.
func (Adaptador) Handle(_ context.Context, descriptor string, payload []byte) (mobileinbound.Solicitud, error) {
	return mobileinbound.BuildSolicitudDocumentoCompartido("share-sheet", descriptor, payload)
}

// HandleMultiple traduce varios documentos compartidos a una operación interna
// de firma en lote.
func (Adaptador) HandleMultiple(_ context.Context, entradas []mobileinbound.DocumentoCompartidoEntrada) (mobileinbound.Solicitud, error) {
	return mobileinbound.BuildSolicitudDocumentosCompartidos("share-sheet", entradas)
}
