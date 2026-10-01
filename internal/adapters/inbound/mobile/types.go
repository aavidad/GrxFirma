// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobile

import (
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	presentationmobile "grxfirma/presentation/mobile"
)

// TipoSolicitud describe la operacion traducida desde el borde mobile.
type TipoSolicitud string

const (
	TipoFirma         TipoSolicitud = "firma"
	TipoFirmaEnLote   TipoSolicitud = "lote"
	TipoRecuperacion  TipoSolicitud = "recuperacion"
	TipoSeleccionCert TipoSolicitud = "seleccion_certificado"
)

// Solicitud representa la traduccion neutra de una entrada mobile.
type Solicitud struct {
	Tipo            TipoSolicitud
	AccionFirma     domain.SignatureAction
	Formato         domain.SignatureFormat
	Origen          string
	ViewModel       presentationmobile.ViewModelOperacion
	SignCommand     *application.SignCommand
	BatchCommand    *application.ProcessBatchCommand
	RetrieveCommand *application.RetrieveRequestCommand
	limpiar         func()
}

// Liberar elimina de memoria los buffers sensibles asociados a la solicitud.
func (s *Solicitud) Liberar() {
	if s == nil {
		return
	}
	if s.limpiar != nil {
		s.limpiar()
		s.limpiar = nil
	}
}
