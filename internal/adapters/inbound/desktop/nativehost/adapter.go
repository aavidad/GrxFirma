// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nativehost

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/limits"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const chunkSize = 512 * 1024

// maxMessageSize acota el tamaño de un mensaje entrante Native Messaging para
// impedir que una cabecera de longitud maliciosa fuerce una asignación enorme
// (DoS por agotamiento de memoria). 128 MiB es el mismo tope que aplica el lado
// de la extensión al reensamblar respuestas.
const maxMessageSize = 128 * 1024 * 1024

const (
	nativeRequestReplayTTL = 10 * time.Minute
	maxNativeRequestIDs    = 4096
	maxRequesterOriginLen  = 2048
)

var nativeRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// SignDocumentUseCase define el contrato minimo esperado por la accion sign.
type SignDocumentUseCase interface {
	Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error)
}

// VerifySignatureUseCase define el contrato minimo esperado por la accion verify.
type VerifySignatureUseCase interface {
	Execute(ctx context.Context, cmd application.VerifyCommand) (application.VerifyResult, error)
}

// Adaptador implementa el host Native Messaging para el borde de escritorio.
type Adaptador struct {
	Catalogo        ports.CertificateCatalog
	Firmar          SignDocumentUseCase
	Verificar       VerifySignatureUseCase
	ProbarIdentidad GeneradorPruebaIdentidadLocal
	TrustPolicy     ports.TrustPolicy
	Metrics         ports.OperationMetrics
	Limits          limits.Limits
	RequireCaller   bool

	replayMu sync.Mutex
	seenIDs  map[string]time.Time
}

// New construye el adaptador Native Messaging.
func New(catalogo ports.CertificateCatalog, firmar SignDocumentUseCase, verificar VerifySignatureUseCase, trust ports.TrustPolicy) *Adaptador {
	return &Adaptador{
		Catalogo:    catalogo,
		Firmar:      firmar,
		Verificar:   verificar,
		TrustPolicy: trust,
		Limits:      limits.FromEnv(limits.Default()),
	}
}

func (a *Adaptador) WithMetrics(metricas ports.OperationMetrics) *Adaptador {
	if a == nil {
		return nil
	}
	a.Metrics = metricas
	return a
}

func (a *Adaptador) WithLimits(l limits.Limits) *Adaptador {
	if a == nil {
		return nil
	}
	a.Limits = l
	return a
}

type request struct {
	RequestID        any               `json:"requestId"`
	Action           string            `json:"action"`
	CertificateID    string            `json:"certificateId"`
	Data             string            `json:"data"`
	Format           string            `json:"format"`
	MIMEType         string            `json:"mimeType"`
	Name             string            `json:"name"`
	SignatureOptions map[string]string `json:"signatureOptions"`
	OriginalData     string            `json:"originalData"`
	SignatureData    string            `json:"signatureData"`
	RequesterOrigin  string            `json:"requesterOrigin"`
}

type response struct {
	RequestID     string                     `json:"requestId"`
	Success       bool                       `json:"success"`
	Code          string                     `json:"code,omitempty"`
	Error         string                     `json:"error,omitempty"`
	Certificates  []certificateJSON          `json:"certificates,omitempty"`
	Signature     string                     `json:"signature,omitempty"`
	SignatureLen  int                        `json:"signatureLen,omitempty"`
	Result        *verifyResultJSON          `json:"result,omitempty"`
	IdentityProof *identityProofMetadataJSON `json:"identityProof,omitempty"`
	Chunk         int                        `json:"chunk"`
	TotalChunks   int                        `json:"totalChunks,omitempty"`
}

type certificateJSON struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Issuer      string `json:"issuer"`
	NotAfter    string `json:"notAfter,omitempty"`
	Fingerprint string `json:"fingerprint"`
}

type verifyResultJSON struct {
	Valid             bool     `json:"valid"`
	Reason            string   `json:"reason,omitempty"`
	Details           []string `json:"details,omitempty"`
	Signers           []string `json:"signers,omitempty"`
	CertificateStatus string   `json:"certificateStatus,omitempty"`
}

// ServeOnce procesa una unica peticion Native Messaging sobre stdin/stdout.
func (a *Adaptador) ServeOnce(ctx context.Context, callerID string, r io.Reader, w io.Writer) error {
	payload, err := ReadMessage(r)
	if err != nil {
		return err
	}
	responses, err := a.Process(ctx, callerID, payload)
	if err != nil {
		return err
	}
	for _, resp := range responses {
		if err := WriteMessage(w, resp); err != nil {
			return err
		}
	}
	return nil
}

// Process traduce un payload Native Messaging a una o varias respuestas JSON.
func (a *Adaptador) Process(ctx context.Context, callerID string, payload []byte) ([][]byte, error) {
	inicio := time.Now()
	op := "desconocido"
	estado := "error"
	trace := nativehostTraceLogger()
	defer func() {
		if a != nil && a.Metrics != nil {
			a.Metrics.RecordProtocolRequest(ctx, op, estado, time.Since(inicio))
		}
	}()

	if err := a.validarCaller(ctx, callerID); err != nil {
		return marshalResponses([]response{errorResponse("", err.Error())})
	}

	var req request
	if err := json.Unmarshal(payload, &req); err != nil {
		trace.DebugContext(ctx, "nativehost_payload_invalido", "caller", callerID, "payload_len", len(payload), "error", err)
		return marshalResponses([]response{errorResponse("", "formato de solicitud invalido")})
	}

	reqID := normalizeRequestID(req.RequestID)
	op = strings.ToLower(strings.TrimSpace(req.Action))
	if a != nil && a.RequireCaller {
		if err := a.claimRequestID(reqID, time.Now()); err != nil {
			return marshalResponses([]response{errorResponse(reqID, err.Error())})
		}
	}
	trace.DebugContext(ctx, "nativehost_request", "caller", callerID, "request_id", reqID, "action", op, "payload_len", len(payload))
	switch op {
	case "ping":
		estado = "ok"
		return marshalResponses([]response{{
			RequestID: reqID,
			Success:   true,
			Chunk:     0,
		}})
	case "getcertificates":
		responses, err := a.handleGetCertificates(ctx, reqID)
		if err == nil {
			estado = "ok"
		}
		return responses, err
	case "sign":
		responses, err := a.handleSign(ctx, reqID, callerID, req)
		if err == nil {
			estado = "ok"
		}
		return responses, err
	case "verify":
		responses, err := a.handleVerify(ctx, reqID, req)
		if err == nil {
			estado = "ok"
		}
		return responses, err
	case "proveidentity":
		responses, err := a.handleIdentityProof(ctx, reqID, payload)
		if err == nil {
			estado = "ok"
		}
		return responses, err
	default:
		trace.WarnContext(ctx, "nativehost_accion_no_soportada", "caller", callerID, "request_id", reqID, "action", op)
		return marshalResponses([]response{errorResponse(reqID, "accion no soportada")})
	}
}

func nativehostTraceLogger() *slog.Logger {
	return logging.New("desktop/nativehost", os.Stderr)
}

// ReadMessage lee un mensaje con framing Native Messaging.
func ReadMessage(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length > maxMessageSize {
		return nil, fmt.Errorf("nativehost: mensaje entrante demasiado grande: %d bytes (máximo %d)", length, maxMessageSize)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// WriteMessage escribe un mensaje con framing Native Messaging.
func WriteMessage(w io.Writer, payload []byte) error {
	payloadLength := uint64(len(payload))
	if payloadLength > math.MaxUint32 {
		return fmt.Errorf("nativehost: mensaje saliente fuera del rango del protocolo: %d bytes", payloadLength)
	}
	if payloadLength > maxMessageSize {
		return fmt.Errorf("nativehost: mensaje saliente demasiado grande: %d bytes (máximo %d)", payloadLength, maxMessageSize)
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(payloadLength)); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func (a *Adaptador) handleGetCertificates(ctx context.Context, reqID string) ([][]byte, error) {
	if a.Catalogo == nil {
		return marshalResponses([]response{errorResponse(reqID, "catalogo de certificados no configurado")})
	}
	certs, err := a.Catalogo.List(ctx)
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	items := make([]certificateJSON, 0, len(certs))
	for _, cert := range certs {
		item := certificateJSON{
			ID:          cert.ID,
			Subject:     cert.Subject,
			Issuer:      cert.Issuer,
			Fingerprint: cert.Fingerprint,
		}
		if !cert.NotAfter.IsZero() {
			item.NotAfter = cert.NotAfter.UTC().Format(time.RFC3339)
		}
		items = append(items, item)
	}
	return marshalResponses([]response{{
		RequestID:    reqID,
		Success:      true,
		Certificates: items,
		Chunk:        0,
	}})
}

func (a *Adaptador) handleSign(ctx context.Context, reqID, callerID string, req request) ([][]byte, error) {
	if a.Firmar == nil {
		return marshalResponses([]response{errorResponse(reqID, "caso de uso de firma no configurado")})
	}
	content, err := decodeBase64Limited(req.Data, "data", a.effectiveLimits())
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	cmd, err := application.NewSignCommand(
		firstNonEmpty(req.Name, "documento.bin"),
		content,
		firstNonEmpty(req.MIMEType, "application/octet-stream"),
		firstNonEmpty(req.Format, "cades"),
		"sign",
		req.CertificateID,
		req.SignatureOptions,
	)
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	cmd.RequesterApplication = strings.TrimSpace(callerID)
	cmd.RequesterOrigin = normalizeRequesterOrigin(req.RequesterOrigin)
	result, err := a.Firmar.Execute(ctx, cmd)
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	signature := base64.StdEncoding.EncodeToString(result.Result.Data)
	return marshalResponses(chunkSignature(reqID, signature))
}

func (a *Adaptador) claimRequestID(requestID string, now time.Time) error {
	if !nativeRequestIDPattern.MatchString(requestID) {
		return errors.New("solicitud del navegador no válida: falta un identificador seguro; actualice o reinstale la extensión oficial y vuelva a intentarlo")
	}
	a.replayMu.Lock()
	defer a.replayMu.Unlock()
	if a.seenIDs == nil {
		a.seenIDs = make(map[string]time.Time)
	}
	cutoff := now.Add(-nativeRequestReplayTTL)
	var oldestID string
	var oldest time.Time
	for id, seenAt := range a.seenIDs {
		if seenAt.Before(cutoff) {
			delete(a.seenIDs, id)
			continue
		}
		if oldestID == "" || seenAt.Before(oldest) {
			oldestID, oldest = id, seenAt
		}
	}
	if _, exists := a.seenIDs[requestID]; exists {
		return errors.New("solicitud repetida bloqueada por seguridad; cierre el firmador, vuelva a abrirlo desde la sede y, si persiste, reinicie el navegador")
	}
	if len(a.seenIDs) >= maxNativeRequestIDs && oldestID != "" {
		delete(a.seenIDs, oldestID)
	}
	a.seenIDs[requestID] = now
	return nil
}

func normalizeRequesterOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxRequesterOriginLen {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil ||
		parsed.Opaque != "" ||
		parsed.User != nil ||
		parsed.Host == "" ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return ""
	}
	switch parsed.Scheme {
	case "https", "chrome-extension", "moz-extension":
	default:
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (a *Adaptador) handleVerify(ctx context.Context, reqID string, req request) ([][]byte, error) {
	if a.Verificar == nil {
		return marshalResponses([]response{errorResponse(reqID, "caso de uso de verificacion no configurado")})
	}
	raw := firstNonEmpty(req.SignatureData, req.Data)
	content, err := decodeBase64Limited(raw, "signatureData", a.effectiveLimits())
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	doc, err := domain.NewDocument(
		firstNonEmpty(req.Name, "firma.bin"),
		content,
		firstNonEmpty(req.MIMEType, "application/octet-stream"),
	)
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	cmd := application.VerifyCommand{SignedDocument: doc}
	if strings.TrimSpace(req.OriginalData) != "" {
		originalContent, err := decodeBase64Limited(req.OriginalData, "originalData", a.effectiveLimits())
		if err != nil {
			return marshalResponses([]response{errorResponse(reqID, err.Error())})
		}
		originalDoc, err := domain.NewDocument(
			firstNonEmpty(req.Name, "original.bin"),
			originalContent,
			inferOriginalMIMEType(req.MIMEType),
		)
		if err != nil {
			return marshalResponses([]response{errorResponse(reqID, err.Error())})
		}
		cmd.OriginalDocument = &originalDoc
	}
	result, err := a.Verificar.Execute(ctx, cmd)
	if err != nil {
		return marshalResponses([]response{errorResponse(reqID, err.Error())})
	}
	signers := make([]string, 0, len(result.Firmantes))
	for _, signer := range result.Firmantes {
		signers = append(signers, signer.ID)
	}
	return marshalResponses([]response{{
		RequestID: reqID,
		Success:   true,
		Result: &verifyResultJSON{
			Valid:             result.Verification.Valid,
			Reason:            result.Verification.Reason,
			Details:           result.Verification.Details,
			Signers:           signers,
			CertificateStatus: string(result.Verification.Certificate.Status),
		},
		Chunk: 0,
	}})
}

func (a *Adaptador) effectiveLimits() limits.Limits {
	if a == nil || a.Limits.MaxPayloadBytes <= 0 {
		return limits.FromEnv(limits.Default())
	}
	return a.Limits
}

func inferOriginalMIMEType(signedMIME string) string {
	signedMIME = strings.ToLower(strings.TrimSpace(signedMIME))
	switch signedMIME {
	case "application/pkcs7-signature", "application/cms", "application/pkcs7-mime":
		return "application/octet-stream"
	default:
		return signedMIME
	}
}

func (a *Adaptador) validarCaller(ctx context.Context, callerID string) error {
	callerID = strings.TrimSpace(callerID)
	if callerID == "" {
		if a != nil && a.RequireCaller {
			return errors.New("no se pudo identificar la extensión solicitante; reinstale los conectores oficiales y reinicie el navegador")
		}
		return nil
	}
	if a == nil || a.TrustPolicy == nil {
		return nil
	}
	decision, err := a.TrustPolicy.Evaluate(ctx, callerID)
	if err != nil {
		return fmt.Errorf("no se pudo validar la extensión solicitante; reinstale los conectores oficiales y reinicie el navegador: %w", err)
	}
	if !decision.IsAllowed() {
		return fmt.Errorf("extensión solicitante no autorizada (%s); use la extensión oficial o el despliegue gestionado de su organización", callerID)
	}
	return nil
}

func decodeBase64(raw string, field string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%s no es base64 valido", field)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s no puede estar vacio", field)
	}
	return data, nil
}

func decodeBase64Limited(raw string, field string, limite limits.Limits) ([]byte, error) {
	encoded := strings.TrimSpace(raw)
	estimado := int64(base64.StdEncoding.DecodedLen(len(encoded)))
	if strings.HasSuffix(encoded, "==") {
		estimado -= 2
	} else if strings.HasSuffix(encoded, "=") {
		estimado--
	}
	if estimado < 0 {
		estimado = 0
	}
	if limite.MaxPayloadBytes > 0 && estimado > limite.MaxPayloadBytes {
		return nil, &limits.ErrPayloadExcedido{Tamaño: estimado, Maximo: limite.MaxPayloadBytes}
	}

	data, err := decodeBase64(encoded, field)
	if err != nil {
		return nil, err
	}
	if err := limite.CheckPayload(data); err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeRequestID(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func errorResponse(reqID, message string) response {
	return response{
		RequestID: reqID,
		Success:   false,
		Error:     message,
		Chunk:     0,
	}
}

func marshalResponses(resps []response) ([][]byte, error) {
	out := make([][]byte, 0, len(resps))
	for _, resp := range resps {
		payload, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}
		out = append(out, payload)
	}
	return out, nil
}

func chunkSignature(reqID, signature string) []response {
	if len(signature) <= chunkSize {
		return []response{{
			RequestID:    reqID,
			Success:      true,
			Signature:    signature,
			SignatureLen: len(signature),
			Chunk:        0,
			TotalChunks:  1,
		}}
	}

	total := (len(signature) + chunkSize - 1) / chunkSize
	out := make([]response, 0, total)
	for i, start := 0, 0; start < len(signature); i, start = i+1, start+chunkSize {
		end := start + chunkSize
		if end > len(signature) {
			end = len(signature)
		}
		out = append(out, response{
			RequestID:    reqID,
			Success:      true,
			Signature:    signature[start:end],
			SignatureLen: len(signature),
			Chunk:        i,
			TotalChunks:  total,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
