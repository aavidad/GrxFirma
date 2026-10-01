// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"bytes"
	"crypto/des" // #nosec G502 -- DES is required for V1.9 payloads and is gated by explicit operator opt-in.
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"grxfirma/internal/adapters/inbound/legacy/legacycrypto"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type retrievedBatchEnvelope struct {
	XMLName xml.Name                      `xml:"batch"`
	Entries []retrievedBatchEnvelopeEntry `xml:"e"`
}

type retrievedBatchEnvelopeEntry struct {
	K string `xml:"k,attr"`
	V string `xml:"v,attr"`
}

// ResolveRetrievedBatch traduce el contenido recuperado desde RetrieveService a un lote
// procesable por V2. Mantiene la compatibilidad con el envelope XML de V1 y devuelve
// los orígenes implicados para que el llamador pueda volver a validarlos.
func ResolveRetrievedBatch(raw []byte, sesion domain.ExchangeSession) (*RemoteBatchCommand, *application.ProcessBatchCommand, []string, error) {
	payload, params, sesionResuelta, err := resolveRetrievedBatchPayload(raw, sesion)
	if err != nil {
		return nil, nil, nil, err
	}

	if remote, err := buildRetrievedRemoteBatch(params, sesionResuelta, payload); err != nil {
		return nil, nil, nil, err
	} else if remote != nil {
		origenes, err := origenesDesdeEndpoints(remote.Session.UploadEndpoint, remote.Session.RetrieveEndpoint, remote.PreSignEndpoint, remote.PostSignEndpoint)
		if err != nil {
			return nil, nil, nil, err
		}
		return remote, nil, origenes, nil
	}

	adaptador := &Adaptador{}
	cmd, err := adaptador.ParseBatchPayload(payload, sesionResuelta)
	if err != nil {
		return nil, nil, nil, err
	}
	origenes, err := origenesDesdeEndpoints(sesionResuelta.UploadEndpoint, sesionResuelta.RetrieveEndpoint)
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, &cmd, origenes, nil
}

func ResolveRetrievedBatchMetadata(raw []byte, sesion domain.ExchangeSession) (BatchMetadata, error) {
	payload, _, _, err := resolveRetrievedBatchPayload(raw, sesion)
	if err != nil {
		return BatchMetadata{}, err
	}
	return ParseBatchMetadata(payload)
}

func resolveRetrievedBatchPayload(raw []byte, sesion domain.ExchangeSession) ([]byte, url.Values, domain.ExchangeSession, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, domain.ExchangeSession{}, fmt.Errorf("el retrieve remoto del lote está vacío")
	}
	trimmed = normalizarPayloadRetrieve(trimmed, sesion.SessionKey, sesion.RetrieveEndpoint, sesion.UploadEndpoint)

	params := make(url.Values)
	sesionResuelta := sesion
	if env, ok, err := parseRetrievedBatchEnvelope(trimmed); err != nil {
		return nil, nil, domain.ExchangeSession{}, err
	} else if ok {
		params = env
		aplicarSessionEnvelope(&sesionResuelta, params)
		payloadRaw := strings.TrimSpace(parametro(params, "dat", "data"))
		if payloadRaw == "" {
			return nil, nil, domain.ExchangeSession{}, fmt.Errorf("el envelope remoto no contiene 'dat'")
		}
		payload, err := decodeProtocolBase64(payloadRaw)
		if err != nil {
			return nil, nil, domain.ExchangeSession{}, fmt.Errorf("el payload batch remoto no es base64 válido: %w", err)
		}
		payload = normalizarPayloadRetrieve(payload, sesionResuelta.SessionKey, sesionResuelta.RetrieveEndpoint, sesionResuelta.UploadEndpoint)
		return payload, params, sesionResuelta, nil
	}

	payload := append([]byte(nil), trimmed...)
	if !bytes.HasPrefix(trimmed, []byte("{")) && !bytes.HasPrefix(trimmed, []byte("<")) {
		if decoded, err := decodeProtocolBase64(string(trimmed)); err == nil {
			payload = decoded
		}
	}
	payload = normalizarPayloadRetrieve(payload, sesionResuelta.SessionKey, sesionResuelta.RetrieveEndpoint, sesionResuelta.UploadEndpoint)
	return payload, params, sesionResuelta, nil
}

func parseRetrievedBatchEnvelope(raw []byte) (url.Values, bool, error) {
	if !bytes.HasPrefix(raw, []byte("<batch>")) && !bytes.HasPrefix(raw, []byte("<batch ")) {
		return nil, false, nil
	}

	var env retrievedBatchEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, false, fmt.Errorf("el envelope XML del lote es inválido: %w", err)
	}
	if len(env.Entries) == 0 {
		return nil, false, fmt.Errorf("el envelope XML del lote no contiene parámetros")
	}

	params := make(url.Values, len(env.Entries))
	for _, entry := range env.Entries {
		key := strings.TrimSpace(strings.ToLower(entry.K))
		value := strings.TrimSpace(entry.V)
		if key == "" || value == "" {
			continue
		}
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		params.Set(key, value)
	}
	if len(params) == 0 {
		return nil, false, fmt.Errorf("el envelope XML del lote no contiene valores utilizables")
	}
	return params, true, nil
}

func aplicarSessionEnvelope(sesion *domain.ExchangeSession, params url.Values) {
	if sesion == nil || len(params) == 0 {
		return
	}
	if requestID := strings.TrimSpace(parametro(params, "fileid", "id", "fileId", "requestId")); requestID != "" {
		sesion.RequestID = requestID
	}
	if retrieve := normalizarEndpoint(strings.TrimSpace(parametro(params, "rtservlet", "retrieveservlet", "rtServlet", "retrieveServlet"))); retrieve != "" {
		sesion.RetrieveEndpoint = retrieve
	}
	if upload := normalizarEndpoint(strings.TrimSpace(parametro(params, "stservlet", "storageservlet", "stServlet", "storageServlet"))); upload != "" {
		sesion.UploadEndpoint = upload
	}
	if sesion.UploadEndpoint == "" {
		sesion.UploadEndpoint = sesion.RetrieveEndpoint
	}
	if sesion.RetrieveEndpoint == "" {
		sesion.RetrieveEndpoint = sesion.UploadEndpoint
	}
	if key := strings.TrimSpace(parametro(params, "key", "cipherKey")); key != "" {
		sesion.SessionKey = key
	}
}

func buildRetrievedRemoteBatch(params url.Values, sesion domain.ExchangeSession, payload []byte) (*RemoteBatchCommand, error) {
	isJSONBatch := parseBoolLegacy(parametro(params, "jsonbatch", "jsonBatch"))
	preURL := normalizarEndpoint(strings.TrimSpace(parametro(params, "batchpresignerurl", "batchPreSignerUrl", "batchPreSignerURL")))
	postURL := normalizarEndpoint(strings.TrimSpace(parametro(params, "batchpostsignerurl", "batchPostSignerUrl", "batchPostSignerURL")))
	if !isJSONBatch || preURL == "" || postURL == "" {
		return nil, nil
	}
	return &RemoteBatchCommand{
		Session:          sesion,
		Payload:          append([]byte(nil), payload...),
		IsJSONBatch:      true,
		PreSignEndpoint:  preURL,
		PostSignEndpoint: postURL,
		NeedCert:         parseBoolLegacy(parametro(params, "needcert", "needCert")),
		LegacyParams:     clonarValores(params),
	}, nil
}

func normalizarPayloadRetrieve(raw []byte, sessionKey string, endpoints ...string) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}

	candidatos := [][]byte{append([]byte(nil), trimmed...)}
	if sessionKey != "" && requiereDescifradoLegacy(trimmed) {
		if plano, err := descifrarRetrieveLegacyDES(trimmed, sessionKey, endpoints...); err == nil {
			plano = bytes.TrimSpace(plano)
			if len(plano) > 0 {
				candidatos = append(candidatos, plano)
			}
		}
	}

	for _, candidato := range candidatos {
		if payloadRecuperadoParecePlano(candidato) {
			return candidato
		}
		if decoded, ok := decodeBase64CompatRetrieve(candidato); ok {
			return decoded
		}
	}
	return append([]byte(nil), trimmed...)
}

func requiereDescifradoLegacy(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false
	}
	return !payloadRecuperadoParecePlano([]byte(trimmed))
}

func payloadRecuperadoParecePlano(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, "[") ||
		strings.HasPrefix(trimmed, "<") ||
		strings.HasPrefix(trimmed, "%PDF")
}

func decodeBase64CompatRetrieve(data []byte) ([]byte, bool) {
	inputs := []string{strings.TrimSpace(string(data))}
	if dot := strings.IndexByte(inputs[0], '.'); dot >= 0 && dot+1 < len(inputs[0]) {
		inputs = append(inputs, strings.TrimSpace(inputs[0][dot+1:]))
	}
	decoders := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, input := range inputs {
		if input == "" {
			continue
		}
		for _, enc := range decoders {
			decoded, err := enc.DecodeString(input)
			if err != nil {
				continue
			}
			decoded = bytes.TrimSpace(decoded)
			if payloadRecuperadoParecePlano(decoded) {
				return decoded, true
			}
		}
	}
	return nil, false
}

func descifrarRetrieveLegacyDES(data []byte, keyRaw string, endpoints ...string) ([]byte, error) {
	if err := legacycrypto.RequireDESEnIntercambio(endpoints...); err != nil {
		return nil, err
	}
	texto := strings.TrimSpace(string(data))
	if texto == "" {
		return nil, fmt.Errorf("payload retrieve vacío")
	}

	padding := 0
	payload := texto
	if dot := strings.IndexByte(texto, '.'); dot >= 0 {
		valorPadding, err := strconv.Atoi(texto[:dot])
		if err != nil {
			return nil, fmt.Errorf("padding legacy inválido: %w", err)
		}
		padding = valorPadding
		payload = texto[dot+1:]
	}

	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")
	cifrado, err := decodeProtocolBase64(payload)
	if err != nil {
		return nil, fmt.Errorf("payload retrieve base64 inválido: %w", err)
	}

	clave := []byte(strings.TrimSpace(keyRaw))
	if len(clave) < 8 {
		padded := make([]byte, 8)
		copy(padded, clave)
		clave = padded
	} else if len(clave) > 8 {
		clave = clave[:8]
	}

	block, err := des.NewCipher(clave) // #nosec G405 -- read-only V1.9 compatibility path, gated by explicit operator opt-in above.
	if err != nil {
		return nil, err
	}
	if len(cifrado)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("ciphertext retrieve no es múltiplo del bloque DES")
	}

	plano := make([]byte, len(cifrado))
	for i := 0; i < len(cifrado); i += block.BlockSize() {
		block.Decrypt(plano[i:i+block.BlockSize()], cifrado[i:i+block.BlockSize()])
	}
	if padding > 0 && padding < len(plano) {
		plano = plano[:len(plano)-padding]
	}
	return plano, nil
}
