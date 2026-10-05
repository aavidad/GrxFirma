// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"grxfirma/internal/application"
)

// mobileBatchRequest es el contrato público del lote Android. No admite
// sesión de intercambio remoto: remote_exchange sigue deshabilitado.
type mobileBatchRequest struct {
	Items         []batchItemRequest `json:"items"`
	CertificateID string             `json:"certificate_id"`
	Options       map[string]string  `json:"options,omitempty"`
}

type mobileBatchItemResponse struct {
	Index               int    `json:"index"`
	OK                  bool   `json:"ok"`
	Format              string `json:"format,omitempty"`
	Algorithm           string `json:"algorithm,omitempty"`
	SignedContentBase64 string `json:"signed_content_base64,omitempty"`
	Error               string `json:"error,omitempty"`
}

type mobileBatchResponse struct {
	OK            bool                      `json:"ok"`
	CertificateID string                    `json:"certificate_id"`
	Items         []mobileBatchItemResponse `json:"items"`
}

// ProcessBatchJSON firma varios documentos con una única aprobación y la
// identidad de la sesión. Cada documento pasa las mismas validaciones que
// SignJSON y el resultado se devuelve por posición, con éxito o error propio.
func (f *Facade) ProcessBatchJSON(payload string) (string, error) {
	if f == nil || f.batchService == nil {
		return "", errNoConfigurado("lote")
	}
	f.operationMu.Lock()
	defer f.operationMu.Unlock()
	var req mobileBatchRequest
	if err := decodeJSONStrict(payload, maxBatchJSONBytes, "lote", &req); err != nil {
		return "", err
	}
	if len(req.Items) == 0 {
		return "", newFacadeError("el lote no puede estar vacio")
	}
	if len(req.Items) > maxBatchItems {
		return "", newFacadeError("el lote supera el maximo de documentos")
	}
	if err := validateBoundedText("certificate_id", req.CertificateID, 128, false); err != nil {
		return "", err
	}
	if err := validateOptions(req.Options); err != nil {
		return "", err
	}
	req.Options = f.withRegionOptions(req.Options)
	items := make([]application.BatchItemInput, 0, len(req.Items))
	defer func() {
		for index := range items {
			zeroBytes(items[index].Contenido)
		}
	}()
	total := 0
	for _, item := range req.Items {
		input, err := mobileBatchItem(item, req.Options, maxBatchInputBytes-total)
		if err != nil {
			return "", err
		}
		total += len(input.Contenido)
		items = append(items, input)
	}
	// Un formato solo RSA con una identidad ECDSA falla en su posición, como
	// cualquier otro error del motor, y no detiene el resto del lote.
	cmd, err := application.NewProcessBatchCommandWithDefaults(items, req.Options)
	if err != nil {
		return "", newFacadeError("solicitud de lote no valida")
	}
	cmd.CertificateID = req.CertificateID
	timeout := f.timeout * time.Duration(len(items))
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := f.batchService.Execute(ctx, cmd)
	if err != nil {
		return "", safeOperationError("lote")
	}
	defer func() {
		for index := range result.Results {
			zeroBytes(result.Results[index].Result.Data)
		}
	}()
	return marshal(mobileBatchResponseFrom(result, len(items)))
}

func mobileBatchItem(item batchItemRequest, defaults map[string]string, remaining int) (application.BatchItemInput, error) {
	if err := validateDocumentMetadata(item.Name, item.MIMEType); err != nil {
		return application.BatchItemInput{}, err
	}
	if err := validateBoundedText("format", item.Format, 32, true); err != nil {
		return application.BatchItemInput{}, err
	}
	if err := validateBoundedText("action", item.Action, 32, true); err != nil {
		return application.BatchItemInput{}, err
	}
	if err := validateSignatureAction(item.Action); err != nil {
		return application.BatchItemInput{}, err
	}
	if err := validateOptions(item.Options); err != nil {
		return application.BatchItemInput{}, err
	}
	merged := make(map[string]string, len(defaults)+len(item.Options)+1)
	for key, value := range defaults {
		merged[key] = value
	}
	for key, value := range item.Options {
		merged[key] = value
	}
	if len(merged) > maxOptions {
		return application.BatchItemInput{}, newFacadeError("options contiene demasiadas entradas")
	}
	if strings.TrimSpace(merged["profile"]) == "" {
		merged["profile"] = "baseline"
	}
	if remaining <= 0 {
		return application.BatchItemInput{}, newFacadeError("el lote supera el tamano total permitido")
	}
	limit := min(remaining, maxDocumentBytes)
	content, err := decodeBase64Limited(item.ContentBase64, "content_base64", limit)
	if err != nil {
		if len(item.ContentBase64) > base64.StdEncoding.EncodedLen(limit) && limit < maxDocumentBytes {
			return application.BatchItemInput{}, newFacadeError("el lote supera el tamano total permitido")
		}
		return application.BatchItemInput{}, err
	}
	format, err := resolveSignatureFormat(item.Format, item.Name, item.MIMEType, content)
	if err == nil {
		err = validateSigningOptions(format, item.Action, merged)
	}
	if err == nil && format == "verifactu" {
		// Cada registro se comprueba y firma por separado; no se firma en lote.
		err = newFacadeError("format no esta habilitado en mobile")
	}
	if err != nil {
		zeroBytes(content)
		return application.BatchItemInput{}, err
	}
	return application.BatchItemInput{
		Nombre:    item.Name,
		Contenido: content,
		TipoMIME:  item.MIMEType,
		Formato:   format,
		Accion:    item.Action,
		Opciones:  merged,
	}, nil
}

// mobileBatchResponseFrom recompone el resultado por posición: el caso de uso
// compartido añade solo los éxitos, en orden, e indexa los errores.
func mobileBatchResponseFrom(result application.BatchResult, count int) mobileBatchResponse {
	response := mobileBatchResponse{OK: true, Items: make([]mobileBatchItemResponse, 0, count)}
	next := 0
	outputBytes := 0
	for index := 0; index < count; index++ {
		item := mobileBatchItemResponse{Index: index}
		switch {
		case result.Errores[index] != nil:
			item.Error = safeOperationError("firma").Error()
		case next < len(result.Results):
			signed := result.Results[next]
			next++
			size := len(signed.Result.Data)
			if size == 0 || size > maxSignedDocumentBytes || outputBytes+size > maxBatchOutputBytes {
				item.Error = "el resultado de firma tiene un tamano no permitido"
				break
			}
			outputBytes += size
			item.OK = true
			item.Format = sanitizeOutputText(string(signed.Result.Format), 64)
			item.Algorithm = sanitizeOutputText(signed.Result.Algorithm, 128)
			item.SignedContentBase64 = base64.StdEncoding.EncodeToString(signed.Result.Data)
			if response.CertificateID == "" {
				response.CertificateID = sanitizeOutputText(signed.CertificateUsed.ID, 128)
			}
		default:
			item.Error = "documento no procesado"
		}
		response.OK = response.OK && item.OK
		response.Items = append(response.Items, item)
	}
	return response
}
