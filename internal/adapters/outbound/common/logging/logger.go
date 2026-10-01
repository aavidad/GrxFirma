// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const (
	envNivel = "GRXFIRMA_LOG_LEVEL"
	envModo  = "GRXFIRMA_ENV"
	envDebug = "GRXFIRMA_DEBUG"
	envFich  = "GRXFIRMA_LOG_FILE"

	redactedSecretMarker   = "[REDACTED]"
	redactedIDMarker       = "[REDACTED_ID]"
	redactedPathMarker     = "[LOCAL_PATH_REDACTED]"
	redactedErrorFallback  = "operation_failed"
	redactedOriginMarker   = "[ORIGIN_REDACTED]"
	redactedEndpointMarker = "[ENDPOINT_REDACTED]"
)

var (
	reLogSensitiveAssignment   = regexp.MustCompile(`(?i)((?:token|authorization|bearer|challenge|dat|data|payload|password|passphrase|secret|certificate|signature|private[_-]?key|api[_-]?key|cookie)=)([^&\s"'\\]+)`)
	reLogSensitiveJSON         = regexp.MustCompile(`(?i)(["']?(?:token|authorization|bearer|challenge|dat|data|payload|password|passphrase|secret|certificate|signature|private[_-]?key|api[_-]?key|cookie)["']?\s*:\s*["']?)([^"'\s,}\\]+)`)
	reLogSensitiveIDAssignment = regexp.MustCompile(`(?i)((?:idsession|session_id|expected_session|got_session|request_id|certificate_id|sticky_id)=)([^&\s"'\\]+)`)
	reLogSensitiveIDJSON       = regexp.MustCompile(`(?i)(["']?(?:idsession|session_id|expected_session|got_session|request_id|certificate_id|sticky_id)["']?\s*:\s*["']?)([^"'\s,}\\]+)`)
	reLogAfirmaURI             = regexp.MustCompile(`(?i)afirma://[^\s"']+`)
	reLogHTTPURL               = regexp.MustCompile(`(?i)(?:https?|wss?)://[^\s"']+`)
	reLogPEMBlock              = regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]+-----.*?-----END [A-Z0-9 ]+-----`)
	reLogLongEncodedData       = regexp.MustCompile(`\b[A-Za-z0-9+/_-]{64,}={0,2}\b`)
	reLogWindowsPath           = regexp.MustCompile(`(?i)(^|[\s"'=])(?:[A-Z]:[\\/]|\\\\)[^\s"'<>:,;)}\]]+`)
	reLogUnixPath              = regexp.MustCompile(`(^|[\s"'=:])/(?:[^/\s"'<>]+/)*[^ \t\r\n"'<>:,;)}\]]+`)
	reLogDocumentName          = regexp.MustCompile(`(?i)\b[^\s"'<>/\\]+\.(?:pdf|xml|json|txt|csv|odt|ods|odp|docx?|xlsx?|pptx?|p7s|csig|xsig|zip|bin|p12|pfx|pem|key)\b`)
	reLogEmail                 = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	reLogSpanishPersonalID     = regexp.MustCompile(`(?i)\b(?:[XYZ]\d{7}[A-Z]|\d{8}[A-Z])\b`)
	reLogIPv4                  = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b`)
	reStableErrorCode          = regexp.MustCompile(`(?i)\b(?:SAF[_-]?\d{2,3}|ERR[-_ ]?\d{2,3}|HTTP[_ -]?[45]\d{2})\b`)
)

// New construye un logger estructurado para un componente concreto.
func New(component string, writer io.Writer) *slog.Logger {
	if writer == nil {
		writer = os.Stderr
	}
	writer = composeWriter(pathRedactingWriter{dst: writer})

	opts := &slog.HandlerOptions{
		Level:       parseLevel(os.Getenv(envNivel), os.Getenv(envDebug)),
		ReplaceAttr: replaceSensitiveAttr,
	}

	var handler slog.Handler
	if isProduction(os.Getenv(envModo)) {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	return slog.New(handler).With("component", component)
}

func replaceSensitiveAttr(_ []string, attr slog.Attr) slog.Attr {
	key := strings.ToLower(strings.TrimSpace(attr.Key))
	switch sensitiveAttrKind(key) {
	case "error":
		return slog.String(attr.Key, classifyErrorForLog(attr.Value))
	case "id":
		return slog.String(attr.Key, redactedIDMarker)
	case "path":
		return slog.String(attr.Key, redactedPathMarker)
	case "origin":
		return slog.String(attr.Key, redactedOriginMarker)
	case "endpoint":
		return slog.String(attr.Key, redactedEndpointMarker)
	case "secret":
		return slog.String(attr.Key, redactedSecretMarker)
	}
	if attr.Value.Kind() == slog.KindString {
		return slog.String(attr.Key, redactLogText(attr.Value.String()))
	}
	return attr
}

func sensitiveAttrKind(key string) string {
	switch key {
	case "error", "err", "error_detail", "error_message", "cause":
		return "error"
	case "id", "request_id", "requestid", "session_id", "sessionid", "idsession",
		"certificate_id", "certificateid", "sticky_id", "expected_session", "got_session":
		return "id"
	case "path", "filepath", "file_path", "file", "filename", "file_name", "name",
		"document", "document_name", "document_path", "exe", "executable":
		return "path"
	case "origin":
		return "origin"
	case "uri", "url", "endpoint", "upload_endpoint", "retrieve_endpoint", "stservlet",
		"rtservlet", "storage_servlet", "addr", "address", "remote_addr",
		"local_addr", "listen_addr", "host", "hostname", "server_name":
		return "endpoint"
	case "panic", "stack", "prefix", "raw_prefix", "message_prefix", "payload_prefix",
		"uri_prefix", "response_prefix", "reply", "normalized", "diagnostic",
		"user_agent", "body", "content", "params", "properties", "headers",
		"header", "cookie", "cookies", "query", "form", "request", "response",
		"result", "username", "email", "subject", "issuer":
		return "secret"
	}
	if strings.HasPrefix(key, "has_") ||
		strings.HasSuffix(key, "_len") ||
		strings.HasSuffix(key, "_count") ||
		strings.HasSuffix(key, "_bytes") ||
		strings.HasSuffix(key, "_size") ||
		strings.HasSuffix(key, "_ms") {
		return ""
	}
	if strings.HasSuffix(key, "_id") && key != "pid" {
		return "id"
	}
	if strings.Contains(key, "path") || strings.Contains(key, "filename") {
		return "path"
	}
	switch key {
	case "token", "authorization", "x-api-token", "bearer", "challenge", "challengeb64",
		"signatureb64", "certificatepem", "certificateb64", "content_base64", "original_content_base64",
		"payload", "dat", "data", "raw", "response", "secret", "password", "passphrase",
		"private_key", "privatekey", "sec-websocket-key":
		return "secret"
	}
	if strings.Contains(key, "token") ||
		strings.Contains(key, "authorization") ||
		strings.Contains(key, "cert") ||
		strings.Contains(key, "challenge") ||
		strings.Contains(key, "signature") ||
		strings.Contains(key, "payload") ||
		strings.Contains(key, "content_base64") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "secret") {
		return "secret"
	}
	return ""
}

func classifyErrorForLog(value slog.Value) string {
	if value.Kind() == slog.KindAny {
		if err, ok := value.Any().(error); ok {
			switch {
			case errors.Is(err, context.Canceled):
				return "cancelled"
			case errors.Is(err, context.DeadlineExceeded):
				return "deadline_exceeded"
			case errors.Is(err, os.ErrPermission):
				return "permission_denied"
			case errors.Is(err, os.ErrNotExist):
				return "not_found"
			}
			return classifyErrorText(err.Error())
		}
	}
	if value.Kind() == slog.KindString {
		return classifyErrorText(value.String())
	}
	return redactedErrorFallback
}

func classifyErrorText(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	class := redactedErrorFallback
	switch {
	case lower == "":
		class = redactedErrorFallback
	case strings.Contains(lower, "cancel"):
		class = "cancelled"
	case strings.Contains(lower, "deadline"), strings.Contains(lower, "timeout"),
		strings.Contains(lower, "tiempo de espera"):
		class = "deadline_exceeded"
	case strings.Contains(lower, "permission"), strings.Contains(lower, "permiso"),
		strings.Contains(lower, "access denied"), strings.Contains(lower, "acceso denegado"):
		class = "permission_denied"
	case strings.Contains(lower, "not found"), strings.Contains(lower, "no existe"),
		strings.Contains(lower, "no encontrado"):
		class = "not_found"
	case strings.Contains(lower, "too large"), strings.Contains(lower, "demasiado grande"),
		strings.Contains(lower, "quota"), strings.Contains(lower, "cuota"),
		strings.Contains(lower, "limit"), strings.Contains(lower, "límite"):
		class = "limit_exceeded"
	case strings.Contains(lower, "parse"), strings.Contains(lower, "decode"),
		strings.Contains(lower, "invalid"), strings.Contains(lower, "inválid"),
		strings.Contains(lower, "malformed"):
		class = "invalid_input"
	case strings.Contains(lower, "connection"), strings.Contains(lower, "conexión"),
		strings.Contains(lower, "dial "), strings.Contains(lower, "network"),
		strings.Contains(lower, "tls"), strings.Contains(lower, "x509"):
		class = "transport_failure"
	}
	if code := stableErrorCode(raw); code != "" {
		return class + " code=" + code
	}
	return class
}

func stableErrorCode(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(reStableErrorCode.FindString(raw)))
	code = strings.ReplaceAll(code, " ", "_")
	if strings.HasPrefix(code, "SAF-") {
		code = strings.Replace(code, "SAF-", "SAF_", 1)
	}
	if strings.HasPrefix(code, "ERR_") {
		code = strings.Replace(code, "ERR_", "ERR-", 1)
	}
	if strings.HasPrefix(code, "HTTP-") {
		code = strings.Replace(code, "HTTP-", "HTTP_", 1)
	}
	return code
}

func parseLevel(raw string, debug string) slog.Level {
	if debugLevelAllowed && isDebugEnabled(debug) {
		return slog.LevelDebug
	}
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DEBUG":
		if debugLevelAllowed {
			return slog.LevelDebug
		}
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// DebugAllowed indica si el binario fue compilado para admitir trazas DEBUG.
// Los empaquetadores públicos usan la etiqueta `production`, que devuelve
// siempre false aunque el entorno intente activar GRXFIRMA_DEBUG.
func DebugAllowed() bool {
	return debugLevelAllowed
}

func composeWriter(base io.Writer) io.Writer {
	logPath := strings.TrimSpace(os.Getenv(envFich))
	if logPath == "" {
		return base
	}
	fileWriter, err := newAppendFileWriter(logPath)
	if err != nil {
		return base
	}
	return bestEffortMultiWriter{
		writers: []io.Writer{base, pathRedactingWriter{dst: fileWriter}},
	}
}

// bestEffortMultiWriter evita que un destino no disponible (por ejemplo,
// stderr en un binario Windows enlazado como GUI) impida escribir en el resto
// de destinos configurados. Conserva el primer error para el contrato
// io.Writer, pero siempre intenta todas las escrituras.
type bestEffortMultiWriter struct {
	writers []io.Writer
}

func (w bestEffortMultiWriter) Write(p []byte) (int, error) {
	var firstErr error
	for _, dst := range w.writers {
		if dst == nil {
			continue
		}
		n, err := dst.Write(p)
		if err == nil && n != len(p) {
			err = io.ErrShortWrite
		}
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return len(p), firstErr
	}
	return len(p), nil
}

type appendFileWriter struct {
	path string
	mu   sync.Mutex
}

func newAppendFileWriter(path string) (*appendFileWriter, error) {
	path, err := prepararRutaLog(path)
	if err != nil {
		return nil, err
	}
	file, err := abrirLogAppendSeguro(path)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return &appendFileWriter{path: path}, nil
}

func (w *appendFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := prepararRutaLog(w.path); err != nil {
		return 0, err
	}
	file, err := abrirLogAppendSeguro(w.path)
	if err != nil {
		return 0, err
	}
	n, writeErr := file.Write(p)
	closeErr := file.Close()
	if writeErr != nil {
		return n, writeErr
	}
	if closeErr != nil {
		return n, closeErr
	}
	return n, nil
}

func isProduction(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "prod", "production", "produccion":
		return true
	default:
		return false
	}
}

func isDebugEnabled(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "si", "on", "debug":
		return true
	default:
		return false
	}
}

type pathRedactingWriter struct {
	dst io.Writer
}

func (w pathRedactingWriter) Write(p []byte) (int, error) {
	if w.dst == nil {
		return len(p), nil
	}
	redacted := []byte(redactLogText(string(p)))
	n, err := w.dst.Write(redacted)
	if err != nil {
		return n, err
	}
	return len(p), nil
}

func redactLogText(raw string) string {
	out := reLogPEMBlock.ReplaceAllString(raw, "[PEM_REDACTED]")
	out = reLogSensitiveAssignment.ReplaceAllString(out, `${1}[REDACTED]`)
	out = reLogSensitiveJSON.ReplaceAllString(out, `${1}[REDACTED]`)
	out = reLogSensitiveIDAssignment.ReplaceAllString(out, `${1}`+redactedIDMarker)
	out = reLogSensitiveIDJSON.ReplaceAllString(out, `${1}`+redactedIDMarker)
	out = reLogAfirmaURI.ReplaceAllString(out, "[AFIRMA_REQUEST_REDACTED]")
	out = reLogHTTPURL.ReplaceAllString(out, redactedEndpointMarker)
	out = redactPathPrefixes(out)
	out = reLogWindowsPath.ReplaceAllString(out, `${1}`+redactedPathMarker)
	out = reLogUnixPath.ReplaceAllString(out, `${1}`+redactedPathMarker)
	out = reLogDocumentName.ReplaceAllString(out, "[DOCUMENT_REDACTED]")
	out = reLogLongEncodedData.ReplaceAllString(out, "[ENCODED_DATA_REDACTED]")
	out = reLogEmail.ReplaceAllString(out, "[EMAIL_REDACTED]")
	out = reLogSpanishPersonalID.ReplaceAllString(out, "[PERSONAL_ID_REDACTED]")
	out = reLogIPv4.ReplaceAllString(out, "[ADDRESS_REDACTED]")
	return out
}

func redactPathPrefixes(raw string) string {
	out := raw
	for _, prefix := range sensitivePathPrefixes() {
		if prefix == "" {
			continue
		}
		out = replacePathPrefixOccurrences(out, prefix, redactedPathMarker)
	}
	return out
}

func sensitivePathPrefixes() []string {
	prefixes := make([]string, 0, 6)
	if home, err := os.UserHomeDir(); err == nil {
		home = strings.TrimSpace(home)
		if home != "" {
			prefixes = appendPathPrefixVariants(prefixes, home)
		}
	}
	for _, envName := range []string{"HOME", "USERPROFILE"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			prefixes = appendPathPrefixVariants(prefixes, value)
		}
	}
	prefixes = uniqueStrings(prefixes)
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return prefixes
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func appendPathPrefixVariants(prefixes []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return prefixes
	}
	prefixes = append(prefixes, value)
	withSlashes := strings.ReplaceAll(value, `\`, `/`)
	withBackslashes := strings.ReplaceAll(value, `/`, `\`)
	if withSlashes != value {
		prefixes = append(prefixes, withSlashes)
	}
	if withBackslashes != value {
		prefixes = append(prefixes, withBackslashes)
	}
	return prefixes
}

func replacePathPrefixOccurrences(raw, prefix, marker string) string {
	if raw == "" || prefix == "" {
		return raw
	}
	var out strings.Builder
	out.Grow(len(raw))
	searchFrom := 0
	for searchFrom < len(raw) {
		idx := strings.Index(raw[searchFrom:], prefix)
		if idx == -1 {
			out.WriteString(raw[searchFrom:])
			break
		}
		idx += searchFrom
		end := idx + len(prefix)
		if !isPathPrefixBoundary(raw, idx, end) {
			out.WriteString(raw[searchFrom : idx+1])
			searchFrom = idx + 1
			continue
		}
		out.WriteString(raw[searchFrom:idx])
		out.WriteString(marker)
		searchFrom = consumePathSuffix(raw, end)
	}
	return out.String()
}

func consumePathSuffix(raw string, start int) int {
	end := start
	for end < len(raw) {
		switch raw[end] {
		case ' ', '\t', '\r', '\n', '"', '\'', ')', ']', '}', ',', ';', ':':
			return end
		default:
			end++
		}
	}
	return end
}

func isPathPrefixBoundary(raw string, start, end int) bool {
	if start > 0 {
		prev := rune(raw[start-1])
		if !isPathPrefixLeadBoundary(prev) {
			return false
		}
	}
	if end >= len(raw) {
		return true
	}
	next := rune(raw[end])
	return next == '/' || next == '\\' || unicode.IsSpace(next) ||
		next == '"' || next == '\'' || next == ')' || next == ']' ||
		next == '}' || next == ',' || next == ';' || next == ':'
}

func isPathPrefixLeadBoundary(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '"', '\'', '(', '[', '{', '=', ':':
		return true
	default:
		return false
	}
}
