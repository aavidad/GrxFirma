// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
)

type legacySecurityEvidenceLogger struct {
	items []ports.Evidence
}

func (l *legacySecurityEvidenceLogger) Log(_ context.Context, evidence ports.Evidence) error {
	l.items = append(l.items, evidence)
	return nil
}

func TestAuditarActivacionWebSocketNoPersisteUsuarioRutaNiErrorCrudo(t *testing.T) {
	t.Setenv("USER", "usuario-secreto")
	t.Setenv("USERNAME", "usuario-windows-secreto")

	evidence := &legacySecurityEvidenceLogger{}
	auditor := application.NuevoAuditUseCase(nil, evidence)
	cfg := config.Default()
	cfg.WebsocketHabilitado = true

	auditarActivacionWebSocket(
		context.Background(),
		auditor,
		cfg,
		false,
		errors.New("open /mnt/private/nomina-secreta.pdf: token=TOKEN-SECRETO"),
	)

	if len(evidence.items) != 1 {
		t.Fatalf("evidencias = %d, want 1", len(evidence.items))
	}
	got := string(evidence.items[0].Payload)
	for _, secret := range []string{
		"usuario-secreto",
		"usuario-windows-secreto",
		"/mnt/private",
		"nomina-secreta.pdf",
		"TOKEN-SECRETO",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("la auditoría contiene %q: %s", secret, got)
		}
	}
	for _, safe := range []string{"local_operator", "activation_failed", "websocket_activacion"} {
		if !strings.Contains(got, safe) {
			t.Fatalf("la auditoría no conserva %q: %s", safe, got)
		}
	}
}

func TestSafeLegacyServiceOperationUsaVocabularioCerrado(t *testing.T) {
	for raw, want := range map[string]string{
		"":                       "missing",
		"check":                  "check",
		"PUT":                    "put",
		"get":                    "get",
		"TOKEN-SECRETO/file.pdf": "unsupported",
	} {
		if got := safeLegacyServiceOperation(raw); got != want {
			t.Fatalf("safeLegacyServiceOperation(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestServiceSocketNoPersisteIndicesMalformados(t *testing.T) {
	t.Setenv("GRXFIRMA_ENV", "production")
	t.Setenv("GRXFIRMA_DEBUG", "1")
	logPath := filepath.Join(t.TempDir(), "legacy-service.log")
	t.Setenv("GRXFIRMA_LOG_FILE", logPath)

	srv := &LegacySocketServer{}
	got := srv.processCommand(
		context.Background(),
		"fragment=x@TOKEN-SECRETO/nomina.pdf@2@Y2h1bms=",
	)
	if !strings.HasPrefix(got, "SAF_03") {
		t.Fatalf("processCommand = %q", got)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath): %v", err)
	}
	logText := string(data)
	for _, secret := range []string{"TOKEN-SECRETO", "nomina.pdf", "Y2h1bms="} {
		if strings.Contains(logText, secret) {
			t.Fatalf("el log service contiene %q: %s", secret, logText)
		}
	}
	if !strings.Contains(logText, "service_socket_fragment_invalid") ||
		!strings.Contains(logText, `"reason":"range"`) {
		t.Fatalf("el log no conserva el evento seguro: %s", logText)
	}
}

func TestHTTPServerErrorWriterClasificaSinPersistirTextoLibre(t *testing.T) {
	t.Setenv("GRXFIRMA_ENV", "production")
	t.Setenv("GRXFIRMA_DEBUG", "1")

	var dst bytes.Buffer
	writer := redactedHTTPServerErrorWriter{
		logger: logging.New("legacy/websocket-test", &dst),
	}
	raw := "http: TLS handshake error from 127.0.0.1:54321: " +
		"client sent an HTTP request to an HTTPS server; " +
		"token=TOKEN-SECRETO path=/mnt/private/nomina-secreta.pdf"
	if _, err := writer.Write([]byte(raw)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := dst.String()
	for _, want := range []string{
		`"msg":"websocket_tls_handshake_error"`,
		`"failure_kind":"plaintext_http"`,
		`"error":"transport_failure"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("el diagnóstico no contiene %q: %s", want, got)
		}
	}
	for _, secret := range []string{
		"127.0.0.1",
		"54321",
		"TOKEN-SECRETO",
		"/mnt/private",
		"nomina-secreta.pdf",
		"client sent an HTTP request",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el diagnóstico contiene texto sensible/libre %q: %s", secret, got)
		}
	}
}

func TestClassifyHTTPServerDiagnosticUsaVocabularioCerrado(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantEvent   string
		wantFailure string
	}{
		{
			name:        "version_tls",
			raw:         "http: TLS handshake error from peer: tls: client offered only unsupported versions",
			wantEvent:   "websocket_tls_handshake_error",
			wantFailure: "unsupported_tls_version",
		},
		{
			name:        "certificado_rechazado",
			raw:         "http: TLS handshake error from peer: remote error: tls: bad certificate",
			wantEvent:   "websocket_tls_handshake_error",
			wantFailure: "certificate_rejected",
		},
		{
			name:        "aceptacion_tcp",
			raw:         "http: Accept error: accept tcp: temporary failure",
			wantEvent:   "websocket_tcp_accept_error",
			wantFailure: "accept_failed",
		},
		{
			name:        "error_http_no_clasificado",
			raw:         "http: internal server condition with arbitrary detail",
			wantEvent:   "websocket_http_server_error",
			wantFailure: "server_error",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotEvent, gotFailure := classifyHTTPServerDiagnostic(tt.raw)
			if gotEvent != tt.wantEvent || gotFailure != tt.wantFailure {
				t.Fatalf(
					"classifyHTTPServerDiagnostic() = (%q, %q), want (%q, %q)",
					gotEvent,
					gotFailure,
					tt.wantEvent,
					tt.wantFailure,
				)
			}
		})
	}
}

func TestStartTLSServerRegistraAceptacionTCPYFalloTLSRedactados(t *testing.T) {
	t.Setenv("GRXFIRMA_ENV", "production")
	t.Setenv("GRXFIRMA_DEBUG", "1")
	logPath := filepath.Join(t.TempDir(), "legacy-wss.log")
	t.Setenv("GRXFIRMA_LOG_FILE", logPath)

	cfg := config.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := startTestTLSServer(ctx, cfg, New(nil, nil), t.TempDir())
	if err != nil {
		t.Fatalf("startTestTLSServer() error = %v", err)
	}
	if srv == nil {
		t.Fatal("se esperaba servidor activo")
	}
	defer func() {
		_ = srv.Server.Close()
	}()

	conn, err := net.DialTimeout("tcp", srv.Addr, 3*time.Second)
	if err != nil {
		t.Fatalf("net.DialTimeout() error = %v", err)
	}
	_, writeErr := io.WriteString(
		conn,
		"GET /nomina-secreta.pdf?token=TOKEN-SECRETO HTTP/1.1\r\n"+
			"Host: localhost\r\n\r\n",
	)
	if writeErr != nil {
		_ = conn.Close()
		t.Fatalf("escribiendo HTTP plano en el puerto TLS: %v", writeErr)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.Copy(io.Discard, conn)
	_ = conn.Close()

	deadline := time.Now().Add(3 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(logPath)
		hasTLSFailure := bytes.Contains(data, []byte("websocket_tls_handshake_error"))
		hasDebugAcceptance := bytes.Contains(data, []byte("websocket_tcp_connection_accepted"))
		if err == nil && hasTLSFailure &&
			(!logging.DebugAllowed() || hasDebugAcceptance) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("ReadFile(logPath): %v", err)
	}
	got := string(data)
	wants := []string{
		`"msg":"websocket_tls_handshake_error"`,
		`"failure_kind":"plaintext_http"`,
		`"error":"transport_failure"`,
	}
	if logging.DebugAllowed() {
		wants = append(wants,
			`"msg":"websocket_tcp_connection_accepted"`,
			`"transport":"tcp"`,
		)
	} else if strings.Contains(got, "websocket_tcp_connection_accepted") {
		t.Fatalf("el build production persistió la aceptación TCP en DEBUG: %s", got)
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("el log no contiene %q: %s", want, got)
		}
	}
	for _, secret := range []string{
		"TOKEN-SECRETO",
		"nomina-secreta.pdf",
		srv.Addr,
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("el log contiene %q: %s", secret, got)
		}
	}
}
