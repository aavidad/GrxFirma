// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bufio"
	"context"
	"crypto/sha1" // #nosec G505 -- RFC 6455 mandates SHA-1 only for the WebSocket handshake accept token.
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
)

const (
	websocketPortStart      = 8080
	websocketPortEnd        = 8089
	websocketGUID           = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	websocketMaxFrame       = 8 * 1024 * 1024
	compatMaxFormBytes      = websocketMaxFrame + 1024*1024
	websocketReadLimit      = 90 * time.Second
	websocketWriteLimit     = 30 * time.Second
	legacyStorageMaxEntries = 128
	legacyStorageMaxBytes   = 32 * 1024 * 1024
	legacyStorageTTL        = 10 * time.Minute
)

type legacyStorageEntry struct {
	value     string
	expiresAt time.Time
}

type legacyStorage struct {
	mu         sync.Mutex
	data       map[string]legacyStorageEntry
	totalBytes int
	now        func() time.Time
}

func newLegacyStorage() *legacyStorage {
	return &legacyStorage{
		data: make(map[string]legacyStorageEntry),
		now:  time.Now,
	}
}

func (s *legacyStorage) put(id, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked()
	previous, replacing := s.data[id]
	nextTotal := s.totalBytes + len(value)
	if replacing {
		nextTotal -= len(previous.value)
	}
	if (!replacing && len(s.data) >= legacyStorageMaxEntries) || nextTotal > legacyStorageMaxBytes {
		return false
	}
	s.data[id] = legacyStorageEntry{
		value:     value,
		expiresAt: s.now().Add(legacyStorageTTL),
	}
	s.totalBytes = nextTotal
	return true
}

func (s *legacyStorage) take(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked()
	entry, ok := s.data[id]
	if ok {
		delete(s.data, id)
		s.totalBytes -= len(entry.value)
	}
	return entry.value, ok
}

func (s *legacyStorage) cleanupExpiredLocked() {
	now := s.now()
	for id, entry := range s.data {
		if !entry.expiresAt.After(now) {
			delete(s.data, id)
			s.totalBytes -= len(entry.value)
		}
	}
}

const SecurityWarningMessage = `⚠ Atención: vas a activar el servidor WebSocket local

Al activar esta opción, GrxFirma abrirá un puerto de red en este equipo
(localhost:8080-8089). Esto implica:

  • Cualquier aplicación instalada en este equipo podrá solicitar
    firmas electrónicas mientras el servidor esté activo.

  • Un programa malicioso con acceso a la red local podría enviar
    solicitudes de firma sin que tú las hayas iniciado.

  • La firma electrónica tiene validez legal. Firma solo documentos
    que hayas revisado y aprobado explícitamente.

Esta opción es necesaria para compatibilidad con algunas webs de
firma que usan el protocolo antiguo. Si no sabes si la necesitas,
probablemente no la necesitas.

¿Deseas continuar?

  [ Entiendo los riesgos, activar WebSocket ]   [ Cancelar ]`

// TLSServer expone el servidor WSS local heredado.
type TLSServer struct {
	Server   *http.Server
	Addr     string
	CertFile string
	KeyFile  string
}

// SessionHooks permite observar el ciclo de vida de una sesión legacy puntual.
// Se usa en lanzamientos afirma://websocket por sesión, no en el servicio base.
type SessionHooks struct {
	// ExpectedSessionID liga los mensajes que incluyan idsession al
	// lanzamiento afirma://. Solo se admite una conexión activa por sesión.
	ExpectedSessionID string
	active            atomic.Bool
	OnSocketConnected func(origin string)
	OnMessageReceived func(operation string)
	OnSocketClosed    func()
	OnMessageHandled  func(Resultado)
	OnMessageError    func(operation string, err error)
}

type tlsServerDependencies struct {
	ensureTrusted        func(context.Context, string) error
	ensureManagedTrusted func(context.Context, string) error
}

func defaultTLSServerDependencies() tlsServerDependencies {
	return tlsServerDependencies{
		ensureTrusted:        localtlstrust.EnsureTrusted,
		ensureManagedTrusted: localtlstrust.EnsureManagedTrusted,
	}
}

// StartTLSServer arranca el servidor WSS legacy solo si la configuración lo permite.
func StartTLSServer(ctx context.Context, cfg config.Config, adapter *Adaptador, certDir string) (*TLSServer, error) {
	return StartTLSServerOnPorts(ctx, cfg, adapter, certDir, nil)
}

// StartTLSServerOnPorts arranca el servidor WSS legacy en los puertos indicados.
// Si ports es nil o vacio, usa el rango por defecto 8080-8089.
func StartTLSServerOnPorts(ctx context.Context, cfg config.Config, adapter *Adaptador, certDir string, ports []int) (*TLSServer, error) {
	return startTLSServerOnPortsWithHooks(ctx, cfg, adapter, certDir, ports, nil)
}

// StartTLSServerOnPortsWithHooks arranca un servidor WSS legacy con hooks de sesión.
func StartTLSServerOnPortsWithHooks(ctx context.Context, cfg config.Config, adapter *Adaptador, certDir string, ports []int, hooks *SessionHooks) (*TLSServer, error) {
	return startTLSServerOnPortsWithHooks(ctx, cfg, adapter, certDir, ports, hooks)
}

func startTLSServerOnPortsWithHooks(ctx context.Context, cfg config.Config, adapter *Adaptador, certDir string, ports []int, hooks *SessionHooks) (*TLSServer, error) {
	return startTLSServerOnPortsWithHooksAndDependencies(ctx, cfg, adapter, certDir, ports, hooks, defaultTLSServerDependencies())
}

func startTLSServerOnPortsWithHooksAndDependencies(ctx context.Context, cfg config.Config, adapter *Adaptador, certDir string, ports []int, hooks *SessionHooks, deps tlsServerDependencies) (*TLSServer, error) {
	if !cfg.PermiteWebSocket() || !cfg.WebsocketHabilitado {
		return nil, nil
	}
	if adapter == nil {
		return nil, errors.New("websocket: adaptador no configurado")
	}
	if deps.ensureTrusted == nil {
		return nil, errors.New("websocket: instalador de confianza TLS no configurado")
	}
	if deps.ensureManagedTrusted == nil {
		// Compatibilidad del seam de pruebas: una dependencia explícita de
		// confianza intercepta ambos modos. Producción configura ambas APIs.
		deps.ensureManagedTrusted = deps.ensureTrusted
	}

	certFile, keyFile, rootCertFile, certSource, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(certDir, "websocket-localhost")
	if err != nil {
		return nil, err
	}
	ensureTrust := deps.ensureTrusted
	if certSource == "grxfirma-local-ca" {
		ensureTrust = deps.ensureManagedTrusted
	}
	if err := ensureTrust(ctx, rootCertFile); err != nil &&
		!errors.Is(err, localtlstrust.ErrSoporteNoDisponible) &&
		!errors.Is(err, localtlstrust.ErrHerramientaNoDisponible) {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("websocket: no se pudo cargar el par TLS: %w", err)
	}

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	var ln net.Listener
	candidates := puertosCandidatos(ports)
	for _, port := range candidates {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err = tls.Listen("tcp", addr, tlsCfg)
		if err == nil {
			break
		}
	}
	if ln == nil {
		if len(candidates) == 0 {
			return nil, fmt.Errorf("websocket: no se proporcionaron puertos válidos")
		}
		return nil, fmt.Errorf("websocket: no se pudo abrir ningún puerto solicitado (%v): %w", candidates, err)
	}

	mux := http.NewServeMux()
	store := newLegacyStorage()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := serveLegacyWebSocket(ctx, adapter, hooks, nil, w, r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/StorageService", func(w http.ResponseWriter, r *http.Request) {
		handleStorageService(store, w, r, adapter.Trust, nil)
	})
	mux.HandleFunc("/RetrieveService", func(w http.ResponseWriter, r *http.Request) {
		handleRetrieveService(store, w, r, adapter.Trust, nil)
	})
	mux.HandleFunc("/afirma-signature-storage/StorageService", func(w http.ResponseWriter, r *http.Request) {
		handleStorageService(store, w, r, adapter.Trust, nil)
	})
	mux.HandleFunc("/afirma-signature-retriever/RetrieveService", func(w http.ResponseWriter, r *http.Request) {
		handleRetrieveService(store, w, r, adapter.Trust, nil)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	configureWebSocketServerDiagnostics(srv, legacyWebSocketTraceLogger())
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		_ = srv.Serve(ln)
	}()

	return &TLSServer{
		Server:   srv,
		Addr:     ln.Addr().String(),
		CertFile: certFile,
		KeyFile:  keyFile,
	}, nil
}

type redactedHTTPServerErrorWriter struct {
	logger *slog.Logger
}

func (w redactedHTTPServerErrorWriter) Write(p []byte) (int, error) {
	raw := strings.TrimSpace(string(p))
	if raw == "" || w.logger == nil {
		return len(p), nil
	}
	event, failureKind := classifyHTTPServerDiagnostic(raw)
	// El atributo error pasa por la clasificación cerrada del logger común:
	// nunca se persiste el texto libre que puede incluir direcciones o datos
	// enviados por un cliente antes de completar el handshake WebSocket.
	w.logger.Warn(event, "failure_kind", failureKind, "error", errors.New(raw))
	return len(p), nil
}

func configureWebSocketServerDiagnostics(srv *http.Server, trace *slog.Logger) {
	if srv == nil || trace == nil {
		return
	}
	srv.ErrorLog = log.New(redactedHTTPServerErrorWriter{logger: trace}, "", 0)
	srv.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			// Una única señal en DEBUG por conexión. No se registra la dirección
			// ni se añaden eventos para el resto de transiciones del socket.
			trace.Debug("websocket_tcp_connection_accepted", "transport", "tcp")
		}
	}
}

func classifyHTTPServerDiagnostic(raw string) (event, failureKind string) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(lower, "tls handshake error"):
		switch {
		case strings.Contains(lower, "client sent an http request"):
			return "websocket_tls_handshake_error", "plaintext_http"
		case strings.Contains(lower, "unsupported versions"),
			strings.Contains(lower, "protocol version"):
			return "websocket_tls_handshake_error", "unsupported_tls_version"
		case strings.Contains(lower, "certificate"):
			return "websocket_tls_handshake_error", "certificate_rejected"
		default:
			return "websocket_tls_handshake_error", "handshake_failed"
		}
	case strings.Contains(lower, "accept tcp"),
		strings.Contains(lower, "accept error"):
		return "websocket_tcp_accept_error", "accept_failed"
	default:
		return "websocket_http_server_error", "server_error"
	}
}

func handleStorageService(store *legacyStorage, w http.ResponseWriter, r *http.Request, trust ports.TrustPolicy, allowedDomains []string) {
	trace := legacyWebSocketTraceLogger()
	trace.DebugContext(r.Context(), "storage_service_request_start", "remote_addr", r.RemoteAddr, "method", r.Method, "origin", r.Header.Get("Origin"))
	if !isLoopbackRemoteAddr(r.RemoteAddr) {
		trace.WarnContext(r.Context(), "storage_service_remote_rejected", "remote_addr", r.RemoteAddr)
		http.Error(w, compatError("ERR-99", "Peticion externa no permitida"), http.StatusForbidden)
		return
	}
	if ok := setCompatHeaders(r.Context(), w, r, trust, allowedDomains); !ok {
		trace.WarnContext(r.Context(), "storage_service_origin_rejected", "remote_addr", r.RemoteAddr, "origin", r.Header.Get("Origin"))
		http.Error(w, compatError("ERR-99", "Origen no permitido"), http.StatusForbidden)
		return
	}
	if r.Method == http.MethodOptions {
		trace.DebugContext(r.Context(), "storage_service_options")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := parseCompatForm(w, r); err != nil {
		trace.WarnContext(r.Context(), "storage_service_parse_error", "error", err)
		_, _ = w.Write([]byte(compatError("ERR-07", "Los datos solicitados o enviados son inválidos")))
		return
	}
	op := strings.TrimSpace(r.Form.Get("op"))
	trace.DebugContext(r.Context(), "storage_service_request", "operation", safeLegacyServiceOperation(op), "id", maskLegacyTraceValue(strings.TrimSpace(r.Form.Get("id"))), "has_data", strings.TrimSpace(r.Form.Get("dat")) != "")
	if op == "check" {
		_, _ = w.Write([]byte("OK"))
		return
	}
	if op == "" {
		_, _ = w.Write([]byte(compatError("ERR-00", "No se ha indicado código de operación")))
		return
	}
	if strings.TrimSpace(r.Form.Get("v")) == "" {
		_, _ = w.Write([]byte(compatError("ERR-20", "No se ha indicado la versión de la sintaxis de la operación")))
		return
	}
	if op != "put" {
		_, _ = w.Write([]byte(compatError("ERR-01", "Código de operación no soportado")))
		return
	}
	id := strings.TrimSpace(r.Form.Get("id"))
	if id == "" {
		_, _ = w.Write([]byte(compatError("ERR-05", "No se ha proporcionado un identificador para los datos")))
		return
	}
	data := r.Form.Get("dat")
	if strings.TrimSpace(data) == "" {
		trace.WarnContext(r.Context(), "storage_service_empty_data", "id", maskLegacyTraceValue(id))
		_, _ = w.Write([]byte(compatError("ERR-07", "Los datos solicitados o enviados son inválidos")))
		return
	}
	if !store.put(id, data) {
		trace.WarnContext(r.Context(), "storage_service_quota_exceeded", "id", maskLegacyTraceValue(id), "data_len", len(data))
		_, _ = w.Write([]byte(legacyMemoryError))
		return
	}
	trace.DebugContext(r.Context(), "storage_service_put_ok", "id", maskLegacyTraceValue(id), "data_len", len(data))
	_, _ = w.Write([]byte("OK"))
}

func handleRetrieveService(store *legacyStorage, w http.ResponseWriter, r *http.Request, trust ports.TrustPolicy, allowedDomains []string) {
	trace := legacyWebSocketTraceLogger()
	trace.DebugContext(r.Context(), "retrieve_service_request_start", "remote_addr", r.RemoteAddr, "method", r.Method, "origin", r.Header.Get("Origin"))
	if !isLoopbackRemoteAddr(r.RemoteAddr) {
		trace.WarnContext(r.Context(), "retrieve_service_remote_rejected", "remote_addr", r.RemoteAddr)
		http.Error(w, compatError("ERR-99", "Peticion externa no permitida"), http.StatusForbidden)
		return
	}
	if ok := setCompatHeaders(r.Context(), w, r, trust, allowedDomains); !ok {
		trace.WarnContext(r.Context(), "retrieve_service_origin_rejected", "remote_addr", r.RemoteAddr, "origin", r.Header.Get("Origin"))
		http.Error(w, compatError("ERR-99", "Origen no permitido"), http.StatusForbidden)
		return
	}
	if r.Method == http.MethodOptions {
		trace.DebugContext(r.Context(), "retrieve_service_options")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := parseCompatForm(w, r); err != nil {
		trace.WarnContext(r.Context(), "retrieve_service_parse_error", "error", err)
		_, _ = w.Write([]byte(compatError("ERR-07", "Los datos solicitados o enviados son inválidos")))
		return
	}
	op := strings.TrimSpace(r.Form.Get("op"))
	trace.DebugContext(r.Context(), "retrieve_service_request", "operation", safeLegacyServiceOperation(op), "id", maskLegacyTraceValue(strings.TrimSpace(r.Form.Get("id"))))
	if op == "check" {
		_, _ = w.Write([]byte("OK"))
		return
	}
	if op == "" {
		_, _ = w.Write([]byte(compatError("ERR-00", "No se ha indicado código de operación")))
		return
	}
	if strings.TrimSpace(r.Form.Get("v")) == "" {
		_, _ = w.Write([]byte(compatError("ERR-20", "No se ha indicado la versión de la sintaxis de la operación")))
		return
	}
	if op != "get" {
		_, _ = w.Write([]byte(compatError("ERR-01", "Código de operación no soportado")))
		return
	}
	id := strings.TrimSpace(r.Form.Get("id"))
	if id == "" {
		_, _ = w.Write([]byte(compatError("ERR-05", "No se ha proporcionado un identificador para los datos")))
		return
	}
	data, ok := store.take(id)
	if !ok {
		trace.WarnContext(r.Context(), "retrieve_service_missing_id", "id", maskLegacyTraceValue(id))
		_, _ = w.Write([]byte(compatError("ERR-06", "El identificador para los datos es inválido")))
		return
	}
	trace.DebugContext(r.Context(), "retrieve_service_get_ok", "id", maskLegacyTraceValue(id), "data_len", len(data))
	_, _ = w.Write([]byte(data))
}

func puertosCandidatos(ports []int) []int {
	if len(ports) == 0 {
		out := make([]int, 0, websocketPortEnd-websocketPortStart+1)
		for port := websocketPortStart; port <= websocketPortEnd; port++ {
			out = append(out, port)
		}
		return out
	}
	seen := make(map[int]struct{}, len(ports))
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	return out
}

func compatError(code, message string) string {
	return code + ":=" + message
}

func safeLegacyServiceOperation(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "missing"
	case "check":
		return "check"
	case "put":
		return "put"
	case "get":
		return "get"
	default:
		return "unsupported"
	}
}

func setCompatHeaders(ctx context.Context, w http.ResponseWriter, r *http.Request, trust ports.TrustPolicy, allowedDomains []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if !isAllowedWebOriginWithPolicy(ctx, origin, trust, allowedDomains) {
		return false
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	return true
}

func parseCompatForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, compatMaxFormBytes)
	return r.ParseForm()
}

// Por defecto se rechazan los handshakes sin Origin y los orígenes opacos
// "null" (iframes con sandbox, file://, data:). Para integraciones V1
// estrictamente locales que no envían Origin, la única excepción es la
// política de máquina permitir_origen_vacio, fijada por un administrador. La
// la variable GRXFIRMA_WS_ALLOW_EMPTY_ORIGIN ya no rebaja la seguridad.
const envWSPermitirOrigenVacio = "GRXFIRMA_WS_ALLOW_EMPTY_ORIGIN"

func origenEstrictoRequerido() bool {
	return !machinepolicy.OptIn(machinepolicy.PermitirOrigenVacio)
}

func isAllowedWebOriginWithPolicy(ctx context.Context, origin string, trust ports.TrustPolicy, fallbackDomains []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.EqualFold(origin, "null") {
		return !origenEstrictoRequerido()
	}
	if isOwnLocalSignerOrigin(origin) {
		return true
	}
	if trust == nil {
		return isAllowedWebOrigin(origin, fallbackDomains)
	}
	decision, err := trust.Evaluate(ctx, origin)
	if err != nil {
		return false
	}
	return decision.IsAllowed()
}

func isAllowedWebOrigin(origin string, allowedDomains []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.EqualFold(origin, "null") {
		return !origenEstrictoRequerido()
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return false
	}
	if isOwnLocalSignerOrigin(origin) {
		return true
	}
	// Ni las extensiones ni otros servidores locales se aceptan por defecto:
	// cualquier extensión instalada o cualquier web servida en loopback podría
	// pedir firmas. Deben autorizarse de forma explícita en la política.
	if scheme != "https" || isLoopbackHost(host) {
		return false
	}
	// External HTTPS: only allow hosts that match a configured trusted domain.
	for _, domain := range allowedDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		exactHostOnly := false
		if parsed, err := url.Parse(domain); err == nil && parsed.Hostname() != "" {
			if strings.ToLower(strings.TrimSpace(parsed.Scheme)) != "https" {
				continue
			}
			domain = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
			exactHostOnly = true
		}
		if exactHostOnly {
			if host == domain {
				return true
			}
			continue
		}
		if strings.HasPrefix(domain, "*.") {
			suffix := domain[1:] // e.g. ".gob.es"
			if host == domain[2:] || strings.HasSuffix(host, suffix) {
				return true
			}
		} else {
			if host == domain || strings.HasSuffix(host, "."+domain) {
				return true
			}
		}
	}
	return false
}

// ownLocalSignerPort es el puerto del firmador local HTTPS de GrxFirma.
const ownLocalSignerPort = "63118"

// isOwnLocalSignerOrigin acepta únicamente la página del propio firmador
// local. Antes se aceptaba cualquier origen loopback (cualquier web servida
// por otra aplicación del equipo) y cualquier extensión del navegador.
func isOwnLocalSignerOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" || strings.EqualFold(origin, "null") {
		return !origenEstrictoRequerido()
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https") &&
		isLoopbackHost(u.Hostname()) &&
		u.Port() == ownLocalSignerPort &&
		strings.TrimRight(u.Path, "/") == "" && u.RawQuery == "" && u.Fragment == ""
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackRemoteAddr(addr string) bool {
	host := strings.TrimSpace(addr)
	if parsedHost, _, err := net.SplitHostPort(addr); err == nil {
		host = parsedHost
	}
	return isLoopbackHost(host)
}

// StartTLSServerWithGovernance arranca el servidor WSS heredado exigiendo aprobación explícita
// y registrando la activación en auditoría.
func StartTLSServerWithGovernance(
	ctx context.Context,
	cfg config.Config,
	adapter *Adaptador,
	certDir string,
	aprobador ports.UserApproval,
	auditor *application.AuditUseCase,
) (*TLSServer, error) {
	return startTLSServerWithGovernanceAndDependencies(ctx, cfg, adapter, certDir, aprobador, auditor, defaultTLSServerDependencies())
}

func startTLSServerWithGovernanceAndDependencies(
	ctx context.Context,
	cfg config.Config,
	adapter *Adaptador,
	certDir string,
	aprobador ports.UserApproval,
	auditor *application.AuditUseCase,
	deps tlsServerDependencies,
) (*TLSServer, error) {
	if !cfg.PermiteWebSocket() || !cfg.WebsocketHabilitado {
		return nil, nil
	}
	if aprobador == nil {
		return nil, errors.New("websocket: aprobador de usuario no configurado")
	}
	ok, err := aprobador.Request(ctx, SecurityWarningMessage)
	if err != nil {
		auditarActivacionWebSocket(ctx, auditor, cfg, false, fmt.Errorf("error de aprobacion: %w", err))
		return nil, fmt.Errorf("websocket: no se pudo solicitar aprobación: %w", err)
	}
	if !ok {
		err = errors.New("el usuario canceló la activación del WebSocket")
		auditarActivacionWebSocket(ctx, auditor, cfg, false, err)
		return nil, err
	}

	srv, err := startTLSServerOnPortsWithHooksAndDependencies(ctx, cfg, adapter, certDir, nil, nil, deps)
	if err != nil {
		auditarActivacionWebSocket(ctx, auditor, cfg, false, err)
		return nil, err
	}
	auditarActivacionWebSocket(ctx, auditor, cfg, true, nil)
	return srv, nil
}

func serveLegacyWebSocket(ctx context.Context, adapter *Adaptador, hooks *SessionHooks, allowedDomains []string, w http.ResponseWriter, r *http.Request) error {
	trace := legacyWebSocketTraceLogger()
	trace.DebugContext(ctx, "websocket_handshake_start", "remote_addr", r.RemoteAddr, "host_present", strings.TrimSpace(r.Host) != "", "origin", r.Header.Get("Origin"), "user_agent", r.UserAgent())
	if !headerContainsToken(r.Header, "Connection", "Upgrade") || !headerContainsToken(r.Header, "Upgrade", "websocket") {
		trace.WarnContext(
			ctx,
			"websocket_handshake_invalid_upgrade",
			"remote_addr", r.RemoteAddr,
			"has_connection_upgrade", headerContainsToken(r.Header, "Connection", "Upgrade"),
			"has_upgrade_websocket", headerContainsToken(r.Header, "Upgrade", "websocket"),
			"origin", r.Header.Get("Origin"),
		)
		return errors.New("websocket: handshake inválido")
	}

	// H-06: Validar Origin en el handshake WebSocket antes de aceptar la conexión.
	// Orígenes externos sólo se aceptan si la política de confianza central los permite.
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if !isAllowedWebOriginWithPolicy(ctx, origin, adapter.Trust, allowedDomains) {
		trace.WarnContext(ctx, "websocket_handshake_forbidden_origin", "remote_addr", r.RemoteAddr, "origin", origin, "path", r.URL.Path)
		http.Error(w, "WebSocket: origen no permitido", http.StatusForbidden)
		return nil
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		trace.WarnContext(ctx, "websocket_handshake_missing_key", "remote_addr", r.RemoteAddr, "origin", r.Header.Get("Origin"))
		return errors.New("websocket: falta Sec-WebSocket-Key")
	}
	if hooks != nil && hooks.ExpectedSessionID != "" {
		if !hooks.active.CompareAndSwap(false, true) {
			http.Error(w, "WebSocket: sesión ocupada", http.StatusConflict)
			return nil
		}
		defer hooks.active.Store(false)
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		trace.ErrorContext(ctx, "websocket_hijack_not_supported")
		return errors.New("websocket: hijack no soportado")
	}
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		trace.ErrorContext(ctx, "websocket_hijack_error", "error", err)
		return fmt.Errorf("websocket: hijack: %w", err)
	}
	defer conn.Close()
	socketDone := make(chan struct{})
	defer close(socketDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
			_ = conn.Close()
		case <-socketDone:
		}
	}()
	accept := computeWebSocketAccept(key)
	if err := conn.SetWriteDeadline(time.Now().Add(websocketWriteLimit)); err != nil {
		return fmt.Errorf("websocket: configurar deadline de handshake: %w", err)
	}
	if _, err := fmt.Fprintf(buf, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept); err != nil {
		trace.ErrorContext(ctx, "websocket_handshake_write_error", "error", err)
		return err
	}
	if err := buf.Flush(); err != nil {
		trace.ErrorContext(ctx, "websocket_handshake_flush_error", "error", err)
		return err
	}
	// Solo una conexión WebSocket aceptada consume la sesión temporal.
	// Si el handshake falla, el listener sigue disponible para el portal.
	defer func() {
		if hooks != nil && hooks.OnSocketClosed != nil {
			hooks.OnSocketClosed()
		}
	}()
	_ = conn.SetWriteDeadline(time.Time{})
	trace.InfoContext(ctx, "websocket_handshake_ok", "remote_addr", r.RemoteAddr, "origin", r.Header.Get("Origin"), "host_present", strings.TrimSpace(r.Host) != "")
	if hooks != nil && hooks.OnSocketConnected != nil {
		hooks.OnSocketConnected(strings.TrimSpace(r.Header.Get("Origin")))
	}

	for {
		if err := conn.SetReadDeadline(time.Now().Add(websocketReadLimit)); err != nil {
			return fmt.Errorf("websocket: configurar deadline de lectura: %w", err)
		}
		frame, err := readClientFrame(buf.Reader)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				trace.DebugContext(ctx, "websocket_peer_closed")
				return nil
			}
			trace.ErrorContext(ctx, "websocket_read_error", "error", err)
			return err
		}

		switch frame.opcode {
		case 0x8:
			trace.DebugContext(ctx, "websocket_close_received")
			return nil
		case 0x9:
			trace.DebugContext(ctx, "websocket_ping_received")
			if err := writeWebSocketFrame(conn, func(w io.Writer) error {
				return writeServerControlFrame(w, 0xA, frame.payload)
			}); err != nil {
				trace.ErrorContext(ctx, "websocket_pong_error", "error", err)
				return err
			}
			continue
		case 0x1:
			message := string(frame.payload)
			if hooks != nil && hooks.ExpectedSessionID != "" {
				if got := extractLegacySessionID(message); got != "" &&
					(len(got) != len(hooks.ExpectedSessionID) || subtle.ConstantTimeCompare([]byte(got), []byte(hooks.ExpectedSessionID)) != 1) {
					trace.WarnContext(ctx, "websocket_session_mismatch")
					return errors.New("websocket: sesión no coincide con el lanzamiento")
				}
			}
			trace.DebugContext(ctx, "websocket_message_received", "prefix", truncateForTrace(message, 160), "length", len(message), "origin", r.Header.Get("Origin"))
			op := detectLegacyOperationForTrace(message)
			// El eco de AutoScript ("echo=-idsession=...@EOF") precede a cada
			// operación: no es una operación desconocida ni debe fijar la fase.
			esEco := strings.HasPrefix(strings.ToLower(strings.TrimSpace(message)), EchoRequestPrefix)
			if op == "" && !esEco {
				trace.WarnContext(ctx, "websocket_message_operation_unknown", "length", len(message), "prefix", truncateForTrace(message, 160))
			}
			if hooks != nil && hooks.OnMessageReceived != nil && !esEco {
				hooks.OnMessageReceived(op)
			}
			// Si la web cierra el canal mientras se espera al usuario (por
			// ejemplo, en el editor del sello), la operación se cancela y no
			// deja ventanas ni procesos abiertos.
			handleCtx, stopWatching := watchPeerClose(ctx, conn, buf.Reader)
			result, err := adapter.HandleText(handleCtx, strings.TrimSpace(r.Header.Get("Origin")), message)
			stopWatching()
			if err != nil {
				reply := legacyWebSocketErrorText(err)
				trace.WarnContext(ctx, "websocket_handle_error", "operation", op, "error", err, "reply", reply, "origin", r.Header.Get("Origin"), "prefix", truncateForTrace(message, 160))
				if hooks != nil && hooks.OnMessageError != nil {
					hooks.OnMessageError(op, err)
				}
				if err := writeWebSocketFrame(conn, func(w io.Writer) error {
					return writeServerTextFrame(w, reply)
				}); err != nil {
					trace.ErrorContext(ctx, "websocket_write_error", "error", err)
					return err
				}
				continue
			}
			trace.DebugContext(ctx, "websocket_message_handled", "result_type", result.Tipo, "operation", string(result.Operacion), "requires_exchange", result.RequiereIntercambio, "origin", r.Header.Get("Origin"))
			if err := writeWebSocketFrame(conn, func(w io.Writer) error {
				return writeServerTextFrame(w, FormatearRespuesta(result))
			}); err != nil {
				trace.ErrorContext(ctx, "websocket_write_error", "error", err)
				if hooks != nil && hooks.OnMessageError != nil {
					hooks.OnMessageError(op, err)
				}
				return err
			}
			if hooks != nil && hooks.OnMessageHandled != nil {
				hooks.OnMessageHandled(result)
			}
		default:
			trace.WarnContext(ctx, "websocket_unsupported_opcode", "opcode", frame.opcode, "origin", r.Header.Get("Origin"))
			return fmt.Errorf("websocket: opcode no soportado: %d", frame.opcode)
		}
	}
}

// watchPeerClose cancela el contexto devuelto si el portal cierra la conexión
// o envía un marco de cierre mientras se atiende un mensaje. Solo mira el
// siguiente byte sin consumirlo: el bucle principal lo leerá después. La
// función de parada debe llamarse antes de volver a leer de reader.
func watchPeerClose(ctx context.Context, conn net.Conn, reader *bufio.Reader) (context.Context, func()) {
	handleCtx, cancel := context.WithCancel(ctx)
	// La espera del usuario puede superar websocketReadLimit.
	_ = conn.SetReadDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		first, err := reader.Peek(1)
		switch {
		case err != nil:
			if !errors.Is(err, os.ErrDeadlineExceeded) {
				cancel()
			}
		case first[0]&0x0F == 0x8:
			cancel()
		}
	}()
	return handleCtx, func() {
		_ = conn.SetReadDeadline(time.Now())
		<-done
		cancel()
		_ = conn.SetReadDeadline(time.Time{})
	}
}

func writeWebSocketFrame(conn net.Conn, write func(io.Writer) error) error {
	if err := conn.SetWriteDeadline(time.Now().Add(websocketWriteLimit)); err != nil {
		return err
	}
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	return write(conn)
}

func detectLegacyOperationForTrace(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(raw, "afirma://batch"), strings.Contains(raw, "op=batch"):
		return "batch"
	case strings.Contains(raw, "afirma://save"), strings.Contains(raw, "op=save"):
		return "save"
	case strings.Contains(raw, "afirma://load"), strings.Contains(raw, "op=load"):
		return "load"
	case strings.Contains(raw, "signandsave"), strings.Contains(raw, "op=signandsave"):
		return "signandsave"
	case strings.Contains(raw, "selectcert"), strings.Contains(raw, "op=selectcert"):
		return "selectcert"
	case strings.Contains(raw, "afirma://countersign"), strings.Contains(raw, "op=countersign"):
		return "countersign"
	case strings.Contains(raw, "afirma://cosign"), strings.Contains(raw, "op=cosign"):
		return "cosign"
	case strings.Contains(raw, "afirma://sign"), strings.Contains(raw, "op=sign"):
		return "sign"
	default:
		return ""
	}
}

func legacyWebSocketTraceLogger() *slog.Logger {
	return logging.New("legacy/websocket", os.Stderr)
}

func truncateForTrace(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= limit {
		return raw
	}
	return raw[:limit] + "..."
}

func legacyWebSocketErrorText(err error) string {
	if err == nil {
		return compatError("SAF_03", "Parametros incorrectos")
	}
	if errors.Is(err, cryptopolicy.ErrSHA1Disabled) {
		return compatError(
			"SAF_09",
			"Firma bloqueada: el portal solicita SHA-1, un algoritmo obsoleto e inseguro. No se ha generado ninguna firma; la entidad responsable del portal debe actualizarlo a SHA-256 o superior.",
		)
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return compatError("SAF_03", "Parametros incorrectos")
	}
	if code, detail, ok := parseLegacyExplicitError(msg); ok {
		return compatError(code, detail)
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "cancel"):
		return "CANCEL"
	case strings.Contains(lower, "lote de firma"),
		strings.Contains(lower, "proceso local del lote"),
		strings.Contains(lower, "operacion de lote"):
		return compatError("SAF_20", "Error en el proceso local del lote de firma")
	case strings.Contains(lower, "no hay certificados"),
		strings.Contains(lower, "catalogo de certificados"),
		strings.Contains(lower, "almacen de certificados"):
		return compatError("SAF_08", "Error accediendo al almacen de certificados")
	case strings.Contains(lower, "operacion de firma"),
		strings.Contains(lower, "clave de firma"),
		strings.Contains(lower, "clave privada"),
		strings.Contains(lower, "clave no encontrada"),
		strings.Contains(lower, "certificado seleccionado"),
		strings.Contains(lower, "aprobacion al usuario"):
		return compatError("SAF_09", "Error en la operacion de firma")
	default:
		return compatError("SAF_03", "Parametros incorrectos")
	}
}

func parseLegacyExplicitError(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	upper := strings.ToUpper(raw)
	if !strings.HasPrefix(upper, "SAF_") && !strings.HasPrefix(upper, "ERR-") {
		return "", "", false
	}
	if idx := strings.Index(raw, ":="); idx > 0 {
		return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx+2:]), true
	}
	if idx := strings.Index(raw, ":"); idx > 0 {
		return strings.TrimSpace(raw[:idx]), strings.TrimSpace(raw[idx+1:]), true
	}
	return strings.TrimSpace(raw), "Parametros incorrectos", true
}

func computeWebSocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID)) // #nosec G401 -- required verbatim by RFC 6455, not used as a signature or password hash.
	return base64.StdEncoding.EncodeToString(sum[:])
}

type clientFrame struct {
	opcode  byte
	payload []byte
}

func readClientFrame(r io.Reader) (clientFrame, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return clientFrame{}, err
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	if !masked {
		return clientFrame{}, errors.New("websocket: se esperaba frame enmascarado del cliente")
	}
	payloadLen := int(header[1] & 0x7f)
	switch payloadLen {
	case 126:
		var ext uint16
		if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
			return clientFrame{}, err
		}
		payloadLen = int(ext)
	case 127:
		var ext uint64
		if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
			return clientFrame{}, err
		}
		if ext > websocketMaxFrame {
			return clientFrame{}, fmt.Errorf("websocket: frame cliente demasiado grande: %d bytes", ext)
		}
		payloadLen = int(ext)
	}
	if payloadLen < 0 || payloadLen > websocketMaxFrame {
		return clientFrame{}, fmt.Errorf("websocket: frame cliente demasiado grande: %d bytes", payloadLen)
	}
	var maskingKey [4]byte
	if _, err := io.ReadFull(r, maskingKey[:]); err != nil {
		return clientFrame{}, err
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return clientFrame{}, err
	}
	for i := range payload {
		payload[i] ^= maskingKey[i%4]
	}
	return clientFrame{opcode: opcode, payload: payload}, nil
}

func writeServerTextFrame(w io.Writer, message string) error {
	return writeServerControlFrame(w, 0x1, []byte(message))
}

func writeServerControlFrame(w io.Writer, opcode byte, payload []byte) error {
	if opcode >= 0x8 && len(payload) > 125 {
		return errors.New("websocket: payload de frame de control demasiado grande")
	}
	if len(payload) > websocketMaxFrame {
		return fmt.Errorf("websocket: payload de salida demasiado grande: %d bytes", len(payload))
	}
	frame := []byte{0x80 | opcode}
	switch {
	case len(payload) <= 125:
		payloadLength, err := websocketPayloadLengthByte(len(payload))
		if err != nil {
			return err
		}
		frame = append(frame, payloadLength)
	case len(payload) <= math.MaxUint16:
		frame = append(frame, 126)
		payloadLength, err := websocketPayloadLengthUint16(len(payload))
		if err != nil {
			return err
		}
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], payloadLength)
		frame = append(frame, ext[:]...)
	default:
		frame = append(frame, 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(len(payload)))
		frame = append(frame, ext[:]...)
	}
	frame = append(frame, payload...)
	_, err := w.Write(frame)
	return err
}

func websocketPayloadLengthByte(length int) (byte, error) {
	if length < 0 || length > 125 {
		return 0, fmt.Errorf("websocket: longitud corta fuera de rango: %d", length)
	}
	return byte(length), nil
}

func websocketPayloadLengthUint16(length int) (uint16, error) {
	if length < 0 || length > math.MaxUint16 {
		return 0, fmt.Errorf("websocket: longitud extendida fuera de rango uint16: %d", length)
	}
	return uint16(length), nil
}

func headerContainsToken(header http.Header, key, token string) bool {
	values := header.Values(key)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func auditarActivacionWebSocket(ctx context.Context, auditor *application.AuditUseCase, cfg config.Config, success bool, err error) {
	if auditor == nil {
		return
	}
	_ = auditor.Registrar(ctx, application.AuditCommand{
		OperationType: "websocket_activacion",
		Origin:        "local_operator",
		DocumentName:  "websocket_local_tls",
		Format:        "wss",
		Success:       success,
		ErrorSummary:  legacyWebSocketAuditError(err),
		DocumentHash:  fmt.Sprintf("websocket_habilitado=%t", cfg.WebsocketHabilitado),
	})
}

func legacyWebSocketAuditError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "activation_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "activation_deadline_exceeded"
	default:
		return "activation_failed"
	}
}
