// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
)

// Cofirma y contrafirma XAdES con la estructura de AutoFirma Java:
//   - Cofirma: otra ds:Signature sobre los mismos datos. En Detached y
//     Enveloping las firmas conviven bajo <AFIRMA>; en Enveloped se añade
//     dentro del XML (la exclusión XPath mantiene válidas las anteriores).
//   - Contrafirma: una ds:Signature dentro de
//     xades:UnsignedSignatureProperties/xades:CounterSignature de cada firma
//     hoja, que firma su ds:SignatureValue (tipo CountersignedSignature).
//
// El documento existente se modifica solo insertando texto en posiciones
// obtenidas del parser: nunca se re-serializa lo que ya firmó otra persona.

const (
	typeCountersignedSignature = "http://uri.etsi.org/01903#CountersignedSignature"
	maxContrafirmasXAdES       = 64
)

func nuevoIDXAdES(base string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base + "-" + hex.EncodeToString(b[:])
}

func idsXAdESNuevos() xadesIDs {
	sig := nuevoIDXAdES("Signature")
	return xadesIDs{sig: sig, keyInfo: sig + "-KeyInfo", signedProps: sig + "-SignedProperties", reference: sig + "-Reference"}
}

// ensamblarFirmaXAdES compone la ds:Signature final con sus objetos.
func ensamblarFirmaXAdES(ids xadesIDs, namespace, signedInfoXML, sigValueB64, keyInfoXML, objetosXML, signedPropsXML string) string {
	if signedPropsXML == "" {
		// XMLDSig pura: sin propiedades cualificadas.
		return fmt.Sprintf(`<ds:Signature xmlns:ds="%s" Id="%s">%s<ds:SignatureValue Id="%s-SignatureValue">%s</ds:SignatureValue>%s%s</ds:Signature>`,
			nsXMLDSig, ids.sig, signedInfoXML, ids.sig, sigValueB64, keyInfoXML, objetosXML)
	}
	return fmt.Sprintf(`<ds:Signature xmlns:ds="%s" Id="%s">%s<ds:SignatureValue Id="%s-SignatureValue">%s</ds:SignatureValue>%s%s`+
		`<ds:Object><xades:QualifyingProperties xmlns:xades="%s" Id="%s-QualifyingProperties" Target="#%s">%s</xades:QualifyingProperties></ds:Object>`+
		`</ds:Signature>`,
		nsXMLDSig, ids.sig, signedInfoXML, ids.sig, sigValueB64, keyInfoXML, objetosXML,
		namespace, ids.sig, ids.sig, signedPropsXML)
}

type documentoXAdES struct {
	data  []byte
	spans []xmlElementSpan
}

func analizarDocumentoXAdES(data []byte) (documentoXAdES, error) {
	spans, err := scanXMLElements(data)
	if err != nil || len(spans) == 0 {
		return documentoXAdES{}, fmt.Errorf("los datos no son un XML de firma válido: %v", err)
	}
	return documentoXAdES{data: data, spans: spans}, nil
}

func esFirmaXML(s xmlElementSpan) bool {
	return s.name.Space == nsXMLDSig && s.name.Local == "Signature"
}

func contiene(padre, hijo xmlElementSpan) bool {
	return hijo.start > padre.start && hijo.end <= padre.end
}

// firmaPropietaria devuelve el índice de la ds:Signature más interna que
// contiene al elemento, o -1.
func (d documentoXAdES) firmaPropietaria(i int) int {
	mejor := -1
	for j, s := range d.spans {
		if j != i && esFirmaXML(s) && contiene(s, d.spans[i]) {
			if mejor < 0 || s.start > d.spans[mejor].start {
				mejor = j
			}
		}
	}
	return mejor
}

func (d documentoXAdES) firmasPrincipales() []int {
	var out []int
	for i, s := range d.spans {
		if esFirmaXML(s) && d.firmaPropietaria(i) < 0 {
			out = append(out, i)
		}
	}
	return out
}

// hijoDirecto busca el primer descendiente con ese nombre local cuya firma
// propietaria sea la indicada (no entra en contrafirmas anidadas).
func (d documentoXAdES) descendientePropio(firma int, local string) int {
	for i, s := range d.spans {
		if s.name.Local == local && contiene(d.spans[firma], s) && d.firmaPropietaria(i) == firma {
			return i
		}
	}
	return -1
}

func (d documentoXAdES) literal(i int) string {
	return string(d.data[d.spans[i].start:d.spans[i].end])
}

func (d documentoXAdES) etiquetaApertura(i int) string {
	lit := d.literal(i)
	if fin := strings.Index(lit, ">"); fin >= 0 {
		return lit[:fin+1]
	}
	return lit
}

func atributoEtiqueta(etiqueta, nombre string) string {
	decoder := xml.NewDecoder(strings.NewReader(etiqueta + "</x>"))
	decoder.Strict = false
	tok, err := decoder.Token()
	if err != nil {
		return ""
	}
	if start, ok := tok.(xml.StartElement); ok {
		for _, a := range start.Attr {
			if a.Name.Local == nombre {
				return a.Value
			}
		}
	}
	return ""
}

func prefijoEtiqueta(etiqueta string) string {
	nombre := strings.TrimPrefix(etiqueta, "<")
	if i := strings.IndexAny(nombre, " \t\r\n/>"); i >= 0 {
		nombre = nombre[:i]
	}
	if i := strings.Index(nombre, ":"); i >= 0 {
		return nombre[:i+1]
	}
	return ""
}

// insertarAntesDelCierre inserta texto justo antes de la etiqueta de cierre
// del elemento indicado.
func (d documentoXAdES) insertarAntesDelCierre(i int, texto string) ([]byte, error) {
	span := d.spans[i]
	lit := d.literal(i)
	if strings.HasSuffix(lit, "/>") {
		return nil, errors.New("elemento vacío sin etiqueta de cierre")
	}
	cierre := int(span.start) + strings.LastIndex(lit, "</")
	out := make([]byte, 0, len(d.data)+len(texto))
	out = append(out, d.data[:cierre]...)
	out = append(out, texto...)
	return append(out, d.data[cierre:]...), nil
}

func cosignXAdES(job domain.SignatureJob, key *LocalSigningKey) ([]byte, string, error) {
	doc, err := analizarDocumentoXAdES(job.Document.Content)
	if err != nil {
		return nil, "", err
	}
	principales := doc.firmasPrincipales()
	if len(principales) == 0 {
		return nil, "", errors.New("los datos no contienen ninguna firma XAdES que cofirmar")
	}
	raiz := doc.spans[0]
	switch {
	case !esFirmaXML(raiz) && raiz.name.Local != "AFIRMA":
		// Enveloped: nueva firma dentro del mismo XML.
		return buildXAdESEnvelopConIDs(job, key, xadesVarianteEnveloped, idsXAdESNuevos())
	case raiz.name.Local == "AFIRMA" && doc.hijoContent() >= 0:
		return cofirmarDetached(job, key, doc)
	default:
		return cofirmarEnveloping(job, key, doc, principales[0])
	}
}

func (d documentoXAdES) hijoContent() int {
	for i, s := range d.spans {
		if s.name.Local == "CONTENT" && d.firmaPropietaria(i) < 0 {
			return i
		}
	}
	return -1
}

func cofirmarDetached(job domain.SignatureJob, key *LocalSigningKey, doc documentoXAdES) ([]byte, string, error) {
	algOpts, err := resolveXAdESAlgorithmOptions(job.Options)
	if err != nil {
		return nil, "", err
	}
	buildOpts := resolveXAdESBuildOptions(job.Options, algOpts)
	contenido := doc.hijoContent()
	apertura := doc.etiquetaApertura(contenido)
	id := atributoEtiqueta(apertura, "Id")
	mime := atributoEtiqueta(apertura, "MimeType")
	if mime == "" {
		mime = "application/octet-stream"
	}
	var (
		documentURI string
		digest      []byte
	)
	switch {
	case id != "" && strings.EqualFold(atributoEtiqueta(apertura, "Encoding"), algBase64Transform):
		datos, err := decodeBase64TransformInput([]byte(doc.literal(contenido)))
		if err != nil {
			return nil, "", err
		}
		digest, err = digestBytes(algOpts.Hash, datos)
		if err != nil {
			return nil, "", err
		}
		buildOpts.DocumentTransforms = []string{algBase64Transform}
		documentURI = "#" + id
	case id != "":
		canonical, err := canonicalizeElementByIDInDocument(doc.data, id, algC14N)
		if err != nil {
			return nil, "", err
		}
		digest, err = digestBytes(algOpts.Hash, canonical)
		if err != nil {
			return nil, "", err
		}
		buildOpts.DocumentTransforms = []string{algC14N}
		documentURI = "#" + id
	default:
		// Detached de V2 con referencia externa por nombre (CONTENT Name).
		nombre := atributoEtiqueta(apertura, "Name")
		datos, err := decodeBase64TransformInput([]byte(doc.literal(contenido)))
		if err != nil || nombre == "" {
			return nil, "", errors.New("contenido detached sin identificador utilizable")
		}
		canonical, err := canonicalizeXML(datos, algExcC14N)
		if err != nil {
			return nil, "", err
		}
		if digest, err = digestBytes(algOpts.Hash, canonical); err != nil {
			return nil, "", err
		}
		documentURI = nombre
	}
	buildOpts.DocumentReferenceType = typeXMLDSigObjectRef
	ids := idsXAdESNuevos()
	signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML, err := firmarPartesXAdES(
		key, algOpts, buildOpts, ids, documentURI, base64.StdEncoding.EncodeToString(digest), mime)
	if err != nil {
		return nil, "", err
	}
	firma := ensamblarFirmaXAdES(ids, buildOpts.Namespace, signedInfoXML, sigValueB64, keyInfoXML, "", signedPropsXML)
	out, err := doc.insertarAntesDelCierre(0, firma)
	return out, buildOpts.Algorithm.Label, err
}

func cofirmarEnveloping(job domain.SignatureJob, key *LocalSigningKey, doc documentoXAdES, firma int) ([]byte, string, error) {
	datos, mime, err := doc.datosEnveloping(firma)
	if err != nil {
		return nil, "", err
	}
	copia := job
	copia.Document.Content = datos
	copia.Document.MIMEType = mime
	nueva, etiqueta, err := buildXAdESEnvelopConIDs(copia, key, xadesVarianteEnveloping, idsXAdESNuevos())
	if err != nil {
		return nil, "", err
	}
	nuevaSinDeclaracion := stripXMLDeclaration(nueva)
	if doc.spans[0].name.Local == "AFIRMA" {
		out, err := doc.insertarAntesDelCierre(0, nuevaSinDeclaracion)
		return out, etiqueta, err
	}
	// Firma enveloping suelta: Java agrupa ambas bajo una raíz AFIRMA.
	out := `<?xml version="1.0" encoding="UTF-8"?><AFIRMA Id="` + nuevoIDXAdES("AfirmaRoot") + `">` +
		stripXMLDeclaration(doc.data) + nuevaSinDeclaracion + `</AFIRMA>`
	return []byte(out), etiqueta, nil
}

// datosEnveloping recupera los datos firmados del ds:Object referenciado por
// la primera referencia de la firma.
func (d documentoXAdES) datosEnveloping(firma int) ([]byte, string, error) {
	signedInfo := d.descendientePropio(firma, "SignedInfo")
	if signedInfo < 0 {
		return nil, "", errors.New("firma sin SignedInfo")
	}
	var info signedInfoForVerify
	if err := xml.Unmarshal([]byte(d.literal(signedInfo)), &info); err != nil || len(info.References) == 0 {
		return nil, "", errors.New("firma sin referencias")
	}
	for _, ref := range info.References {
		uri := strings.TrimSpace(ref.URI)
		if !strings.HasPrefix(uri, "#") {
			continue
		}
		objetivo, err := uniqueElementByID(d.spans, strings.TrimPrefix(uri, "#"))
		if err != nil || objetivo.name.Local != "Object" {
			continue
		}
		lit := string(d.data[objetivo.start:objetivo.end])
		apertura := lit[:strings.Index(lit, ">")+1]
		mime := atributoEtiqueta(apertura, "MimeType")
		if strings.EqualFold(atributoEtiqueta(apertura, "Encoding"), algBase64Transform) {
			datos, err := decodeBase64TransformInput([]byte(lit))
			return datos, mime, err
		}
		interior := strings.TrimSuffix(lit[len(apertura):], lit[strings.LastIndex(lit, "</"):])
		return []byte(strings.TrimSpace(interior)), mime, nil
	}
	return nil, "", errors.New("no se localizan los datos firmados de la firma enveloping")
}

func countersignXAdES(job domain.SignatureJob, key *LocalSigningKey) ([]byte, string, error) {
	doc, err := analizarDocumentoXAdES(job.Document.Content)
	if err != nil {
		return nil, "", err
	}
	var hojas []string
	for i, s := range doc.spans {
		if !esFirmaXML(s) {
			continue
		}
		if doc.descendientePropio(i, "CounterSignature") >= 0 {
			continue
		}
		if s.id == "" {
			return nil, "", errors.New("una firma sin atributo Id no puede contrafirmarse")
		}
		hojas = append(hojas, s.id)
	}
	if len(hojas) == 0 {
		return nil, "", errors.New("los datos no contienen ninguna firma XAdES que contrafirmar")
	}
	if len(hojas) > maxContrafirmasXAdES {
		return nil, "", errors.New("demasiadas firmas que contrafirmar")
	}
	etiqueta := ""
	for _, id := range hojas {
		if doc, etiqueta, err = contrafirmarFirma(job, key, doc, id); err != nil {
			return nil, "", err
		}
	}
	return doc.data, etiqueta, nil
}

func contrafirmarFirma(job domain.SignatureJob, key *LocalSigningKey, doc documentoXAdES, firmaID string) (documentoXAdES, string, error) {
	localizar := func(d documentoXAdES) (int, error) {
		for i, s := range d.spans {
			if esFirmaXML(s) && s.id == firmaID {
				return i, nil
			}
		}
		return -1, fmt.Errorf("firma %q no encontrada", firmaID)
	}
	firma, err := localizar(doc)
	if err != nil {
		return doc, "", err
	}
	valor := doc.descendientePropio(firma, "SignatureValue")
	if valor < 0 {
		return doc, "", errors.New("firma sin SignatureValue")
	}
	valorID := doc.spans[valor].id
	if valorID == "" {
		// Java identifica el SignatureValue; si falta, se añade el Id (el
		// SignatureValue no forma parte de lo firmado).
		valorID = firmaID + "-SignatureValue"
		apertura := doc.etiquetaApertura(valor)
		nombre := strings.TrimSuffix(strings.Fields(strings.TrimPrefix(apertura, "<"))[0], ">")
		pos := int(doc.spans[valor].start) + 1 + len(nombre)
		datos := append(append(append([]byte(nil), doc.data[:pos]...), ` Id="`+valorID+`"`...), doc.data[pos:]...)
		if doc, err = analizarDocumentoXAdES(datos); err != nil {
			return doc, "", err
		}
		if firma, err = localizar(doc); err != nil {
			return doc, "", err
		}
	}

	algOpts, err := resolveXAdESAlgorithmOptions(job.Options)
	if err != nil {
		return doc, "", err
	}
	buildOpts := resolveXAdESBuildOptions(job.Options, algOpts)
	canonical, err := canonicalizeElementByIDInDocument(doc.data, valorID, algC14N)
	if err != nil {
		return doc, "", err
	}
	digest, err := digestBytes(algOpts.Hash, canonical)
	if err != nil {
		return doc, "", err
	}
	buildOpts.DocumentTransforms = []string{algC14N}
	buildOpts.DocumentReferenceType = typeCountersignedSignature
	ids := idsXAdESNuevos()
	signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML, err := firmarPartesXAdES(
		key, algOpts, buildOpts, ids, "#"+valorID, base64.StdEncoding.EncodeToString(digest), "text/xml")
	if err != nil {
		return doc, "", err
	}
	contrafirma := ensamblarFirmaXAdES(ids, buildOpts.Namespace, signedInfoXML, sigValueB64, keyInfoXML, "", signedPropsXML)

	qp := doc.descendientePropio(firma, "QualifyingProperties")
	if qp < 0 {
		return doc, "", errors.New("la firma no es XAdES: falta QualifyingProperties")
	}
	p := prefijoEtiqueta(doc.etiquetaApertura(qp))
	var datos []byte
	switch usp, up := doc.descendientePropio(firma, "UnsignedSignatureProperties"), doc.descendientePropio(firma, "UnsignedProperties"); {
	case usp >= 0:
		datos, err = doc.insertarAntesDelCierre(usp, "<"+p+"CounterSignature>"+contrafirma+"</"+p+"CounterSignature>")
	case up >= 0:
		datos, err = doc.insertarAntesDelCierre(up, "<"+p+"UnsignedSignatureProperties><"+p+"CounterSignature>"+contrafirma+
			"</"+p+"CounterSignature></"+p+"UnsignedSignatureProperties>")
	default:
		datos, err = doc.insertarAntesDelCierre(qp, "<"+p+"UnsignedProperties><"+p+"UnsignedSignatureProperties><"+p+"CounterSignature>"+
			contrafirma+"</"+p+"CounterSignature></"+p+"UnsignedSignatureProperties></"+p+"UnsignedProperties>")
	}
	if err != nil {
		return doc, "", err
	}
	nuevo, err := analizarDocumentoXAdES(datos)
	return nuevo, buildOpts.Algorithm.Label, err
}
