// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

const (
	legacyServiceCertPrefix        = "websocket-localhost"
	legacyServiceTLSHandshakeLimit = 10 * time.Second
	legacyServiceReadTimeout       = 90 * time.Second
	legacyResponseMaxSize          = 1000000
	legacyReadBufferSize           = 2048
	legacyBufferedSecRange         = 36
	legacyMaxReadEmptyTries        = 10
	legacyEOFMarker                = "@EOF"
	legacyMemoryError              = "MEMORY_ERROR"
	legacyMaxServicePayload        = 32 * 1024 * 1024
	legacyMaxRawRequestBytes       = 8 * 1024 * 1024
	legacyMaxConcurrentConnections = 32
)

var errLegacyRequestTooLarge = errors.New("legacy request too large")

type LegacySocketServer struct {
	adapter *Adaptador

	session         string
	protocolVersion int

	mu             sync.Mutex
	listener       net.Listener
	connections    chan struct{}
	fragments      []string
	fragmentsTotal int
	responseParts  []string
	preparedURI    string
}

type legacySocketReadResult struct {
	RequestBody string
	SessionID   string
	HTTPMethod  string
	Origin      string
}

func StartLegacySocketServer(
	ctx context.Context,
	adapter *Adaptador,
	certDir string,
	ports []int,
	sessionID string,
	protocolVersion int,
) (*LegacySocketServer, error) {
	if adapter == nil {
		return nil, errors.New("websocket: adaptador no configurado")
	}
	if protocolVersion <= 0 {
		protocolVersion = 1
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("service: id de sesion obligatorio")
	}

	// El flujo legacy `service` debe reutilizar el mismo certificado localhost
	// que `websocket`. La app Go que sí funcionaba usaba una única identidad TLS
	// local para ambos canales. Si generamos otra distinta, algunas webs/clientes
	// confían el canal websocket pero rechazan service con `unknown certificate`.
	certFile, keyFile, rootCertFile, certSource, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(certDir, legacyServiceCertPrefix)
	if err != nil {
		return nil, err
	}
	ensureTrust := localtlstrust.EnsureTrusted
	if certSource == "grxfirma-local-ca" {
		ensureTrust = localtlstrust.EnsureManagedTrusted
	}
	if err := ensureTrust(ctx, rootCertFile); err != nil &&
		!errors.Is(err, localtlstrust.ErrSoporteNoDisponible) &&
		!errors.Is(err, localtlstrust.ErrHerramientaNoDisponible) {
		return nil, err
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("service: no se pudo cargar el par TLS: %w", err)
	}
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{pair},
		NextProtos:   []string{"http/1.1"},
	}

	var ln net.Listener
	for _, port := range puertosCandidatos(ports) {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err = tls.Listen("tcp", addr, cfg)
		if err == nil {
			break
		}
	}
	if ln == nil {
		return nil, fmt.Errorf("service: no se pudo abrir ningún puerto solicitado (%v): %w", ports, err)
	}

	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         strings.TrimSpace(sessionID),
		protocolVersion: protocolVersion,
		listener:        ln,
		connections:     make(chan struct{}, legacyMaxConcurrentConnections),
	}
	legacyServiceSocketTraceLogger().DebugContext(
		ctx,
		"service_socket_tls_ready",
		"listen_addr",
		ln.Addr().String(),
		"cert_prefix",
		legacyServiceCertPrefix,
		"cert_source",
		certSource,
	)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go srv.acceptLoop(ctx)
	return srv, nil
}

func (s *LegacySocketServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *LegacySocketServer) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		select {
		case s.connections <- struct{}{}:
			go func() {
				defer func() { <-s.connections }()
				s.handleConn(ctx, conn)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func (s *LegacySocketServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	trace := legacyServiceSocketTraceLogger()
	trace.DebugContext(ctx, "service_socket_conn_start", "remote_addr", conn.RemoteAddr().String(), "local_addr", conn.LocalAddr().String())
	if !isLoopbackRemoteAddr(conn.RemoteAddr().String()) {
		trace.WarnContext(ctx, "service_socket_remote_rejected", "remote_addr", conn.RemoteAddr().String())
		return
	}
	if err := completeLegacyServiceTLSHandshake(ctx, conn, trace); err != nil {
		return
	}
	readRes, err := readLegacySocketPayload(conn)
	if errors.Is(err, errLegacyRequestTooLarge) {
		trace.WarnContext(ctx, "service_socket_request_too_large", "remote_addr", conn.RemoteAddr().String())
		_, _ = conn.Write(buildLegacyHTTPResponse(legacyMemoryError, ""))
		return
	}
	if err != nil {
		trace.WarnContext(ctx, "service_socket_read_error", "remote_addr", conn.RemoteAddr().String(), "error", err)
		_, _ = conn.Write(buildLegacyHTTPResponse("SAF_03: Parametros incorrectos", ""))
		return
	}
	trace.DebugContext(
		ctx,
		"service_socket_payload_read",
		"remote_addr",
		conn.RemoteAddr().String(),
		"session_id",
		maskLegacyTraceValue(readRes.SessionID),
		"http_method",
		readRes.HTTPMethod,
		"origin",
		readRes.Origin,
		"payload_len",
		len(readRes.RequestBody),
		"payload_prefix",
		truncateForTrace(readRes.RequestBody, 220),
	)
	if !isAllowedWebOriginWithPolicy(ctx, readRes.Origin, s.adapter.Trust, nil) {
		trace.WarnContext(ctx, "service_socket_origin_rejected", "remote_addr", conn.RemoteAddr().String(), "origin", readRes.Origin)
		_, _ = conn.Write(buildLegacyHTTPForbiddenResponse())
		return
	}
	if strings.EqualFold(readRes.HTTPMethod, "OPTIONS") {
		trace.DebugContext(ctx, "service_socket_cors_preflight", "remote_addr", conn.RemoteAddr().String())
		_, _ = conn.Write(buildLegacyHTTPPreflightResponse(readRes.Origin))
		return
	}
	if strings.TrimSpace(readRes.RequestBody) == "" {
		trace.DebugContext(ctx, "service_socket_empty_request", "remote_addr", conn.RemoteAddr().String())
		return
	}
	if err := s.checkSessionID(readRes.SessionID); err != nil {
		trace.WarnContext(
			ctx,
			"service_socket_session_mismatch",
			"remote_addr",
			conn.RemoteAddr().String(),
			"expected_session",
			maskLegacyTraceValue(s.session),
			"got_session",
			maskLegacyTraceValue(readRes.SessionID),
		)
		_, _ = conn.Write(buildLegacyHTTPResponse("SAF_03: Parametros incorrectos", readRes.Origin))
		return
	}
	res := s.processCommandWithOrigin(ctx, readRes.Origin, readRes.RequestBody)
	trace.DebugContext(ctx, "service_socket_response_ready", "remote_addr", conn.RemoteAddr().String(), "response_prefix", truncateForTrace(res, 220))
	_, _ = conn.Write(buildLegacyHTTPResponse(res, readRes.Origin))
}

func completeLegacyServiceTLSHandshake(ctx context.Context, conn net.Conn, trace *slog.Logger) error {
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil
	}
	start := time.Now()
	_ = tlsConn.SetDeadline(time.Now().Add(legacyServiceTLSHandshakeLimit))
	err := tlsConn.HandshakeContext(ctx)
	elapsed := time.Since(start)
	_ = tlsConn.SetDeadline(time.Time{})
	if err != nil {
		trace.WarnContext(
			ctx,
			"service_socket_tls_handshake_error",
			"remote_addr",
			conn.RemoteAddr().String(),
			"elapsed_ms",
			elapsed.Milliseconds(),
			"error",
			err,
		)
		return err
	}
	state := tlsConn.ConnectionState()
	trace.DebugContext(
		ctx,
		"service_socket_tls_handshake_ok",
		"remote_addr",
		conn.RemoteAddr().String(),
		"elapsed_ms",
		elapsed.Milliseconds(),
		"tls_version",
		legacyTLSVersionName(state.Version),
		"cipher_suite",
		tls.CipherSuiteName(state.CipherSuite),
		"server_name_present",
		strings.TrimSpace(state.ServerName) != "",
	)
	return nil
}

func legacyTLSVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS1.0"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}

func readLegacySocketPayload(conn net.Conn) (legacySocketReadResult, error) {
	_ = conn.SetReadDeadline(time.Now().Add(legacyServiceReadTimeout))
	var data strings.Builder
	var rawHTTP strings.Builder
	subFragment := ""
	buf := make([]byte, legacyReadBufferSize)
	emptyReads := 0
	firstRead := true
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			insert := string(buf[:n])
			if firstRead {
				firstRead = false
				slog.Debug("service_socket_first_read",
					"remote_addr", conn.RemoteAddr().String(),
					"bytes", n,
					"raw_prefix", truncateForTrace(insert, 200),
				)
			}
			rawHTTP.WriteString(insert)
			if rawHTTP.Len() > legacyMaxRawRequestBytes {
				return legacySocketReadResult{}, errLegacyRequestTooLarge
			}
			if res, ok := readCompleteLegacyHTTPRequest(rawHTTP.String()); ok {
				return res, nil
			}
			if strings.TrimSpace(insert) == "" {
				emptyReads++
				if emptyReads > legacyMaxReadEmptyTries {
					return legacySocketReadResult{}, nil
				}
				continue
			}
			emptyReads = 0

			headLen := minLegacyInt(legacyBufferedSecRange, len(insert))
			subFragment += insert[:headLen]

			if hasLegacyEOF(subFragment) || hasLegacyEOF(insert) {
				foundOnSub := hasLegacyEOF(subFragment)
				source := insert
				if foundOnSub {
					source = subFragment
				}
				eofPos := indexLegacyEOF(source)
				lowerSource := strings.ToLower(source)
				idPos := strings.Index(lowerSource, "idsession")
				requestSessionID := extractLegacySessionID(source)

				if foundOnSub {
					if idPos >= 0 && idPos < legacyBufferedSecRange {
						trimLegacyBuilderTail(&data, legacyBufferedSecRange-idPos)
					} else if eofPos >= 0 && eofPos < legacyBufferedSecRange {
						trimLegacyBuilderTail(&data, legacyBufferedSecRange-eofPos)
					} else if idPos > 0 || eofPos > 0 {
						end := eofPos
						if idPos >= 0 {
							end = idPos
						}
						if end > 0 {
							writeLegacyMissingTail(&data, source[:end])
							if data.Len() > legacyMaxRawRequestBytes {
								return legacySocketReadResult{}, errLegacyRequestTooLarge
							}
						}
					}
				} else {
					end := eofPos
					if idPos >= 0 {
						end = idPos
					}
					if end > 0 {
						writeLegacyMissingTail(&data, insert[:end])
						if data.Len() > legacyMaxRawRequestBytes {
							return legacySocketReadResult{}, errLegacyRequestTooLarge
						}
					}
				}
				return legacySocketReadResult{
					RequestBody: data.String(),
					SessionID:   requestSessionID,
				}, nil
			}

			data.WriteString(insert)
			if data.Len() > legacyMaxRawRequestBytes {
				return legacySocketReadResult{}, errLegacyRequestTooLarge
			}
			if len(insert) > legacyBufferedSecRange {
				subFragment = insert[len(insert)-legacyBufferedSecRange:]
			} else {
				subFragment = insert
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return legacySocketReadResult{RequestBody: data.String()}, nil
			}
			return legacySocketReadResult{RequestBody: data.String()}, err
		}
	}
}

func readCompleteLegacyHTTPRequest(raw string) (legacySocketReadResult, bool) {
	method, headerEnd, contentLength, ok := parseLegacyHTTPFraming(raw)
	if !ok {
		return legacySocketReadResult{}, false
	}
	bodyStart := headerEnd
	bodyAvailable := len(raw) - bodyStart
	if strings.EqualFold(method, "OPTIONS") {
		return legacySocketReadResult{
			HTTPMethod: method,
			Origin:     parseLegacyHTTPHeader(raw[:headerEnd], "Origin"),
		}, true
	}
	if contentLength < 0 || bodyAvailable < contentLength {
		return legacySocketReadResult{}, false
	}
	body := raw[bodyStart : bodyStart+contentLength]
	return legacySocketReadResult{
		RequestBody: body,
		SessionID:   extractLegacySessionID(body),
		HTTPMethod:  method,
		Origin:      parseLegacyHTTPHeader(raw[:headerEnd], "Origin"),
	}, true
}

func parseLegacyHTTPHeader(rawHeaders, wanted string) string {
	for _, line := range strings.Split(rawHeaders, "\n") {
		name, value, found := strings.Cut(line, ":")
		if found && strings.EqualFold(strings.TrimSpace(name), wanted) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseLegacyHTTPFraming(raw string) (method string, bodyStart int, contentLength int, ok bool) {
	firstLineEnd := strings.Index(raw, "\n")
	if firstLineEnd <= 0 {
		return "", 0, 0, false
	}
	firstLine := strings.TrimSpace(raw[:firstLineEnd])
	parts := strings.Fields(firstLine)
	if len(parts) < 3 || !strings.HasPrefix(strings.ToUpper(parts[2]), "HTTP/") {
		return "", 0, 0, false
	}
	headerEnd := strings.Index(raw, "\r\n\r\n")
	separatorLen := 4
	if headerEnd < 0 {
		headerEnd = strings.Index(raw, "\n\n")
		separatorLen = 2
	}
	if headerEnd < 0 {
		return "", 0, 0, false
	}
	contentLength = 0
	for _, line := range strings.Split(raw[:headerEnd], "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return "", 0, 0, false
		}
		contentLength = n
		break
	}
	return strings.ToUpper(parts[0]), headerEnd + separatorLen, contentLength, true
}

func writeLegacyMissingTail(b *strings.Builder, tail string) {
	if tail == "" {
		return
	}
	current := b.String()
	if strings.HasSuffix(current, tail) {
		return
	}
	b.WriteString(tail)
}

func buildLegacyHTTPResponse(message, origin string) []byte {
	// El cliente JavaScript legacy de `afirma://service` decodifica en bloques
	// de 4 caracteres y espera Base64 URL-safe con padding. Si respondemos sin
	// '=' final (RawURLEncoding), el `echo` nunca se considera OK y el portal se
	// queda repitiendo sondas sin pasar a `cmd/fragment/firm/send`.
	body := base64.URLEncoding.EncodeToString([]byte(message))
	resp := "HTTP/1.1 200 OK\r\n" +
		"Connection: close\r\n" +
		"Pragma: no-cache\r\n" +
		"Server: GrxFirma\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		legacyServiceOriginResponseHeader(origin) +
		"\r\n" +
		body
	return []byte(resp)
}

func buildLegacyHTTPPreflightResponse(origin string) []byte {
	resp := "HTTP/1.1 204 No Content\r\n" +
		"Connection: close\r\n" +
		"Pragma: no-cache\r\n" +
		"Server: GrxFirma\r\n" +
		legacyServiceOriginResponseHeader(origin) +
		"Access-Control-Allow-Methods: POST, OPTIONS\r\n" +
		"Access-Control-Allow-Headers: Content-Type, Access-Control-Request-Private-Network\r\n" +
		"Access-Control-Allow-Private-Network: true\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"
	return []byte(resp)
}

func legacyServiceOriginResponseHeader(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.EqualFold(origin, "null") {
		return ""
	}
	return "Access-Control-Allow-Origin: " + origin + "\r\nVary: Origin\r\n"
}

func buildLegacyHTTPForbiddenResponse() []byte {
	return []byte("HTTP/1.1 403 Forbidden\r\nConnection: close\r\nContent-Length: 0\r\n\r\n")
}

func (s *LegacySocketServer) processCommand(ctx context.Context, req string) string {
	return s.processCommandWithOrigin(ctx, "", req)
}

func (s *LegacySocketServer) processCommandWithOrigin(ctx context.Context, origin, req string) string {
	trace := legacyServiceSocketTraceLogger()
	if v, ok := extractLegacyParam(req, "cmd="); ok {
		trace.DebugContext(ctx, "service_socket_command_detected", "command", "cmd", "value_len", len(v))
		cmd, err := decodeProtocolB64Compat(v)
		if err != nil {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_cmd_decode_error", "error", err)
			return "SAF_03: Comando cmd invalido"
		}
		uri := strings.TrimSpace(string(cmd))
		uri = ensureProtocolVersionParam(uri, s.protocolVersion)
		trace.DebugContext(ctx, "service_socket_cmd_uri", "uri_prefix", truncateForTrace(uri, 220))
		if !strings.HasPrefix(strings.ToLower(uri), "afirma://") {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_cmd_invalid_uri", "uri_prefix", truncateForTrace(uri, 220))
			return "SAF_03: Comando cmd invalido"
		}
		if isLegacyDirectSaveURI(uri) {
			return s.processDirectSaveURI(ctx, uri)
		}
		s.mu.Lock()
		alreadyPrepared := len(s.responseParts) > 0 && s.preparedURI == uri
		totalPrepared := len(s.responseParts)
		s.mu.Unlock()
		if alreadyPrepared {
			trace.DebugContext(ctx, "service_socket_cmd_reuse_prepared", "parts", totalPrepared)
			return strconv.Itoa(totalPrepared)
		}
		s.resetState()
		if err := s.prepareResponse(ctx, origin, uri); err != nil {
			trace.WarnContext(ctx, "service_socket_prepare_error", "command", "cmd", "uri_prefix", truncateForTrace(uri, 220), "error", err)
			return err.Error()
		}
		s.mu.Lock()
		total := len(s.responseParts)
		s.mu.Unlock()
		trace.DebugContext(ctx, "service_socket_cmd_prepared", "parts", total)
		if total <= 0 {
			return "0"
		}
		return strconv.Itoa(total)
	}

	if v, ok := extractLegacyParam(req, "echo="); ok {
		trace.DebugContext(ctx, "service_socket_command_detected", "command", "echo", "value_len", len(v))
		if strings.Contains(v, "-") {
			s.resetState()
		}
		return "OK"
	}

	if v, ok := extractLegacyParam(req, "fragment="); ok {
		trace.DebugContext(ctx, "service_socket_command_detected", "command", "fragment", "value_len", len(v))
		parts := strings.Split(v, "@")
		if len(parts) >= 2 && strings.TrimSpace(parts[1]) == "1" {
			s.resetState()
		}
		if len(parts) < 4 {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_fragment_invalid", "reason", "parts")
			return "SAF_03: Fragmento invalido"
		}
		partIdx, err1 := strconv.Atoi(strings.TrimSpace(parts[1]))
		partTotal, err2 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err1 != nil || err2 != nil || partIdx < 1 || partIdx > partTotal {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_fragment_invalid", "reason", "range")
			return "SAF_03: Fragmento invalido"
		}
		chunk, err := decodeProtocolB64Compat(parts[3])
		if err != nil {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_fragment_decode_error", "part_idx", partIdx, "part_total", partTotal, "error", err)
			return "SAF_03: Fragmento invalido"
		}
		s.mu.Lock()
		if s.fragmentsTotal != 0 && s.fragmentsTotal != partTotal {
			expectedTotal := s.fragmentsTotal
			s.clearFragmentAssemblyLocked()
			s.mu.Unlock()
			trace.WarnContext(ctx, "service_socket_fragment_invalid", "reason", "total_mismatch", "expected_total", expectedTotal, "part_total", partTotal)
			return "SAF_03: Fragmento invalido"
		}
		next, upErr := upsertLegacyFragment(s.fragments, partIdx, string(chunk))
		if upErr != nil {
			s.clearFragmentAssemblyLocked()
			s.mu.Unlock()
			trace.WarnContext(ctx, "service_socket_fragment_upsert_error", "part_idx", partIdx, "part_total", partTotal, "error", upErr)
			return "SAF_03: Fragmento invalido"
		}
		if totalLegacyFragmentsSize(next) > legacyMaxServicePayload {
			s.clearFragmentAssemblyLocked()
			s.mu.Unlock()
			trace.WarnContext(ctx, "service_socket_fragment_memory_error", "part_idx", partIdx, "part_total", partTotal)
			return legacyMemoryError
		}
		s.fragments = next
		s.fragmentsTotal = partTotal
		s.mu.Unlock()
		trace.DebugContext(ctx, "service_socket_fragment_stored", "part_idx", partIdx, "part_total", partTotal, "chunk_len", len(chunk))
		if partIdx == partTotal {
			return "OK"
		}
		return "MORE_DATA_NEED"
	}

	if v, ok := extractLegacyParam(req, "firm="); ok {
		trace.DebugContext(ctx, "service_socket_command_detected", "command", "firm", "value_len", len(v))
		// El JavaScript oficial V1.9 emite exactamente
		// `firm=idsession=<id>@EOF`, sin índices. Conservamos además la variante
		// indexada que aceptaban integraciones no oficiales, pero sus índices no
		// son obligatorios para el cliente canónico.
		requestedTotal := 0
		if strings.TrimSpace(v) != "" {
			parts := strings.Split(v, "@")
			if len(parts) < 3 {
				s.resetState()
				trace.WarnContext(ctx, "service_socket_firm_invalid", "reason", "parts")
				return "SAF_03: Peticion firm invalida"
			}
			partIdx, errIdx := strconv.Atoi(parts[1])
			partTotal, errTotal := strconv.Atoi(parts[2])
			if errIdx != nil || errTotal != nil || partIdx <= 0 || partTotal <= 0 || partIdx > partTotal {
				s.resetState()
				trace.WarnContext(ctx, "service_socket_firm_invalid", "reason", "range")
				return "SAF_03: Peticion firm invalida"
			}
			if partIdx != 1 {
				s.resetState()
				trace.WarnContext(ctx, "service_socket_firm_invalid", "reason", "part_idx_must_be_one", "part_idx", partIdx, "part_total", partTotal)
				return "SAF_03: Peticion firm invalida"
			}
			requestedTotal = partTotal
		}
		s.mu.Lock()
		joined := strings.Join(s.fragments, "")
		fragmentsLen := len(s.fragments)
		fragmentsTotal := s.fragmentsTotal
		alreadyPrepared := len(s.responseParts) > 0 && s.preparedURI == ensureProtocolVersionParam(joined, s.protocolVersion)
		totalPrepared := len(s.responseParts)
		s.mu.Unlock()
		if fragmentsTotal <= 0 && fragmentsLen > 0 {
			fragmentsTotal = fragmentsLen
		}
		trace.DebugContext(ctx, "service_socket_firm_state", "already_prepared", alreadyPrepared, "prepared_parts", totalPrepared, "joined_len", len(joined), "fragments_len", fragmentsLen, "fragments_total", fragmentsTotal)
		if requestedTotal > 0 && fragmentsTotal > 0 && requestedTotal != fragmentsTotal {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_firm_invalid", "reason", "total_mismatch", "part_total", requestedTotal, "fragments_total", fragmentsTotal)
			return "SAF_03: Peticion firm invalida"
		}
		if alreadyPrepared {
			return strconv.Itoa(totalPrepared)
		}
		if strings.TrimSpace(joined) == "" {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_firm_without_fragments")
			return "SAF_03: No hay datos fragmentados"
		}
		if fragmentsTotal <= 0 || fragmentsLen != fragmentsTotal {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_firm_incomplete_fragments", "fragments_len", fragmentsLen, "fragments_total", fragmentsTotal)
			return "SAF_03: Peticion firm invalida"
		}
		joined = ensureProtocolVersionParam(joined, s.protocolVersion)
		if isLegacyDirectSaveURI(joined) {
			return s.processDirectSaveURI(ctx, joined)
		}
		// No descartamos los fragmentos hasta que llegue `echo=-` o una nueva
		// parte 1. V1.9 conserva la petición para poder responder de nuevo a un
		// `firm=` reintentado cuando el navegador perdió la primera respuesta.
		s.mu.Lock()
		s.responseParts = nil
		s.preparedURI = ""
		s.mu.Unlock()
		if err := s.prepareResponse(ctx, origin, joined); err != nil {
			s.resetState()
			trace.WarnContext(ctx, "service_socket_prepare_error", "command", "firm", "uri_prefix", truncateForTrace(joined, 220), "error", err)
			return err.Error()
		}
		s.mu.Lock()
		total := len(s.responseParts)
		s.mu.Unlock()
		trace.DebugContext(ctx, "service_socket_firm_prepared", "parts", total)
		if total <= 0 {
			return "0"
		}
		return strconv.Itoa(total)
	}

	if v, ok := extractLegacyParam(req, "send="); ok {
		trace.DebugContext(ctx, "service_socket_command_detected", "command", "send", "value_len", len(v))
		parts := strings.Split(v, "@")
		if len(parts) < 3 {
			trace.WarnContext(ctx, "service_socket_send_invalid", "reason", "parts")
			return "SAF_03: Peticion send invalida"
		}
		partIdx, err1 := strconv.Atoi(strings.TrimSpace(parts[1]))
		partTotal, err2 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err1 != nil || err2 != nil || partIdx < 1 || partTotal < 1 || partIdx > partTotal {
			trace.WarnContext(ctx, "service_socket_send_invalid", "reason", "index")
			return "SAF_03: Peticion send invalida"
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if partTotal != len(s.responseParts) {
			trace.WarnContext(ctx, "service_socket_send_total_mismatch", "part_total", partTotal, "parts", len(s.responseParts))
			return "SAF_03: Peticion send invalida"
		}
		if partIdx > len(s.responseParts) {
			trace.WarnContext(ctx, "service_socket_send_out_of_range", "part_idx", partIdx, "parts", len(s.responseParts))
			return "SAF_03: Peticion send invalida"
		}
		trace.DebugContext(ctx, "service_socket_send_part", "part_idx", partIdx, "parts", len(s.responseParts), "response_part_len", len(s.responseParts[partIdx-1]))
		return s.responseParts[partIdx-1]
	}

	s.resetState()
	trace.WarnContext(ctx, "service_socket_unknown_command", "payload_prefix", truncateForTrace(req, 220))
	return "SAF_03: Parametros incorrectos"
}

func isLegacyDirectSaveURI(uri string) bool {
	lower := strings.ToLower(strings.TrimSpace(uri))
	return strings.HasPrefix(lower, "afirma://save?") ||
		strings.HasPrefix(lower, "afirma://save/?") ||
		strings.HasPrefix(lower, "afirma://signandsave?") ||
		strings.HasPrefix(lower, "afirma://signandsave/?")
}

func (s *LegacySocketServer) processDirectSaveURI(ctx context.Context, uri string) string {
	trace := legacyServiceSocketTraceLogger()
	s.resetState()
	result, err := s.adapter.HandleText(ctx, "", uri)
	if err != nil {
		trace.WarnContext(ctx, "service_socket_direct_save_error", "uri_prefix", truncateForTrace(uri, 220), "error", err)
		if normalized, ok := normalizeLegacySocketExplicitError(err.Error()); ok {
			return normalized
		}
		return "SAF_03: Parametros incorrectos"
	}
	formatted := FormatearRespuesta(result)
	switch strings.ToUpper(strings.TrimSpace(formatted)) {
	case "SAVE_OK", "OK":
		return "SAVE_OK"
	case "CANCEL":
		return "CANCEL"
	default:
		if normalized, ok := normalizeLegacySocketExplicitResult(formatted); ok {
			return normalized
		}
		if normalized, ok := normalizeLegacySocketExplicitError(formatted); ok {
			return normalized
		}
		return "SAF_03: Parametros incorrectos"
	}
}

func normalizeLegacySocketExplicitResult(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	upper := strings.ToUpper(raw)
	if upper == "CANCEL" {
		return "CANCEL", true
	}
	if !strings.HasPrefix(upper, "SAF_") && !strings.HasPrefix(upper, "ERR-") {
		return "", false
	}
	if idx := strings.Index(raw, ":="); idx > 0 {
		return strings.TrimSpace(raw[:idx]) + ": " + strings.TrimSpace(raw[idx+2:]), true
	}
	return raw, true
}

func normalizeLegacySocketExplicitError(raw string) (string, bool) {
	if normalized, ok := normalizeLegacySocketExplicitResult(raw); ok {
		return normalized, true
	}
	lower := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(lower, "cancel") {
		return "CANCEL", true
	}
	switch {
	case strings.Contains(lower, "comunicacion con el servicio de firma de lotes"),
		strings.Contains(lower, "comunicación con el servicio de firma de lotes"),
		strings.Contains(lower, "servicio de firma de lotes"),
		strings.Contains(lower, "servicio de lote"),
		strings.Contains(lower, "prefirma"),
		strings.Contains(lower, "postfirma"),
		strings.Contains(lower, "retrieve"),
		strings.Contains(lower, "storage"),
		strings.Contains(lower, "upload"),
		strings.Contains(lower, "timeout"),
		strings.Contains(lower, "connection"),
		strings.Contains(lower, "broken pipe"),
		strings.Contains(lower, "eof"),
		strings.Contains(lower, "http"):
		return "SAF_26: Error en la comunicación con el servicio de firma de lotes", true
	case strings.Contains(lower, "firma por lotes"),
		strings.Contains(lower, "error de lote remoto"),
		strings.Contains(lower, "batch remote"):
		return "SAF_27: Error en la firma por lotes", true
	case strings.Contains(lower, "lote de firma"),
		strings.Contains(lower, "proceso local del lote"),
		strings.Contains(lower, "operacion de lote"):
		return "SAF_20: Error en el proceso local del lote de firma", true
	case strings.Contains(lower, "no hay certificados"),
		strings.Contains(lower, "catalogo de certificados"),
		strings.Contains(lower, "almacen de certificados"):
		return "SAF_08: Error accediendo al almacen de certificados", true
	case strings.Contains(lower, "operacion de firma"),
		strings.Contains(lower, "clave de firma"),
		strings.Contains(lower, "aprobacion al usuario"):
		return "SAF_09: Error en la operacion de firma", true
	}
	return "", false
}

func (s *LegacySocketServer) prepareResponse(ctx context.Context, origin, uri string) error {
	trace := legacyServiceSocketTraceLogger()
	trace.DebugContext(ctx, "service_socket_prepare_start", "uri_prefix", truncateForTrace(uri, 220))
	result, err := s.adapter.HandleText(ctx, origin, uri)
	var formatted string
	if err != nil {
		trace.WarnContext(ctx, "service_socket_prepare_handle_error", "uri_prefix", truncateForTrace(uri, 220), "error", err)
		if normalized, ok := normalizeLegacySocketExplicitError(err.Error()); ok {
			formatted = normalized
		} else {
			formatted = "SAF_03: Parametros incorrectos"
		}
	} else {
		formatted = FormatearRespuesta(result)
		if normalized, ok := normalizeLegacySocketExplicitError(formatted); ok {
			trace.DebugContext(ctx, "service_socket_prepare_normalized_legacy_result", "result_type", result.Tipo, "operation", string(result.Operacion), "normalized", truncateForTrace(normalized, 220))
			formatted = normalized
		}
	}
	s.mu.Lock()
	s.preparedURI = uri
	s.responseParts = splitLegacyResponse(formatted)
	s.mu.Unlock()
	trace.DebugContext(
		ctx,
		"service_socket_prepare_done",
		"result_type",
		result.Tipo,
		"operation",
		string(result.Operacion),
		"requires_exchange",
		result.RequiereIntercambio,
		"response_parts",
		len(s.responseParts),
	)
	return nil
}

func (s *LegacySocketServer) checkSessionID(got string) error {
	expected := strings.TrimSpace(s.session)
	if expected == "" {
		return fmt.Errorf("session not configured")
	}
	actual := strings.TrimSpace(got)
	if actual == "" || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return fmt.Errorf("session mismatch")
	}
	return nil
}

func (s *LegacySocketServer) resetState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearFragmentAssemblyLocked()
}

func (s *LegacySocketServer) clearFragmentAssemblyLocked() {
	s.fragments = nil
	s.fragmentsTotal = 0
	s.responseParts = nil
	s.preparedURI = ""
}

func decodeProtocolB64Compat(v string) ([]byte, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("vacío")
	}
	normalized := strings.ReplaceAll(v, " ", "+")
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		out, err := enc.DecodeString(normalized)
		if err == nil {
			return out, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

func ensureProtocolVersionParam(uri string, v int) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return uri
	}
	u, err := url.Parse(uri)
	if err != nil || !strings.EqualFold(u.Scheme, "afirma") {
		return uri
	}
	action := strings.ToLower(strings.Trim(strings.TrimSpace(u.Host+u.Path), "/"))
	if val := strings.TrimSpace(u.Host); val != "" {
		action = strings.ToLower(val)
	}
	if action == "websocket" || action == "service" {
		return uri
	}
	if strings.TrimSpace(u.Query().Get("v")) != "" {
		return uri
	}
	q := u.Query()
	q.Set("v", strconv.Itoa(v))
	u.RawQuery = q.Encode()
	return u.String()
}

func splitLegacyResponse(resp string) []string {
	if resp == "" {
		return nil
	}
	validUTF8 := utf8.ValidString(resp)
	parts := make([]string, 0, (len(resp)/legacyResponseMaxSize)+1)
	for start := 0; start < len(resp); {
		end := start + legacyResponseMaxSize
		if end >= len(resp) {
			end = len(resp)
		} else if validUTF8 {
			// Cada parte se codifica en Base64 y autoscript.js la decodifica
			// como UTF-8 por separado. Cortar dentro de una runa corrompería
			// descripciones de error no ASCII al recomponer la respuesta.
			for end > start && !utf8.RuneStart(resp[end]) {
				end--
			}
		}
		parts = append(parts, resp[start:end])
		start = end
	}
	return parts
}

func hasLegacyEOF(s string) bool {
	return indexLegacyEOF(s) >= 0
}

func indexLegacyEOF(s string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(legacyEOFMarker))
}

func extractLegacySessionID(raw string) string {
	idx := strings.Index(strings.ToLower(raw), "idsession=")
	if idx < 0 {
		return ""
	}
	start := idx + len("idsession=")
	end := len(raw)
	lowerTail := strings.ToLower(raw[start:])
	for _, sep := range []string{"@eof", "&", " http/", "\r", "\n"} {
		if i := strings.Index(lowerTail, sep); i >= 0 && start+i < end {
			end = start + i
		}
	}
	return strings.TrimSpace(raw[start:end])
}

func trimLegacyBuilderTail(b *strings.Builder, n int) {
	if n <= 0 {
		return
	}
	s := b.String()
	if n >= len(s) {
		b.Reset()
		return
	}
	b.Reset()
	b.WriteString(s[:len(s)-n])
}

func minLegacyInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractLegacyParam(raw, key string) (string, bool) {
	lowerRaw := strings.ToLower(raw)
	lowerKey := strings.ToLower(key)
	idx := strings.Index(lowerRaw, lowerKey)
	if idx < 0 {
		return "", false
	}
	start := idx + len(key)
	end := len(raw)
	lowerTail := strings.ToLower(raw[start:])
	for _, sep := range []string{"idsession=", "@eof", "&", " http/", "\r", "\n"} {
		if i := strings.Index(lowerTail, sep); i >= 0 && start+i < end {
			end = start + i
		}
	}
	val := strings.TrimSpace(raw[start:end])
	val = strings.TrimPrefix(val, "?")
	if decoded, err := url.QueryUnescape(val); err == nil {
		val = decoded
	}
	return val, true
}

func totalLegacyFragmentsSize(frags []string) int {
	total := 0
	for _, f := range frags {
		total += len(f)
	}
	return total
}

func legacyServiceSocketTraceLogger() *slog.Logger {
	return logging.New("legacy/service-socket", os.Stderr)
}

func maskLegacyTraceValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return "[REDACTED]"
	}
	return raw[:3] + "***" + raw[len(raw)-2:]
}

func upsertLegacyFragment(frags []string, partIdx int, chunk string) ([]string, error) {
	if partIdx <= 0 {
		return nil, fmt.Errorf("invalid part")
	}
	if len(frags) == partIdx {
		frags[partIdx-1] = chunk
		return frags, nil
	}
	insertPos := partIdx - 1
	if insertPos < 0 || insertPos > len(frags) {
		return nil, fmt.Errorf("out of order")
	}
	frags = append(frags, "")
	copy(frags[insertPos+1:], frags[insertPos:])
	frags[insertPos] = chunk
	return frags, nil
}
