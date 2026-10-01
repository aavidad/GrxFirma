// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	restcfg "grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
)

type signMock struct {
	result application.SignResult
	err    error
}

type failingHandshakeWriter struct{}

func (failingHandshakeWriter) Write([]byte) (int, error) {
	return 0, errors.New("fallo sintético de escritura")
}

type failingHandshakeHijacker struct {
	*httptest.ResponseRecorder
	conn net.Conn
}

func (h failingHandshakeHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(failingHandshakeWriter{})), nil
}

type requestHandlerMock struct {
	last afirmauri.Solicitud
	err  error
}

type trustPolicyMock struct {
	decision domain.TrustDecision
	err      error
}

func (m trustPolicyMock) Evaluate(context.Context, string) (domain.TrustDecision, error) {
	return m.decision, m.err
}

func (m trustPolicyMock) Allow(context.Context, string) error  { return m.err }
func (m trustPolicyMock) Deny(context.Context, string) error   { return m.err }
func (m trustPolicyMock) Remove(context.Context, string) error { return m.err }

func TestLegacyWebSocketErrorText_ClaveNoDisponibleEsErrorFirma(t *testing.T) {
	t.Parallel()

	got := legacyWebSocketErrorText(errors.New("clave de firma no disponible para el certificado seleccionado: clave no encontrada"))
	want := compatError("SAF_09", "Error en la operacion de firma")
	if got != want {
		t.Fatalf("legacyWebSocketErrorText() = %q, want %q", got, want)
	}
}

type legacyMessageHandlerMock struct {
	result  Resultado
	handled bool
	err     error
}

type countingLegacyMessageHandlerMock struct {
	result  Resultado
	handled bool
	err     error
	calls   int
}

type routingLegacyMessageHandlerMock struct {
	results map[string]Resultado
	calls   int
}

func (m *requestHandlerMock) HandleRequest(_ context.Context, solicitud afirmauri.Solicitud) error {
	m.last = solicitud
	return m.err
}

func (m legacyMessageHandlerMock) HandleLegacy(_ context.Context, _ string, _ afirmauri.Solicitud) (Resultado, bool, error) {
	return m.result, m.handled, m.err
}

func assertLegacyServicePreparedResponse(t *testing.T, srv *LegacySocketServer, request, sessionID, want string) {
	t.Helper()
	if got := srv.processCommand(context.Background(), request); got != "1" {
		t.Fatalf("respuesta inicial = %q, want número de partes 1", got)
	}
	send := "send=@1@1&idsession=" + sessionID + "@EOF"
	if got := srv.processCommand(context.Background(), send); got != want {
		t.Fatalf("respuesta final mediante send = %q, want %q", got, want)
	}
}

func (m *countingLegacyMessageHandlerMock) HandleLegacy(_ context.Context, _ string, _ afirmauri.Solicitud) (Resultado, bool, error) {
	m.calls++
	return m.result, m.handled, m.err
}

func (m *routingLegacyMessageHandlerMock) HandleLegacy(_ context.Context, raw string, _ afirmauri.Solicitud) (Resultado, bool, error) {
	m.calls++
	result, ok := m.results[raw]
	return result, ok, nil
}

type approvalMock struct {
	ok       bool
	err      error
	messages []string
}

func (m *approvalMock) Request(_ context.Context, message string) (bool, error) {
	m.messages = append(m.messages, message)
	return m.ok, m.err
}

type memoryEvidenceLogger struct {
	items []ports.Evidence
}

func (m *memoryEvidenceLogger) Log(_ context.Context, evidence ports.Evidence) error {
	m.items = append(m.items, evidence)
	return nil
}

func (m signMock) Execute(context.Context, application.SignCommand) (application.SignResult, error) {
	return m.result, m.err
}

func TestHandleTextEcho(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil)
	result, err := adapter.HandleText(context.Background(), "", "echo=test")
	if err != nil {
		t.Fatalf("HandleText() error = %v", err)
	}
	if result.Texto != EchoResponseOK {
		t.Fatalf("Texto = %q, want %q", result.Texto, EchoResponseOK)
	}
}

func TestHandleTextAfirmaDirectSign(t *testing.T) {
	t.Parallel()

	signature := []byte("firma-generada")
	adapter := New(
		afirmauri.New(nil),
		signMock{result: application.SignResult{
			Result: domain.SignatureResult{
				Format:    domain.FormatCAdES,
				Algorithm: "SHA256withRSA",
				Data:      signature,
			},
			CertificateUsed: domain.CertificateRef{ID: "cert-1"},
		}},
	)

	message := "afirma://sign?dat=" + base64.StdEncoding.EncodeToString([]byte("hola")) +
		"&format=CAdES&fileid=123&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	result, err := adapter.HandleText(context.Background(), "", message)
	if err != nil {
		t.Fatalf("HandleText() error = %v", err)
	}
	if result.Tipo != "firma" {
		t.Fatalf("Tipo = %q, want firma", result.Tipo)
	}
	if result.FirmaBase64 != base64.StdEncoding.EncodeToString(signature) {
		t.Fatalf("FirmaBase64 = %q", result.FirmaBase64)
	}
	if FormatearRespuesta(result) != EchoResponseOK {
		t.Fatalf("FormatearRespuesta = %q, want %q", FormatearRespuesta(result), EchoResponseOK)
	}
}

func TestHandleTextAfirmaRetrieveFlow(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil)
	message := "afirma://batch?fileid=123&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	result, err := adapter.HandleText(context.Background(), "", message)
	if err != nil {
		t.Fatalf("HandleText() error = %v", err)
	}
	if !result.RequiereIntercambio {
		t.Fatalf("RequiereIntercambio = false, want true")
	}
	if result.Solicitud == nil || result.Solicitud.RetrieveCommand == nil {
		t.Fatalf("solicitud = %+v, want retrieve command", result.Solicitud)
	}
	if FormatearRespuesta(result) != "PENDING_REMOTE" {
		t.Fatalf("FormatearRespuesta = %q", FormatearRespuesta(result))
	}
}

func TestHandleTextAfirmaRetrieveFlow_WithFullHandlerReturnsOK(t *testing.T) {
	t.Parallel()

	handler := &requestHandlerMock{}
	adapter := New(afirmauri.New(nil), nil, handler)
	message := "afirma://batch?fileid=123&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	result, err := adapter.HandleText(context.Background(), "", message)
	if err != nil {
		t.Fatalf("HandleText() error = %v", err)
	}
	if result.Texto != EchoResponseOK {
		t.Fatalf("Texto = %q, want %q", result.Texto, EchoResponseOK)
	}
	if handler.last.Operacion != afirmauri.OperacionLote {
		t.Fatalf("operacion enviada al handler = %q, want %q", handler.last.Operacion, afirmauri.OperacionLote)
	}
}

func TestHandleTextAfirmaSign_WithLegacyHandlerDevuelveRespuestaObservableV1(t *testing.T) {
	t.Parallel()

	adapter := New(
		afirmauri.New(nil),
		nil,
	).WithLegacyHandler(legacyMessageHandlerMock{
		handled: true,
		result: Resultado{
			Tipo:      "firma",
			Texto:     "cert-b64|firma-b64",
			Operacion: afirmauri.OperacionFirma,
		},
	})

	message := "afirma://sign?dat=QUJD&format=CAdES&fileid=req-legacy&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	result, err := adapter.HandleText(context.Background(), "", message)
	if err != nil {
		t.Fatalf("HandleText() error = %v", err)
	}
	if got := FormatearRespuesta(result); got != "cert-b64|firma-b64" {
		t.Fatalf("FormatearRespuesta = %q, want %q", got, "cert-b64|firma-b64")
	}
	if got := result.Texto; got == EchoResponseOK {
		t.Fatalf("Texto = %q, no debe degradar a %q", got, EchoResponseOK)
	}
}

func TestHandleTextRejectsUnsupportedMessage(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil)
	if _, err := adapter.HandleText(context.Background(), "", "hola"); err == nil {
		t.Fatalf("HandleText() error = nil, want error")
	}
}

func startTestTLSServer(ctx context.Context, cfg restcfg.Config, adapter *Adaptador, certDir string) (*TLSServer, error) {
	return startTLSServerOnPortsWithHooksAndDependencies(
		ctx,
		cfg,
		adapter,
		certDir,
		nil,
		nil,
		tlsServerDependencies{ensureTrusted: func(context.Context, string) error { return nil }},
	)
}

func TestStartTLSServer_DeshabilitadoNoAbreSocket(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := StartTLSServer(ctx, cfg, New(afirmauri.New(nil), nil), t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv != nil {
		t.Fatal("no se esperaba servidor cuando websocket_habilitado=false")
	}
}

func TestStartTLSServer_WebsocketPermitidoFalseNoAbreSocket(t *testing.T) {
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	no := false
	cfg.WebsocketPermitido = &no
	srv, err := startTestTLSServer(context.Background(), cfg, New(afirmauri.New(nil), nil), t.TempDir())
	if err != nil || srv != nil {
		t.Fatalf("permiso false debe impedir listener: srv=%v err=%v", srv, err)
	}
}

func TestSesionTemporal_OrigenSesionYUnaConexionActiva(t *testing.T) {
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hooks := &SessionHooks{ExpectedSessionID: "sesion-esperada"}
	srv, err := startTLSServerOnPortsWithHooksAndDependencies(
		ctx, cfg, New(afirmauri.New(nil), nil), t.TempDir(), nil, hooks,
		tlsServerDependencies{ensureTrusted: func(context.Context, string) error { return nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	connect := func(origin string) (net.Conn, *bufio.Reader, int) {
		t.Helper()
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		req := "GET / HTTP/1.1\r\nHost: " + srv.Addr + "\r\nOrigin: " + origin +
			"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGVzdC1jb25uLWs=\r\nSec-WebSocket-Version: 13\r\n\r\n"
		if _, err := io.WriteString(conn, req); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
		if err != nil {
			conn.Close()
			t.Fatal(err)
		}
		return conn, br, resp.StatusCode
	}
	bad, _, status := connect("https://origen-no-autorizado.example")
	bad.Close()
	if status != http.StatusForbidden {
		t.Fatalf("origen no autorizado: status=%d", status)
	}
	first, firstReader, status := connect("https://127.0.0.1:63118")
	if status != http.StatusSwitchingProtocols {
		first.Close()
		t.Fatalf("primera conexión: status=%d", status)
	}
	second, _, status := connect("https://127.0.0.1:63118")
	second.Close()
	if status != http.StatusConflict {
		first.Close()
		t.Fatalf("conexión simultánea: status=%d", status)
	}
	if err := writeMaskedClientTextFrame(first, "echo=-idsession=sesion-esperada@EOF"); err != nil {
		first.Close()
		t.Fatal(err)
	}
	if got, err := readServerTextFrame(firstReader); err != nil || got != EchoResponseOK {
		first.Close()
		t.Fatalf("echo de sesión: respuesta=%q err=%v", got, err)
	}
	if err := writeMaskedClientTextFrame(first, "echo=-idsession=otra-sesion@EOF"); err != nil {
		first.Close()
		t.Fatal(err)
	}
	if _, err := readServerTextFrame(firstReader); err == nil {
		first.Close()
		t.Fatal("idsession diferente debe cerrar el canal")
	}
	first.Close()
	// El cierre libera la exclusividad para un reintento del mismo portal.
	deadline := time.Now().Add(2 * time.Second)
	for hooks.active.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hooks.active.Load() {
		t.Fatal("la sesión no liberó la conexión cerrada")
	}
	third, _, status := connect("https://127.0.0.1:63118")
	third.Close()
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("reconexión tras cierre: status=%d", status)
	}
}

func TestSesionTemporal_HandshakeFallidoNoConsumeSesion(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	closed := false
	hooks := &SessionHooks{ExpectedSessionID: "sesion-esperada", OnSocketClosed: func() { closed = true }}
	req := httptest.NewRequest(http.MethodGet, "https://127.0.0.1/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Origin", "https://127.0.0.1:63118")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "dGVzdC1jb25uLWs=")
	w := failingHandshakeHijacker{ResponseRecorder: httptest.NewRecorder(), conn: serverConn}
	if err := serveLegacyWebSocket(context.Background(), New(afirmauri.New(nil), nil), hooks, nil, w, req); err == nil {
		t.Fatal("se esperaba error de Flush")
	}
	if hooks.active.Load() || closed {
		t.Fatalf("handshake fallido consumió sesión: active=%t closed=%t", hooks.active.Load(), closed)
	}
}

func TestStartTLSServer_UsaInstaladorConfianzaInyectado(t *testing.T) {
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var trustedPath string
	srv, err := startTLSServerOnPortsWithHooksAndDependencies(
		ctx,
		cfg,
		New(afirmauri.New(nil), nil),
		t.TempDir(),
		nil,
		nil,
		tlsServerDependencies{ensureTrusted: func(_ context.Context, path string) error {
			trustedPath = path
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("startTLSServerOnPortsWithHooksAndDependencies() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}
	if strings.TrimSpace(trustedPath) == "" {
		t.Fatal("no se invocó el instalador de confianza TLS")
	}
}

func TestStartTLSServer_HandshakeYEcho(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := New(afirmauri.New(nil), nil)
	srv, err := startTestTLSServer(ctx, cfg, adapter, t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.Dial() error = %v", err)
	}
	defer conn.Close()

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + srv.Addr + "\r\n" +
		"Origin: https://127.0.0.1:63118\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGVzdC1jb25uLWs=\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("escribiendo handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("leyendo respuesta handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status handshake = %d, want 101", resp.StatusCode)
	}

	if err := writeMaskedClientTextFrame(conn, "echo=test"); err != nil {
		t.Fatalf("escribiendo frame cliente: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err := readServerTextFrame(br)
	if err != nil {
		t.Fatalf("leyendo frame servidor: %v", err)
	}
	if msg != EchoResponseOK {
		t.Fatalf("respuesta websocket = %q, want %q", msg, EchoResponseOK)
	}
}

func TestStartTLSServer_MantieneSesionParaVariosMensajes(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := New(afirmauri.New(nil), nil)
	srv, err := startTestTLSServer(ctx, cfg, adapter, t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.Dial() error = %v", err)
	}
	defer conn.Close()

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + srv.Addr + "\r\n" +
		"Origin: https://127.0.0.1:63118\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGVzdC1jb25uLWs=\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("escribiendo handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("leyendo respuesta handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status handshake = %d, want 101", resp.StatusCode)
	}

	for i := 0; i < 2; i++ {
		if err := writeMaskedClientTextFrame(conn, "echo=test"); err != nil {
			t.Fatalf("escribiendo frame cliente iter=%d: %v", i, err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		msg, err := readServerTextFrame(br)
		if err != nil {
			t.Fatalf("leyendo frame servidor iter=%d: %v", i, err)
		}
		if msg != EchoResponseOK {
			t.Fatalf("respuesta websocket iter=%d = %q, want %q", i, msg, EchoResponseOK)
		}
	}
}

func TestStartTLSServer_ErrorDeProtocoloNoCierraLaSesion(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := New(afirmauri.New(nil), nil)
	srv, err := startTestTLSServer(ctx, cfg, adapter, t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.Dial() error = %v", err)
	}
	defer conn.Close()

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + srv.Addr + "\r\n" +
		"Origin: https://127.0.0.1:63118\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGVzdC1jb25uLWs=\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("escribiendo handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("leyendo respuesta handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status handshake = %d, want 101", resp.StatusCode)
	}

	if err := writeMaskedClientTextFrame(conn, "afirma://operacion-desconocida?x=1"); err != nil {
		t.Fatalf("escribiendo frame cliente con error: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err := readServerTextFrame(br)
	if err != nil {
		t.Fatalf("leyendo frame servidor con error: %v", err)
	}
	if got := msg; got != compatError("SAF_03", "Parametros incorrectos") {
		t.Fatalf("respuesta de error = %q", got)
	}

	if err := writeMaskedClientTextFrame(conn, "echo=test"); err != nil {
		t.Fatalf("escribiendo frame cliente tras error: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err = readServerTextFrame(br)
	if err != nil {
		t.Fatalf("leyendo frame servidor tras error: %v", err)
	}
	if msg != EchoResponseOK {
		t.Fatalf("respuesta websocket tras error = %q, want %q", msg, EchoResponseOK)
	}
}

func TestStartTLSServer_AdmiteFrameClienteExtendido(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := New(afirmauri.New(nil), nil)
	srv, err := startTestTLSServer(ctx, cfg, adapter, t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.Dial() error = %v", err)
	}
	defer conn.Close()

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + srv.Addr + "\r\n" +
		"Origin: https://127.0.0.1:63118\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGVzdC1jb25uLWs=\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("escribiendo handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("leyendo respuesta handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status handshake = %d, want 101", resp.StatusCode)
	}

	msgLargo := "echo=" + string(bytes.Repeat([]byte("a"), 180))
	if err := writeMaskedClientTextFrame(conn, msgLargo); err != nil {
		t.Fatalf("escribiendo frame extendido: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err := readServerTextFrame(br)
	if err != nil {
		t.Fatalf("leyendo frame servidor: %v", err)
	}
	if msg != EchoResponseOK {
		t.Fatalf("respuesta websocket = %q, want %q", msg, EchoResponseOK)
	}
}

func TestStartTLSServer_CancelarContextoCierraSocketHijacked(t *testing.T) {
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := startTestTLSServer(ctx, cfg, New(afirmauri.New(nil), nil), t.TempDir())
	if err != nil {
		t.Fatalf("startTestTLSServer() error = %v", err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.DialWithDialer() error = %v", err)
	}
	defer conn.Close()

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + srv.Addr + "\r\n" +
		"Origin: https://127.0.0.1:63118\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGVzdC1jYW5jZWw=\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("escribiendo handshake: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("leyendo respuesta handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status handshake = %d, want 101", resp.StatusCode)
	}

	cancel()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := br.ReadByte(); err == nil {
		t.Fatal("la conexión hijackeada siguió abierta tras cancelar el contexto")
	}
}

func TestStartTLSServer_ProcesaFrameBufferizadoConHandshake(t *testing.T) {
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := startTestTLSServer(ctx, cfg, New(afirmauri.New(nil), nil), t.TempDir())
	if err != nil {
		t.Fatalf("startTestTLSServer() error = %v", err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.DialWithDialer() error = %v", err)
	}
	defer conn.Close()

	var request bytes.Buffer
	request.WriteString("GET / HTTP/1.1\r\n" +
		"Host: " + srv.Addr + "\r\n" +
		"Origin: https://127.0.0.1:63118\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGVzdC1idWZmZXI=\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n")
	if err := writeMaskedClientTextFrame(&request, "echo=test"); err != nil {
		t.Fatalf("preparando frame cliente: %v", err)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.Copy(conn, &request); err != nil {
		t.Fatalf("escribiendo handshake y frame: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("leyendo respuesta handshake: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status handshake = %d, want 101", resp.StatusCode)
	}
	msg, err := readServerTextFrame(br)
	if err != nil {
		t.Fatalf("leyendo respuesta al frame bufferizado: %v", err)
	}
	if msg != EchoResponseOK {
		t.Fatalf("respuesta websocket = %q, want %q", msg, EchoResponseOK)
	}
}

func TestLegacyWebSocketErrorText_RespetaCodigoSAFExplicito(t *testing.T) {
	t.Parallel()

	got := legacyWebSocketErrorText(errors.New("SAF_20: Error en el proceso local del lote de firma"))
	if got != compatError("SAF_20", "Error en el proceso local del lote de firma") {
		t.Fatalf("legacyWebSocketErrorText(SAF) = %q", got)
	}
}

func TestLegacyWebSocketErrorText_RespetaCodigoERRExplicito(t *testing.T) {
	t.Parallel()

	got := legacyWebSocketErrorText(errors.New("ERR-06:=El identificador para los datos es inválido"))
	if got != compatError("ERR-06", "El identificador para los datos es inválido") {
		t.Fatalf("legacyWebSocketErrorText(ERR) = %q", got)
	}
}

func TestLegacyWebSocketErrorText_MapeaErrorGenericoDeLote(t *testing.T) {
	t.Parallel()

	got := legacyWebSocketErrorText(errors.New("error en el proceso local del lote de firma"))
	if got != compatError("SAF_20", "Error en el proceso local del lote de firma") {
		t.Fatalf("legacyWebSocketErrorText(batch generic) = %q", got)
	}
}

func TestStartTLSServer_StorageAndRetrieveServiceCompat(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := startTestTLSServer(ctx, cfg, New(afirmauri.New(nil), nil), t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
		},
	}

	checkResp, err := postCompatForm(client, "https://"+srv.Addr+"/StorageService", url.Values{
		"op": {"check"},
	})
	if err != nil {
		t.Fatalf("PostForm(check storage): %v", err)
	}
	checkBody, err := io.ReadAll(checkResp.Body)
	_ = checkResp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(check storage): %v", err)
	}
	if string(checkBody) != "OK" {
		t.Fatalf("check storage = %q, want OK", string(checkBody))
	}

	putResp, err := postCompatForm(client, "https://"+srv.Addr+"/afirma-signature-storage/StorageService", url.Values{
		"op":  {"put"},
		"v":   {"4"},
		"id":  {"req-1"},
		"dat": {"QUJDREVGRw=="},
	})
	if err != nil {
		t.Fatalf("PostForm(put storage): %v", err)
	}
	putBody, err := io.ReadAll(putResp.Body)
	_ = putResp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(put storage): %v", err)
	}
	if string(putBody) != "OK" {
		t.Fatalf("put storage = %q, want OK", string(putBody))
	}

	getResp, err := postCompatForm(client, "https://"+srv.Addr+"/afirma-signature-retriever/RetrieveService", url.Values{
		"op": {"get"},
		"v":  {"4"},
		"id": {"req-1"},
	})
	if err != nil {
		t.Fatalf("PostForm(get retrieve): %v", err)
	}
	getBody, err := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(get retrieve): %v", err)
	}
	if string(getBody) != "QUJDREVGRw==" {
		t.Fatalf("retrieve = %q, want %q", string(getBody), "QUJDREVGRw==")
	}

	getResp2, err := postCompatForm(client, "https://"+srv.Addr+"/RetrieveService", url.Values{
		"op": {"get"},
		"v":  {"4"},
		"id": {"req-1"},
	})
	if err != nil {
		t.Fatalf("PostForm(get retrieve second): %v", err)
	}
	getBody2, err := io.ReadAll(getResp2.Body)
	_ = getResp2.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(get retrieve second): %v", err)
	}
	if string(getBody2) != compatError("ERR-06", "El identificador para los datos es inválido") {
		t.Fatalf("retrieve segunda vez = %q", string(getBody2))
	}
}

func postCompatForm(client *http.Client, target string, values url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://127.0.0.1:63118")
	return client.Do(req)
}

func TestWriteServerTextFrame_SoportaPayloadExtendido(t *testing.T) {
	t.Parallel()

	raw := string(bytes.Repeat([]byte("z"), 1024))
	var out bytes.Buffer
	if err := writeServerTextFrame(&out, raw); err != nil {
		t.Fatalf("writeServerTextFrame() error = %v", err)
	}
	got, err := readServerTextFrame(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("readServerTextFrame() error = %v", err)
	}
	if got != raw {
		t.Fatalf("payload reconstruido distinto: len=%d want=%d", len(got), len(raw))
	}
}

func TestLegacySocketServer_ProcessCommandEcho(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-1",
		protocolVersion: 3,
	}
	got := srv.processCommand(context.Background(), "echo=hola&idsession=sid-1@EOF")
	if got != "OK" {
		t.Fatalf("processCommand(echo) = %q, want OK", got)
	}
}

func TestLegacySocketServer_ProcessCommandEchoConGuionInvalidaEstadoPreparado(t *testing.T) {
	t.Parallel()

	adapter := New(
		afirmauri.New(nil),
		signMock{result: application.SignResult{
			Result: domain.SignatureResult{
				Format:    domain.FormatCAdES,
				Algorithm: "SHA256withRSA",
				Data:      []byte("firma-generada"),
			},
			CertificateUsed: domain.CertificateRef{ID: "cert-1"},
		}},
	)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-echo-reset-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	if got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-echo-reset-1@EOF"); got != "1" {
		t.Fatalf("processCommand(cmd) = %q, want 1", got)
	}
	if got := srv.processCommand(context.Background(), "echo=close-connection&idsession=sid-echo-reset-1@EOF"); got != "OK" {
		t.Fatalf("processCommand(echo reset) = %q, want OK", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-echo-reset-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras echo con guion debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdAndSend(t *testing.T) {
	t.Parallel()

	adapter := New(
		afirmauri.New(nil),
		signMock{result: application.SignResult{
			Result: domain.SignatureResult{
				Format:    domain.FormatCAdES,
				Algorithm: "SHA256withRSA",
				Data:      []byte("firma-generada"),
			},
			CertificateUsed: domain.CertificateRef{ID: "cert-1"},
		}},
	)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-2",
		protocolVersion: 3,
	}

	cmdURI := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	total := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-2@EOF")
	if total != "1" {
		t.Fatalf("total partes = %q, want 1", total)
	}
	part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-2@EOF")
	if part != EchoResponseOK {
		t.Fatalf("parte service = %q, want %q", part, EchoResponseOK)
	}
}

func TestLegacySocketServer_ProcessCommandDesconocidoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	adapter := New(
		afirmauri.New(nil),
		signMock{result: application.SignResult{
			Result: domain.SignatureResult{
				Format:    domain.FormatCAdES,
				Algorithm: "SHA256withRSA",
				Data:      []byte("firma-generada"),
			},
			CertificateUsed: domain.CertificateRef{ID: "cert-1"},
		}},
	)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-unknown-reset-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))
	if got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-unknown-reset-1@EOF"); got != "1" {
		t.Fatalf("cmd inicial = %q, want 1", got)
	}

	if got := srv.processCommand(context.Background(), "desconocido=1&idsession=sid-unknown-reset-1@EOF"); got != "SAF_03: Parametros incorrectos" {
		t.Fatalf("comando desconocido = %q, want SAF_03", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-unknown-reset-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras comando desconocido debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdReutilizaRespuestaPreparada(t *testing.T) {
	t.Parallel()

	handler := &countingLegacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	}
	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(handler)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-reuse-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))
	req := "cmd=" + cmdEncoded + "&idsession=sid-reuse-1@EOF"

	first := srv.processCommand(context.Background(), req)
	if first != "1" {
		t.Fatalf("primera respuesta cmd = %q, want 1", first)
	}
	second := srv.processCommand(context.Background(), req)
	if second != "1" {
		t.Fatalf("segunda respuesta cmd = %q, want 1", second)
	}
	if handler.calls != 1 {
		t.Fatalf("calls = %d, want 1", handler.calls)
	}
}

func TestLegacySocketServer_ProcessCommandCmdInvalidoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-cmd-invalid-reset-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))
	if got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-cmd-invalid-reset-1@EOF"); got != "1" {
		t.Fatalf("cmd inicial = %q, want 1", got)
	}

	if got := srv.processCommand(context.Background(), "cmd=%%%&idsession=sid-cmd-invalid-reset-1@EOF"); got != "SAF_03: Comando cmd invalido" {
		t.Fatalf("cmd invalido = %q, want comando cmd invalido", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-cmd-invalid-reset-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras cmd invalido debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdURIInvalidaLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-cmd-invalid-reset-2",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))
	if got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-cmd-invalid-reset-2@EOF"); got != "1" {
		t.Fatalf("cmd inicial = %q, want 1", got)
	}

	badURI := base64.StdEncoding.EncodeToString([]byte("http://example.invalid/no-afirma"))
	if got := srv.processCommand(context.Background(), "cmd="+badURI+"&idsession=sid-cmd-invalid-reset-2@EOF"); got != "SAF_03: Comando cmd invalido" {
		t.Fatalf("cmd uri invalida = %q, want comando cmd invalido", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-cmd-invalid-reset-2@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras cmd uri invalida debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdNuevaPeticionNoReutilizaRespuestaPreparada(t *testing.T) {
	t.Parallel()

	firstURI := ensureProtocolVersionParam("afirma://selectcert?sticky=true", 3)
	secondURI := ensureProtocolVersionParam("afirma://selectcert?sticky=false", 3)
	handler := &routingLegacyMessageHandlerMock{
		results: map[string]Resultado{
			firstURI:  {Tipo: "selectcert", Texto: "CERT_OK"},
			secondURI: {Tipo: "selectcert", Texto: "CERT_ALT"},
		},
	}
	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(handler)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-reuse-new-1",
		protocolVersion: 3,
	}

	firstReq := "cmd=" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=true")) + "&idsession=sid-reuse-new-1@EOF"
	secondReq := "cmd=" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=false")) + "&idsession=sid-reuse-new-1@EOF"

	if got := srv.processCommand(context.Background(), firstReq); got != "1" {
		t.Fatalf("primera respuesta cmd = %q, want 1", got)
	}
	if got := srv.processCommand(context.Background(), secondReq); got != "1" {
		t.Fatalf("segunda respuesta cmd = %q, want 1", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-reuse-new-1@EOF"); got != "CERT_ALT" {
		t.Fatalf("send tras nueva peticion = %q, want CERT_ALT", got)
	}
	if handler.calls != 2 {
		t.Fatalf("calls = %d, want 2", handler.calls)
	}
}

func TestLegacySocketServer_ProcessCommandSendRechazaTotalIncoherente(t *testing.T) {
	t.Parallel()

	adapter := New(
		afirmauri.New(nil),
		signMock{result: application.SignResult{
			Result: domain.SignatureResult{
				Format:    domain.FormatCAdES,
				Algorithm: "SHA256withRSA",
				Data:      []byte("firma-generada"),
			},
			CertificateUsed: domain.CertificateRef{ID: "cert-1"},
		}},
	)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-send-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	total := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-send-1@EOF")
	if total != "1" {
		t.Fatalf("total partes = %q, want 1", total)
	}
	part := srv.processCommand(context.Background(), "send=@1@2&idsession=sid-send-1@EOF")
	if part != "SAF_03: Peticion send invalida" {
		t.Fatalf("send con total incoherente = %q", part)
	}
}

func TestLegacySocketServer_ProcessCommandPrimerFragmentoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-frag-reset-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	fragPayload := base64.StdEncoding.EncodeToString([]byte("afirma://sign?dat=QUJD"))
	got := srv.processCommand(context.Background(), "fragment=@1@2@"+fragPayload+"&idsession=sid-frag-reset-1@EOF")
	if got != "MORE_DATA_NEED" {
		t.Fatalf("primer fragmento = %q, want MORE_DATA_NEED", got)
	}

	part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-frag-reset-1@EOF")
	if part != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras nuevo primer fragmento debe invalidarse, obtenido=%q", part)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.fragments) != 1 || srv.fragments[0] != "afirma://sign?dat=QUJD" {
		t.Fatalf("fragments = %#v", srv.fragments)
	}
}

func TestLegacySocketServer_ProcessCommandPrimerFragmentoInvalidoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-frag-reset-invalid-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
		fragmentsTotal:  1,
		preparedURI:     "afirma://selectcert?sticky=true&v=3",
	}

	got := srv.processCommand(context.Background(), "fragment=@1@2@%%%&idsession=sid-frag-reset-invalid-1@EOF")
	if got != "SAF_03: Fragmento invalido" {
		t.Fatalf("primer fragmento invalido = %q, want fragmento invalido", got)
	}

	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-frag-reset-invalid-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras primer fragmento invalido debe invalidarse, obtenido=%q", got)
	}
	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-frag-reset-invalid-1@EOF"); got != "SAF_03: No hay datos fragmentados" {
		t.Fatalf("firm tras primer fragmento invalido = %q, want no hay datos fragmentados", got)
	}
}

func TestLegacySocketServer_ProcessCommandPrimerFragmentoMalformadoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-frag-reset-invalid-2",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
		fragmentsTotal:  1,
		preparedURI:     "afirma://selectcert?sticky=true&v=3",
	}

	got := srv.processCommand(context.Background(), "fragment=@1@&idsession=sid-frag-reset-invalid-2@EOF")
	if got != "SAF_03: Fragmento invalido" {
		t.Fatalf("primer fragmento malformado = %q, want fragmento invalido", got)
	}

	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-frag-reset-invalid-2@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras primer fragmento malformado debe invalidarse, obtenido=%q", got)
	}
	if got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-frag-reset-invalid-2@EOF"); got != "SAF_03: No hay datos fragmentados" {
		t.Fatalf("firm tras primer fragmento malformado = %q, want no hay datos fragmentados", got)
	}
}

func TestLegacySocketServer_ProcessCommandFragmentMalformadoNoInicialLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-frag-invalid-reset-3",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))
	if got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-frag-invalid-reset-3@EOF"); got != "1" {
		t.Fatalf("cmd inicial = %q, want 1", got)
	}

	got := srv.processCommand(context.Background(), "fragment=@2@&idsession=sid-frag-invalid-reset-3@EOF")
	if got != "SAF_03: Fragmento invalido" {
		t.Fatalf("fragmento malformado no inicial = %q, want fragmento invalido", got)
	}

	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-frag-invalid-reset-3@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras fragmento malformado no inicial debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFragmentRechazaCambioDeTotal(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-frag-total-1",
		protocolVersion: 3,
	}

	frag1 := "fragment=@1@2@" + base64.StdEncoding.EncodeToString([]byte("afirma://sign?dat=QUJD")) + "&idsession=sid-frag-total-1@EOF"
	if got := srv.processCommand(context.Background(), frag1); got != "MORE_DATA_NEED" {
		t.Fatalf("primer fragmento = %q, want MORE_DATA_NEED", got)
	}

	frag2 := "fragment=@2@3@" + base64.StdEncoding.EncodeToString([]byte("&format=CAdES")) + "&idsession=sid-frag-total-1@EOF"
	if got := srv.processCommand(context.Background(), frag2); got != "SAF_03: Fragmento invalido" {
		t.Fatalf("segundo fragmento con total incoherente = %q, want fragmento invalido", got)
	}

	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-frag-total-1@EOF"); got != "SAF_03: No hay datos fragmentados" {
		t.Fatalf("firm tras total incoherente = %q, want no hay datos fragmentados", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmRechazaFragmentosIncompletos(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-firm-incomplete-1",
		protocolVersion: 3,
	}

	frag1 := "fragment=@1@2@" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=true")) + "&idsession=sid-firm-incomplete-1@EOF"
	if got := srv.processCommand(context.Background(), frag1); got != "MORE_DATA_NEED" {
		t.Fatalf("primer fragmento = %q, want MORE_DATA_NEED", got)
	}

	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-firm-incomplete-1@EOF"); got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("firm con fragmentos incompletos = %q, want peticion firm invalida", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmAceptaTramaCanonicaV19YReintento(t *testing.T) {
	t.Parallel()

	const (
		sessionID = "sid-v19-batch-1"
		response  = "RESPUESTA_BATCH_V1"
	)
	uri := ensureProtocolVersionParam(
		"afirma://batch?fileid=req-v19&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload",
		3,
	)
	handler := &routingLegacyMessageHandlerMock{
		results: map[string]Resultado{
			uri: {Tipo: "batch", Texto: response},
		},
	}
	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil).WithLegacyHandler(handler),
		session:         sessionID,
		protocolVersion: 3,
	}

	// Tramas literales generadas por autoscript.js de AutoFirma V1.9:
	// fragment=@i@N@<base64>idsession=<id>@EOF y, al finalizar,
	// firm=idsession=<id>@EOF (sin @i@N).
	split := len(uri) / 2
	for i, chunk := range []string{uri[:split], uri[split:]} {
		part := i + 1
		want := "MORE_DATA_NEED"
		if part == 2 {
			want = "OK"
		}
		frame := fmt.Sprintf(
			"fragment=@%d@2@%sidsession=%s@EOF",
			part,
			base64.URLEncoding.EncodeToString([]byte(chunk)),
			sessionID,
		)
		if got := srv.processCommand(context.Background(), frame); got != want {
			t.Fatalf("fragmento V1.9 %d = %q, want %q", part, got, want)
		}
	}

	firmFrame := "firm=idsession=" + sessionID + "@EOF"
	if got := srv.processCommand(context.Background(), firmFrame); got != "1" {
		t.Fatalf("firm canónico V1.9 = %q, want 1", got)
	}
	// autoscript.js reintenta doFirm() si pierde la respuesta HTTP. La
	// operación no debe ejecutarse dos veces.
	if got := srv.processCommand(context.Background(), firmFrame); got != "1" {
		t.Fatalf("reintento firm canónico V1.9 = %q, want 1", got)
	}
	if handler.calls != 1 {
		t.Fatalf("ejecuciones de batch = %d, want 1", handler.calls)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1idsession="+sessionID+"@EOF"); got != response {
		t.Fatalf("send canónico V1.9 = %q, want %q", got, response)
	}
}

func TestLegacySocketServer_ProcessCommandFragmentFueraDeOrdenDescartaEstadoPrevio(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-frag-order-1",
		protocolVersion: 3,
	}

	frag1 := "fragment=@1@3@" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=true")) + "&idsession=sid-frag-order-1@EOF"
	if got := srv.processCommand(context.Background(), frag1); got != "MORE_DATA_NEED" {
		t.Fatalf("primer fragmento = %q, want MORE_DATA_NEED", got)
	}

	frag3 := "fragment=@3@3@" + base64.StdEncoding.EncodeToString([]byte("&x=1")) + "&idsession=sid-frag-order-1@EOF"
	if got := srv.processCommand(context.Background(), frag3); got != "SAF_03: Fragmento invalido" {
		t.Fatalf("fragmento fuera de orden = %q, want fragmento invalido", got)
	}

	if got := srv.processCommand(context.Background(), "firm=@1@3&idsession=sid-frag-order-1@EOF"); got != "SAF_03: No hay datos fragmentados" {
		t.Fatalf("firm tras fragmento fuera de orden = %q, want no hay datos fragmentados", got)
	}
}

func TestLegacySocketServer_ProcessCommandFragmentDecodeErrorDescartaEstadoPrevio(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-frag-decode-reset-1",
		protocolVersion: 3,
	}

	frag1 := "fragment=@1@2@" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=true")) + "&idsession=sid-frag-decode-reset-1@EOF"
	if got := srv.processCommand(context.Background(), frag1); got != "MORE_DATA_NEED" {
		t.Fatalf("primer fragmento = %q, want MORE_DATA_NEED", got)
	}

	frag2 := "fragment=@2@2@%%%&idsession=sid-frag-decode-reset-1@EOF"
	if got := srv.processCommand(context.Background(), frag2); got != "SAF_03: Fragmento invalido" {
		t.Fatalf("segundo fragmento con decode invalido = %q, want fragmento invalido", got)
	}

	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-frag-decode-reset-1@EOF"); got != "SAF_03: No hay datos fragmentados" {
		t.Fatalf("firm tras decode invalido = %q, want no hay datos fragmentados", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdSelectCertSinServlets(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-3",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	total := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-3@EOF")
	if total != "1" {
		t.Fatalf("total partes = %q, want 1", total)
	}
	part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-3@EOF")
	if part != "CERT_OK" {
		t.Fatalf("parte service = %q, want CERT_OK", part)
	}
}

func TestLegacySocketServer_ProcessCommandCmdPreservaErrorLegacyExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("SAF_20: Error en el proceso local del lote de firma"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-explicit-err-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-explicit-err-1@EOF",
		"sid-explicit-err-1",
		"SAF_20: Error en el proceso local del lote de firma",
	)
}

func TestLegacySocketServer_ProcessCommandCmdMapeaErrorGenericoDeAlmacen(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("no hay certificados disponibles en el almacen de certificados"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-generic-store-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-generic-store-1@EOF",
		"sid-generic-store-1",
		"SAF_08: Error accediendo al almacen de certificados",
	)
}

func TestLegacySocketServer_ProcessCommandCmdMapeaErrorGenericoDeLote(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("error en el proceso local del lote de firma"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-generic-batch-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://batch?fileid=req-batch-1&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-generic-batch-1@EOF",
		"sid-generic-batch-1",
		"SAF_20: Error en el proceso local del lote de firma",
	)
}

func TestLegacySocketServer_ProcessCommandCmdMapeaResultadoGenericoDeLote(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "batch", Texto: "error en el proceso local del lote de firma"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-generic-batch-result-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://batch?fileid=req-batch-1&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-generic-batch-result-1@EOF",
		"sid-generic-batch-result-1",
		"SAF_20: Error en el proceso local del lote de firma",
	)
}

func TestLegacySocketServer_ProcessCommandCmdMapeaErrorGenericoDeLoteRemoto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("error en la comunicación con el servicio de firma de lotes"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-generic-batch-remote-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://batch?fileid=req-batch-remote-1&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-generic-batch-remote-1@EOF",
		"sid-generic-batch-remote-1",
		"SAF_26: Error en la comunicación con el servicio de firma de lotes",
	)
}

func TestLegacySocketServer_ProcessCommandCmdMapeaErrorRetrieveTimeoutComoLoteRemoto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("retrieve timeout"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-generic-batch-remote-comm-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://batch?fileid=req-batch-remote-comm-1&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-generic-batch-remote-comm-1@EOF",
		"sid-generic-batch-remote-comm-1",
		"SAF_26: Error en la comunicación con el servicio de firma de lotes",
	)
}

func TestLegacySocketServer_ProcessCommandFirmMapeaResultadoGenericoDeFirmaPorLotes(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "batch", Texto: "error en la firma por lotes"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-batch-remote-generic-1",
		protocolVersion: 3,
		fragments: []string{
			"afirma://batch?fileid=req-batch-remote-2&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload",
		},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-batch-remote-generic-1@EOF",
		"sid-firm-batch-remote-generic-1",
		"SAF_27: Error en la firma por lotes",
	)
}

func TestLegacySocketServer_ProcessCommandFirmMapeaErrorUploadBrokenPipeComoLoteRemoto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("upload broken pipe"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-batch-remote-comm-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments: []string{
			"afirma://batch?fileid=req-batch-remote-comm-2&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload",
		},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-batch-remote-comm-1@EOF",
		"sid-firm-batch-remote-comm-1",
		"SAF_26: Error en la comunicación con el servicio de firma de lotes",
	)
}

func TestLegacySocketServer_ProcessCommandCmdBatchCancelExplicitoReemplazaSendResidual(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "batch", Texto: "CANCEL"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-batch-cancel-explicit-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://batch?fileid=req-batch-cancel-1&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-batch-cancel-explicit-1@EOF",
		"sid-batch-cancel-explicit-1",
		"CANCEL",
	)
}

func TestLegacySocketServer_ProcessCommandCmdBatchCancelGenericoReemplazaSendResidual(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("el usuario ha cancelado la firma por lotes"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-batch-cancel-generic-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://batch?fileid=req-batch-cancel-2&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-batch-cancel-generic-1@EOF",
		"sid-batch-cancel-generic-1",
		"CANCEL",
	)
}

func TestLegacySocketServer_ProcessCommandCmdPreservaResultadoLegacyExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "batch", Texto: "SAF_20:=Error en el proceso local del lote de firma"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-explicit-result-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-explicit-result-1@EOF",
		"sid-explicit-result-1",
		"SAF_20: Error en el proceso local del lote de firma",
	)
}

func TestLegacySocketServer_ProcessCommandCmdPreservaCancelExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CANCEL"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-explicit-cancel-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-explicit-cancel-1@EOF",
		"sid-explicit-cancel-1",
		"CANCEL",
	)
}

func TestLegacySocketServer_ProcessCommandCmdMapeaCancelGenerico(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("el usuario ha cancelado la selección del certificado"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-generic-cancel-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://selectcert?sticky=true"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-generic-cancel-1@EOF",
		"sid-generic-cancel-1",
		"CANCEL",
	)
}

func TestLegacySocketServer_ProcessCommandCmdSaveDirecto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "save", Texto: "SAVE_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-save-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://save?dat=QUJD&filename=demo.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-save-1@EOF")
	if got != "SAVE_OK" {
		t.Fatalf("processCommand(cmd save) = %q, want SAVE_OK", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdSignAndSaveDirecto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "signandsave", Texto: "SAVE_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-signsave-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://signandsave?dat=QUJD&format=CAdES&filename=demo.csig"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-signsave-1@EOF")
	if got != "SAVE_OK" {
		t.Fatalf("processCommand(cmd signandsave) = %q, want SAVE_OK", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdSaveDirectoPreservaErrorLegacyComoError(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("SAF_05: No se pudo guardar el fichero"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-save-err-2",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://save?dat=QUJD&filename=demo.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-save-err-2@EOF")
	if got != "SAF_05: No se pudo guardar el fichero" {
		t.Fatalf("processCommand(cmd save explicit err) = %q, want SAF_05", got)
	}
	if part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-save-err-2@EOF"); part != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras cmd save explicit err debe invalidarse, obtenido=%q", part)
	}
}

func TestLegacySocketServer_ProcessCommandCmdSaveDirectoMapeaCancelGenerico(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("el usuario ha cancelado el guardado del fichero"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-save-cancel-err-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://save?dat=QUJD&filename=demo.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-save-cancel-err-1@EOF")
	if got != "CANCEL" {
		t.Fatalf("processCommand(cmd save generic cancel) = %q, want CANCEL", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdSaveDirectoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "save", Texto: "SAVE_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-save-stale",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://save?dat=QUJD&filename=demo.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-save-stale@EOF")
	if got != "SAVE_OK" {
		t.Fatalf("processCommand(cmd save) = %q, want SAVE_OK", got)
	}
	part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-save-stale@EOF")
	if part != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras save directo debe invalidarse, obtenido=%q", part)
	}
}

func TestLegacySocketServer_ProcessCommandFirmSaveDirecto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "save", Texto: "CANCEL"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-save-2",
		protocolVersion: 3,
		fragments:       []string{"afirma://save?dat=QUJD&filename=demo.txt"},
	}

	got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-save-2@EOF")
	if got != "CANCEL" {
		t.Fatalf("processCommand(firm save) = %q, want CANCEL", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmSignAndSaveDirecto(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "signandsave", Texto: "CANCEL"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-signsave-2",
		protocolVersion: 3,
		fragments:       []string{"afirma://signandsave?dat=QUJD&format=CAdES&filename=demo.csig"},
	}

	got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-signsave-2@EOF")
	if got != "CANCEL" {
		t.Fatalf("processCommand(firm signandsave) = %q, want CANCEL", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmSignAndSaveDirectoMapeaCancelGenerico(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("el usuario ha cancelado el guardado del fichero firmado"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-signsave-cancel-err-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://signandsave?dat=QUJD&format=CAdES&filename=demo.csig"},
	}

	got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-signsave-cancel-err-1@EOF")
	if got != "CANCEL" {
		t.Fatalf("processCommand(firm signandsave generic cancel) = %q, want CANCEL", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmPreservaResultadoLegacyExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "batch", Texto: "ERR-06:=El identificador para los datos es inválido"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-explicit-result-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-explicit-result-1@EOF",
		"sid-firm-explicit-result-1",
		"ERR-06: El identificador para los datos es inválido",
	)
}

func TestLegacySocketServer_ProcessCommandFirmPreservaCancelExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CANCEL"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-explicit-cancel-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sticky=true"},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-explicit-cancel-1@EOF",
		"sid-firm-explicit-cancel-1",
		"CANCEL",
	)
}

func TestLegacySocketServer_ProcessCommandFirmMapeaCancelGenerico(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("el usuario ha cancelado la selección del certificado"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-generic-cancel-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sticky=true"},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-generic-cancel-1@EOF",
		"sid-firm-generic-cancel-1",
		"CANCEL",
	)
}

func TestLegacySocketServer_ProcessCommandFirmBatchPreservaErrorLegacyExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("SAF_20: Error en el proceso local del lote de firma"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-batch-explicit-err-1",
		protocolVersion: 3,
		fragments: []string{
			"afirma://batch?fileid=req-batch-1&rtservlet=https://firma.example/retrieve&stservlet=https://firma.example/upload",
		},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-batch-explicit-err-1@EOF",
		"sid-firm-batch-explicit-err-1",
		"SAF_20: Error en el proceso local del lote de firma",
	)
}

func TestLegacySocketServer_ProcessCommandFirmMapeaErrorGenericoDeFirma(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		err: errors.New("error en la operacion de firma del documento"),
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-generic-sign-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-generic-sign-1@EOF",
		"sid-firm-generic-sign-1",
		"SAF_09: Error en la operacion de firma",
	)
}

func TestLegacySocketServer_ProcessCommandFirmMapeaResultadoGenericoDeFirma(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "firma", Texto: "error en la operacion de firma del documento"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-generic-sign-result-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://sign?dat=QUJD&format=CAdES&algorithm=SHA256withRSA"},
	}

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"firm=@1@1&idsession=sid-firm-generic-sign-result-1@EOF",
		"sid-firm-generic-sign-result-1",
		"SAF_09: Error en la operacion de firma",
	)
}

func TestLegacySocketServer_ProcessCommandFirmRechazaPeticionMalformada(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-invalid-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sticky=true"},
	}

	got := srv.processCommand(context.Background(), "firm=invalido&idsession=sid-firm-invalid-1@EOF")
	if got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("processCommand(firm malformed) = %q, want peticion firm invalida", got)
	}

	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-invalid-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm malformed debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmRechazaRangoInvalido(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-invalid-2",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sticky=true"},
	}

	got := srv.processCommand(context.Background(), "firm=@2@1&idsession=sid-firm-invalid-2@EOF")
	if got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("processCommand(firm invalid range) = %q, want peticion firm invalida", got)
	}

	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-invalid-2@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm invalid range debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmSinFragmentosLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-empty-reset-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		preparedURI:     "afirma://selectcert?sticky=true&v=3",
	}

	if got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-firm-empty-reset-1@EOF"); got != "SAF_03: No hay datos fragmentados" {
		t.Fatalf("firm sin fragmentos = %q, want no hay datos fragmentados", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-empty-reset-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm sin fragmentos debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmIncompletoLimpiaEstadoPrevio(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-firm-incomplete-reset-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		preparedURI:     "afirma://selectcert?sticky=false&v=3",
		fragments:       []string{"afirma://selectcert?sticky=true"},
		fragmentsTotal:  2,
	}

	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-firm-incomplete-reset-1@EOF"); got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("firm incompleto = %q, want peticion firm invalida", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-incomplete-reset-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm incompleto debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmRechazaTotalIncoherenteConFragmentos(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-firm-total-mismatch-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sticky=true"},
		fragmentsTotal:  1,
	}

	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-firm-total-mismatch-1@EOF"); got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("firm con total incoherente = %q, want peticion firm invalida", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-total-mismatch-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm con total incoherente debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmNoReutilizaPreparadoConTotalIncoherente(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-total-mismatch-2",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sticky=true"},
		fragmentsTotal:  1,
		responseParts:   []string{"CERT_OK"},
		preparedURI:     "afirma://selectcert?sticky=true&v=3",
	}

	if got := srv.processCommand(context.Background(), "firm=@1@2&idsession=sid-firm-total-mismatch-2@EOF"); got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("firm con total incoherente sobre preparado = %q, want peticion firm invalida", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-total-mismatch-2@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm incoherente sobre preparado debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmRechazaIndiceDistintoDeUno(t *testing.T) {
	t.Parallel()

	srv := &LegacySocketServer{
		adapter:         New(afirmauri.New(nil), nil),
		session:         "sid-firm-part-idx-1",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sty", "cky=true"},
		fragmentsTotal:  2,
	}

	if got := srv.processCommand(context.Background(), "firm=@2@2&idsession=sid-firm-part-idx-1@EOF"); got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("firm con partIdx distinto de uno = %q, want peticion firm invalida", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-part-idx-1@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm con partIdx distinto de uno debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmNoReutilizaPreparadoConIndiceDistintoDeUno(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "selectcert", Texto: "CERT_OK"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-part-idx-2",
		protocolVersion: 3,
		fragments:       []string{"afirma://selectcert?sty", "cky=true"},
		fragmentsTotal:  2,
		responseParts:   []string{"CERT_OK"},
		preparedURI:     "afirma://selectcert?sticky=true&v=3",
	}

	if got := srv.processCommand(context.Background(), "firm=@2@2&idsession=sid-firm-part-idx-2@EOF"); got != "SAF_03: Peticion firm invalida" {
		t.Fatalf("firm con partIdx distinto de uno sobre preparado = %q, want peticion firm invalida", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-part-idx-2@EOF"); got != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras firm con partIdx distinto de uno sobre preparado debe invalidarse, obtenido=%q", got)
	}
}

func TestLegacySocketServer_ProcessCommandFirmNuevaPeticionNoReutilizaRespuestaPreparada(t *testing.T) {
	t.Parallel()

	firstURI := ensureProtocolVersionParam("afirma://selectcert?sticky=true", 3)
	secondURI := ensureProtocolVersionParam("afirma://selectcert?sticky=false", 3)
	handler := &routingLegacyMessageHandlerMock{
		results: map[string]Resultado{
			firstURI:  {Tipo: "selectcert", Texto: "CERT_OK"},
			secondURI: {Tipo: "selectcert", Texto: "CERT_ALT"},
		},
	}
	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(handler)
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-firm-new-1",
		protocolVersion: 3,
	}

	firstFragment := "fragment=@1@1@" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=true")) + "&idsession=sid-firm-new-1@EOF"
	secondFragment := "fragment=@1@1@" + base64.StdEncoding.EncodeToString([]byte("afirma://selectcert?sticky=false")) + "&idsession=sid-firm-new-1@EOF"

	if got := srv.processCommand(context.Background(), firstFragment); got != "OK" {
		t.Fatalf("primer fragmento final = %q, want OK", got)
	}
	if got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-firm-new-1@EOF"); got != "1" {
		t.Fatalf("firm inicial = %q, want 1", got)
	}
	if got := srv.processCommand(context.Background(), secondFragment); got != "OK" {
		t.Fatalf("segundo fragmento final = %q, want OK", got)
	}
	if got := srv.processCommand(context.Background(), "firm=@1@1&idsession=sid-firm-new-1@EOF"); got != "1" {
		t.Fatalf("firm nueva peticion = %q, want 1", got)
	}
	if got := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-firm-new-1@EOF"); got != "CERT_ALT" {
		t.Fatalf("send tras nueva peticion firm = %q, want CERT_ALT", got)
	}
	if handler.calls != 2 {
		t.Fatalf("calls = %d, want 2", handler.calls)
	}
}

func TestLegacySocketServer_ProcessCommandCmdSaveDirectoPreservaErrorLegacy(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "save", Texto: "SAF_05:=No se pudo guardar el fichero"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-save-err-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://save?dat=QUJD&filename=demo.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-save-err-1@EOF")
	if got != "SAF_05: No se pudo guardar el fichero" {
		t.Fatalf("processCommand(cmd save error) = %q, want SAF_05", got)
	}
}

func TestLegacySocketServer_ProcessCommandCmdLoadPreparaRespuestaYSendLaEntrega(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "load", Texto: "demo.txt:ZGVtby1jb250ZW5pZG8="},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-load-1",
		protocolVersion: 3,
	}

	cmdURI := "afirma://load?filePath=%2Ftmp%2Fdemo.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-load-1@EOF")
	if got != "1" {
		t.Fatalf("processCommand(cmd load) = %q, want 1", got)
	}
	part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-load-1@EOF")
	if part != "demo.txt:ZGVtby1jb250ZW5pZG8=" {
		t.Fatalf("send tras load = %q, want payload de load", part)
	}
}

func TestLegacySocketServer_ProcessCommandCmdLoadPreservaErrorLegacyExplicito(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "load", Texto: "SAF_25:=No se pudo cargar el fichero"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-load-err-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://load?filePath=%2Ftmp%2Finexistente.txt"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	assertLegacyServicePreparedResponse(
		t,
		srv,
		"cmd="+cmdEncoded+"&idsession=sid-load-err-1@EOF",
		"sid-load-err-1",
		"SAF_25: No se pudo cargar el fichero",
	)
}

func TestLegacySocketServer_ProcessCommandCmdSignAndSaveDirectoPreservaErrorLegacy(t *testing.T) {
	t.Parallel()

	adapter := New(afirmauri.New(nil), nil).WithLegacyHandler(legacyMessageHandlerMock{
		result:  Resultado{Tipo: "signandsave", Texto: "SAF_05:=No se pudo guardar el fichero firmado"},
		handled: true,
	})
	srv := &LegacySocketServer{
		adapter:         adapter,
		session:         "sid-signsave-err-1",
		protocolVersion: 3,
		responseParts:   []string{"respuesta-antigua"},
		fragments:       []string{"fragmento-antiguo"},
	}

	cmdURI := "afirma://signandsave?dat=QUJD&format=CAdES&filename=demo.csig"
	cmdEncoded := base64.StdEncoding.EncodeToString([]byte(cmdURI))

	got := srv.processCommand(context.Background(), "cmd="+cmdEncoded+"&idsession=sid-signsave-err-1@EOF")
	if got != "SAF_05: No se pudo guardar el fichero firmado" {
		t.Fatalf("processCommand(cmd signandsave error) = %q, want SAF_05", got)
	}
	part := srv.processCommand(context.Background(), "send=@1@1&idsession=sid-signsave-err-1@EOF")
	if part != "SAF_03: Peticion send invalida" {
		t.Fatalf("send tras signandsave directo debe invalidarse, obtenido=%q", part)
	}
}

func TestStartTLSServerWithGovernance_CanceladoPorUsuario(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aprobador := &approvalMock{ok: false}
	memlog := &memoryEvidenceLogger{}
	auditor := application.NuevoAuditUseCase(nil, memlog)

	srv, err := startTLSServerWithGovernanceAndDependencies(
		ctx,
		cfg,
		New(afirmauri.New(nil), nil),
		t.TempDir(),
		aprobador,
		auditor,
		tlsServerDependencies{ensureTrusted: func(context.Context, string) error { return nil }},
	)
	if err == nil {
		t.Fatal("se esperaba cancelación de usuario")
	}
	if srv != nil {
		t.Fatal("no se esperaba servidor cuando el usuario cancela")
	}
	if len(memlog.items) != 1 {
		t.Fatalf("evidencias = %d, want 1", len(memlog.items))
	}
	if len(aprobador.messages) != 1 || aprobador.messages[0] != SecurityWarningMessage {
		t.Fatalf("mensaje de advertencia inesperado: %#v", aprobador.messages)
	}
}

func TestStartTLSServerWithGovernance_ApruebaYAudita(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aprobador := &approvalMock{ok: true}
	memlog := &memoryEvidenceLogger{}
	auditor := application.NuevoAuditUseCase(nil, memlog)

	srv, err := startTLSServerWithGovernanceAndDependencies(
		ctx,
		cfg,
		New(afirmauri.New(nil), nil),
		t.TempDir(),
		aprobador,
		auditor,
		tlsServerDependencies{ensureTrusted: func(context.Context, string) error { return nil }},
	)
	if err != nil {
		t.Fatalf("StartTLSServerWithGovernance() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}
	if len(memlog.items) != 1 {
		t.Fatalf("evidencias = %d, want 1", len(memlog.items))
	}
	if len(aprobador.messages) != 1 || aprobador.messages[0] != SecurityWarningMessage {
		t.Fatalf("mensaje de advertencia inesperado: %#v", aprobador.messages)
	}
	var payload map[string]any
	if err := json.Unmarshal(memlog.items[0].Payload, &payload); err != nil {
		t.Fatalf("json.Unmarshal evidencia = %v", err)
	}
	if payload["operacion"] != "websocket_activacion" {
		t.Fatalf("operacion auditada = %v", payload["operacion"])
	}
	if payload["resultado"] != "ok" {
		t.Fatalf("resultado auditado = %v", payload["resultado"])
	}
}

func TestReadLegacySocketPayload_HTTPOptionsPreflight(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan legacySocketReadResult, 1)
	errs := make(chan error, 1)
	go func() {
		res, err := readLegacySocketPayload(server)
		if err != nil {
			errs <- err
			return
		}
		done <- res
	}()

	_, err := client.Write([]byte("OPTIONS /afirma HTTP/1.1\r\nHost: 127.0.0.1\r\nOrigin: https://firma.example\r\nAccess-Control-Request-Method: POST\r\n\r\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case err := <-errs:
		t.Fatalf("readLegacySocketPayload error = %v", err)
	case res := <-done:
		if res.HTTPMethod != "OPTIONS" || res.RequestBody != "" || res.Origin != "https://firma.example" {
			t.Fatalf("resultado preflight inesperado: method=%q origin=%q body=%q", res.HTTPMethod, res.Origin, res.RequestBody)
		}
	case <-time.After(time.Second):
		t.Fatal("readLegacySocketPayload no devolvio preflight HTTP sin @EOF")
	}
}

func TestReadLegacySocketPayload_HTTPPostUsesContentLengthBody(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan legacySocketReadResult, 1)
	errs := make(chan error, 1)
	go func() {
		res, err := readLegacySocketPayload(server)
		if err != nil {
			errs <- err
			return
		}
		done <- res
	}()

	body := "echo=-idsession=sid-http@EOF"
	req := "POST /afirma HTTP/1.1\r\nHost: 127.0.0.1\r\nOrigin: http://127.0.0.1\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\n\r\n" + body
	_, err := client.Write([]byte(req))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case err := <-errs:
		t.Fatalf("readLegacySocketPayload error = %v", err)
	case res := <-done:
		if res.HTTPMethod != "POST" || res.RequestBody != body || res.SessionID != "sid-http" || res.Origin != "http://127.0.0.1" {
			t.Fatalf("resultado POST inesperado: method=%q session=%q origin=%q body=%q", res.HTTPMethod, res.SessionID, res.Origin, res.RequestBody)
		}
	case <-time.After(time.Second):
		t.Fatal("readLegacySocketPayload no devolvio POST al completar Content-Length")
	}
}

func TestLegacyServiceHTTPResponsesNoUsanCORSWildcard(t *testing.T) {
	t.Parallel()

	response := string(buildLegacyHTTPResponse("OK", "https://firma.example"))
	preflight := string(buildLegacyHTTPPreflightResponse("https://firma.example"))
	for name, raw := range map[string]string{"response": response, "preflight": preflight} {
		if strings.Contains(raw, "Access-Control-Allow-Origin: *") {
			t.Fatalf("%s contiene wildcard CORS: %s", name, raw)
		}
		if !strings.Contains(raw, "Access-Control-Allow-Origin: https://firma.example\r\n") {
			t.Fatalf("%s no refleja el origen aprobado: %s", name, raw)
		}
	}
}

func TestSplitLegacyResponseNoCortaUTF8EntrePartes(t *testing.T) {
	t.Parallel()

	exact := splitLegacyResponse(strings.Repeat("A", legacyResponseMaxSize))
	if len(exact) != 1 || len(exact[0]) != legacyResponseMaxSize {
		t.Fatalf("respuesta exacta al límite: partes=%d", len(exact))
	}

	response := strings.Repeat("A", legacyResponseMaxSize-1) + "á" + "fin"
	parts := splitLegacyResponse(response)
	if len(parts) != 2 {
		t.Fatalf("partes = %d, want 2", len(parts))
	}
	for i, part := range parts {
		if !utf8.ValidString(part) {
			t.Fatalf("parte %d no es UTF-8 válido", i)
		}
		if len(part) > legacyResponseMaxSize {
			t.Fatalf("parte %d excede el máximo: %d", i, len(part))
		}
	}
	if got := strings.Join(parts, ""); got != response {
		t.Fatalf("respuesta recompuesta distinta: len=%d, want=%d", len(got), len(response))
	}
}

func TestLegacySocketServerCheckSessionIDEsEstricto(t *testing.T) {
	t.Parallel()

	if err := (&LegacySocketServer{}).checkSessionID(""); err == nil {
		t.Fatal("checkSessionID() acepto servidor sin sesion configurada")
	}
	server := &LegacySocketServer{session: "secreta"}
	if err := server.checkSessionID("otra"); err == nil {
		t.Fatal("checkSessionID() acepto una sesion distinta")
	}
	if err := server.checkSessionID("secreta"); err != nil {
		t.Fatalf("checkSessionID() rechazo la sesion correcta: %v", err)
	}
}

func TestLegacyStorageAplicaCuotaYExpiracion(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	store := newLegacyStorage()
	store.now = func() time.Time { return now }
	if !store.put("uno", "dato") {
		t.Fatal("put() rechazo una entrada valida")
	}
	now = now.Add(legacyStorageTTL + time.Second)
	if _, ok := store.take("uno"); ok {
		t.Fatal("take() devolvio una entrada expirada")
	}
	if store.totalBytes != 0 {
		t.Fatalf("totalBytes tras expirar = %d, want 0", store.totalBytes)
	}

	oversized := strings.Repeat("x", legacyStorageMaxBytes+1)
	if store.put("grande", oversized) {
		t.Fatal("put() acepto una entrada por encima de la cuota")
	}
	if len(store.data) != 0 || store.totalBytes != 0 {
		t.Fatalf("el rechazo altero el storage: entries=%d bytes=%d", len(store.data), store.totalBytes)
	}
}

func writeMaskedClientTextFrame(w io.Writer, msg string) error {
	payload := []byte(msg)
	mask := [4]byte{0x11, 0x22, 0x33, 0x44}
	frame := []byte{0x81}
	switch {
	case len(payload) <= 125:
		frame = append(frame, 0x80|byte(len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, 0x80|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(len(payload)))
		frame = append(frame, ext[:]...)
	default:
		frame = append(frame, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(len(payload)))
		frame = append(frame, ext[:]...)
	}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	_, err := w.Write(frame)
	return err
}

// TestStartTLSServer_RechazaOrigenNoPermitido verifica H-06: el handshake WebSocket
// debe devolver 403 cuando el Origin no está permitido, antes de completar el upgrade.
func TestStartTLSServer_RechazaOrigenNoPermitido(t *testing.T) {
	t.Parallel()

	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := New(afirmauri.New(nil), nil)
	srv, err := startTestTLSServer(ctx, cfg, adapter, t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}

	origenes := []struct {
		name   string
		origin string
	}{
		{"origen_http_externo", "http://malicioso.example.com"},
		{"origen_https_externo", "https://malicioso.example.com"},
		{"origen_archivo", "file:///etc/passwd"},
	}

	for _, tc := range origenes {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			})
			if err != nil {
				t.Fatalf("tls.Dial() error = %v", err)
			}
			defer conn.Close()

			req := "GET / HTTP/1.1\r\n" +
				"Host: " + srv.Addr + "\r\n" +
				"Origin: " + tc.origin + "\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Key: dGVzdC1vcmlnaW4=\r\n" +
				"Sec-WebSocket-Version: 13\r\n\r\n"
			if _, err := io.WriteString(conn, req); err != nil {
				t.Fatalf("escribiendo handshake: %v", err)
			}

			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			br := bufio.NewReader(conn)
			resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
			if err != nil {
				t.Fatalf("leyendo respuesta: %v", err)
			}
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("origen=%q: status = %d, want 403 Forbidden", tc.origin, resp.StatusCode)
			}
		})
	}
}

func readServerTextFrame(r io.Reader) (string, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return "", err
	}
	payloadLen := int(header[1] & 0x7f)
	if payloadLen == 126 {
		var ext uint16
		if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
			return "", err
		}
		payloadLen = int(ext)
	} else if payloadLen == 127 {
		var ext uint64
		if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
			return "", err
		}
		payloadLen = int(ext)
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return "", err
	}
	return string(payload), nil
}

func TestLegacyWebSocketErrorText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil_err", nil, "SAF_03:=Parametros incorrectos"},
		{"empty_msg", errors.New("   "), "SAF_03:=Parametros incorrectos"},
		{"cancel", errors.New("el usuario canceló la operacion"), "CANCEL"},
		{"cancel_uppercase", errors.New("CANCEL"), "CANCEL"},
		{"lote_de_firma", errors.New("error en el lote de firma"), "SAF_20:=Error en el proceso local del lote de firma"},
		{"proceso_local_del_lote", errors.New("proceso local del lote fallido"), "SAF_20:=Error en el proceso local del lote de firma"},
		{"operacion_de_lote", errors.New("operacion de lote abortada"), "SAF_20:=Error en el proceso local del lote de firma"},
		{"no_hay_certificados", errors.New("no hay certificados en el almacen"), "SAF_08:=Error accediendo al almacen de certificados"},
		{"catalogo_certificados", errors.New("catalogo de certificados vacío"), "SAF_08:=Error accediendo al almacen de certificados"},
		{"almacen_certificados", errors.New("almacen de certificados no disponible"), "SAF_08:=Error accediendo al almacen de certificados"},
		{"clave_privada", errors.New("clave privada no encontrada"), "SAF_09:=Error en la operacion de firma"},
		{"clave_de_firma", errors.New("error en clave de firma"), "SAF_09:=Error en la operacion de firma"},
		{"operacion_de_firma", errors.New("operacion de firma fallida"), "SAF_09:=Error en la operacion de firma"},
		{"clave_no_encontrada", errors.New("clave no encontrada en el slot"), "SAF_09:=Error en la operacion de firma"},
		{"certificado_seleccionado", errors.New("el certificado seleccionado no es valido"), "SAF_09:=Error en la operacion de firma"},
		{"aprobacion_al_usuario", errors.New("aprobacion al usuario denegada"), "SAF_09:=Error en la operacion de firma"},
		{"sha1_inseguro", fmt.Errorf("firma local fallida: %w", cryptopolicy.ErrSHA1Disabled), "SAF_09:=Firma bloqueada: el portal solicita SHA-1, un algoritmo obsoleto e inseguro. No se ha generado ninguna firma; la entidad responsable del portal debe actualizarlo a SHA-256 o superior."},
		{"desconocido", errors.New("otro error no clasificado"), "SAF_03:=Parametros incorrectos"},
		{"explicit_saf_coloneq", errors.New("SAF_09:=detalle concreto"), "SAF_09:=detalle concreto"},
		{"explicit_err_coloneq", errors.New("ERR-05:=id invalido"), "ERR-05:=id invalido"},
		{"explicit_saf_colon", errors.New("SAF_08:solo codigo"), "SAF_08:=solo codigo"},
		{"explicit_saf_sin_sep", errors.New("SAF_03"), "SAF_03:=Parametros incorrectos"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := legacyWebSocketErrorText(tc.err)
			if got != tc.want {
				t.Errorf("legacyWebSocketErrorText(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestPuertosCandidatos(t *testing.T) {
	t.Parallel()

	t.Run("nil_usa_rango_defecto", func(t *testing.T) {
		got := puertosCandidatos(nil)
		if len(got) != 10 {
			t.Fatalf("len = %d, want 10", len(got))
		}
		if got[0] != 8080 || got[9] != 8089 {
			t.Fatalf("rango = %v..%v, want 8080..8089", got[0], got[9])
		}
	})

	t.Run("vacio_usa_rango_defecto", func(t *testing.T) {
		got := puertosCandidatos([]int{})
		if len(got) != 10 {
			t.Fatalf("len = %d, want 10", len(got))
		}
	})

	t.Run("puertos_especificos", func(t *testing.T) {
		got := puertosCandidatos([]int{9000, 9001})
		if len(got) != 2 || got[0] != 9000 || got[1] != 9001 {
			t.Fatalf("got %v, want [9000 9001]", got)
		}
	})

	t.Run("elimina_duplicados", func(t *testing.T) {
		got := puertosCandidatos([]int{9000, 9000, 9001, 9000})
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (deduplicado); got %v", len(got), got)
		}
	})

	t.Run("descarta_fuera_de_rango", func(t *testing.T) {
		got := puertosCandidatos([]int{0, -1, 65536, 9000, 99999})
		if len(got) != 1 || got[0] != 9000 {
			t.Fatalf("got %v, want [9000]", got)
		}
	})

	t.Run("todos_invalidos_devuelve_vacio", func(t *testing.T) {
		got := puertosCandidatos([]int{0, -5, 65536})
		if len(got) != 0 {
			t.Fatalf("got %v, want []", got)
		}
	})
}

func TestDetectLegacyOperationForTrace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{"afirma://batch?op=batch", "batch"},
		{"op=batch&dat=...", "batch"},
		{"afirma://save?op=save", "save"},
		{"op=save&v=1.0", "save"},
		{"afirma://load?op=load", "load"},
		{"op=load", "load"},
		{"SignAndSave&op=signandsave", "signandsave"},
		{"op=signandsave", "signandsave"},
		{"selectcert&op=selectcert", "selectcert"},
		{"afirma://sign?op=sign", "sign"},
		{"op=sign&dat=abc", "sign"},
		{"afirma://countersign?op=countersign&format=AUTO", "countersign"},
		{"afirma://cosign?op=cosign", "cosign"},
		{"", ""},
		{"op=unknown_op", ""},
		{"   ", ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.raw+"→"+tc.want, func(t *testing.T) {
			t.Parallel()
			got := detectLegacyOperationForTrace(tc.raw)
			if got != tc.want {
				t.Errorf("detectLegacyOperationForTrace(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsAllowedWebOrigin(t *testing.T) {
	t.Parallel()

	allowed := []string{"*.gob.es", "sede.example.org", "dipgra.es", "https://valide.redsara.es"}

	cases := []struct {
		origin string
		want   bool
	}{
		// Vacío / null → rechazado por defecto.
		{"", false},
		{"null", false},
		{"NULL", false},

		// Loopback: solo la página del propio firmador local.
		{"https://127.0.0.1:63118", true},
		{"https://localhost:63118", true},
		{"http://127.0.0.1:63118", false},
		{"http://localhost", false},
		{"https://localhost", false},
		{"http://127.0.0.1", false},
		{"https://127.0.0.1:8080", false},
		{"http://[::1]", false},

		// Extensiones de navegador: no se aceptan sin autorización explícita.
		{"chrome-extension://abcdef", false},
		{"moz-extension://abcdef", false},

		// Dominios de confianza: exact match
		{"https://dipgra.es", true},
		{"https://sede.example.org", true},
		{"https://valide.redsara.es", true},

		// Dominios de confianza: subdominio de wildcard *.gob.es
		{"https://sede.administracion.gob.es", true},
		{"https://portal.gob.es", true},

		// Dominios de confianza: subdominio de exact-domain
		{"https://sub.dipgra.es", true},

		// Externos no en la lista → rechazados
		{"https://malicioso.example.com", false},
		{"https://evil-dipgra.es.com", false},
		{"https://evil.valide.redsara.es", false},
		{"https://notgob.es", false},

		// HTTP externo → rechazado
		{"http://sede.administracion.gob.es", false},

		// file:// → rechazado
		{"file:///etc/passwd", false},

		// URL malformada → rechazada
		{"://no-scheme", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.origin, func(t *testing.T) {
			t.Parallel()
			got := isAllowedWebOrigin(tc.origin, allowed)
			if got != tc.want {
				t.Errorf("isAllowedWebOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

func TestIsAllowedWebOrigin_PorDefectoRechazaVacioYNull(t *testing.T) {
	// No es paralelo: manipula una variable de entorno de proceso.
	machinepolicy.SetForTest(t, machinepolicy.PermitirOrigenVacio, false)

	allowed := []string{"*.gob.es"}
	cases := []struct {
		origin string
		want   bool
	}{
		// En modo estricto los orígenes ausentes u opacos se rechazan.
		{"", false},
		{"null", false},
		{"NULL", false},

		// El resto de la política no cambia.
		{"https://127.0.0.1:63118", true},
		{"http://localhost", false},
		{"chrome-extension://abcdef", false},
		{"https://sede.administracion.gob.es", true},
		{"https://malicioso.example.com", false},
	}
	for _, tc := range cases {
		if got := isAllowedWebOrigin(tc.origin, allowed); got != tc.want {
			t.Errorf("modo estricto: isAllowedWebOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
		}
		if got := isOwnLocalSignerOrigin(tc.origin); (tc.origin == "" || strings.EqualFold(tc.origin, "null")) && got {
			t.Errorf("modo estricto: isOwnLocalSignerOrigin(%q) debería ser false", tc.origin)
		}
	}
	if isAllowedWebOriginWithPolicy(context.Background(), "", nil, allowed) {
		t.Error("modo estricto: isAllowedWebOriginWithPolicy con Origin vacío debería rechazar")
	}
	if isAllowedWebOriginWithPolicy(context.Background(), "null", nil, allowed) {
		t.Error("modo estricto: isAllowedWebOriginWithPolicy con Origin null debería rechazar")
	}
}

func TestIsAllowedWebOrigin_VariableEntornoNoRelajaLaPolitica(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirOrigenVacio, false)
	t.Setenv(envWSPermitirOrigenVacio, "1")
	for _, origin := range []string{"", "null"} {
		if isAllowedWebOrigin(origin, []string{"*.gob.es"}) {
			t.Errorf("la variable de entorno no debe permitir Origin %q", origin)
		}
	}
}

func TestIsAllowedWebOrigin_AliasEstrictoLegacyNoRelajaLaPolitica(t *testing.T) {
	// GRXFIRMA_WS_STRICT_ORIGIN dejó de ser una vía alternativa para aceptar
	// orígenes vacíos. Solo el opt-in nombrado por
	// la política de máquina permitir_origen_vacio puede relajar el rechazo.
	t.Setenv("GRXFIRMA_WS_STRICT_ORIGIN", "0")
	machinepolicy.SetForTest(t, machinepolicy.PermitirOrigenVacio, false)

	for _, origin := range []string{"", "null", "NULL"} {
		if isAllowedWebOrigin(origin, []string{"*.gob.es"}) {
			t.Errorf("el alias legacy no debe permitir Origin %q", origin)
		}
	}
}

func TestIsAllowedWebOrigin_CompatPermiteVacioYNullSoloConOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirOrigenVacio, true)

	allowed := []string{"*.gob.es"}
	for _, origin := range []string{"", "null", "NULL"} {
		if !isAllowedWebOrigin(origin, allowed) {
			t.Errorf("modo compat: isAllowedWebOrigin(%q) debería permitir", origin)
		}
		if !isAllowedWebOriginWithPolicy(context.Background(), origin, nil, allowed) {
			t.Errorf("modo compat: isAllowedWebOriginWithPolicy(%q) debería permitir", origin)
		}
	}
}

func TestIsAllowedWebOriginWithPolicy_DeniegaOrigenPendiente(t *testing.T) {
	policy := trustPolicyMock{decision: domain.TrustDecision{
		Origin: "https://firma.example",
		Status: domain.TrustPending,
	}}
	if isAllowedWebOriginWithPolicy(context.Background(), "https://firma.example", policy, nil) {
		t.Fatal("un origen pendiente no debe tratarse como autorizado")
	}
}
