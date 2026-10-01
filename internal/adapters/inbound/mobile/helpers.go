// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobile

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	presentationmobile "grxfirma/presentation/mobile"
)

type DocumentoCompartidoEntrada struct {
	Descriptor string
	Payload    []byte
}

func ParsearDeepLinkMobile(ctx context.Context, legacy *afirmauri.Adaptador, raw, origin string) (Solicitud, error) {
	if strings.HasPrefix(strings.ToLower(raw), "afirma://") {
		solicitud, err := legacy.Parse(ctx, raw)
		if err != nil {
			return Solicitud{}, err
		}
		resultado := Solicitud{
			Tipo:            tipoDesdeLegacy(solicitud),
			AccionFirma:     solicitud.AccionFirma,
			Formato:         solicitud.Formato,
			Origen:          origin,
			ViewModel:       viewModelDesdeLegacy(solicitud),
			SignCommand:     solicitud.SignCommand,
			BatchCommand:    solicitud.BatchCommand,
			RetrieveCommand: solicitud.RetrieveCommand,
		}
		resultado.limpiar = construirLiberador(resultado)
		return resultado, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Solicitud{}, fmt.Errorf("%s: deep link invalido: %w", origin, err)
	}
	if !strings.EqualFold(u.Scheme, "grxfirma") {
		return Solicitud{}, fmt.Errorf("%s: esquema no soportado: %s", origin, u.Scheme)
	}

	action := strings.Trim(strings.ToLower(u.Host), "/")
	if action == "" {
		action = strings.Trim(strings.ToLower(u.Path), "/")
	}
	switch action {
	case "sign", "firmar":
		params := u.Query()
		rawData := strings.TrimSpace(parametroURL(params, "dat", "data", "content"))
		if rawData == "" {
			return Solicitud{}, fmt.Errorf("%s: falta el payload base64 del deep link", origin)
		}
		data, err := base64.StdEncoding.DecodeString(rawData)
		if err != nil {
			return Solicitud{}, fmt.Errorf("%s: payload base64 invalido", origin)
		}
		return BuildSolicitudDocumento("mobile-deeplink", parametroURL(params, "name", "filename"), parametroURL(params, "mime", "mimetype"), parametroURL(params, "format"), parametroURL(params, "action"), data)
	default:
		return Solicitud{}, fmt.Errorf("%s: accion no soportada: %s", origin, action)
	}
}

func BuildSolicitudDocumentoCompartido(origin, descriptor string, payload []byte) (Solicitud, error) {
	nombre, mime := inferirDescriptor(descriptor, payload)
	return BuildSolicitudDocumento(origin, nombre, mime, "", "", payload)
}

func BuildSolicitudDocumentosCompartidos(origin string, entradas []DocumentoCompartidoEntrada) (Solicitud, error) {
	if len(entradas) == 0 {
		return Solicitud{}, fmt.Errorf("%s: lote de documentos compartidos vacio", origin)
	}
	items := make([]application.BatchItemInput, 0, len(entradas))
	for _, entrada := range entradas {
		payload := entrada.Payload
		if len(payload) == 0 {
			return Solicitud{}, fmt.Errorf("%s: documento compartido vacio", origin)
		}
		ownedPayload := copiarYSobrescribir(payload)
		nombre, mime := inferirDescriptor(entrada.Descriptor, ownedPayload)
		items = append(items, application.BatchItemInput{
			Nombre:    nombre,
			Contenido: ownedPayload,
			TipoMIME:  mime,
			Formato:   inferirFormato(mime, ownedPayload),
			Accion:    "sign",
		})
	}
	cmd, err := application.NewProcessBatchCommand(items)
	if err != nil {
		for i := range items {
			zeroBytes(items[i].Contenido)
		}
		return Solicitud{}, err
	}
	resultado := Solicitud{
		Tipo:         TipoFirmaEnLote,
		AccionFirma:  domain.ActionSign,
		Formato:      cmd.Jobs[0].Format,
		Origen:       origin,
		ViewModel:    viewModelLoteCompartido(len(cmd.Jobs)),
		BatchCommand: &cmd,
	}
	resultado.limpiar = construirLiberador(resultado)
	return resultado, nil
}

func BuildSolicitudDocumento(origin, name, mime, format, action string, payload []byte) (Solicitud, error) {
	if len(payload) == 0 {
		return Solicitud{}, fmt.Errorf("%s: documento vacio", origin)
	}
	ownedPayload := copiarYSobrescribir(payload)
	if strings.TrimSpace(format) == "" {
		format = inferirFormato(mime, ownedPayload)
	}
	if strings.TrimSpace(action) == "" {
		action = "sign"
	}
	if strings.TrimSpace(name) == "" {
		name = nombrePorDefecto(format)
	}
	if strings.TrimSpace(mime) == "" {
		mime = mimePorFormato(format)
	}

	cmd, err := application.NewSignCommand(name, ownedPayload, mime, format, action, "", nil)
	if err != nil {
		zeroBytes(ownedPayload)
		return Solicitud{}, err
	}
	vm := presentationmobile.NuevaOperacionFirma(name)
	resultado := Solicitud{
		Tipo:        TipoFirma,
		AccionFirma: cmd.Action,
		Formato:     cmd.Format,
		Origen:      origin,
		ViewModel:   vm,
		SignCommand: &cmd,
	}
	resultado.limpiar = construirLiberador(resultado)
	return resultado, nil
}

func tipoDesdeLegacy(s afirmauri.Solicitud) TipoSolicitud {
	switch {
	case s.BatchCommand != nil:
		return TipoFirmaEnLote
	case s.RetrieveCommand != nil:
		return TipoRecuperacion
	case s.Operacion == afirmauri.OperacionSelectCert:
		return TipoSeleccionCert
	default:
		return TipoFirma
	}
}

func viewModelDesdeLegacy(s afirmauri.Solicitud) presentationmobile.ViewModelOperacion {
	switch {
	case s.SignCommand != nil:
		return presentationmobile.NuevaOperacionFirma(s.SignCommand.Document.Name)
	case s.BatchCommand != nil:
		return presentationmobile.ViewModelOperacion{
			Titulo:   "Firma en lote",
			Detalle:  fmt.Sprintf("trabajos=%d", len(s.BatchCommand.Jobs)),
			Estado:   presentationmobile.EstadoReposo,
			AccionID: "firmar_lote",
		}
	case s.RetrieveCommand != nil:
		return presentationmobile.ViewModelOperacion{
			Titulo:   "Recuperacion remota",
			Detalle:  s.RetrieveCommand.Session.RequestID,
			Estado:   presentationmobile.EstadoReposo,
			AccionID: "recuperar_solicitud",
		}
	default:
		return presentationmobile.ViewModelOperacion{
			Titulo:   "Operacion mobile",
			Estado:   presentationmobile.EstadoReposo,
			AccionID: "operacion_mobile",
		}
	}
}

func viewModelLoteCompartido(total int) presentationmobile.ViewModelOperacion {
	return presentationmobile.ViewModelOperacion{
		Titulo:   "Firma en lote",
		Detalle:  fmt.Sprintf("trabajos=%d", total),
		Estado:   presentationmobile.EstadoReposo,
		AccionID: "firmar_lote",
	}
}

func inferirDescriptor(descriptor string, payload []byte) (string, string) {
	descriptor = strings.TrimSpace(descriptor)
	if descriptor == "" {
		return nombrePorDefecto(inferirFormato("", payload)), mimePorFormato(inferirFormato("", payload))
	}
	if strings.Contains(descriptor, "/") {
		return nombrePorDefecto(inferirFormato(descriptor, payload)), descriptor
	}
	if !strings.Contains(descriptor, ".") {
		return nombrePorDefecto(inferirFormato("", payload)), mimePorFormato(inferirFormato("", payload))
	}
	return descriptor, mimePorFormato(inferirFormato("", payload))
}

func inferirFormato(mime string, payload []byte) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case mime == "application/pdf" || strings.HasPrefix(string(payload), "%PDF-"):
		return string(domain.FormatPAdES)
	case strings.HasPrefix(mime, "application/vnd.oasis.opendocument."):
		return "ODF"
	case strings.HasPrefix(mime, "application/vnd.openxmlformats-officedocument."):
		return "OOXML"
	case mime == "application/xmldsig+xml":
		return "XMLdSig"
	case mime == "application/xml" || mime == "text/xml" || pareceXML(payload):
		return string(domain.FormatXAdES)
	default:
		return string(domain.FormatCAdES)
	}
}

func mimePorFormato(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pades":
		return "application/pdf"
	case "odf":
		return "application/vnd.oasis.opendocument.text"
	case "ooxml":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xmldsig":
		return "application/xmldsig+xml"
	case "xades":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

func nombrePorDefecto(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pades":
		return "documento.pdf"
	case "odf":
		return "documento.odt"
	case "ooxml":
		return "documento.docx"
	case "xmldsig":
		return "documento.dsig"
	case "xades":
		return "documento.xml"
	default:
		return "documento.bin"
	}
}

func pareceXML(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	return strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">")
}

func parametroURL(v url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(v.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func construirLiberador(s Solicitud) func() {
	return func() {
		if s.SignCommand != nil {
			zeroBytes(s.SignCommand.Document.Content)
		}
		if s.BatchCommand != nil {
			for i := range s.BatchCommand.Jobs {
				zeroBytes(s.BatchCommand.Jobs[i].Document.Content)
			}
		}
	}
}

func copiarYSobrescribir(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := append([]byte(nil), src...)
	zeroBytes(src)
	return dst
}

func zeroBytes(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
