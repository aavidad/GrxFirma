// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/limits"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type signingKeyContextKey struct{}

const maxReintentos = 3

// retrieveResponse es la respuesta JSON del RTServlet.
type retrieveResponse struct {
	Dat         string `json:"dat"`
	Op          string `json:"op"`
	Format      string `json:"format"`
	Algorithm   string `json:"algorithm"`
	ExtraParams string `json:"extraParams"`
}

// presignResponse es la respuesta JSON de la fase PreSign.
type presignResponse struct {
	Sign        string `json:"sign"`
	ExtraParams string `json:"extraParams"`
}

// Executor ejecuta el protocolo trifásico completo de AutoFirma.
type Executor struct {
	httpClient         *http.Client
	fallbackHTTPClient *http.Client
	limits             limits.Limits
}

// ContextWithSigningKey fija en el contexto la clave de firma que debe usar el
// protocolo trifásico para la firma local del hash recibido en presign.
func ContextWithSigningKey(ctx context.Context, key ports.SigningKey) context.Context {
	if key == nil {
		return ctx
	}
	return context.WithValue(ctx, signingKeyContextKey{}, key)
}

func signingKeyFromContext(ctx context.Context) ports.SigningKey {
	if ctx == nil {
		return nil
	}
	key, _ := ctx.Value(signingKeyContextKey{}).(ports.SigningKey)
	return key
}

// New construye un Executor con el cliente HTTP proporcionado.
// Usa los límites por defecto del sistema.
func New(httpClient *http.Client) *Executor {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: time.Duration(limits.Default().HTTPTimeoutSec) * time.Second}
	}
	legacyClient := legacyCompatHTTPClient(httpClient)
	fallbackClient := http2CapableHTTPClient(httpClient)
	return &Executor{
		httpClient:         legacyClient,
		fallbackHTTPClient: fallbackClient,
		limits:             limits.Default(),
	}
}

func legacyCompatHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: time.Duration(limits.Default().HTTPTimeoutSec) * time.Second}
	}
	cloned := *client
	cloned.Transport = legacyCompatRoundTripper(client.Transport)
	return &cloned
}

func http2CapableHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: time.Duration(limits.Default().HTTPTimeoutSec) * time.Second}
	}
	cloned := *client
	cloned.Transport = http2CapableRoundTripper(client.Transport)
	return &cloned
}

func freshHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: time.Duration(limits.Default().HTTPTimeoutSec) * time.Second}
	}
	cloned := *client
	cloned.Transport = http2CapableRoundTripper(client.Transport)
	if transport, ok := client.Transport.(*http.Transport); ok {
		cloned.Transport = transport.Clone()
	} else if client.Transport == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			cloned.Transport = transport.Clone()
		}
	}
	return &cloned
}

func legacyCompatRoundTripper(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		base := http.DefaultTransport
		if transport, ok := base.(*http.Transport); ok {
			return disableHTTP2Transport(transport)
		}
		return base
	}
	if transport, ok := rt.(*http.Transport); ok {
		return disableHTTP2Transport(transport)
	}
	return rt
}

func http2CapableRoundTripper(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		base := http.DefaultTransport
		if transport, ok := base.(*http.Transport); ok {
			return transport.Clone()
		}
		return base
	}
	if transport, ok := rt.(*http.Transport); ok {
		return transport.Clone()
	}
	return rt
}

func disableHTTP2Transport(base *http.Transport) *http.Transport {
	if base == nil {
		return nil
	}
	cloned := base.Clone()
	cloned.ForceAttemptHTTP2 = false
	cloned.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	return cloned
}

func shouldRetryLegacyProtocolWithFallback(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "http/1.x transport connection broken") ||
		strings.Contains(text, "malformed http response") ||
		strings.Contains(text, "use of closed network connection") ||
		strings.Contains(text, "compression_error") ||
		strings.Contains(text, "server sent goaway") ||
		text == "eof" ||
		strings.HasSuffix(text, ": eof")
}

func (e *Executor) doWithFallback(do func(client *http.Client) (*http.Response, error)) (*http.Response, error) {
	resp, err := do(freshHTTPClient(e.httpClient))
	if err == nil || e.fallbackHTTPClient == nil || !shouldRetryLegacyProtocolWithFallback(err) {
		return resp, err
	}
	return do(freshHTTPClient(e.fallbackHTTPClient))
}

// Execute ejecuta el flujo trifásico completo:
//  1. Retrieve — GET al RTServlet para obtener los datos de sesión.
//  2. Descifrado AES-128-CBC (si hay SessionKey).
//  3. PreSign — POST al servlet con los datos y parámetros de firma.
//  4. Firma local — el motor de firma firma el hash devuelto por PreSign.
//  5. PostSign — POST al servlet con la firma producida.
//  6. Upload — PUT/POST al STServlet con el resultado final cifrado.
func (e *Executor) Execute(ctx context.Context, session domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error) {
	if err := session.Validate(); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("sesion invalida: %w", err)
	}

	// Paso 1: Retrieve.
	rawData, err := e.retrieve(ctx, session)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("retrieve fallido: %w", err)
	}

	// Paso 2: Descifrado de sesión solo cuando el retrieve no llega ya en claro.
	// Algunas integraciones legacy envían `key` pero devuelven JSON plano en retrieve.
	if session.SessionKey != "" && requiereDescifradoSesion(rawData) {
		rawData, err = descifrarAES128CBC(rawData, session.SessionKey, session.RetrieveEndpoint, session.UploadEndpoint)
		if err != nil {
			return domain.SignatureResult{}, fmt.Errorf("descifrado de sesion fallido: %w", err)
		}
	}

	// Paso 3: Parsear la respuesta retrieve.
	var retrieved retrieveResponse
	if err := json.Unmarshal(rawData, &retrieved); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("respuesta retrieve no es JSON valido: %w", err)
	}

	formato := strings.TrimSpace(retrieved.Format)
	if formato == "" {
		formato = string(job.Format)
	}
	algoritmo := strings.TrimSpace(retrieved.Algorithm)
	if algoritmo == "" {
		algoritmo = "SHA256withRSA"
	}

	// Paso 4: PreSign.
	presigned, err := e.presign(ctx, session, retrieved, formato, algoritmo)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("presign fallido: %w", err)
	}

	// Paso 5: Firma local — PKCS#1 pura sobre el dato del presign.
	// En el protocolo trifásico el cliente SOLO realiza la operación criptográfica
	// RSA; el ensamblado del formato final (XAdES, CAdES, PAdES) lo hace el servidor
	// remoto (mFirma). Usar el motor de formato completo aquí es incorrecto y
	// produce una firma que el servidor no puede interpretar.
	hashBytes, err := base64.StdEncoding.DecodeString(presigned.Sign)
	if err != nil {
		hashBytes, err = base64.RawStdEncoding.DecodeString(presigned.Sign)
		if err != nil {
			return domain.SignatureResult{}, fmt.Errorf("hash presign no es base64 valido: %w", err)
		}
	}

	signingKey, ok := signingKeyFromContext(ctx).(legacyBatchSigningKey)
	if !ok {
		return domain.SignatureResult{}, errors.New("clave de firma no disponible o sin soporte PKCS#1 para el protocolo trifasico")
	}
	hashAlg, err := legacyHashFromAlgorithm(algoritmo)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("algoritmo de firma no soportado: %w", err)
	}
	pk1B64, err := signLegacyBatchPreData(hashBytes, signingKey, algoritmo, hashAlg, nil)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("firma local PKCS#1 fallida: %w", err)
	}
	pkcs1sig, err := base64.StdEncoding.DecodeString(pk1B64)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error interno decodificando PKCS#1: %w", err)
	}

	// Paso 6: PostSign.
	finalData, err := e.postsign(ctx, session, pkcs1sig)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("postsign fallido: %w", err)
	}

	// Paso 7: Upload del resultado final.
	uploadData := finalData
	if session.SessionKey != "" {
		uploadData, err = cifrarAES128CBC(finalData, session.SessionKey, session.RetrieveEndpoint, session.UploadEndpoint)
		if err != nil {
			return domain.SignatureResult{}, fmt.Errorf("cifrado del resultado fallido: %w", err)
		}
	}

	if err := e.upload(ctx, session, uploadData); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("upload fallido: %w", err)
	}

	return domain.SignatureResult{
		Format:    domain.SignatureFormat(formato),
		Data:      finalData,
		Algorithm: algoritmo,
	}, nil
}

// retrieve realiza el GET al RTServlet para obtener los datos de sesión cifrados.
func requiereDescifradoSesion(data []byte) bool {
	var plainJSON json.RawMessage
	return json.Unmarshal(data, &plainJSON) != nil
}

// RetrieveRaw expone la recuperación del payload remoto con compatibilidad V1:
// primero intenta GET y, si no obtiene un cuerpo utilizable, cae a POST.
func (e *Executor) RetrieveRaw(ctx context.Context, session domain.ExchangeSession) ([]byte, error) {
	if err := session.Validate(); err != nil {
		return nil, fmt.Errorf("sesion invalida: %w", err)
	}
	return e.retrieveCompat(ctx, session)
}

func (e *Executor) retrieve(ctx context.Context, session domain.ExchangeSession) ([]byte, error) {
	u, err := url.Parse(session.RetrieveEndpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint retrieve invalido: %w", err)
	}
	valores := url.Values{}
	valores.Set("op", "get")
	valores.Set("v", "1_0")
	valores.Set("id", session.RequestID)
	u.RawQuery = valores.Encode()

	resp, err := conReintentos(ctx, maxReintentos, func() (*http.Response, error) {
		return e.doWithFallback(func(client *http.Client) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
			if err != nil {
				return nil, err
			}
			return client.Do(req)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("POST %s fallido: %w", u.String(), err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(io.LimitReader(resp.Body, e.limits.MaxPayloadBytes))
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta retrieve: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retrieve devolvio HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respData)))
	}
	return respData, nil
}

func (e *Executor) retrieveCompat(ctx context.Context, session domain.ExchangeSession) ([]byte, error) {
	u, err := url.Parse(session.RetrieveEndpoint)
	if err != nil {
		return nil, fmt.Errorf("endpoint retrieve invalido: %w", err)
	}
	try := func(method string) ([]byte, error) {
		reqURL := *u
		valores := url.Values{}
		valores.Set("op", "get")
		valores.Set("v", "1_0")
		valores.Set("id", session.RequestID)
		encodedBody := valores.Encode()

		if method == http.MethodGet {
			reqURL.RawQuery = encodedBody
		} else {
			reqURL.RawQuery = ""
		}

		resp, err := conReintentos(ctx, maxReintentos, func() (*http.Response, error) {
			return e.doWithFallback(func(client *http.Client) (*http.Response, error) {
				var body io.Reader
				if method == http.MethodPost {
					body = strings.NewReader(encodedBody)
				}
				req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), body)
				if err != nil {
					return nil, err
				}
				if method == http.MethodPost {
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				}
				return client.Do(req)
			})
		})
		if err != nil {
			return nil, fmt.Errorf("%s %s fallido: %w", method, reqURL.String(), err)
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(io.LimitReader(resp.Body, e.limits.MaxPayloadBytes))
		if err != nil {
			return nil, fmt.Errorf("error leyendo respuesta retrieve: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("retrieve devolvio HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}
		return data, nil
	}

	if data, err := try(http.MethodGet); err == nil && !looksLikeRetrieveHTML(data) {
		return data, nil
	}
	if data, err := try(http.MethodPost); err == nil {
		return data, nil
	} else {
		return nil, err
	}
}

func looksLikeRetrieveHTML(data []byte) bool {
	trimmed := strings.ToLower(strings.TrimSpace(string(data)))
	return strings.HasPrefix(trimmed, "<!doctype html") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.Contains(trimmed, "<head>") && strings.Contains(trimmed, "<body>")
}

// presign realiza el POST de PreSign al servlet de firma.
func (e *Executor) presign(ctx context.Context, session domain.ExchangeSession, retrieved retrieveResponse, formato, algoritmo string) (presignResponse, error) {
	extraParamsB64 := base64.StdEncoding.EncodeToString([]byte(retrieved.ExtraParams))

	formData := url.Values{}
	formData.Set("op", "sign")
	formData.Set("dat", retrieved.Dat)
	formData.Set("algorithm", algoritmo)
	formData.Set("format", formato)
	formData.Set("extraParams", extraParamsB64)

	encodedBody := formData.Encode()

	resp, err := conReintentos(ctx, maxReintentos, func() (*http.Response, error) {
		return e.doWithFallback(func(client *http.Client) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, session.RetrieveEndpoint, strings.NewReader(encodedBody))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return client.Do(req)
		})
	})
	if err != nil {
		return presignResponse{}, fmt.Errorf("POST presign fallido: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return presignResponse{}, fmt.Errorf("presign devolvio HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	respData, err := io.ReadAll(io.LimitReader(resp.Body, e.limits.MaxPayloadBytes))
	if err != nil {
		return presignResponse{}, fmt.Errorf("error leyendo respuesta presign: %w", err)
	}

	var ps presignResponse
	if err := json.Unmarshal(respData, &ps); err != nil {
		return presignResponse{}, fmt.Errorf("respuesta presign no es JSON valido: %w", err)
	}
	return ps, nil
}

// postsign realiza el POST de PostSign con la firma producida localmente.
// Devuelve el resultado final (datos del documento firmado).
func (e *Executor) postsign(ctx context.Context, session domain.ExchangeSession, firma []byte) ([]byte, error) {
	firmaB64 := base64.StdEncoding.EncodeToString(firma)

	formData := url.Values{}
	formData.Set("op", "aspsign")
	formData.Set("dat", firmaB64)
	encoded := formData.Encode()

	resp, err := conReintentos(ctx, maxReintentos, func() (*http.Response, error) {
		return e.doWithFallback(func(client *http.Client) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, session.RetrieveEndpoint, bytes.NewBufferString(encoded))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return client.Do(req)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("POST postsign fallido: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("postsign devolvio HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, e.limits.MaxPayloadBytes))
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta postsign: %w", err)
	}
	return data, nil
}

// upload realiza el PUT al STServlet con el resultado final (posiblemente cifrado).
func (e *Executor) upload(ctx context.Context, session domain.ExchangeSession, data []byte) error {
	return e.Upload(ctx, session, data)
}

// Upload realiza la subida normal de datos arbitrarios al STServlet.
func (e *Executor) Upload(ctx context.Context, session domain.ExchangeSession, data []byte) error {
	datEncoded := base64.StdEncoding.EncodeToString(data)
	return e.uploadEncoded(ctx, session, datEncoded)
}

// UploadCertificate realiza la subida del certificado DER para flujos
// `selectcert`, alineando la codificación con la V1 Go.
func (e *Executor) UploadCertificate(ctx context.Context, session domain.ExchangeSession, certDER []byte, legacyParams url.Values) error {
	certB64 := base64.StdEncoding.EncodeToString(certDER)
	var datEncoded string
	if session.SessionKey != "" {
		cifrado, err := cifrarLegacyDES(certDER, session.SessionKey, session.RetrieveEndpoint, session.UploadEndpoint)
		if err != nil {
			return fmt.Errorf("cifrado legacy del certificado fallido: %w", err)
		}
		datEncoded = string(cifrado)
	} else {
		datEncoded = base64.URLEncoding.EncodeToString(certDER)
	}
	traceLegacyBatch("selectcert inicio request_id=%s upload=%s key=%t cert_fp=%s",
		strings.TrimSpace(session.RequestID),
		summarizeLegacyBatchURL(session.UploadEndpoint),
		strings.TrimSpace(session.SessionKey) != "",
		fingerprintLegacyBatchBody(certB64),
	)

	u, err := url.Parse(session.UploadEndpoint)
	if err != nil {
		return fmt.Errorf("endpoint upload invalido: %w", err)
	}
	sendBody := func(form url.Values) (bool, string, int, error) {
		body, status, err := e.doLegacyPOST(ctx, u.String(), form)
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
	javaStyle.Set("dat", datEncoded)

	// La clave de sesión nunca sale del equipo (la AutoFirma Java tampoco la
	// envía): con ella el servidor intermedio, sus logs o un proxy podrían
	// descifrar el intercambio. La variante heredada con el certificado en
	// claro solo se admite cuando el portal no ha pedido cifrado.
	legacy := clonarLegacyForm(legacyParams)
	legacy.Set("op", "put")
	legacy.Set("v", "1_0")
	legacy.Set("id", session.RequestID)
	legacy.Set("dat", certB64)
	legacy.Set("cert", certB64)

	type uploadAttempt struct {
		name  string
		form  url.Values
		trace string
	}
	attempts := []uploadAttempt{
		{name: "java", form: javaStyle, trace: fmt.Sprintf("selectcert upload java dat_fp=%s", fingerprintLegacyBatchBody(datEncoded))},
	}
	if strings.TrimSpace(session.SessionKey) == "" {
		attempts = append(attempts, uploadAttempt{name: "legacy", form: legacy, trace: fmt.Sprintf("selectcert upload legacy cert_fp=%s", fingerprintLegacyBatchBody(certB64))})
	}
	if len(attempts) > 1 && shouldPreferLegacySelectCertUpload(session.UploadEndpoint) {
		traceLegacyBatch("selectcert prefer_legacy_first upload=%s", summarizeLegacyBatchURL(session.UploadEndpoint))
		attempts[0], attempts[1] = attempts[1], attempts[0]
	}

	var (
		ok       bool
		bodyText string
		status   int
		sendErr  error
	)
	for _, attempt := range attempts {
		traceLegacyBatch("%s", attempt.trace)
		ok, bodyText, status, sendErr = sendBody(attempt.form)
		if sendErr != nil {
			traceLegacyBatch("selectcert upload %s error err=%v", attempt.name, sendErr)
			return fmt.Errorf("subida de certificado fallida: %w", sendErr)
		}
		traceLegacyBatch("selectcert upload %s status=%d ok=%v body=%s keys=%s", attempt.name, status, ok, summarizeLegacyBatchBody(bodyText), summarizeLegacyBatchKeys(attempt.form))
		if ok {
			return nil
		}
	}
	if status != 0 {
		return fmt.Errorf("upload de certificado devolvio HTTP %d: %s", status, bodyText)
	}
	return fmt.Errorf("la subida de certificado devolvio cuerpo no-OK: %s", bodyText)
}

// UploadSignature realiza la subida legacy Cert|Firma para flujos de
// `afirma://sign` directos basados en RetrieveService + manifiesto XML.
func (e *Executor) UploadSignature(ctx context.Context, session domain.ExchangeSession, certDER, signature []byte) error {
	u, err := url.Parse(session.UploadEndpoint)
	if err != nil {
		return fmt.Errorf("endpoint upload invalido: %w", err)
	}

	var payload string
	if strings.TrimSpace(session.SessionKey) != "" {
		encCertVal, err := cifrarLegacyDES(certDER, session.SessionKey, session.RetrieveEndpoint, session.UploadEndpoint)
		if err != nil {
			return fmt.Errorf("cifrado legacy del certificado fallido: %w", err)
		}
		encSigVal, err := cifrarLegacyDES(signature, session.SessionKey, session.RetrieveEndpoint, session.UploadEndpoint)
		if err != nil {
			return fmt.Errorf("cifrado legacy de la firma fallido: %w", err)
		}
		payload = string(encCertVal) + "|" + string(encSigVal)
	} else {
		payload = base64.URLEncoding.EncodeToString(certDER) + "|" + base64.URLEncoding.EncodeToString(signature)
	}

	sendBody := func(form url.Values) (bool, string, int, error) {
		body, status, err := e.doLegacyPOST(ctx, u.String(), form)
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
	javaStyle.Set("dat", payload)

	if ok, bodyText, status, err := sendBody(javaStyle); err != nil {
		return err
	} else if ok {
		return nil
	} else if status != 0 && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests {
		return fmt.Errorf("la subida de firma devolvió HTTP %d: %s", status, bodyText)
	}

	legacy := url.Values{}
	legacy.Set("op", "put")
	legacy.Set("v", "1_0")
	legacy.Set("id", session.RequestID)
	legacy.Set("dat", payload)

	ok, bodyText, status, err := sendBody(legacy)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if status != 0 {
		return fmt.Errorf("la subida de firma devolvió HTTP %d: %s", status, bodyText)
	}
	return fmt.Errorf("la subida de firma devolvió cuerpo no-OK: %s", bodyText)
}

func shouldPreferLegacySelectCertUpload(uploadEndpoint string) bool {
	raw := strings.ToLower(strings.TrimSpace(uploadEndpoint))
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err == nil {
		host := strings.ToLower(strings.TrimSpace(u.Hostname()))
		path := strings.ToLower(strings.TrimSpace(u.Path))
		if strings.Contains(host, "guadaltel") || strings.Contains(path, "pfirmav3") {
			return true
		}
	}
	return strings.Contains(raw, "guadaltel") || strings.Contains(raw, "pfirmav3")
}

func (e *Executor) uploadEncoded(ctx context.Context, session domain.ExchangeSession, datEncoded string) error {
	u, err := url.Parse(session.UploadEndpoint)
	if err != nil {
		return fmt.Errorf("endpoint upload invalido: %w", err)
	}
	q := u.Query()
	q.Set("op", "put")
	q.Set("v", "1_0")
	q.Set("id", session.RequestID)
	q.Set("dat", datEncoded)
	u.RawQuery = q.Encode()
	reqURL := u.String()

	resp, err := conReintentos(ctx, maxReintentos, func() (*http.Response, error) {
		return e.doWithFallback(func(client *http.Client) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
			if err != nil {
				return nil, err
			}
			return client.Do(req)
		})
	})
	if err != nil {
		return fmt.Errorf("POST upload fallido: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload devolvio HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (e *Executor) doLegacyPOST(ctx context.Context, endpoint string, form url.Values) ([]byte, int, error) {
	encodedBody := form.Encode()
	resp, err := conReintentos(ctx, maxReintentos, func() (*http.Response, error) {
		return e.doWithFallback(func(client *http.Client) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(encodedBody))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
			return client.Do(req)
		})
	})
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, e.limits.MaxPayloadBytes))
	if readErr != nil {
		return nil, resp.StatusCode, readErr
	}
	return respBody, resp.StatusCode, nil
}

// clonarLegacyForm copia los parámetros de la invocación para reenviarlos al
// servidor, excepto la clave de sesión: esa clave cifra el intercambio y no
// debe llegar nunca al servidor intermedio ni quedar en sus registros.
func clonarLegacyForm(src url.Values) url.Values {
	if len(src) == 0 {
		return make(url.Values)
	}
	dst := make(url.Values, len(src))
	for k, vals := range src {
		if esParametroClaveSesion(k) {
			continue
		}
		dst[k] = append([]string(nil), vals...)
	}
	return dst
}

func esParametroClaveSesion(nombre string) bool {
	switch strings.ToLower(strings.TrimSpace(nombre)) {
	case "key", "cipherkey":
		return true
	default:
		return false
	}
}
