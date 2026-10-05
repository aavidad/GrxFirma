// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package eni genera documentos electrónicos ENI según la Norma Técnica de
// Interoperabilidad de Documento Electrónico (Resolución de 19 de julio de
// 2011, BOE-A-2011-13169, anexo II): contenido, metadatos obligatorios y
// firmas, para intercambiarlos con otras Administraciones o incorporarlos a
// un expediente electrónico.
package eni

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	nsDocumento = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/documento-e"
	nsContenido = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/documento-e/contenido"
	nsMetadatos = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/documento-e/metadatos"
	nsFirma     = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/firma"
	// VersionNTI es el valor del metadato VersionNTI del documento ENI 1.0.
	VersionNTI = nsDocumento
)

// TipoFirma son los valores del anexo II de la NTI.
type TipoFirma string

const (
	FirmaXAdESDetached TipoFirma = "TF02"
	FirmaXAdESEnvelope TipoFirma = "TF03"
	FirmaCAdESExplicit TipoFirma = "TF04"
	FirmaCAdESImplicit TipoFirma = "TF05"
	FirmaPAdES         TipoFirma = "TF06"
)

// Metadatos obligatorios del documento electrónico.
type Metadatos struct {
	// Identificador con el formato ES_<Órgano>_<AAAA>_<ID específico>; si
	// está vacío se genera uno aleatorio.
	Identificador string
	// Organos son los códigos DIR3 de los órganos responsables.
	Organos []string
	// FechaCaptura; si es cero se usa la fecha actual.
	FechaCaptura time.Time
	// OrigenAdministracion es true si el documento lo genera la
	// Administración y false si lo aporta el ciudadano.
	OrigenAdministracion bool
	// EstadoElaboracion: EE01 original, EE02-EE04 copias auténticas, EE99 otros.
	EstadoElaboracion string
	// IdentificadorDocumentoOrigen es obligatorio para las copias (EE02-EE04).
	IdentificadorDocumentoOrigen string
	// TipoDocumental: TD01-TD20 o TD99.
	TipoDocumental string
}

// Firma incluida en el documento.
type Firma struct {
	Tipo TipoFirma
	// Datos de la firma separada (TF02, TF04) o que contiene el documento
	// (TF03, TF05). En PAdES (TF06) la firma está dentro del contenido.
	Datos []byte
}

// Documento reúne lo necesario para generar el documento ENI.
type Documento struct {
	Contenido     []byte
	NombreFormato string
	Metadatos     Metadatos
	Firmas        []Firma
}

var (
	reDIR3          = regexp.MustCompile(`^[A-Z][0-9]{8}$`)
	reIdentificador = regexp.MustCompile(`^ES_([A-Z][0-9]{8})_(\d{4})_([A-Za-z0-9_]{1,30})$`)
	reFormato       = regexp.MustCompile(`^[A-Za-z0-9.+-]{1,32}$`)
	tiposFirma      = map[TipoFirma]bool{FirmaXAdESDetached: true, FirmaXAdESEnvelope: true, FirmaCAdESExplicit: true, FirmaCAdESImplicit: true, FirmaPAdES: true}
)

// TipoDocumentalValido indica si el código es un tipo documental de la NTI.
func TipoDocumentalValido(td string) bool {
	return slices.Contains(TiposDocumentales(), td)
}

// Validar comprueba los metadatos y firmas antes de generar el documento.
func (d *Documento) Validar() error {
	m := &d.Metadatos
	if len(d.Contenido) > MaxContenidoBytes || len(m.Organos) > 128 || len(d.Firmas) > 128 {
		return problema("documento", "limit")
	}
	if !fechaTiempoValida(m.FechaCaptura) {
		return problema("FechaCaptura", "date")
	}
	if m.IdentificadorDocumentoOrigen != "" && !reIdentificador.MatchString(m.IdentificadorDocumentoOrigen) {
		return problema("IdentificadorDocumentoOrigen", "source")
	}
	total := int64(len(d.Contenido))
	for _, f := range d.Firmas {
		total += int64(len(f.Datos))
		if total > MaxContenidoBytes {
			return problema("firma", "limit")
		}
	}
	if len(d.Contenido) == 0 {
		return problema("contenido", "structure")
	}
	if !reFormato.MatchString(d.NombreFormato) {
		return problema("NombreFormato", "format")
	}
	if len(m.Organos) == 0 {
		return problema("Organo", "dir3")
	}
	for _, o := range m.Organos {
		if !reDIR3.MatchString(o) {
			return problema("Organo", "dir3")
		}
	}
	if m.Identificador != "" {
		if !reIdentificador.MatchString(m.Identificador) {
			return problema("Identificador", "identifier")
		}
	}
	if !slices.Contains(EstadosElaboracion(), m.EstadoElaboracion) {
		return problema("EstadoElaboracion", "value")
	}
	copia := m.EstadoElaboracion == "EE02" || m.EstadoElaboracion == "EE03" || m.EstadoElaboracion == "EE04"
	if copia && strings.TrimSpace(m.IdentificadorDocumentoOrigen) == "" {
		return problema("IdentificadorDocumentoOrigen", "source")
	}
	if !TipoDocumentalValido(m.TipoDocumental) {
		return problema("TipoDocumental", "value")
	}
	for i, f := range d.Firmas {
		if !tiposFirma[f.Tipo] {
			return problema(fmt.Sprintf("firma[%d]/TipoFirma", i+1), "value")
		}
		if f.Tipo != FirmaPAdES && len(f.Datos) == 0 {
			return problema(fmt.Sprintf("firma[%d]/Datos", i+1), "structure")
		}
	}
	return nil
}

// Generar produce el XML del documento ENI.
func Generar(d Documento, ahora time.Time) ([]byte, error) {
	if err := d.Validar(); err != nil {
		return nil, err
	}
	m := d.Metadatos
	if m.Identificador == "" {
		sufijo, err := identificadorAleatorio(30)
		if err != nil {
			return nil, err
		}
		m.Identificador = fmt.Sprintf("ES_%s_%04d_%s", m.Organos[0], ahora.Year(), sufijo)
	}
	fecha := m.FechaCaptura
	if fecha.IsZero() {
		fecha = ahora
	}
	idContenido := "CONTENIDO_" + m.Identificador

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<enidoc:documento xmlns:enidoc="%s" xmlns:enifile="%s" xmlns:enidocmeta="%s" xmlns:enids="%s" Id="%s">`,
		nsDocumento, nsContenido, nsMetadatos, nsFirma, esc(m.Identificador))
	fmt.Fprintf(&b, `<enifile:contenido Id="%s"><enifile:ValorBinario>%s</enifile:ValorBinario><enifile:NombreFormato>%s</enifile:NombreFormato></enifile:contenido>`,
		esc(idContenido), base64.StdEncoding.EncodeToString(d.Contenido), esc(d.NombreFormato))
	b.WriteString(`<enidocmeta:metadatos>`)
	fmt.Fprintf(&b, `<enidocmeta:VersionNTI>%s</enidocmeta:VersionNTI>`, VersionNTI)
	fmt.Fprintf(&b, `<enidocmeta:Identificador>%s</enidocmeta:Identificador>`, esc(m.Identificador))
	for _, o := range m.Organos {
		fmt.Fprintf(&b, `<enidocmeta:Organo>%s</enidocmeta:Organo>`, esc(o))
	}
	fmt.Fprintf(&b, `<enidocmeta:FechaCaptura>%s</enidocmeta:FechaCaptura>`, fecha.Format(time.RFC3339))
	fmt.Fprintf(&b, `<enidocmeta:OrigenCiudadanoAdministracion>%t</enidocmeta:OrigenCiudadanoAdministracion>`, m.OrigenAdministracion)
	fmt.Fprintf(&b, `<enidocmeta:EstadoElaboracion><enidocmeta:ValorEstadoElaboracion>%s</enidocmeta:ValorEstadoElaboracion>`, m.EstadoElaboracion)
	if origen := strings.TrimSpace(m.IdentificadorDocumentoOrigen); origen != "" {
		fmt.Fprintf(&b, `<enidocmeta:IdentificadorDocumentoOrigen>%s</enidocmeta:IdentificadorDocumentoOrigen>`, esc(origen))
	}
	b.WriteString(`</enidocmeta:EstadoElaboracion>`)
	fmt.Fprintf(&b, `<enidocmeta:TipoDocumental>%s</enidocmeta:TipoDocumental>`, m.TipoDocumental)
	b.WriteString(`</enidocmeta:metadatos>`)
	if len(d.Firmas) > 0 {
		b.WriteString(`<enids:firmas>`)
		for i, f := range d.Firmas {
			fmt.Fprintf(&b, `<enids:firma Id="FIRMA_%d" ref="%s"><enids:TipoFirma>%s</enids:TipoFirma><enids:ContenidoFirma><enids:FirmaConCertificado>`,
				i+1, esc(idContenido), f.Tipo)
			if f.Tipo == FirmaPAdES && len(f.Datos) == 0 {
				// La firma PAdES está dentro del PDF del contenido.
				fmt.Fprintf(&b, `<enids:ReferenciaFirma>#%s</enids:ReferenciaFirma>`, esc(idContenido))
			} else {
				fmt.Fprintf(&b, `<enids:FirmaBase64>%s</enids:FirmaBase64>`, base64.StdEncoding.EncodeToString(f.Datos))
			}
			b.WriteString(`</enids:FirmaConCertificado></enids:ContenidoFirma></enids:firma>`)
		}
		b.WriteString(`</enids:firmas>`)
	}
	b.WriteString(`</enidoc:documento>` + "\n")
	if err := validarSalida(b.Bytes()); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func identificadorAleatorio(n int) (string, error) {
	const alfabeto = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alfabeto[int(buf[i])%len(alfabeto)]
	}
	return string(buf), nil
}
