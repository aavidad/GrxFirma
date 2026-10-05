// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxDocumentBytes       = 32 << 20
	maxSignedDocumentBytes = 48 << 20
	maxCertificateBytes    = 4 << 20
	maxPasswordBytes       = 1024
	maxSignJSONBytes       = 45 << 20
	maxVerifyJSONBytes     = 108 << 20
	maxImportJSONBytes     = 6 << 20
	maxSelectJSONBytes     = 16 << 10
	maxBatchJSONBytes      = 96 << 20
	maxRemoteJSONBytes     = 64 << 20
	maxIntentJSONBytes     = 48 << 20
	maxOptions             = 32
	maxOptionKeyBytes      = 64
	maxOptionValueBytes    = 2048
)

func decodeJSONStrict(payload string, maximum int, operation string, target any) error {
	if len(payload) == 0 {
		return newFacadeError("json de " + operation + " vacio")
	}
	if len(payload) > maximum {
		return newFacadeError("json de " + operation + " supera el limite permitido")
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errJSONInvalido(operation)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errJSONInvalido(operation)
	}
	return nil
}

func decodeBase64Limited(encoded, field string, maximum int) ([]byte, error) {
	if encoded == "" {
		return nil, newFacadeError(field + " es obligatorio")
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(maximum) {
		return nil, newFacadeError(field + " supera el limite permitido")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, errCampoBase64(field)
	}
	if len(decoded) == 0 || len(decoded) > maximum {
		zeroBytes(decoded)
		return nil, newFacadeError(field + " tiene un tamano no permitido")
	}
	return decoded, nil
}

func validateDocumentMetadata(name, mimeType string) error {
	if err := validateBoundedText("name", name, 160, false); err != nil {
		return err
	}
	if strings.ContainsAny(name, "/\\") {
		return newFacadeError("name no puede contener separadores de ruta")
	}
	if err := validateBoundedText("mime_type", mimeType, 160, false); err != nil {
		return err
	}
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil || !strings.Contains(mediaType, "/") || strings.Contains(mediaType, "*") {
		return newFacadeError("mime_type no es valido")
	}
	return nil
}

func validateBoundedText(field, value string, maximum int, allowEmpty bool) error {
	if value == "" && allowEmpty {
		return nil
	}
	if strings.TrimSpace(value) == "" {
		return newFacadeError(field + " es obligatorio")
	}
	if len(value) > maximum {
		return newFacadeError(field + " supera el limite permitido")
	}
	if !utf8.ValidString(value) {
		return newFacadeError(field + " no es UTF-8 valido")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return newFacadeError(field + " contiene caracteres de control")
		}
	}
	return nil
}

func validateOptions(options map[string]string) error {
	if len(options) > maxOptions {
		return newFacadeError("options contiene demasiadas entradas")
	}
	for key, value := range options {
		if err := validateBoundedText("options key", key, maxOptionKeyBytes, false); err != nil {
			return err
		}
		limit := maxOptionValueBytes
		switch key {
		case "visibleSealPlacements":
			limit = 32 << 10
		case "visibleSealImageBase64":
			limit = base64.StdEncoding.EncodedLen(10 << 20)
		case "visibleSealImagePath":
			return newFacadeError("las rutas de imagen no están permitidas en mobile")
		}
		if len(value) > limit || !utf8.ValidString(value) {
			return newFacadeError("options contiene un valor no permitido")
		}
		for _, r := range value {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return newFacadeError("options contiene caracteres de control")
			}
		}
	}
	return nil
}

func resolveSignatureFormat(requested, name, mimeType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(requested))
	switch normalized {
	case "cades", "pades", "xades":
		return normalized, nil
	case "", "auto":
	default:
		return "", newFacadeError("format no esta habilitado en mobile")
	}
	mediaType, _, _ := mime.ParseMediaType(mimeType)
	switch {
	case mediaType == "application/pdf" || strings.HasSuffix(strings.ToLower(name), ".pdf"):
		return "pades", nil
	case strings.Contains(mediaType, "xml") || strings.HasSuffix(strings.ToLower(name), ".xml"):
		return "xades", nil
	default:
		return "cades", nil
	}
}

func validateSignatureAction(action string) error {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "sign", "cosign", "countersign":
		return nil
	default:
		return newFacadeError("action no esta habilitada en mobile")
	}
}

func safeOperationError(operation string) error {
	return newFacadeError("la operacion de " + operation + " no pudo completarse")
}

func sanitizeOutputText(value string, maximumRunes int) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
	runes := []rune(strings.TrimSpace(clean))
	if len(runes) > maximumRunes {
		runes = runes[:maximumRunes]
	}
	return string(runes)
}

func sanitizeOutputStrings(values []string, maximumItems, maximumRunes int) []string {
	if len(values) > maximumItems {
		values = values[:maximumItems]
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := sanitizeOutputText(value, maximumRunes)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// validateSigningOptions evita degradar perfiles y valida la TSA antes de usar
// la identidad. El motor compartido conserva la validación RFC 3161 y de red.
func validateSigningOptions(format, action string, options map[string]string) error {
	profile := strings.ToLower(strings.TrimSpace(options["profile"]))
	switch profile {
	case "", "baseline", "t", "lt", "lta":
	default:
		return newFacadeError("perfil de firma no permitido")
	}
	for key := range options {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "nivel" || normalized == "level" || normalized == "baseline" || normalized == "ltv" ||
			(normalized == "profile" && key != "profile") || (strings.HasPrefix(normalized, "tsa") && key != "tsaURL") {
			return newFacadeError("opcion de perfil o TSA no permitida")
		}
	}
	tsa := options["tsaURL"]
	if tsa != "" {
		endpoint, err := url.Parse(tsa)
		if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" || strings.Contains(tsa, "#") ||
			(endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Opaque != "" || strings.TrimSpace(tsa) != tsa {
			return newFacadeError("URL HTTP(S) de TSA invalida")
		}
		if port := endpoint.Port(); port != "" {
			number, err := strconv.Atoi(port)
			if err != nil || number < 1 || number > 65535 {
				return newFacadeError("puerto de TSA invalido")
			}
		}
	}
	if (profile == "t" || profile == "lt" || profile == "lta") && tsa == "" {
		return newFacadeError("el perfil requiere una TSA configurada")
	}
	if format == "xades" && (profile == "lt" || profile == "lta") || format == "pades" && profile == "lta" {
		return newFacadeError("perfil no soportado para este formato")
	}
	if format == "pades" && strings.EqualFold(strings.TrimSpace(action), "countersign") {
		return newFacadeError("PAdES no admite contrafirma; use CAdES o XAdES")
	}
	return nil
}
