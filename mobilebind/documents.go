// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/eni"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
)

// Veri*Factu, ENI y la leyenda CSV devuelven claves de localización cerradas
// (verifactu.*, eni.validacion.*, eni.error.*, csv.error.*). La UI Android
// las traduce con sus recursos; el núcleo no envía textos para mostrar.

const (
	maxVeriFactuFiles     = 64
	maxVeriFactuJSONBytes = 45 << 20
	// maxENIInputBytes suma firma y original: una firma implícita o PAdES
	// llega sola y una separada (CAdES explícita, XAdES) es pequeña frente a
	// su original de hasta maxDocumentBytes.
	maxENIInputBytes = maxSignedDocumentBytes
	// maxENIJSONBytes es el Base64 de maxENIInputBytes (4/3) más 1 MiB de
	// metadatos: 65 MiB, antes 108 MiB.
	maxENIJSONBytes        = maxENIInputBytes/3*4 + 1<<20
	maxENIOrgans           = 16
	maxIssueFieldRunes     = 160
	eniErrorUnsignedPDF    = "eni.error.unsigned_pdf"
	eniErrorExplicitCAdES  = "eni.error.explicit_cades"
	eniErrorUnrecognized   = "eni.error.unrecognized"
	eniErrorContentFormat  = "eni.error.content_format"
	eniErrorOrigin         = "eni.error.origin"
	eniErrorMismatch       = "eni.error.signature_mismatch"
	csvErrorCodeMissing    = "csv.error.code_missing"
	csvErrorCodeInvalid    = "csv.error.code_invalid"
	csvErrorURLMissing     = "csv.error.url_missing"
	csvErrorURLInvalid     = "csv.error.url_invalid"
	csvErrorTextInvalid    = "csv.error.text_invalid"
	maxCSVCodeBytes        = 128
	maxCSVTextBytes        = 512
	maxCSVJSONBytes        = 8 << 10
	maxENIIdentifierBytes  = 64
	maxENIContentNameBytes = 32
)

type veriFactuFileRequest struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
}

type veriFactuValidateRequest struct {
	Files []veriFactuFileRequest `json:"files"`
}

type issueResponse struct {
	Field string `json:"field"`
	Key   string `json:"key"`
	Level string `json:"level,omitempty"`
}

type veriFactuRecordResponse struct {
	File           string          `json:"file"`
	Type           string          `json:"type"`
	Hash           string          `json:"hash"`
	CalculatedHash string          `json:"calculated_hash"`
	PreviousHash   string          `json:"previous_hash"`
	Signed         bool            `json:"signed"`
	Valid          bool            `json:"valid"`
	Issues         []issueResponse `json:"issues"`
}

type veriFactuValidateResponse struct {
	Valid    bool                      `json:"valid"`
	Errors   int                       `json:"errors"`
	Warnings int                       `json:"warnings"`
	Records  []veriFactuRecordResponse `json:"records"`
}

// ValidateVeriFactuJSON comprueba registros Veri*Factu aportados (estructura,
// huella, encadenamiento y firma) con el motor del escritorio. No consulta a
// la AEAT ni accede a la red.
func (f *Facade) ValidateVeriFactuJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("Veri*Factu")
	}
	var req veriFactuValidateRequest
	if err := decodeJSONStrict(payload, maxVeriFactuJSONBytes, "Veri*Factu", &req); err != nil {
		return "", err
	}
	if len(req.Files) == 0 || len(req.Files) > maxVeriFactuFiles {
		return "", newFacadeError(verifactuKeyPrefix + "limit")
	}
	files := make(map[string][]byte, len(req.Files))
	defer func() {
		for _, data := range files {
			zeroBytes(data)
		}
	}()
	total := 0
	for _, file := range req.Files {
		if err := validateDocumentMetadata(file.Name, "application/xml"); err != nil {
			return "", err
		}
		data, err := decodeBase64Limited(file.ContentBase64, "content_base64", commonsigner.VeriFactuMaxXMLBytes)
		if err != nil {
			return "", newFacadeError(verifactuKeyPrefix + "limit")
		}
		total += len(data)
		if total > maxDocumentBytes {
			zeroBytes(data)
			return "", newFacadeError(verifactuKeyPrefix + "limit")
		}
		files[uniqueName(files, file.Name)] = data
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	result := commonsigner.ValidarRegistrosVeriFactu(ctx, files)
	response := veriFactuValidateResponse{
		Valid:    result.Valid,
		Errors:   result.Errors,
		Warnings: result.Warnings,
		Records:  make([]veriFactuRecordResponse, 0, len(result.Records)),
	}
	for _, record := range result.Records {
		issues := make([]issueResponse, 0, len(record.Issues))
		for _, issue := range record.Issues {
			issues = append(issues, issueResponse{
				Field: sanitizeOutputText(issue.Field, maxIssueFieldRunes),
				Key:   sanitizeOutputText(issue.Key, 64),
				Level: sanitizeOutputText(issue.Level, 16),
			})
		}
		response.Records = append(response.Records, veriFactuRecordResponse{
			File:           sanitizeOutputText(record.File, 200),
			Type:           sanitizeOutputText(record.Type, 64),
			Hash:           sanitizeOutputText(record.Hash, 128),
			CalculatedHash: sanitizeOutputText(record.CalculatedHash, 128),
			PreviousHash:   sanitizeOutputText(record.PreviousHash, 128),
			Signed:         record.Signed,
			Valid:          record.Valid,
			Issues:         issues,
		})
	}
	return marshal(response)
}

// uniqueName evita que dos ficheros con el mismo nombre se pisen en el mapa
// del motor; el sufijo mantiene el orden alfabético del informe.
func uniqueName(files map[string][]byte, name string) string {
	if _, exists := files[name]; !exists {
		return name
	}
	for index := 2; ; index++ {
		candidate := name + " (" + strconv.Itoa(index) + ")"
		if _, exists := files[candidate]; !exists {
			return candidate
		}
	}
}

type eniDocumentRequest struct {
	SignatureBase64  string   `json:"signature_base64"`
	OriginalBase64   string   `json:"original_base64,omitempty"`
	Organs           []string `json:"organs"`
	Origin           string   `json:"origin"`
	State            string   `json:"state"`
	DocumentType     string   `json:"document_type"`
	Identifier       string   `json:"identifier,omitempty"`
	SourceIdentifier string   `json:"source_identifier,omitempty"`
	CaptureDate      string   `json:"capture_date,omitempty"`
	ContentFormat    string   `json:"content_format,omitempty"`
}

type eniDocumentResponse struct {
	ContentBase64 string `json:"content_base64"`
	SignatureType string `json:"signature_type"`
}

// CreateENIDocumentJSON envuelve una firma PAdES, CAdES o XAdES en un
// documento electrónico ENI con los metadatos obligatorios de la NTI. Reglas
// de detección iguales a las de la herramienta de escritorio.
func (f *Facade) CreateENIDocumentJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("ENI")
	}
	var req eniDocumentRequest
	if err := decodeJSONStrict(payload, maxENIJSONBytes, "ENI", &req); err != nil {
		return "", err
	}
	if len(req.Organs) == 0 || len(req.Organs) > maxENIOrgans {
		return "", newFacadeError("eni.validacion.dir3")
	}
	for _, field := range []struct {
		value string
		limit int
		key   string
	}{
		{req.Origin, 32, eniErrorOrigin},
		{req.State, 8, "eni.validacion.value"},
		{req.DocumentType, 8, "eni.validacion.value"},
		{req.Identifier, maxENIIdentifierBytes, "eni.validacion.identifier"},
		{req.SourceIdentifier, maxENIIdentifierBytes, "eni.validacion.source"},
		{req.CaptureDate, 64, "eni.validacion.date"},
		{req.ContentFormat, maxENIContentNameBytes, "eni.validacion.format"},
	} {
		if err := validateBoundedText("eni", field.value, field.limit, true); err != nil {
			return "", newFacadeError(field.key)
		}
	}
	signature, err := decodeBase64Limited(req.SignatureBase64, "signature_base64", maxSignedDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(signature)
	var original []byte
	if req.OriginalBase64 != "" {
		original, err = decodeBase64Limited(req.OriginalBase64, "original_base64", maxDocumentBytes)
		if err != nil {
			return "", err
		}
		defer zeroBytes(original)
	}
	if len(signature)+len(original) > maxENIInputBytes {
		return "", newFacadeError("original_base64 supera el limite permitido")
	}
	doc, err := eniDocumentFromSignature(signature, original, req)
	if err != nil {
		return "", err
	}
	output, err := eni.Generar(doc, time.Now())
	if err != nil {
		return "", eniError(err)
	}
	defer zeroBytes(output)
	signatureType := ""
	if len(doc.Firmas) > 0 {
		signatureType = string(doc.Firmas[0].Tipo)
	}
	return marshal(eniDocumentResponse{
		ContentBase64: base64.StdEncoding.EncodeToString(output),
		SignatureType: signatureType,
	})
}

// eniDocumentFromSignature reproduce DocumentoENIDesdeFirma de la CLI sin
// arrastrar sus dependencias de escritorio y con errores cerrados.
func eniDocumentFromSignature(signature, original []byte, req eniDocumentRequest) (eni.Documento, error) {
	var doc eni.Documento
	trimmed := bytes.TrimLeft(signature, " \t\r\n\ufeff")
	switch {
	case bytes.HasPrefix(trimmed, []byte("%PDF-")):
		if !bytes.Contains(signature, []byte("/ByteRange")) {
			return doc, newFacadeError(eniErrorUnsignedPDF)
		}
		if commonsigner.ComprobarIntegridadPAdES(context.Background(), signature) != nil {
			return doc, newFacadeError(eniErrorMismatch)
		}
		doc.Contenido, doc.NombreFormato = signature, "PDF"
		doc.Firmas = []eni.Firma{{Tipo: eni.FirmaPAdES}}
	case len(trimmed) > 0 && trimmed[0] == 0x30:
		content, implicit, err := commonsigner.ContenidoCAdESImplicito(signature)
		if err != nil {
			return doc, newFacadeError(eniErrorUnrecognized)
		}
		if implicit {
			doc.Contenido = content
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaCAdESImplicit, Datos: signature}}
		} else {
			if len(original) == 0 {
				return doc, newFacadeError(eniErrorExplicitCAdES)
			}
			if commonsigner.CotejarCAdESExplicita(signature, original) != nil {
				return doc, newFacadeError(eniErrorMismatch)
			}
			doc.Contenido = original
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaCAdESExplicit, Datos: signature}}
		}
	case bytes.HasPrefix(trimmed, []byte("<")) && bytes.Contains(signature, []byte("http://www.w3.org/2000/09/xmldsig#")):
		if len(original) > 0 {
			if commonsigner.CotejarXAdESSeparada(signature, original) != nil {
				return doc, newFacadeError(eniErrorMismatch)
			}
			doc.Contenido = original
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaXAdESDetached, Datos: signature}}
		} else {
			doc.Contenido, doc.NombreFormato = signature, "XML"
			doc.Firmas = []eni.Firma{{Tipo: eni.FirmaXAdESEnvelope, Datos: signature}}
		}
	default:
		return doc, newFacadeError(eniErrorUnrecognized)
	}
	if format := strings.TrimSpace(req.ContentFormat); format != "" {
		doc.NombreFormato = strings.ToUpper(format)
	} else if doc.NombreFormato == "" {
		doc.NombreFormato = eniContentFormat(doc.Contenido)
	}
	if doc.NombreFormato == "" {
		return doc, newFacadeError(eniErrorContentFormat)
	}
	m := &doc.Metadatos
	for _, organ := range req.Organs {
		if organ = strings.ToUpper(strings.TrimSpace(organ)); organ != "" {
			m.Organos = append(m.Organos, organ)
		}
	}
	switch strings.ToLower(strings.TrimSpace(req.Origin)) {
	case "administracion":
		m.OrigenAdministracion = true
	case "ciudadano":
	default:
		return doc, newFacadeError(eniErrorOrigin)
	}
	m.Identificador = strings.TrimSpace(req.Identifier)
	m.EstadoElaboracion = strings.ToUpper(strings.TrimSpace(req.State))
	if m.EstadoElaboracion == "" {
		m.EstadoElaboracion = "EE01"
	}
	m.IdentificadorDocumentoOrigen = strings.TrimSpace(req.SourceIdentifier)
	m.TipoDocumental = strings.ToUpper(strings.TrimSpace(req.DocumentType))
	if m.TipoDocumental == "" {
		m.TipoDocumental = "TD99"
	}
	if date := strings.TrimSpace(req.CaptureDate); date != "" {
		parsed, err := time.Parse(time.RFC3339, date)
		if err != nil {
			return doc, newFacadeError("eni.validacion.date")
		}
		m.FechaCaptura = parsed
	}
	return doc, nil
}

// eniContentFormat es formatoContenidoENI de la CLI.
func eniContentFormat(data []byte) string {
	d := bytes.TrimLeft(data, " \t\r\n\ufeff")
	switch {
	case bytes.HasPrefix(d, []byte("%PDF-")):
		return "PDF"
	case bytes.HasPrefix(d, []byte("<")):
		return "XML"
	case bytes.HasPrefix(d, []byte("PK\x03\x04")):
		return "ZIP"
	case bytes.HasPrefix(d, []byte("\x89PNG")):
		return "PNG"
	case bytes.HasPrefix(d, []byte("\xff\xd8\xff")):
		return "JPEG"
	}
	return ""
}

func eniError(err error) error {
	var problem eni.Problema
	if errors.As(err, &problem) && isClosedENIKey(problem.Clave) {
		return newFacadeError(problem.Clave)
	}
	return safeOperationError("ENI")
}

var closedENIKeys = []string{"dir3", "identifier", "date", "classification", "source", "format", "text", "structure", "value", "xml", "limit"}

func isClosedENIKey(key string) bool {
	suffix, ok := strings.CutPrefix(key, "eni.validacion.")
	if !ok {
		return false
	}
	for _, closed := range closedENIKeys {
		if suffix == closed {
			return true
		}
	}
	return false
}

type eniValidateRequest struct {
	ContentBase64 string `json:"content_base64"`
}

type eniValidateResponse struct {
	Valid  bool            `json:"valid"`
	Issues []issueResponse `json:"issues"`
}

// ValidateENIJSON revisa la estructura de un documento o expediente ENI 1.0.
// No verifica las firmas que contiene ni sustituye a la validación XSD.
func (f *Facade) ValidateENIJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("ENI")
	}
	var req eniValidateRequest
	if err := decodeJSONStrict(payload, maxSignJSONBytes, "ENI", &req); err != nil {
		return "", err
	}
	data, err := decodeBase64Limited(req.ContentBase64, "content_base64", maxDocumentBytes)
	if err != nil {
		return "", err
	}
	defer zeroBytes(data)
	problems := eni.ValidarXML(data)
	response := eniValidateResponse{Valid: len(problems) == 0, Issues: make([]issueResponse, 0, len(problems))}
	for _, problem := range problems {
		key := problem.Clave
		if !isClosedENIKey(key) {
			key = "eni.validacion.structure"
		}
		response.Issues = append(response.Issues, issueResponse{
			Field: sanitizeOutputText(problem.Campo, maxIssueFieldRunes),
			Key:   key,
			Level: "error",
		})
	}
	return marshal(response)
}

// ENICatalogsJSON devuelve los códigos oficiales de la NTI que la UI ofrece en
// sus desplegables. Las descripciones se traducen en Android por código.
func (f *Facade) ENICatalogsJSON() string {
	raw, err := marshal(struct {
		DocumentStates []string `json:"document_states"`
		DocumentTypes  []string `json:"document_types"`
		FileStates     []string `json:"file_states"`
	}{eni.EstadosElaboracion(), eni.TiposDocumentales(), eni.EstadosExpediente()})
	if err != nil {
		return "{}"
	}
	return raw
}

type csvLegendRequest struct {
	Code string `json:"csv"`
	URL  string `json:"csv_url"`
	Text string `json:"csv_text,omitempty"`
}

type csvLegendResponse struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

// CSVLegendJSON valida la leyenda CSV con las mismas reglas que la firma
// PAdES y devuelve la URL de cotejo normalizada (IDN en ASCII) y el texto
// final con {csv} y {url} sustituidos. El error indica el campo que corregir.
func (f *Facade) CSVLegendJSON(payload string) (string, error) {
	if f == nil {
		return "", errNoConfigurado("CSV")
	}
	var req csvLegendRequest
	if err := decodeJSONStrict(payload, maxCSVJSONBytes, "CSV", &req); err != nil {
		return "", err
	}
	code := strings.TrimSpace(req.Code)
	switch {
	case code == "":
		return "", newFacadeError(csvErrorCodeMissing)
	case len(code) > maxCSVCodeBytes || containsControlOrFormat(code):
		return "", newFacadeError(csvErrorCodeInvalid)
	case strings.TrimSpace(req.URL) == "":
		return "", newFacadeError(csvErrorURLMissing)
	case len(req.Text) > maxCSVTextBytes || containsControlOrFormat(req.Text):
		return "", newFacadeError(csvErrorTextInvalid)
	}
	address, text, _, err := desktopsigner.ResolverLeyendaCSV(map[string]string{
		"csv": code, "csvUrl": req.URL, "csvText": req.Text,
	})
	if err != nil {
		return "", newFacadeError(csvErrorURLInvalid)
	}
	return marshal(csvLegendResponse{URL: address, Text: text})
}
