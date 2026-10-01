// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type batchRequest struct {
	StopOnError bool               `json:"stoponerror"`
	SubOp       string             `json:"suboperation"`
	Format      string             `json:"format"`
	Algorithm   string             `json:"algorithm"`
	ExtraParams string             `json:"extraparams"`
	SingleSigns []batchSingleEntry `json:"singlesigns"`
}

type batchRequestJSONCompat struct {
	StopOnError bool                     `json:"stopOnError"`
	SubOp       string                   `json:"subOperation"`
	Format      string                   `json:"format"`
	Algorithm   string                   `json:"algorithm"`
	ExtraParams string                   `json:"extraParams"`
	SingleSigns []batchSingleEntryCompat `json:"singleSigns"`
}

type batchSingleEntryCompat struct {
	ID          string `json:"id"`
	SubOp       string `json:"subOperation"`
	DataRef     string `json:"dataReference"`
	Format      string `json:"format"`
	Algorithm   string `json:"algorithm"`
	ExtraParams string `json:"extraParams"`
}

type batchSingleEntry struct {
	ID          string `json:"id"`
	SubOp       string `json:"suboperation"`
	DataRef     string `json:"datareference"`
	Format      string `json:"format"`
	Algorithm   string `json:"algorithm"`
	ExtraParams string `json:"extraparams"`
}

type BatchMetadata struct {
	IDs         []string
	StopOnError bool
	IsJSON      bool
}

type xmlBatchRequest struct {
	XMLName     xml.Name         `xml:"signbatch"`
	StopOnError string           `xml:"stoponerror,attr"`
	Algorithm   string           `xml:"algorithm,attr"`
	SubOp       string           `xml:"suboperation,attr"`
	ExtraParams string           `xml:"extraparams"`
	SingleSigns []xmlBatchSingle `xml:"singlesign"`
}

type xmlBatchSingle struct {
	ID          string `xml:"Id,attr"`
	DataSource  string `xml:"datasource"`
	Format      string `xml:"format"`
	SubOp       string `xml:"suboperation"`
	ExtraParams string `xml:"extraparams"`
}

// ParseBatchPayload traduce un lote JSON/XML legacy al comando neutro ProcessBatch.
// En esta fase soporta referencias de datos embebidas en base64 o data: URI base64.
func (a *Adaptador) ParseBatchPayload(raw []byte, sesion domain.ExchangeSession) (application.ProcessBatchCommand, error) {
	req, err := parseBatchRequest(raw)
	if err != nil {
		return application.ProcessBatchCommand{}, err
	}
	if len(req.SingleSigns) == 0 {
		return application.ProcessBatchCommand{}, errors.New("el lote no contiene operaciones")
	}

	defaultOptions := decodeBatchExtraParams(req.ExtraParams)
	if algoritmo := strings.TrimSpace(req.Algorithm); algoritmo != "" {
		defaultOptions["algorithm"] = algoritmo
	}

	entradas := make([]application.BatchItemInput, 0, len(req.SingleSigns))
	for i, item := range req.SingleSigns {
		formato := strings.TrimSpace(item.Format)
		if formato == "" {
			formato = strings.TrimSpace(req.Format)
		}
		if formato == "" {
			formato = "CAdES"
		}

		accion, err := resolverAccionLote(item.SubOp, req.SubOp)
		if err != nil {
			return application.ProcessBatchCommand{}, fmt.Errorf("operacion %d del lote invalida: %w", i, err)
		}

		datos, err := resolverDatosLote(item.DataRef)
		if err != nil {
			return application.ProcessBatchCommand{}, fmt.Errorf("datos %d del lote invalidos: %w", i, err)
		}

		opts := decodeBatchExtraParams(item.ExtraParams)
		if algoritmo := strings.TrimSpace(item.Algorithm); algoritmo != "" {
			opts["algorithm"] = algoritmo
		}

		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("item-%d", i+1)
		}

		entradas = append(entradas, application.BatchItemInput{
			Nombre:    nombreDocumentoLote(id, formato),
			Contenido: datos,
			TipoMIME:  mimeTypePorFormato(formato),
			Formato:   formato,
			Accion:    accion,
			Opciones:  opts,
		})
	}

	cmd, err := application.NewProcessBatchCommandWithDefaults(entradas, defaultOptions)
	if err != nil {
		return application.ProcessBatchCommand{}, err
	}
	cmd.StopOnError = req.StopOnError
	cmd.Session = sesion
	return cmd, nil
}

// ParseBatchPayloadFromBase64 decodifica el envelope batch legado y lo traduce
// a ProcessBatchCommand dentro de V2.
func (a *Adaptador) ParseBatchPayloadFromBase64(encoded string, sesion domain.ExchangeSession) (application.ProcessBatchCommand, error) {
	raw, err := decodeProtocolBase64(encoded)
	if err != nil {
		return application.ProcessBatchCommand{}, fmt.Errorf("el payload batch embebido en afirma:// no es base64 valido: %w", err)
	}
	return a.ParseBatchPayload(raw, sesion)
}

func parseBatchRequest(raw []byte) (*batchRequest, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, errors.New("el lote esta vacio")
	}
	if strings.HasPrefix(trimmed, "<") {
		return parseBatchXMLRequest([]byte(trimmed))
	}
	return parseBatchJSONRequest([]byte(trimmed))
}

func ParseBatchMetadata(raw []byte) (BatchMetadata, error) {
	req, err := parseBatchRequest(raw)
	if err != nil {
		return BatchMetadata{}, err
	}
	trimmed := strings.TrimSpace(string(raw))
	meta := BatchMetadata{
		StopOnError: req.StopOnError,
		IsJSON:      !strings.HasPrefix(trimmed, "<"),
		IDs:         make([]string, 0, len(req.SingleSigns)),
	}
	for i, item := range req.SingleSigns {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("item-%d", i+1)
		}
		meta.IDs = append(meta.IDs, id)
	}
	return meta, nil
}

func parseBatchJSONRequest(raw []byte) (*batchRequest, error) {
	var req batchRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("json de lote invalido: %w", err)
	}

	var compat batchRequestJSONCompat
	if err := json.Unmarshal(raw, &compat); err == nil {
		if !req.StopOnError {
			req.StopOnError = compat.StopOnError
		}
		if strings.TrimSpace(req.SubOp) == "" {
			req.SubOp = strings.TrimSpace(compat.SubOp)
		}
		if strings.TrimSpace(req.ExtraParams) == "" {
			req.ExtraParams = strings.TrimSpace(compat.ExtraParams)
		}
		if len(req.SingleSigns) == 0 && len(compat.SingleSigns) > 0 {
			req.SingleSigns = make([]batchSingleEntry, 0, len(compat.SingleSigns))
			for _, s := range compat.SingleSigns {
				req.SingleSigns = append(req.SingleSigns, batchSingleEntry{
					ID:          strings.TrimSpace(s.ID),
					SubOp:       strings.TrimSpace(s.SubOp),
					DataRef:     strings.TrimSpace(s.DataRef),
					Format:      strings.TrimSpace(s.Format),
					Algorithm:   strings.TrimSpace(s.Algorithm),
					ExtraParams: strings.TrimSpace(s.ExtraParams),
				})
			}
		}
	}
	return &req, nil
}

func parseBatchXMLRequest(raw []byte) (*batchRequest, error) {
	var xmlReq xmlBatchRequest
	if err := xml.Unmarshal(raw, &xmlReq); err != nil {
		return nil, fmt.Errorf("xml de lote invalido: %w", err)
	}

	req := &batchRequest{
		StopOnError: strings.EqualFold(strings.TrimSpace(xmlReq.StopOnError), "true"),
		Algorithm:   strings.TrimSpace(xmlReq.Algorithm),
		SubOp:       strings.TrimSpace(xmlReq.SubOp),
		ExtraParams: strings.TrimSpace(xmlReq.ExtraParams),
		SingleSigns: make([]batchSingleEntry, 0, len(xmlReq.SingleSigns)),
	}
	for _, s := range xmlReq.SingleSigns {
		req.SingleSigns = append(req.SingleSigns, batchSingleEntry{
			ID:          strings.TrimSpace(s.ID),
			SubOp:       strings.TrimSpace(s.SubOp),
			DataRef:     strings.TrimSpace(s.DataSource),
			Format:      strings.TrimSpace(s.Format),
			Algorithm:   req.Algorithm,
			ExtraParams: strings.TrimSpace(s.ExtraParams),
		})
	}
	return req, nil
}

func resolverAccionLote(singleOp, globalOp string) (string, error) {
	op := strings.ToLower(strings.TrimSpace(singleOp))
	if op == "" {
		op = strings.ToLower(strings.TrimSpace(globalOp))
	}
	switch op {
	case "", "sign", "firmar":
		return "sign", nil
	case "cosign", "cofirmar":
		return "cosign", nil
	case "countersign", "contrafirmar", "countersigntree", "countersignleafs":
		return "countersign", nil
	default:
		return "", fmt.Errorf("suboperacion no soportada: %s", op)
	}
}

func resolverDatosLote(ref string) ([]byte, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("referencia de datos vacia")
	}
	if raw, ok := extraerDataURI(ref); ok {
		return raw, nil
	}
	if decoded, err := decodeProtocolBase64(ref); err == nil {
		return decoded, nil
	}
	return nil, errors.New("solo se soportan referencias embebidas en base64 o data: URI")
}

func decodeBatchExtraParams(raw string) map[string]string {
	props := make(map[string]string)
	body := strings.TrimSpace(raw)
	if body == "" {
		return props
	}
	// AutoFirma V1.9 define `extraparams` como un fichero Properties UTF-8
	// codificado en Base64, tanto en JSONBatchManager como en los lotes XML.
	// Conservamos la entrada en claro admitida históricamente por V2 para no
	// romper integraciones existentes.
	if decoded, err := decodeProtocolBase64(body); err == nil &&
		len(decoded) > 0 &&
		utf8.Valid(decoded) &&
		isPlausibleBatchProperties(decoded) {
		body = string(decoded)
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, `\n`, "\n"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			props[line] = ""
			continue
		}
		k := strings.TrimSpace(line[:sep])
		v := strings.TrimSpace(line[sep+1:])
		if k != "" {
			props[k] = v
		}
	}
	return props
}

func isPlausibleBatchProperties(raw []byte) bool {
	for _, r := range string(raw) {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < ' ' || r == '\u007f' {
			return false
		}
	}
	return true
}

func decodeProtocolBase64(raw string) ([]byte, error) {
	s := strings.ReplaceAll(strings.TrimSpace(raw), " ", "+")
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func extraerDataURI(ref string) ([]byte, bool) {
	if !strings.HasPrefix(strings.ToLower(ref), "data:") {
		return nil, false
	}
	comma := strings.IndexByte(ref, ',')
	if comma < 0 {
		return nil, false
	}
	meta := strings.ToLower(ref[:comma])
	if !strings.Contains(meta, ";base64") {
		return nil, false
	}
	raw, err := decodeProtocolBase64(ref[comma+1:])
	if err != nil {
		return nil, false
	}
	return raw, true
}

func mimeTypePorFormato(formato string) string {
	switch strings.ToLower(strings.TrimSpace(formato)) {
	case "xades":
		return "application/xml"
	case "pades":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func nombreDocumentoLote(id, formato string) string {
	ext := ".bin"
	switch strings.ToLower(strings.TrimSpace(formato)) {
	case "xades":
		ext = ".xml"
	case "pades":
		ext = ".pdf"
	}
	return "batch-" + strings.TrimSpace(id) + ext
}
