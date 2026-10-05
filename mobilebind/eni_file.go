// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/eni"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
)

// Expediente electrónico ENI en Android. Reproduce la operación
// generar-expediente del escritorio: documentos ENI en orden alfabético,
// índice firmado con XAdES (FirmarNodoXAdES) y metadatos NTI. Los errores son
// las claves cerradas eni.validacion.* del motor; nunca texto para mostrar.

const (
	// maxENIFileDocuments limita el expediente móvil; el motor admite 128.
	maxENIFileDocuments = 64
	// maxENIFileInputBytes suma los documentos; el JSON lleva su Base64.
	maxENIFileInputBytes = maxDocumentBytes
	maxENIFileJSONBytes  = 48 << 20
	maxENIInterested     = 16
	maxENIInterestedLen  = 128
	maxENIClassification = 64
)

type eniFileDocumentRequest struct {
	Name          string `json:"name"`
	ContentBase64 string `json:"content_base64"`
}

type eniFileRequest struct {
	Documents      []eniFileDocumentRequest `json:"documents"`
	CertificateID  string                   `json:"certificate_id"`
	Organs         []string                 `json:"organs"`
	Classification string                   `json:"classification"`
	State          string                   `json:"state"`
	Identifier     string                   `json:"identifier,omitempty"`
	OpeningDate    string                   `json:"opening_date,omitempty"`
	Interested     []string                 `json:"interested,omitempty"`
}

// eniFileResponse trae el expediente o, si algún documento no es un ENI
// válido, la lista de problemas por fichero. En ese caso no se firma nada.
type eniFileResponse struct {
	OK            bool            `json:"ok"`
	ContentBase64 string          `json:"content_base64,omitempty"`
	Documents     int             `json:"documents"`
	Issues        []issueResponse `json:"issues"`
}

type eniFileDocument struct {
	name string
	data []byte
}

// CreateENIFileJSON crea un expediente ENI con los documentos ENI aportados y
// firma su índice con la identidad de la sesión (PKCS#12 o DNIe). La firma del
// índice es XAdES y, como en el escritorio, solo admite claves RSA.
func (f *Facade) CreateENIFileJSON(payload string) (string, error) {
	if f == nil || f.session == nil {
		return "", errNoConfigurado("ENI")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req eniFileRequest
	if err := decodeJSONStrict(payload, maxENIFileJSONBytes, "ENI", &req); err != nil {
		return "", err
	}
	if len(req.Documents) == 0 || len(req.Documents) > maxENIFileDocuments {
		return "", newFacadeError("eni.validacion.limit")
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, false); err != nil {
		return "", err
	}
	meta, err := eniFileMetadata(req)
	if err != nil {
		return "", err
	}
	docs, err := decodeENIFileDocuments(req.Documents)
	defer func() {
		for _, doc := range docs {
			zeroBytes(doc.data)
		}
	}()
	if err != nil {
		return "", err
	}
	// Antes de pedir la firma se revisa cada documento: un fichero que no es
	// un documento ENI se indica por su nombre y no consume la aprobación.
	if issues := eniFileDocumentIssues(docs); len(issues) > 0 {
		return marshal(eniFileResponse{Documents: len(docs), Issues: issues})
	}
	if !f.sessionIdentityIsRSA(req.CertificateID) {
		return "", newFacadeError(mobileFormatRequiresRSAMessage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	key, err := f.session.KeyFor(ctx, domain.CertificateRef{ID: req.CertificateID})
	if err != nil {
		return "", safeOperationError("ENI")
	}
	sign := func(node []byte, id string) ([]byte, error) {
		return commonsigner.FirmarNodoXAdES(node, id, key, map[string]string{})
	}
	input := make([]eni.DocumentoExpediente, 0, len(docs))
	for _, doc := range docs {
		input = append(input, eni.DocumentoExpediente{XML: doc.data})
	}
	output, err := eni.GenerarExpediente(meta, input, sign, commonsigner.CanonicalizarExclusivo, time.Now())
	if err != nil {
		if issue, ok := eniFileDocumentError(err, docs); ok {
			return marshal(eniFileResponse{Documents: len(docs), Issues: []issueResponse{issue}})
		}
		return "", eniError(err)
	}
	defer zeroBytes(output)
	if len(output) == 0 || len(output) > maxSignedDocumentBytes {
		return "", newFacadeError("eni.validacion.limit")
	}
	return marshal(eniFileResponse{
		OK:            true,
		ContentBase64: base64.StdEncoding.EncodeToString(output),
		Documents:     len(docs),
		Issues:        []issueResponse{},
	})
}

func eniFileMetadata(req eniFileRequest) (eni.MetadatosExpediente, error) {
	var meta eni.MetadatosExpediente
	if len(req.Organs) == 0 || len(req.Organs) > maxENIOrgans {
		return meta, newFacadeError("eni.validacion.dir3")
	}
	if len(req.Interested) > maxENIInterested {
		return meta, newFacadeError("eni.validacion.limit")
	}
	for _, field := range []struct {
		value string
		limit int
		key   string
	}{
		{req.Classification, maxENIClassification, "eni.validacion.classification"},
		{req.State, 8, "eni.validacion.value"},
		{req.Identifier, maxENIIdentifierBytes, "eni.validacion.identifier"},
		{req.OpeningDate, 64, "eni.validacion.date"},
	} {
		if err := validateBoundedText("eni", field.value, field.limit, true); err != nil {
			return meta, newFacadeError(field.key)
		}
	}
	for _, organ := range req.Organs {
		if err := validateBoundedText("eni", organ, 16, false); err != nil {
			return meta, newFacadeError("eni.validacion.dir3")
		}
		meta.Organos = append(meta.Organos, strings.ToUpper(strings.TrimSpace(organ)))
	}
	for _, person := range req.Interested {
		if err := validateBoundedText("eni", person, maxENIInterestedLen, true); err != nil {
			return meta, newFacadeError("eni.validacion.text")
		}
		if person = strings.TrimSpace(person); person != "" {
			meta.Interesados = append(meta.Interesados, person)
		}
	}
	meta.Identificador = strings.TrimSpace(req.Identifier)
	meta.Clasificacion = strings.TrimSpace(req.Classification)
	meta.Estado = strings.ToUpper(strings.TrimSpace(req.State))
	if meta.Estado == "" {
		meta.Estado = "E01"
	}
	if date := strings.TrimSpace(req.OpeningDate); date != "" {
		parsed, err := time.Parse(time.RFC3339, date)
		if err != nil || parsed.After(time.Now().Add(24*time.Hour)) {
			return meta, newFacadeError("eni.validacion.date")
		}
		meta.FechaApertura = parsed
	}
	if err := meta.Validar(); err != nil {
		return meta, eniError(err)
	}
	return meta, nil
}

// decodeENIFileDocuments decodifica los documentos y los ordena por nombre,
// como el escritorio al leer la carpeta. Devuelve lo decodificado incluso con
// error para que quien llama lo borre.
func decodeENIFileDocuments(items []eniFileDocumentRequest) ([]eniFileDocument, error) {
	docs := make([]eniFileDocument, 0, len(items))
	total := 0
	for _, item := range items {
		if err := validateDocumentMetadata(item.Name, "application/xml"); err != nil {
			return docs, err
		}
		remaining := maxENIFileInputBytes - total
		if remaining <= 0 || len(item.ContentBase64) > base64.StdEncoding.EncodedLen(remaining) {
			return docs, newFacadeError("eni.validacion.limit")
		}
		data, err := decodeBase64Limited(item.ContentBase64, "content_base64", remaining)
		if err != nil {
			return docs, err
		}
		total += len(data)
		docs = append(docs, eniFileDocument{name: item.Name, data: data})
	}
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].name < docs[j].name })
	return docs, nil
}

func eniFileDocumentIssues(docs []eniFileDocument) []issueResponse {
	var issues []issueResponse
	for _, doc := range docs {
		for _, problem := range eni.ValidarXML(doc.data) {
			key := problem.Clave
			if !isClosedENIKey(key) {
				key = "eni.validacion.structure"
			}
			issues = append(issues, issueResponse{
				Field: sanitizeOutputText(doc.name, maxIssueFieldRunes),
				Key:   key,
				Level: "error",
			})
			break
		}
	}
	return issues
}

var eniFileDocumentPosition = regexp.MustCompile(`^DocumentoIndizado\[(\d{1,3})\]`)

// eniFileDocumentError atribuye al fichero correspondiente los rechazos del
// motor que señalan un documento concreto (duplicado, expediente anidado...).
func eniFileDocumentError(err error, docs []eniFileDocument) (issueResponse, bool) {
	var problem eni.Problema
	if !errors.As(err, &problem) || !isClosedENIKey(problem.Clave) {
		return issueResponse{}, false
	}
	position := eniFileDocumentPosition.FindStringSubmatch(err.Error())
	if position == nil {
		position = eniFileDocumentPosition.FindStringSubmatch(problem.Campo)
	}
	if position == nil {
		return issueResponse{}, false
	}
	index, convErr := strconv.Atoi(position[1])
	if convErr != nil || index < 1 || index > len(docs) {
		return issueResponse{}, false
	}
	return issueResponse{
		Field: sanitizeOutputText(docs[index-1].name, maxIssueFieldRunes),
		Key:   problem.Clave,
		Level: "error",
	}, true
}
