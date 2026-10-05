// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Textos del aviso que se muestra cuando la web pide PAdES y el usuario
// elige un fichero que no es PDF. Son identificadores del catálogo
// (localizador/locales/*.json). Ninguna traducción puede contener «cancel»:
// isLegacyCancellation trataría el error como una cancelación.
const (
	legacyDocumentoNoPDFTitleID  = "El fichero elegido no es un PDF"
	legacyDocumentoNoPDFDetailID = "El portal pide una firma PAdES, que solo admite documentos PDF, y el fichero elegido no es un PDF. No se ha firmado nada. Vuelva a iniciar la firma en el portal y elija un fichero PDF."
	// Etiquetas del filtro del selector nativo de Windows.
	legacyFiltroPermitidosID = "Ficheros permitidos (%s)"
	legacyFiltroTodosID      = "Todos los ficheros (*.*)"
)

// legacyPDFHeaderWindow es la zona inicial donde se busca «%PDF-». La norma
// PDF admite bytes previos a la cabecera y los lectores habituales la buscan
// en los primeros 1024 bytes.
const legacyPDFHeaderWindow = 1024

// legacyFilenameExtsKeys son los nombres con los que AutoFirma Java recibe
// las extensiones permitidas para el selector de la firma sin datos.
var legacyFilenameExtsKeys = []string{"filenameExts", "filenameexts", "filenameExtensions"}

// legacyDocumentoNoPDFError indica que se pidió PAdES sobre un fichero que no
// es PDF. Error() empieza por un código SAF explícito para que la web reciba
// el motivo real y no el genérico.
type legacyDocumentoNoPDFError struct{}

func (*legacyDocumentoNoPDFError) Error() string {
	return "SAF_09: " + tl(legacyDocumentoNoPDFDetailID)
}

// legacyDocumentFilter decide qué ficheros ofrece el selector cuando la web
// pide firmar sin enviar datos. Manda la lista de la web (filenameExts); si
// no la envía, PAdES se limita a PDF y el resto de formatos admiten
// cualquier fichero, como AutoFirma Java.
func legacyDocumentFilter(solicitud afirmauri.Solicitud) ports.DocumentFilter {
	if exts := normalizarExtensionesDocumento(legacyFilenameExts(solicitud)); len(exts) > 0 {
		return ports.DocumentFilter{Extensions: exts}
	}
	formatoWeb := strings.ToLower(strings.TrimSpace(legacyQueryParam(solicitud.LegacyParams, "format", "signFormat")))
	if formatoWeb == "" || formatoWeb == "auto" {
		return ports.DocumentFilter{}
	}
	if solicitud.Formato == domain.FormatPAdES {
		return ports.DocumentFilter{Extensions: []string{"pdf"}}
	}
	return ports.DocumentFilter{}
}

func legacyFilenameExts(solicitud afirmauri.Solicitud) string {
	for clave, valor := range solicitud.Options {
		for _, nombre := range legacyFilenameExtsKeys {
			if strings.EqualFold(strings.TrimSpace(clave), nombre) && strings.TrimSpace(valor) != "" {
				return valor
			}
		}
	}
	return legacyQueryParam(solicitud.LegacyParams, legacyFilenameExtsKeys...)
}

// normalizarExtensionesDocumento acepta «pdf», «.pdf», «*.pdf» separados por
// comas o punto y coma; descarta lo que no sea alfanumérico.
func normalizarExtensionesDocumento(raw string) []string {
	vistas := make(map[string]struct{})
	salida := make([]string, 0, 4)
	for _, parte := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		parte = strings.ToLower(strings.TrimSpace(parte))
		parte = strings.TrimPrefix(strings.TrimPrefix(parte, "*"), ".")
		if parte == "" || len(parte) > 16 {
			continue
		}
		valida := true
		for _, r := range parte {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				valida = false
				break
			}
		}
		if !valida {
			continue
		}
		if _, ok := vistas[parte]; ok {
			continue
		}
		vistas[parte] = struct{}{}
		salida = append(salida, parte)
		if len(salida) >= 32 {
			break
		}
	}
	return salida
}

// pickLegacyDocument abre el selector con filtro cuando el selector lo admite.
func pickLegacyDocument(ctx context.Context, picker ports.DocumentPicker, filter ports.DocumentFilter) (domain.Document, error) {
	if filtered, ok := picker.(ports.FilteredDocumentPicker); ok {
		return filtered.PickFiltered(ctx, filter)
	}
	return picker.Pick(ctx)
}

// validarDocumentoLegacy comprueba que el contenido elegido sirve para el
// formato pedido antes de firmar.
func validarDocumentoLegacy(documento domain.Document, formato domain.SignatureFormat) error {
	if formato == domain.FormatPAdES && !esContenidoPDF(documento.Content) {
		return &legacyDocumentoNoPDFError{}
	}
	return nil
}

func esContenidoPDF(data []byte) bool {
	if len(data) > legacyPDFHeaderWindow {
		data = data[:legacyPDFHeaderWindow]
	}
	return bytes.Contains(data, []byte("%PDF-"))
}

// presentLegacyDocumentoNoPDF muestra el aviso localizado en la ventana del
// protocolo. Devuelve false si el error es de otro tipo.
func presentLegacyDocumentoNoPDF(action string, err error) bool {
	if !isLegacyDocumentoNoPDF(err) {
		return false
	}
	setProtocolPhase(
		"document_pick",
		tl(legacyDocumentoNoPDFTitleID),
		tl(legacyDocumentoNoPDFDetailID),
	)
	protocolDiagnosticPersistFailure(action, err)
	return true
}

func isLegacyDocumentoNoPDF(err error) bool {
	var noPDF *legacyDocumentoNoPDFError
	return errors.As(err, &noPDF)
}
