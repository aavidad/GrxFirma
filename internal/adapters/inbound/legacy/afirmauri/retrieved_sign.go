// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type retrievedSignManifest struct {
	XMLName xml.Name                     `xml:"sign"`
	Entries []retrievedSignManifestEntry `xml:"e"`
}

type retrievedSignManifestEntry struct {
	Key   string `xml:"k,attr"`
	Value string `xml:"v,attr"`
}

// ResolveRetrievedSign intenta traducir un payload recuperado desde
// RetrieveService a un comando de firma local clásico. Devuelve ok=false cuando
// el contenido recuperado no es un manifiesto XML <sign> y debe seguir por el
// flujo trifásico normal.
func ResolveRetrievedSign(raw []byte, solicitud Solicitud) (application.SignCommand, Solicitud, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(trimmed, []byte("<sign>")) && !bytes.HasPrefix(trimmed, []byte("<sign ")) {
		return application.SignCommand{}, solicitud, false, nil
	}

	var manifest retrievedSignManifest
	if err := xml.Unmarshal(trimmed, &manifest); err != nil {
		return application.SignCommand{}, solicitud, true, fmt.Errorf("el manifiesto XML legado es inválido: %w", err)
	}
	if len(manifest.Entries) == 0 {
		return application.SignCommand{}, solicitud, true, fmt.Errorf("el manifiesto XML legado no contiene entradas")
	}

	params := make(url.Values, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		key := strings.TrimSpace(strings.ToLower(entry.Key))
		value := strings.TrimSpace(entry.Value)
		if key == "" || value == "" {
			continue
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		params.Set(key, value)
	}
	if len(params) == 0 {
		return application.SignCommand{}, solicitud, true, fmt.Errorf("el manifiesto XML legado no contiene valores utilizables")
	}

	payloadRaw := strings.TrimSpace(parametro(params, "dat", "data"))
	if payloadRaw == "" {
		return application.SignCommand{}, solicitud, true, fmt.Errorf("el manifiesto XML legado no contiene 'dat'")
	}
	payload, err := decodeProtocolBase64(payloadRaw)
	if err != nil {
		return application.SignCommand{}, solicitud, true, fmt.Errorf("el payload del manifiesto XML legado no es base64 válido: %w", err)
	}

	resuelta := solicitud
	if resuelta.LegacyParams == nil {
		resuelta.LegacyParams = make(url.Values)
	}
	for key, values := range params {
		for _, value := range values {
			resuelta.LegacyParams.Set(key, value)
		}
	}

	if rawFormat := strings.TrimSpace(parametro(params, "format", "signformat")); rawFormat != "" {
		format, err := application.ParseSignatureFormat(rawFormat)
		if err != nil {
			return application.SignCommand{}, solicitud, true, fmt.Errorf("formato de firma inválido en manifiesto XML legado: %w", err)
		}
		resuelta.Formato = format
	}
	if key := strings.TrimSpace(parametro(params, "key", "cipherkey")); key != "" {
		resuelta.Sesion.SessionKey = key
	}
	if requestID := strings.TrimSpace(parametro(params, "fileid", "id", "fileid", "requestid")); requestID != "" {
		resuelta.Sesion.RequestID = requestID
	}
	if uploadURL := normalizarEndpoint(strings.TrimSpace(parametro(params, "stservlet", "storageservlet", "stservlet", "storageservlet"))); uploadURL != "" {
		resuelta.Sesion.UploadEndpoint = uploadURL
	}
	for _, field := range []string{"params", "properties", "extraparams"} {
		rawValue := strings.TrimSpace(params.Get(field))
		if rawValue == "" {
			continue
		}
		body := rawValue
		if decoded, err := decodeProtocolBase64(rawValue); err == nil {
			body = string(decoded)
		}
		if resuelta.Options == nil {
			resuelta.Options = make(map[string]string)
		}
		for k, v := range decodeBatchExtraParams(body) {
			resuelta.Options[k] = v
		}
	}

	name := inferRetrievedSignDocumentName(resuelta)
	mime := inferRetrievedSignMimeType(resuelta.Formato)
	doc, err := domain.NewDocument(name, payload, mime)
	if err != nil {
		return application.SignCommand{}, solicitud, true, err
	}
	cmd := application.SignCommand{
		Document: doc,
		Format:   resuelta.Formato,
		Action:   resuelta.AccionFirma,
		Options:  cloneRetrievedSignOptions(resuelta.Options),
	}
	return cmd, resuelta, true, nil
}

func inferRetrievedSignDocumentName(solicitud Solicitud) string {
	if raw := strings.TrimSpace(parametro(solicitud.LegacyParams, "filename", "filename", "name")); raw != "" {
		return raw
	}
	switch solicitud.Formato {
	case domain.FormatPAdES:
		return "documento.pdf"
	case domain.FormatXAdES:
		return "documento.xml"
	default:
		return "documento.bin"
	}
}

func inferRetrievedSignMimeType(format domain.SignatureFormat) string {
	switch format {
	case domain.FormatPAdES:
		return "application/pdf"
	case domain.FormatXAdES:
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

func cloneRetrievedSignOptions(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
