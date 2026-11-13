// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"

	"grxfirma/internal/adapters/inbound/common/aprobacion"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
)

// localizadorProtocolo devuelve el idioma elegido para el protocolo, o nil
// para que el componente común use el del entorno.
func localizadorProtocolo() ports.Localizador {
	protocolI18n.mu.RLock()
	defer protocolI18n.mu.RUnlock()
	if protocolI18n.loc == nil {
		return nil
	}
	return protocolI18n.loc
}

// componerAprobacionProtocolo es el componedor común de aprobación con el
// idioma del protocolo. Lo usan todos los caminos de afirma://.
func componerAprobacionProtocolo(d application.DetalleAprobacion) string {
	return aprobacion.Componer(localizadorProtocolo(), d)
}

// destinosSolicitud reúne los endpoints que recibirán el documento o la firma,
// o que deciden qué se firma: servlets, pre y post de lote y serverUrl.
func destinosSolicitud(solicitud afirmauri.Solicitud) []string {
	destinos := []string{solicitud.Sesion.UploadEndpoint, solicitud.Sesion.RetrieveEndpoint}
	if solicitud.RemoteBatch != nil {
		destinos = append(destinos, solicitud.RemoteBatch.PreSignEndpoint, solicitud.RemoteBatch.PostSignEndpoint)
	}
	destinos = append(destinos, afirmauri.ServidoresTrifasicos(solicitud.Options)...)
	if solicitud.SignCommand != nil {
		destinos = append(destinos, afirmauri.ServidoresTrifasicos(solicitud.SignCommand.Options)...)
	}
	return destinos
}

// anteponerContextoSelector añade al texto del selector nativo la web que pide
// la operación o, en afirma:// directo, el servidor que recibirá el resultado,
// igual que el selector Fyne (A-02).
func anteponerContextoSelector(ctx context.Context, texto string) string {
	contexto, ok := certpicker.ContextoSolicitudDe(ctx)
	if !ok || contexto.Origen == "" {
		return texto
	}
	loc := localizadorProtocolo()
	if loc == nil {
		loc = localizador.Detectar()
	}
	linea := loc.T("approval.requester.origin", contexto.Origen)
	if contexto.OrigenEsDestino {
		linea = loc.T("approval.destinations", contexto.Origen)
	}
	return linea + "\r\n\r\n" + texto
}
