// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha1" // #nosec G505 -- SHA-1 is available only for explicitly enabled V1.9 signature interoperability.
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/cryptopolicy"
)

var legacyBatchWaitInterval = 10 * time.Second

const legacyBatchRemoteMaxAttempts = 3

type legacyBatchSigningKey interface {
	SignDigest(digest []byte, hash crypto.Hash) ([]byte, error)
	CertificateChainDER() [][]byte
}

type legacyBatchPreSigningKey interface {
	SignPreData(preData []byte, algorithm string, options map[string]string) (string, error)
}

type legacyBatchRequest struct {
	Algorithm   string                  `json:"algorithm"`
	ExtraParams string                  `json:"extraparams"`
	SingleSigns []legacyBatchSingleSign `json:"singlesigns"`
}

type legacyBatchRequestCompat struct {
	Algorithm   string                        `json:"algorithm"`
	ExtraParams string                        `json:"extraParams"`
	SingleSigns []legacyBatchSingleSignCompat `json:"singleSigns"`
}

type legacyBatchSingleSign struct {
	ID          string `json:"id"`
	ExtraParams string `json:"extraparams"`
}

type legacyBatchSingleSignCompat struct {
	ID          string `json:"id"`
	ExtraParams string `json:"extraParams"`
}

type legacyBatchPreResponse struct {
	TD      *legacyTriphaseDataResponse `json:"td"`
	Results []legacyBatchSingleResult   `json:"results"`
}

type legacyTriphaseDataResponse struct {
	Format   string                    `json:"format"`
	SignInfo []legacyTriphaseSignInfo  `json:"signinfo"`
	Signs    []legacyTriphaseSignBlock `json:"signs,omitempty"`
}

type legacyTriphaseSignBlock struct {
	SignInfo []legacyTriphaseSignInfo `json:"signinfo"`
}

type legacyTriphaseSignInfo struct {
	ID     string            `json:"id"`
	SignID string            `json:"signid,omitempty"`
	Params map[string]string `json:"params"`
}

type legacyTriphaseDataRequest struct {
	Format   string                   `json:"format,omitempty"`
	SignInfo []legacyTriphaseSignInfo `json:"signinfo"`
}

type legacyXMLTriphaseData struct {
	XMLName xml.Name                `xml:"xml"`
	Firmas  legacyXMLTriphaseFirmas `xml:"firmas"`
}

type legacyXMLTriphaseFirmas struct {
	Format string                   `xml:"format,attr,omitempty"`
	Firmas []legacyXMLTriphaseFirma `xml:"firma"`
}

type legacyXMLTriphaseFirma struct {
	ID     string                   `xml:"Id,attr,omitempty"`
	SignID string                   `xml:"signid,attr,omitempty"`
	Params []legacyXMLTriphaseParam `xml:"param"`
}

type legacyXMLTriphaseParam struct {
	Name  string `xml:"n,attr"`
	Value string `xml:",chardata"`
}

type legacyBatchResponse struct {
	Signs []legacyBatchSingleResult `json:"signs"`
}

type legacyBatchSingleResult struct {
	ID          string `json:"id"`
	Result      string `json:"result"`
	Description string `json:"description,omitempty"`
}

type legacyBatchParamEncodingVariant struct {
	Name   string
	Encode func(string) string
}

func (b *BatchExecutor) ExecuteLegacyRemote(ctx context.Context, req afirmauri.RemoteBatchCommand) error {
	if b == nil || b.executor == nil {
		return errors.New("el ejecutor batch remoto no está configurado")
	}
	if err := req.Session.Validate(); err != nil {
		return fmt.Errorf("sesión de batch remoto inválida: %w", err)
	}
	resultado, err := b.executor.executeLegacyRemoteBatchResult(ctx, req, true)
	if err != nil {
		return err
	}
	return b.executor.uploadLegacyBatchResult(ctx, req, resultado)
}

func (b *BatchExecutor) ExecuteLegacyRemoteResult(ctx context.Context, req afirmauri.RemoteBatchCommand) (string, error) {
	if b == nil || b.executor == nil {
		return "", errors.New("el ejecutor batch remoto no está configurado")
	}
	return b.executor.executeLegacyRemoteBatchResult(ctx, req, false)
}

func (e *Executor) executeLegacyRemoteBatchResult(ctx context.Context, req afirmauri.RemoteBatchCommand, requireExchange bool) (string, error) {
	traceLegacyBatch("inicio request_id=%s presign=%s postsign=%s upload=%s jsonbatch=%v needcert=%v payload=%s",
		strings.TrimSpace(req.Session.RequestID),
		summarizeLegacyBatchURL(req.PreSignEndpoint),
		summarizeLegacyBatchURL(req.PostSignEndpoint),
		summarizeLegacyBatchURL(req.Session.UploadEndpoint),
		req.IsJSONBatch,
		req.NeedCert,
		summarizeLegacyBatchBody(string(req.Payload)),
	)
	if strings.TrimSpace(req.PreSignEndpoint) == "" || strings.TrimSpace(req.PostSignEndpoint) == "" {
		return "", errors.New("faltan endpoints de prefirma/postfirma del lote")
	}
	if requireExchange {
		if err := e.sendLegacyBatchWait(ctx, req.Session); err != nil {
			traceLegacyBatch("wait error err=%v", err)
			return "", fmt.Errorf("wait remoto del lote fallido: %w", err)
		}
		traceLegacyBatch("wait ok")
		detenerWait := e.startLegacyBatchWaitLoop(ctx, req.Session)
		defer detenerWait()
	}

	signingKey, ok := signingKeyFromContext(ctx).(legacyBatchSigningKey)
	if !ok || signingKey == nil {
		return "", errors.New("la clave seleccionada no soporta firma batch legacy")
	}

	var (
		batchReq          *legacyBatchRequest
		algoritmo         string
		globalSignOptions map[string]string
		perSignOptions    map[string]map[string]string
		err               error
	)
	if req.IsJSONBatch {
		batchReq, err = parseLegacyBatchJSON(req.Payload)
		if err != nil {
			return "", err
		}
		algoritmo = strings.TrimSpace(batchReq.Algorithm)
		if algoritmo == "" {
			algoritmo = "SHA256withRSA"
		}
		globalSignOptions, perSignOptions = buildLegacyBatchSignOptions(batchReq)
	} else {
		algoritmo = extractLegacyBatchAlgorithmFromXML(req.Payload)
	}

	batchVariants := legacyBatchVariants(req.Payload)
	certVariants := legacyBatchCertVariants(signingKey.CertificateChainDER())
	if len(certVariants) == 0 {
		certVariants = []string{""}
	}
	paramEncodingVariants := legacyBatchParamEncodingVariants()

	var (
		preRaw               []byte
		postBatchValue       string
		certsValue           string
		selectedParamVariant = paramEncodingVariants[0]
		lastErr              error
	)
	batchParam := "xml"
	if req.IsJSONBatch {
		batchParam = "json"
	}
	for _, batchVariant := range batchVariants {
		for _, certVariant := range certVariants {
			for _, paramVariant := range paramEncodingVariants {
				candidateURL, err := buildLegacyBatchURL(req.PreSignEndpoint, [][2]string{
					{batchParam, batchVariant.Value},
					{"certs", certVariant},
				}, paramVariant.Encode)
				if err != nil {
					traceLegacyBatch("prefirma construir_url_error variant=%s params=%s err=%v", batchVariant.Value[:minLegacyBatchInt(12, len(batchVariant.Value))], paramVariant.Name, err)
					lastErr = err
					continue
				}
				traceLegacyBatch("prefirma intento batch_kind=%s params=%s certs=%s url=%s",
					legacyBatchEncodingName(batchVariant.Value),
					paramVariant.Name,
					fingerprintLegacyBatchBody(certVariant),
					summarizeLegacyBatchURL(candidateURL),
				)
				candidate, err := e.legacyBatchPostURL(ctx, candidateURL)
				if err != nil {
					traceLegacyBatch("prefirma error params=%s err=%v", paramVariant.Name, err)
					lastErr = err
					continue
				}
				if protoErr := detectLegacyBatchServiceProtocolError(candidate, "prefirma"); protoErr != nil {
					traceLegacyBatch("prefirma protocolo params=%s body=%s err=%v", paramVariant.Name, summarizeLegacyBatchBody(string(candidate)), protoErr)
					lastErr = protoErr
					continue
				}
				if legacyBatchPreResponseIsEmpty(candidate) {
					traceLegacyBatch("prefirma vacia params=%s body=%s", paramVariant.Name, summarizeLegacyBatchBody(string(candidate)))
					if len(preRaw) == 0 {
						preRaw = candidate
						postBatchValue = batchVariant.Value
						certsValue = certVariant
						selectedParamVariant = paramVariant
					}
					continue
				}
				preRaw = candidate
				postBatchValue = batchVariant.Value
				certsValue = certVariant
				selectedParamVariant = paramVariant
				traceLegacyBatch("prefirma ok params=%s body=%s", paramVariant.Name, summarizeLegacyBatchBody(string(candidate)))
				lastErr = nil
				break
			}
			if lastErr == nil && len(preRaw) > 0 {
				break
			}
		}
		if lastErr == nil && len(preRaw) > 0 {
			break
		}
	}
	if lastErr != nil {
		traceLegacyBatch("prefirma fallo_final err=%v", lastErr)
		return "", fmt.Errorf("prefirma remota del lote fallida: %w", lastErr)
	}

	postBatchRaw := req.Payload
	var tridataB64 string
	if req.IsJSONBatch {
		var preResp legacyBatchPreResponse
		if err := json.Unmarshal(preRaw, &preResp); err != nil {
			return "", fmt.Errorf("respuesta de prefirma remota inválida: %w", err)
		}
		normalizeLegacyBatchPreResponse(&preResp)
		emptyRetryDelays := []time.Duration{
			250 * time.Millisecond,
			500 * time.Millisecond,
			900 * time.Millisecond,
			1400 * time.Millisecond,
			2 * time.Second,
		}
		for intento := 1; preResp.TD == nil && len(preResp.Results) == 0 && intento <= len(emptyRetryDelays); intento++ {
			traceLegacyBatch(
				"prefirma vacia sondeo=%d/%d espera=%s batch_kind=%s params=%s cuerpo=%s",
				intento,
				len(emptyRetryDelays),
				emptyRetryDelays[intento-1],
				legacyBatchEncodingName(postBatchValue),
				selectedParamVariant.Name,
				summarizeLegacyBatchBody(string(preRaw)),
			)
			time.Sleep(emptyRetryDelays[intento-1])
			preRawRetry, retryErr := e.legacyBatchPostURL(ctx, buildLegacyRetryURL(req.PreSignEndpoint, batchParam, postBatchValue, certsValue, selectedParamVariant.Encode))
			if retryErr != nil {
				break
			}
			preRaw = preRawRetry
			var retryResp legacyBatchPreResponse
			if err := json.Unmarshal(preRawRetry, &retryResp); err != nil {
				break
			}
			normalizeLegacyBatchPreResponse(&retryResp)
			preResp = retryResp
		}

		if preResp.TD == nil || len(preResp.TD.SignInfo) == 0 {
			if len(preResp.Results) == 0 {
				return "", errors.New("prefirma remota sin datos trifásicos ni resultados")
			}
			raw, err := json.Marshal(legacyBatchResponse{Signs: preResp.Results})
			if err != nil {
				return "", fmt.Errorf("error serializando respuesta batch: %w", err)
			}
			resultado, err := formatLegacyBatchResult(raw, signingKey, req.NeedCert, req.Session.SessionKey, req.Session.RetrieveEndpoint, req.Session.UploadEndpoint)
			if err != nil {
				return "", err
			}
			return resultado, nil
		}

		tdSigned, err := signLegacyBatchTriphase(preResp.TD, signingKey, algoritmo, globalSignOptions, perSignOptions)
		if err != nil {
			return "", fmt.Errorf("firma trifásica batch fallida: %w", err)
		}
		traceLegacyBatchTriphaseIDSummary("prefirma_td", preResp.TD.SignInfo)
		traceLegacyBatchTriphaseIDSummary("postfirma_td", tdSigned.SignInfo)
		tdJSON, err := json.Marshal(tdSigned)
		if err != nil {
			return "", fmt.Errorf("error serializando tridata firmada: %w", err)
		}
		tridataB64 = base64.URLEncoding.EncodeToString(tdJSON)

		if len(preResp.Results) > 0 {
			if updated, mergeErr := mergeLegacyPresignResults(req.Payload, preResp.Results); mergeErr == nil {
				postBatchRaw = updated
			}
		}
	} else {
		traceLegacyBatchXMLParams(preRaw)
		tdXML, err := signLegacyBatchTriphaseXML(preRaw, signingKey, algoritmo, globalSignOptions, perSignOptions)
		if err != nil {
			return "", fmt.Errorf("firma trifásica batch XML fallida: %w", err)
		}
		tridataB64 = base64.URLEncoding.EncodeToString(tdXML)
	}

	if strings.Contains(postBatchValue, "/") || strings.Contains(postBatchValue, "+") {
		postBatchValue = base64.StdEncoding.EncodeToString(postBatchRaw)
	} else {
		postBatchValue = base64.URLEncoding.EncodeToString(postBatchRaw)
	}

	postURL, err := buildLegacyBatchURL(req.PostSignEndpoint, [][2]string{
		{batchParam, postBatchValue},
		{"certs", certsValue},
		{"tridata", tridataB64},
	}, selectedParamVariant.Encode)
	if err != nil {
		traceLegacyBatch("postfirma construir_url_error err=%v", err)
		return "", fmt.Errorf("error construyendo postfirma remota del lote: %w", err)
	}
	traceLegacyBatch("postfirma intento params=%s url=%s tridata=%s", selectedParamVariant.Name, summarizeLegacyBatchURL(postURL), summarizeLegacyBatchBody(tridataB64))
	if req.IsJSONBatch {
		if tdJSON, err := decodeLegacyProtocolBase64(tridataB64); err == nil {
			traceLegacyBatchPostIDCheck(postBatchRaw, tdJSON)
		}
	}
	postRaw, err := e.legacyBatchPostURL(ctx, postURL)
	if err != nil {
		traceLegacyBatch("postfirma error err=%v", err)
		return "", fmt.Errorf("postfirma remota del lote fallida: %w", err)
	}
	if protoErr := detectLegacyBatchServiceProtocolError(postRaw, "postfirma"); protoErr != nil {
		traceLegacyBatch("postfirma protocolo body=%s err=%v", summarizeLegacyBatchBody(string(postRaw)), protoErr)
		return "", protoErr
	}
	traceLegacyBatch("postfirma ok body=%s", summarizeLegacyBatchBody(string(postRaw)))

	resultado, err := formatLegacyBatchResult(postRaw, signingKey, req.NeedCert, req.Session.SessionKey, req.Session.RetrieveEndpoint, req.Session.UploadEndpoint)
	if err != nil {
		traceLegacyBatch("formato_resultado error err=%v", err)
		return "", err
	}
	traceLegacyBatch("resultado listo resumen=%s fp=%s", summarizeLegacyBatchBody(resultado), fingerprintLegacyBatchBody(resultado))
	return resultado, nil
}

type legacyEncodedVariant struct {
	Value string
}

func legacyBatchVariants(raw []byte) []legacyEncodedVariant {
	return []legacyEncodedVariant{
		{Value: base64.URLEncoding.EncodeToString(raw)},
		{Value: base64.StdEncoding.EncodeToString(raw)},
	}
}

func legacyBatchParamEncodingVariants() []legacyBatchParamEncodingVariant {
	// El cliente Java envía los valores Base64 URL-safe sin escapar; algunos
	// servidores (p. ej. JCyL) rechazan con 400 el relleno escapado como %3D.
	return []legacyBatchParamEncodingVariant{
		{Name: "java_raw", Encode: func(v string) string { return v }},
		{Name: "escape", Encode: url.QueryEscape},
		{
			Name: "escape_pct20",
			Encode: func(v string) string {
				return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
			},
		},
	}
}

func legacyBatchCertVariants(chain [][]byte) []string {
	if len(chain) == 0 {
		return nil
	}
	variants := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		variants = append(variants, value)
	}
	add(encodeLegacyBatchCertChain(chain[:1], base64.URLEncoding))
	add(encodeLegacyBatchCertChain(chain[:1], base64.StdEncoding))
	add(encodeLegacyBatchCertChain(chain, base64.URLEncoding))
	add(encodeLegacyBatchCertChain(chain, base64.StdEncoding))
	return variants
}

func buildLegacyRetryURL(endpoint, batchParam, batchValue, certsValue string, encode func(string) string) string {
	u, err := buildLegacyBatchURL(endpoint, [][2]string{
		{batchParam, batchValue},
		{"certs", certsValue},
	}, encode)
	if err != nil {
		return strings.TrimSpace(endpoint)
	}
	return u
}

func encodeLegacyBatchCertChain(chain [][]byte, enc *base64.Encoding) string {
	parts := make([]string, 0, len(chain))
	for _, der := range chain {
		if len(der) == 0 {
			continue
		}
		parts = append(parts, enc.EncodeToString(der))
	}
	return strings.Join(parts, ";")
}

func parseLegacyBatchJSON(raw []byte) (*legacyBatchRequest, error) {
	var req legacyBatchRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("json de lote inválido: %w", err)
	}
	if len(req.SingleSigns) == 0 {
		var compat legacyBatchRequestCompat
		if err := json.Unmarshal(raw, &compat); err == nil && len(compat.SingleSigns) > 0 {
			req.SingleSigns = make([]legacyBatchSingleSign, 0, len(compat.SingleSigns))
			for _, sign := range compat.SingleSigns {
				req.SingleSigns = append(req.SingleSigns, legacyBatchSingleSign{
					ID:          strings.TrimSpace(sign.ID),
					ExtraParams: strings.TrimSpace(sign.ExtraParams),
				})
			}
			if strings.TrimSpace(req.Algorithm) == "" {
				req.Algorithm = strings.TrimSpace(compat.Algorithm)
			}
			if strings.TrimSpace(req.ExtraParams) == "" {
				req.ExtraParams = strings.TrimSpace(compat.ExtraParams)
			}
		}
	}
	if len(req.SingleSigns) == 0 {
		return nil, errors.New("el lote remoto no contiene operaciones")
	}
	return &req, nil
}

func normalizeLegacyBatchSignID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func buildLegacyBatchSignOptions(req *legacyBatchRequest) (map[string]string, map[string]map[string]string) {
	if req == nil {
		return nil, nil
	}
	global := legacyDecodeBatchExtraParams(strings.TrimSpace(req.ExtraParams))
	if len(global) == 0 {
		global = nil
	}
	perSign := make(map[string]map[string]string)
	for _, sign := range req.SingleSigns {
		signID := normalizeLegacyBatchSignID(sign.ID)
		if signID == "" {
			continue
		}
		opts := legacyDecodeBatchExtraParams(strings.TrimSpace(sign.ExtraParams))
		if len(opts) == 0 {
			continue
		}
		perSign[signID] = opts
	}
	if len(perSign) == 0 {
		return global, nil
	}
	return global, perSign
}

func normalizeLegacyBatchPreResponse(preResp *legacyBatchPreResponse) {
	if preResp == nil || preResp.TD == nil {
		return
	}
	if len(preResp.TD.SignInfo) > 0 || len(preResp.TD.Signs) == 0 {
		return
	}
	flat := make([]legacyTriphaseSignInfo, 0, 8)
	for _, block := range preResp.TD.Signs {
		flat = append(flat, block.SignInfo...)
	}
	preResp.TD.SignInfo = flat
}

func legacyBatchPreResponseIsEmpty(raw []byte) bool {
	var preResp legacyBatchPreResponse
	if err := json.Unmarshal(raw, &preResp); err != nil {
		return false
	}
	normalizeLegacyBatchPreResponse(&preResp)
	return preResp.TD == nil && len(preResp.Results) == 0
}

func signLegacyBatchTriphase(td *legacyTriphaseDataResponse, key legacyBatchSigningKey, algorithm string, signOptions map[string]string, perSignOptions map[string]map[string]string) (*legacyTriphaseDataRequest, error) {
	hash, err := legacyHashFromAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}
	out := &legacyTriphaseDataRequest{
		Format:   strings.TrimSpace(td.Format),
		SignInfo: make([]legacyTriphaseSignInfo, 0, len(td.SignInfo)),
	}
	for _, si := range td.SignInfo {
		preB64 := strings.TrimSpace(si.Params["PRE"])
		if preB64 == "" {
			return nil, errors.New("respuesta trifásica sin PRE")
		}
		preData, err := decodeLegacyProtocolBase64(preB64)
		if err != nil {
			return nil, fmt.Errorf("PRE inválido: %w", err)
		}
		opts := cloneLegacySignOptions(signOptions)
		if len(perSignOptions) > 0 {
			signKey := normalizeLegacyBatchSignID(si.ID)
			if signKey == "" {
				signKey = normalizeLegacyBatchSignID(si.SignID)
			}
			if extra := perSignOptions[signKey]; len(extra) > 0 {
				for k, v := range extra {
					opts[k] = v
				}
			}
		}
		pk1B64, err := signLegacyBatchPreData(preData, key, algorithm, hash, opts)
		if err != nil {
			return nil, fmt.Errorf("error firmando PRE: %w", err)
		}
		params := make(map[string]string, len(si.Params)+1)
		for k, v := range si.Params {
			params[k] = v
		}
		params["PK1"] = pk1B64
		postID := strings.TrimSpace(si.ID)
		postSignID := strings.TrimSpace(si.SignID)
		if postID == "" {
			postID = postSignID
		}
		if postID == "" {
			postID = strings.TrimSpace(params["ID"])
		}
		out.SignInfo = append(out.SignInfo, legacyTriphaseSignInfo{
			ID:     postID,
			SignID: postSignID,
			Params: params,
		})
	}
	return out, nil
}

func signLegacyBatchTriphaseXML(rawTD []byte, key legacyBatchSigningKey, algorithm string, signOptions map[string]string, perSignOptions map[string]map[string]string) ([]byte, error) {
	var td legacyXMLTriphaseData
	if err := xml.Unmarshal(rawTD, &td); err != nil {
		return nil, err
	}
	hash, err := legacyHashFromAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}
	for i := range td.Firmas.Firmas {
		preB64 := ""
		for j := range td.Firmas.Firmas[i].Params {
			if strings.EqualFold(td.Firmas.Firmas[i].Params[j].Name, "PRE") {
				preB64 = strings.TrimSpace(td.Firmas.Firmas[i].Params[j].Value)
				break
			}
		}
		if preB64 == "" {
			return nil, errors.New("falta PRE")
		}
		preData, err := decodeLegacyProtocolBase64(preB64)
		if err != nil {
			return nil, err
		}
		opts := cloneLegacySignOptions(signOptions)
		if len(perSignOptions) > 0 {
			signKey := normalizeLegacyBatchSignID(td.Firmas.Firmas[i].ID)
			if signKey == "" {
				signKey = normalizeLegacyBatchSignID(td.Firmas.Firmas[i].SignID)
			}
			if extra := perSignOptions[signKey]; len(extra) > 0 {
				for k, v := range extra {
					opts[k] = v
				}
			}
		}
		pk1B64, err := signLegacyBatchPreData(preData, key, algorithm, hash, opts)
		if err != nil {
			return nil, err
		}
		td.Firmas.Firmas[i].Params = append(td.Firmas.Firmas[i].Params, legacyXMLTriphaseParam{
			Name:  "PK1",
			Value: pk1B64,
		})
	}
	out, err := xml.Marshal(td)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hashLegacyPreData(preData []byte, hash crypto.Hash) ([]byte, error) {
	switch hash {
	case crypto.SHA1:
		if err := cryptopolicy.RequireLegacySHA1(); err != nil {
			return nil, err
		}
		sum := sha1.Sum(preData) // #nosec G401 -- guarded legacy signature path; SHA-256+ remains the default.
		return sum[:], nil
	case crypto.SHA256:
		sum := sha256.Sum256(preData)
		return sum[:], nil
	case crypto.SHA384:
		sum := sha512.Sum384(preData)
		return sum[:], nil
	case crypto.SHA512:
		sum := sha512.Sum512(preData)
		return sum[:], nil
	default:
		return nil, fmt.Errorf("hash no soportado para lote legacy: %v", hash)
	}
}

func cloneLegacySignOptions(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func signLegacyBatchPreData(preData []byte, key legacyBatchSigningKey, algorithm string, hash crypto.Hash, options map[string]string) (string, error) {
	if hash == crypto.SHA1 {
		if err := cryptopolicy.RequireLegacySHA1(); err != nil {
			return "", err
		}
	}
	if typed, ok := key.(legacyBatchPreSigningKey); ok {
		return typed.SignPreData(preData, algorithm, options)
	}
	digest, err := hashLegacyPreData(preData, hash)
	if err != nil {
		return "", err
	}
	firma, err := key.SignDigest(digest, hash)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(firma), nil
}

func legacyHashFromAlgorithm(raw string) (crypto.Hash, error) {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	switch {
	case strings.Contains(upper, "SHA512"):
		return crypto.SHA512, nil
	case strings.Contains(upper, "SHA384"):
		return crypto.SHA384, nil
	case strings.Contains(upper, "SHA256"):
		return crypto.SHA256, nil
	case strings.Contains(upper, "SHA1"):
		return crypto.SHA1, nil
	default:
		return 0, fmt.Errorf("algoritmo de lote no soportado: %s", raw)
	}
}

func extractLegacyBatchAlgorithmFromXML(rawBatch []byte) string {
	type legacyXMLBatchRequest struct {
		XMLName   xml.Name `xml:"signbatch"`
		Algorithm string   `xml:"algorithm,attr"`
	}
	var req legacyXMLBatchRequest
	if err := xml.Unmarshal(rawBatch, &req); err != nil {
		return "SHA256withRSA"
	}
	if strings.TrimSpace(req.Algorithm) == "" {
		return "SHA256withRSA"
	}
	return strings.TrimSpace(req.Algorithm)
}

func decodeLegacyProtocolBase64(raw string) ([]byte, error) {
	s := strings.ReplaceAll(strings.TrimSpace(raw), " ", "+")
	if out, err := base64.StdEncoding.DecodeString(s); err == nil {
		return out, nil
	}
	if out, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return out, nil
	}
	if out, err := base64.URLEncoding.DecodeString(s); err == nil {
		return out, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

func legacyDecodeBatchExtraParams(v string) map[string]string {
	body := strings.TrimSpace(v)
	if body == "" {
		return map[string]string{}
	}
	if decoded, err := decodeLegacyProtocolBase64(body); err == nil && len(decoded) > 0 {
		body = string(decoded)
	}
	props := make(map[string]string)
	body = strings.ReplaceAll(body, `\n`, "\n")
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			props[strings.ToLower(line)] = ""
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:sep]))
		value := strings.TrimSpace(line[sep+1:])
		if decoded, err := url.QueryUnescape(value); err == nil {
			value = decoded
		}
		if key != "" {
			props[key] = value
		}
	}
	return props
}

func mergeLegacyPresignResults(rawBatch []byte, results []legacyBatchSingleResult) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(rawBatch, &root); err != nil {
		return nil, err
	}
	signs, ok := root["singlesigns"].([]any)
	if !ok {
		return rawBatch, nil
	}
	byID := make(map[string]legacyBatchSingleResult, len(results))
	for _, r := range results {
		byID[strings.TrimSpace(r.ID)] = r
	}
	for _, item := range signs {
		sign, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := sign["id"].(string)
		res, ok := byID[strings.TrimSpace(id)]
		if !ok {
			continue
		}
		delete(sign, "datareference")
		delete(sign, "format")
		delete(sign, "suboperation")
		delete(sign, "extraparams")
		sign["result"] = res.Result
		if strings.TrimSpace(res.Description) != "" {
			sign["description"] = res.Description
		}
	}
	return json.Marshal(root)
}

func traceLegacyBatchTriphaseIDSummary(stage string, items []legacyTriphaseSignInfo) {
	if len(items) == 0 {
		traceLegacyBatch("%s: sin firmas en tri-fase", stage)
		return
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.ID)
		signID := strings.TrimSpace(it.SignID)
		if id == "" {
			id = "-"
		}
		if signID == "" {
			signID = "-"
		}
		parts = append(parts, "id="+id+"/signid="+signID)
	}
	traceLegacyBatch("%s: firmas=%d [%s]", stage, len(items), strings.Join(parts, ", "))
}

func traceLegacyBatchPostIDCheck(rawBatch []byte, tdSignedJSON []byte) {
	var root map[string]any
	if err := json.Unmarshal(rawBatch, &root); err != nil {
		traceLegacyBatch("post-id-check: lote JSON no parseable (%v)", err)
		return
	}
	ss, ok := root["singlesigns"].([]any)
	if !ok {
		traceLegacyBatch("post-id-check: sin singlesigns")
		return
	}
	want := make([]string, 0, len(ss))
	for _, it := range ss {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		id = strings.TrimSpace(id)
		if id != "" {
			want = append(want, id)
		}
	}

	var td legacyTriphaseDataRequest
	if err := json.Unmarshal(tdSignedJSON, &td); err != nil {
		traceLegacyBatch("post-id-check: tridata JSON no parseable (%v)", err)
		return
	}
	gotMap := make(map[string]struct{}, len(td.SignInfo))
	for _, si := range td.SignInfo {
		id := strings.TrimSpace(si.ID)
		if id == "" {
			id = strings.TrimSpace(si.SignID)
		}
		if id != "" {
			gotMap[id] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, id := range want {
		if _, ok := gotMap[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		traceLegacyBatch("post-id-check: OK ids_lote=%d ids_trifase=%d", len(want), len(gotMap))
		return
	}
	traceLegacyBatch("post-id-check: faltan_ids_en_trifase=%v ids_lote=%d ids_trifase=%d", missing, len(want), len(gotMap))
}

func formatLegacyBatchResult(result []byte, key legacyBatchSigningKey, needCert bool, sessionKey string, endpoints ...string) (string, error) {
	if strings.TrimSpace(sessionKey) != "" {
		encBatch, err := cifrarLegacyDES(result, sessionKey, endpoints...)
		if err != nil {
			return "", fmt.Errorf("error cifrando resultado batch: %w", err)
		}
		if !needCert {
			return string(encBatch), nil
		}
		chain := key.CertificateChainDER()
		if len(chain) == 0 {
			return string(encBatch), nil
		}
		encCert, err := cifrarLegacyDES(chain[0], sessionKey, endpoints...)
		if err != nil {
			return "", fmt.Errorf("error cifrando certificado batch: %w", err)
		}
		traceLegacyBatch("resultado cert cifrado fp=%s", fingerprintLegacyBatchBody(string(encCert)))
		return string(encBatch) + "|" + string(encCert), nil
	}

	batchB64 := base64.StdEncoding.EncodeToString(result)
	if !needCert {
		return batchB64, nil
	}
	chain := key.CertificateChainDER()
	if len(chain) == 0 {
		return batchB64, nil
	}
	certB64 := base64.StdEncoding.EncodeToString(chain[0])
	traceLegacyBatch("resultado cert plano fp=%s", fingerprintLegacyBatchBody(certB64))
	return batchB64 + "|" + certB64, nil
}

func (e *Executor) legacyBatchPostURL(ctx context.Context, rawURL string) ([]byte, error) {
	target, baseURL, formBody, err := splitLegacyBatchRequest(rawURL)
	if err != nil {
		return nil, err
	}
	return e.legacyBatchPostPrepared(ctx, target, baseURL, formBody)
}

func (e *Executor) legacyBatchPostPrepared(ctx context.Context, target, baseURL, formBody string) ([]byte, error) {
	var lastErr error
	for intento := 1; intento <= legacyBatchRemoteMaxAttempts; intento++ {
		body, status, err := e.legacyBatchPostPreparedOnce(ctx, target, baseURL, formBody)
		if err == nil {
			if status < 200 || status > 299 {
				return nil, fmt.Errorf("HTTP %d: %s", status, strings.TrimSpace(string(body)))
			}
			return body, nil
		}
		lastErr = err
		if !shouldRetryLegacyBatchTransportError(err) || intento == legacyBatchRemoteMaxAttempts {
			break
		}
		espera := legacyBatchRetryDelay(intento)
		traceLegacyBatch("http post retry intento=%d/%d espera=%s target=%s motivo=%v", intento, legacyBatchRemoteMaxAttempts, espera, summarizeLegacyBatchURL(target), err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(espera):
		}
	}
	return nil, lastErr
}

func (e *Executor) legacyBatchPostPreparedOnce(ctx context.Context, target, baseURL, formBody string) ([]byte, int, error) {
	body, status, err := e.doLegacyBatchPOST(ctx, target, nil)
	if err != nil {
		if strings.TrimSpace(formBody) == "" || !shouldRetryLegacyProtocolWithFallback(err) {
			return nil, 0, err
		}
		traceLegacyBatch("http post fallback=form-body motivo=%v target=%s", err, summarizeLegacyBatchURL(target))
		body, status, err = e.doLegacyBatchPOST(ctx, baseURL, strings.NewReader(formBody))
		if err != nil {
			return nil, 0, err
		}
	}
	if shouldFallbackLegacyBatchFormStatus(status) {
		body, status, err = e.doLegacyBatchPOST(ctx, baseURL, strings.NewReader(formBody))
		if err != nil {
			return nil, 0, err
		}
	}
	return body, status, nil
}

func splitLegacyBatchRequest(rawURL string) (string, string, string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", "", fmt.Errorf("endpoint batch inválido: %w", err)
	}
	form := strings.TrimSpace(u.RawQuery)
	withQuery := u.String()
	u.RawQuery = ""
	return withQuery, u.String(), form, nil
}

func buildLegacyBatchURL(endpoint string, params [][2]string, encode func(string) string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("endpoint batch inválido: %w", err)
	}
	parts := make([]string, 0, len(params)+1)
	if raw := strings.TrimSpace(u.RawQuery); raw != "" {
		parts = append(parts, raw)
	}
	for _, kv := range params {
		parts = append(parts, url.QueryEscape(kv[0])+"="+encode(kv[1]))
	}
	u.RawQuery = strings.Join(parts, "&")
	return u.String(), nil
}

func (e *Executor) doLegacyBatchPOST(ctx context.Context, target string, body io.Reader) ([]byte, int, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = io.ReadAll(body)
		if err != nil {
			return nil, 0, err
		}
	}
	newBody := func() io.Reader {
		if payload == nil {
			return nil
		}
		return bytes.NewReader(payload)
	}
	doPost := func(client *http.Client, transportName string) ([]byte, int, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, target, newBody())
		if reqErr != nil {
			return nil, 0, reqErr
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		}
		req.Header.Set("User-Agent", "AutoFirma/1.6.5")
		req.Header.Set("Accept", "*/*")
		traceLegacyBatch("http post target=%s ctype=%q transport=%s", summarizeLegacyBatchURL(target), req.Header.Get("Content-Type"), transportName)
		resp, doErr := client.Do(req)
		if doErr != nil {
			traceLegacyBatch("http post error target=%s transport=%s err=%v", summarizeLegacyBatchURL(target), transportName, doErr)
			return nil, 0, doErr
		}
		defer resp.Body.Close()
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, e.limits.MaxPayloadBytes))
		if readErr != nil {
			return nil, 0, readErr
		}
		traceLegacyBatch("http post status=%d target=%s transport=%s body=%s", resp.StatusCode, summarizeLegacyBatchURL(target), transportName, summarizeLegacyBatchBody(string(data)))
		return data, resp.StatusCode, nil
	}
	data, status, err := doPost(freshHTTPClient(e.httpClient), "legacy-http1")
	if err == nil || e.fallbackHTTPClient == nil || !shouldRetryLegacyProtocolWithFallback(err) {
		return data, status, err
	}
	traceLegacyBatch("http post retry transport=fallback-h2 target=%s motivo=%v", summarizeLegacyBatchURL(target), err)
	return doPost(freshHTTPClient(e.fallbackHTTPClient), "fallback-h2")
}

// shouldFallbackLegacyBatchFormStatus decide si repetir la petición con los
// parámetros en el cuerpo, como hace el cliente Java. Los errores 4xx sobre la
// forma de la petición lo activan aunque el servidor devuelva una página
// genérica (Tomcat responde 400 sin detalle cuando no lee la query en POST).
func shouldFallbackLegacyBatchFormStatus(status int) bool {
	switch status {
	case http.StatusBadRequest,
		http.StatusMethodNotAllowed,
		http.StatusLengthRequired,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType:
		return true
	default:
		return false
	}
}

func shouldRetryLegacyBatchTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func legacyBatchRetryDelay(intento int) time.Duration {
	if intento < 1 {
		intento = 1
	}
	return time.Duration(1<<(intento-1)) * 150 * time.Millisecond
}

func detectLegacyBatchServiceProtocolError(raw []byte, phase string) error {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	up := strings.ToUpper(text)
	if strings.HasPrefix(up, "SAF_") {
		return fmt.Errorf("%s", text)
	}
	if strings.HasPrefix(up, "ERR-") || strings.Contains(up, "ERROR_POST") {
		return fmt.Errorf("error en servicio de %s de lote (%s)", phase, summarizeLegacyBatchBody(text))
	}
	return nil
}

func summarizeLegacyBatchBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) <= 500 {
		return body
	}
	return body[:500]
}

func fingerprintLegacyBatchBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "len=0 sha12=e3b0c44298fc"
	}
	sum := sha256.Sum256([]byte(body))
	return fmt.Sprintf("len=%d sha12=%x", len(body), sum[:6])
}

func summarizeLegacyBatchURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return strings.TrimSpace(raw)
	}
	out := u.Scheme + "://" + u.Host + u.Path
	if strings.TrimSpace(u.RawQuery) != "" {
		out += "?" + summarizeLegacyBatchBody(u.RawQuery)
	}
	return out
}

func (e *Executor) sendLegacyBatchWait(ctx context.Context, session domain.ExchangeSession) error {
	uploadEndpoint := strings.TrimSpace(session.UploadEndpoint)
	if uploadEndpoint == "" {
		uploadEndpoint = strings.TrimSpace(session.RetrieveEndpoint)
	}
	if uploadEndpoint == "" {
		return errors.New("endpoint upload batch vacío para WAIT")
	}
	form := url.Values{}
	form.Set("op", "put")
	form.Set("v", "1_0")
	form.Set("id", session.RequestID)
	form.Set("dat", "#WAIT")
	u, err := url.Parse(uploadEndpoint)
	if err != nil {
		return fmt.Errorf("endpoint WAIT inválido: %w", err)
	}
	body, status, err := e.doLegacyBatchPOST(ctx, func() string {
		u.RawQuery = ""
		return u.String()
	}(), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	bodyText := strings.TrimSpace(string(body))
	traceLegacyBatch("wait status=%d body=%s", status, summarizeLegacyBatchBody(bodyText))
	if status != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", status, bodyText)
	}
	if !isStorageUploadOKResponse(bodyText) {
		return fmt.Errorf("cuerpo no-OK: %s", bodyText)
	}
	return nil
}

func (e *Executor) startLegacyBatchWaitLoop(ctx context.Context, session domain.ExchangeSession) func() {
	intervalo := legacyBatchWaitInterval
	if intervalo <= 0 {
		return func() {}
	}

	detener := make(chan struct{})
	go func() {
		ticker := time.NewTicker(intervalo)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-detener:
				return
			case <-ticker.C:
				if err := e.sendLegacyBatchWait(ctx, session); err != nil {
					traceLegacyBatch("wait periodico error err=%v", err)
					continue
				}
				traceLegacyBatch("wait periodico ok")
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() { close(detener) })
	}
}

func (e *Executor) uploadLegacyBatchResult(ctx context.Context, req afirmauri.RemoteBatchCommand, dat string) error {
	if strings.TrimSpace(dat) == "" {
		return errors.New("resultado batch vacío")
	}
	session := req.Session
	uploadEndpoint := strings.TrimSpace(session.UploadEndpoint)
	if uploadEndpoint == "" {
		uploadEndpoint = strings.TrimSpace(session.RetrieveEndpoint)
	}
	u, err := url.Parse(uploadEndpoint)
	if err != nil {
		return fmt.Errorf("endpoint upload batch inválido: %w", err)
	}
	sendBody := func(form url.Values) (bool, string, int, error) {
		body, status, err := e.doLegacyBatchPOST(ctx, func() string {
			u.RawQuery = ""
			return u.String()
		}(), strings.NewReader(form.Encode()))
		if err != nil {
			return false, "", 0, err
		}
		bodyText := strings.TrimSpace(string(body))
		if status != http.StatusOK {
			return false, bodyText, status, nil
		}
		return isStorageUploadOKResponse(bodyText), bodyText, status, nil
	}
	sendQuery := func(form url.Values) (bool, string, int, error) {
		target := func() string {
			queryURL := *u
			queryURL.RawQuery = form.Encode()
			return queryURL.String()
		}()
		body, status, err := e.doLegacyBatchPOST(ctx, target, nil)
		if err != nil {
			return false, "", 0, err
		}
		bodyText := strings.TrimSpace(string(body))
		if status != http.StatusOK {
			return false, bodyText, status, nil
		}
		return isStorageUploadOKResponse(bodyText), bodyText, status, nil
	}

	javaStyle := url.Values{}
	javaStyle.Set("op", "put")
	javaStyle.Set("v", "1_0")
	javaStyle.Set("id", session.RequestID)
	javaStyle.Set("dat", dat)
	traceLegacyBatch("upload dat fp=%s", fingerprintLegacyBatchBody(dat))
	ok, bodyText, status, err := sendQuery(javaStyle)
	if err != nil {
		traceLegacyBatch("upload java-query error err=%v", err)
		return fmt.Errorf("subida batch fallida: %w", err)
	}
	traceLegacyBatch("upload java-query status=%d ok=%v body=%s keys=%s", status, ok, summarizeLegacyBatchBody(bodyText), summarizeLegacyBatchKeys(javaStyle))
	if ok {
		return nil
	}

	ok, bodyText, status, err = sendBody(javaStyle)
	if err != nil {
		traceLegacyBatch("upload java-body error err=%v", err)
		return fmt.Errorf("subida batch fallida: %w", err)
	}
	traceLegacyBatch("upload java-body status=%d ok=%v body=%s keys=%s", status, ok, summarizeLegacyBatchBody(bodyText), summarizeLegacyBatchKeys(javaStyle))
	if ok {
		return nil
	}

	legacy := clonarLegacyBatchForm(req.LegacyParams)
	legacy.Set("op", "put")
	legacy.Set("v", "1_0")
	legacy.Set("id", session.RequestID)
	legacy.Set("dat", dat)
	ok, bodyText, status, err = sendBody(legacy)
	if err != nil {
		traceLegacyBatch("upload legacy error err=%v", err)
		return fmt.Errorf("subida batch fallida: %w", err)
	}
	traceLegacyBatch("upload legacy status=%d ok=%v body=%s keys=%s", status, ok, summarizeLegacyBatchBody(bodyText), summarizeLegacyBatchKeys(legacy))
	if ok {
		return nil
	}
	if status != 0 {
		return fmt.Errorf("upload batch devolvió HTTP %d: %s", status, bodyText)
	}
	return fmt.Errorf("la subida batch devolvió cuerpo no-OK: %s", bodyText)
}

func summarizeLegacyBatchKeys(v url.Values) string {
	if len(v) == 0 {
		return ""
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func clonarLegacyBatchForm(src url.Values) url.Values {
	return clonarLegacyForm(src)
}

func isStorageUploadOKResponse(bodyText string) bool {
	normalized := strings.TrimSpace(bodyText)
	normalized = strings.Trim(normalized, "\"'")
	switch strings.ToUpper(normalized) {
	case "OK", "SAVE_OK", "1", "1)":
		return true
	}
	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	for _, decode := range decoders {
		decoded, err := decode(normalized)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(decoded))
		value = strings.Trim(value, "\"'")
		switch strings.ToUpper(value) {
		case "OK", "SAVE_OK", "1", "1)":
			return true
		}
	}
	return false
}

func traceLegacyBatchXMLParams(rawTD []byte) {
	var td legacyXMLTriphaseData
	if err := xml.Unmarshal(rawTD, &td); err != nil {
		traceLegacyBatch("prefirma_xml parse_err=%v", err)
		return
	}
	for _, firma := range td.Firmas.Firmas {
		names := make([]string, 0, len(firma.Params))
		for _, p := range firma.Params {
			names = append(names, p.Name)
		}
		traceLegacyBatch("prefirma_xml id=%s format=%q params=%s", firma.ID, td.Firmas.Format, strings.Join(names, ","))
	}
}

func traceLegacyBatch(format string, _ ...any) {
	phase, outcome := legacyBatchTraceVocabulary(format)
	slog.Debug(
		"legacy_batch_event",
		"phase", phase,
		"outcome", outcome,
	)
}

func legacyBatchTraceVocabulary(format string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(format))
	phase := "batch"
	switch {
	case strings.HasPrefix(lower, "inicio"):
		phase = "start"
	case strings.HasPrefix(lower, "wait"):
		phase = "wait"
	case strings.HasPrefix(lower, "prefirma"), strings.HasPrefix(lower, "http post"):
		phase = "presign"
	case strings.HasPrefix(lower, "postfirma"), strings.HasPrefix(lower, "post-id-check"):
		phase = "postsign"
	case strings.HasPrefix(lower, "upload"):
		phase = "upload"
	case strings.HasPrefix(lower, "resultado"):
		phase = "result"
	}

	outcome := "progress"
	switch {
	case strings.Contains(lower, "error"),
		strings.Contains(lower, "fallo"),
		strings.Contains(lower, "vacia"),
		strings.Contains(lower, "no parseable"),
		strings.Contains(lower, "faltan_"):
		outcome = "failed"
	case strings.Contains(lower, " ok"),
		strings.HasSuffix(lower, "ok"),
		strings.Contains(lower, "listo"):
		outcome = "completed"
	}
	return phase, outcome
}

func minLegacyBatchInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func legacyBatchEncodingName(value string) string {
	if strings.Contains(value, "+") || strings.Contains(value, "/") {
		return "std"
	}
	return "urlsafe"
}
