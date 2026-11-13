// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package aprobacion compone el texto del diálogo de aprobación de firma que
// usan todos los adaptadores que piden consentimiento (afirma:// directo y por
// socket, lotes, Native Messaging). Dice qué se firma, con qué certificado,
// quién lo pide y adónde va el resultado; neutraliza todo el texto que llega
// de fuera y toma las frases de los catálogos de idioma.
package aprobacion

import (
	"html"
	"strings"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
)

// Máximo de nombres de documento que se enumeran en un lote.
const maxDocumentosListados = 5

// NuevoComponedor devuelve un componedor que traduce con loc. Con loc nil se
// usa el idioma del entorno.
func NuevoComponedor(loc ports.Localizador) application.ComponedorAprobacion {
	return func(d application.DetalleAprobacion) string { return Componer(loc, d) }
}

// Componer devuelve el texto, una idea por línea. La primera línea es la
// pregunta; después quién lo pide, qué se firma y adónde va.
func Componer(loc ports.Localizador, d application.DetalleAprobacion) string {
	if loc == nil {
		loc = localizador.Detectar()
	}
	campo := func(s string) string {
		return application.NeutralizarTextoAprobacion(s, application.MaxRunasAprobacionCampo)
	}
	certificado := campo(d.Certificado)
	if certificado == "" {
		certificado = loc.T("approval.certificate.unknown")
	}

	var lineas []string
	switch d.Operacion {
	case application.AprobacionLote:
		if n := max(d.Firmas, len(d.Documentos)); n > 0 {
			lineas = append(lineas, loc.T("approval.question.batch", n, certificado))
		} else {
			lineas = append(lineas, loc.T("approval.question.batch_unknown", certificado))
		}
	case application.AprobacionCofirma:
		lineas = append(lineas, loc.T("approval.question.cosign", certificado))
	case application.AprobacionContrafirma:
		lineas = append(lineas, loc.T("approval.question.countersign", certificado))
	default:
		lineas = append(lineas, loc.T("approval.question.sign", certificado))
	}

	if origen := campo(d.Origen); origen != "" {
		lineas = append(lineas, loc.T("approval.requester.origin", origen))
	}
	if app := application.NeutralizarTextoAprobacion(d.Aplicacion, application.MaxRunasAprobacionSolicitud); app != "" {
		lineas = append(lineas, loc.T("approval.requester.application", app))
	}

	if documento := campo(d.Documento); documento != "" && d.Operacion != application.AprobacionLote {
		if d.Tamano > 0 {
			lineas = append(lineas, loc.T("approval.document.sized", documento, tamano(loc, d.Tamano)))
		} else {
			lineas = append(lineas, loc.T("approval.document", documento))
		}
	}
	if d.Operacion == application.AprobacionLote && len(d.Documentos) > 0 {
		nombres := make([]string, 0, maxDocumentosListados)
		for _, nombre := range d.Documentos {
			if len(nombres) == maxDocumentosListados {
				break
			}
			if n := campo(nombre); n != "" {
				nombres = append(nombres, n)
			}
		}
		if len(nombres) > 0 {
			lista := strings.Join(nombres, ", ")
			if resto := len(d.Documentos) - len(nombres); resto > 0 {
				lista = loc.T("approval.documents.more", lista, resto)
			}
			lineas = append(lineas, loc.T("approval.documents", lista))
		}
	}
	if formato := campo(d.Formato); formato != "" {
		lineas = append(lineas, loc.T("approval.format", formato))
	}
	if destinos := application.OrigenesDestino(d.Destinos...); len(destinos) > 0 {
		lineas = append(lineas, loc.T("approval.destinations", strings.Join(destinos, ", ")))
	}
	if d.OrigenNoVerificable || d.SesionNoIdentificada {
		lineas = append(lineas, "")
	}
	if d.OrigenNoVerificable {
		lineas = append(lineas, loc.T("approval.origin.unverifiable"))
	}
	if d.SesionNoIdentificada {
		lineas = append(lineas, loc.T("approval.session.unidentified"))
	}
	return strings.Join(lineas, "\n")
}

func tamano(loc ports.Localizador, bytes int) string {
	if bytes < 1024 {
		return loc.T("approval.size.bytes", bytes)
	}
	return loc.T("approval.size.kb", (bytes+1023)/1024)
}

// TextoZenity prepara un texto para el --text de zenity o qarma, que se
// llaman siempre con --no-markup: sin marcado, el único escape que siguen
// interpretando es la barra invertida (\n, \t...), que se duplica.
func TextoZenity(texto string) string {
	return strings.ReplaceAll(texto, `\`, `\\`)
}

// TextoZenityMarcado prepara un texto para el --text de los diálogos de zenity
// o qarma que no admiten --no-markup, como --list: ahí el texto se pinta como
// marcado Pango después de interpretar las secuencias con barra invertida. Se
// escapan &, <, >, comillas y apóstrofos como entidades y se duplican las
// barras, de modo que el texto externo se ve tal cual.
func TextoZenityMarcado(texto string) string {
	return TextoZenity(html.EscapeString(texto))
}

// TextoKDialog prepara un texto para kdialog, que decide por su cuenta si es
// texto enriquecido de Qt. Se fuerza el modo enriquecido con <qt>, se escapa
// todo (&, <, >, comillas) y los saltos de línea propios se pasan a <br>.
func TextoKDialog(texto string) string {
	lineas := strings.Split(texto, "\n")
	for i, linea := range lineas {
		lineas[i] = html.EscapeString(linea)
	}
	return "<qt>" + strings.Join(lineas, "<br>") + "</qt>"
}
