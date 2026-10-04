// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package eni

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Expediente electrónico ENI según la NTI de Expediente Electrónico
// (Resolución de 19 de julio de 2011, BOE-A-2011-13170, anexo II): índice con
// la huella de cada documento ENI, firmado, y metadatos del expediente.

const (
	nsExpediente       = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e"
	nsIndice           = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e/indice-e"
	nsIndiceContenido  = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e/indice-e/contenido"
	nsMetadatosExp     = "http://administracionelectronica.gob.es/ENI/XSD/v1.0/expediente-e/metadatos"
	versionNTIExp      = nsExpediente
	funcionResumenSHA2 = "http://www.w3.org/2001/04/xmlenc#sha256"
)

var reIdentificadorExp = reIdentificador

// MetadatosExpediente son los metadatos mínimos obligatorios del anexo I.
type MetadatosExpediente struct {
	Identificador string
	Organos       []string
	FechaApertura time.Time
	// Clasificacion es el código SIA del procedimiento o
	// <Órgano>_PRO_<ID> si no está en SIA.
	Clasificacion string
	// Estado: E01 abierto, E02 cerrado, E03 índice para remisión cerrado.
	Estado      string
	Interesados []string
}

// DocumentoExpediente es un documento ENI que se incorpora al índice.
type DocumentoExpediente struct {
	// XML completo del documento ENI.
	XML                []byte
	FechaIncorporacion time.Time
}

// FirmaIndice firma el contenido del índice (el elemento IndiceContenido con
// el Id indicado) y devuelve un elemento ds:Signature.
type FirmaIndice func(indiceContenido []byte, id string) ([]byte, error)

// Validar comprueba los metadatos del expediente.
func (m *MetadatosExpediente) Validar() error {
	if len(m.Organos) > 128 || len(m.Interesados) > 128 {
		return problema("expediente", "limit")
	}
	if !fechaTiempoValida(m.FechaApertura) {
		return problema("FechaAperturaExpediente", "date")
	}
	for _, in := range m.Interesados {
		if !textoSeguro(in, 128) {
			return problema("Interesado", "text")
		}
	}
	if len(m.Organos) == 0 {
		return problema("Organo", "dir3")
	}
	for _, o := range m.Organos {
		if !reDIR3.MatchString(o) {
			return problema("Organo", "dir3")
		}
	}
	if m.Identificador != "" && !reIdentificadorExp.MatchString(m.Identificador) {
		return problema("Identificador", "identifier")
	}
	if !reClasificacion.MatchString(m.Clasificacion) {
		return problema("Clasificacion", "classification")
	}
	if !slices.Contains(EstadosExpediente(), m.Estado) {
		return problema("Estado", "value")
	}

	return nil
}

// identificadorDocumentoENI extrae el identificador de un documento ENI.
func identificadorDocumentoENI(data []byte) (string, error) {
	if err := validarSalida(data); err != nil {
		return "", err
	}
	var d struct {
		XMLName   xml.Name
		Metadatos struct {
			Identificador string `xml:"Identificador"`
		} `xml:"metadatos"`
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	if err := dec.Decode(&d); err != nil {
		return "", problema("documento", "xml")
	}
	if d.XMLName.Space != nsDocumento || d.XMLName.Local != "documento" {
		return "", problema("documento", "structure")
	}
	id := strings.TrimSpace(d.Metadatos.Identificador)
	if !reIdentificador.MatchString(id) {
		return "", problema("Identificador", "identifier")
	}
	return id, nil
}

// GenerarExpediente produce el XML del expediente ENI con el índice firmado.
// canonicalizar debe devolver la canonicalización exclusiva de un XML: la
// huella de cada documento se calcula sobre ella.
func GenerarExpediente(m MetadatosExpediente, docs []DocumentoExpediente, firmar FirmaIndice, canonicalizar func([]byte) ([]byte, error), ahora time.Time) ([]byte, error) {
	if err := m.Validar(); err != nil {
		return nil, err
	}
	if len(docs) == 0 || len(docs) > 128 {
		return nil, problema("DocumentoIndizado", "limit")
	}
	if firmar == nil || canonicalizar == nil {
		return nil, problema("firmas", "structure")
	}
	if m.Identificador == "" {
		sufijo, err := identificadorAleatorio(26)
		if err != nil {
			return nil, err
		}
		m.Identificador = fmt.Sprintf("ES_%s_%04d_EXP_%s", m.Organos[0], ahora.Year(), sufijo)
	}
	apertura := m.FechaApertura
	if apertura.IsZero() {
		apertura = ahora
	}
	idIndiceContenido := "INDICE_CONTENIDO_" + m.Identificador

	var ic bytes.Buffer
	// IndiceContenido es un elemento local de TipoIndice (espacio de nombres
	// del índice); sus hijos pertenecen al del contenido del índice. Declara
	// sus propios prefijos para que la firma no dependa de los ancestros.
	fmt.Fprintf(&ic, `<eniexpind:IndiceContenido xmlns:eniexpind="%s" xmlns:eniconexpind="%s" Id="%s">`, nsIndice, nsIndiceContenido, esc(idIndiceContenido))
	fmt.Fprintf(&ic, `<eniconexpind:FechaIndiceElectronico>%s</eniconexpind:FechaIndiceElectronico>`, ahora.Format(time.RFC3339))
	vistos := map[string]bool{}
	var total int64
	for i, d := range docs {
		total += int64(len(d.XML))
		if total > 256*1024*1024 {
			return nil, problema("DocumentoIndizado", "limit")
		}
		if !fechaTiempoValida(d.FechaIncorporacion) {
			return nil, problema("FechaIncorporacionExpediente", "date")
		}
		id, err := identificadorDocumentoENI(d.XML)
		if err != nil {
			return nil, fmt.Errorf("DocumentoIndizado[%d]: %w", i+1, err)
		}
		if vistos[id] {
			return nil, problema(fmt.Sprintf("DocumentoIndizado[%d]/IdentificadorDocumento", i+1), "value")
		}
		vistos[id] = true
		canon, err := canonicalizar(d.XML)
		if err != nil {
			return nil, fmt.Errorf("DocumentoIndizado[%d]: %w", i+1, err)
		}
		huella := sha256.Sum256(canon)
		fecha := d.FechaIncorporacion
		if fecha.IsZero() {
			fecha = ahora
		}
		fmt.Fprintf(&ic, `<eniconexpind:DocumentoIndizado Id="DOC_%d_%s">`+
			`<eniconexpind:IdentificadorDocumento>%s</eniconexpind:IdentificadorDocumento>`+
			`<eniconexpind:ValorHuella>%s</eniconexpind:ValorHuella>`+
			`<eniconexpind:FuncionResumen>%s</eniconexpind:FuncionResumen>`+
			`<eniconexpind:FechaIncorporacionExpediente>%s</eniconexpind:FechaIncorporacionExpediente>`+
			`<eniconexpind:OrdenDocumentoExpediente>%s</eniconexpind:OrdenDocumentoExpediente>`+
			`</eniconexpind:DocumentoIndizado>`,
			i+1, esc(m.Identificador), esc(id), hex.EncodeToString(huella[:]), funcionResumenSHA2, fecha.Format(time.RFC3339), strconv.Itoa(i+1))
	}
	ic.WriteString(`</eniexpind:IndiceContenido>`)

	firma, err := firmar(ic.Bytes(), idIndiceContenido)
	if err != nil {
		return nil, fmt.Errorf("indice/firmas: %w", err)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(firma), []byte("<ds:Signature")) {
		return nil, problema("indice/firmas/Signature", "structure")
	}

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<eniexp:expediente xmlns:eniexp="%s" xmlns:eniexpind="%s" xmlns:eniexpmeta="%s" xmlns:enids="%s" Id="%s">`,
		nsExpediente, nsIndice, nsMetadatosExp, nsFirma, esc(m.Identificador))
	fmt.Fprintf(&b, `<eniexpind:indice Id="INDICE_%s">`, esc(m.Identificador))
	b.Write(ic.Bytes())
	fmt.Fprintf(&b, `<enids:firmas><enids:firma Id="FIRMA_INDICE" ref="%s"><enids:TipoFirma>%s</enids:TipoFirma><enids:ContenidoFirma><enids:FirmaConCertificado>`,
		esc(idIndiceContenido), FirmaXAdESDetached)
	b.Write(bytes.TrimSpace(firma))
	b.WriteString(`</enids:FirmaConCertificado></enids:ContenidoFirma></enids:firma></enids:firmas></eniexpind:indice>`)
	b.WriteString(`<eniexpmeta:metadatosExp>`)
	fmt.Fprintf(&b, `<eniexpmeta:VersionNTI>%s</eniexpmeta:VersionNTI>`, versionNTIExp)
	fmt.Fprintf(&b, `<eniexpmeta:Identificador>%s</eniexpmeta:Identificador>`, esc(m.Identificador))
	for _, o := range m.Organos {
		fmt.Fprintf(&b, `<eniexpmeta:Organo>%s</eniexpmeta:Organo>`, esc(o))
	}
	fmt.Fprintf(&b, `<eniexpmeta:FechaAperturaExpediente>%s</eniexpmeta:FechaAperturaExpediente>`, apertura.Format(time.RFC3339))
	fmt.Fprintf(&b, `<eniexpmeta:Clasificacion>%s</eniexpmeta:Clasificacion>`, esc(m.Clasificacion))
	fmt.Fprintf(&b, `<eniexpmeta:Estado>%s</eniexpmeta:Estado>`, m.Estado)
	for _, in := range m.Interesados {
		if in = strings.TrimSpace(in); in != "" {
			fmt.Fprintf(&b, `<eniexpmeta:Interesado>%s</eniexpmeta:Interesado>`, esc(in))
		}
	}
	b.WriteString(`</eniexpmeta:metadatosExp></eniexp:expediente>` + "\n")
	if err := validarSalida(b.Bytes()); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
