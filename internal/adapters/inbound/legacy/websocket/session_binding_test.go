// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	restcfg "grxfirma/internal/adapters/outbound/common/config"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
)

// A-07: con sesión de lanzamiento, un mensaje sin idsession cierra el canal
// y no llega al adaptador.
func TestSesionTemporal_MensajeSinIdsessionCierraElCanal(t *testing.T) {
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hooks := &SessionHooks{ExpectedSessionID: "s1"}
	srv, err := startTLSServerOnPortsWithHooksAndDependencies(
		ctx, cfg, New(afirmauri.New(nil), nil), t.TempDir(), nil, hooks,
		tlsServerDependencies{ensureTrusted: func(context.Context, string) error { return nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) // #nosec G402 -- servidor de prueba local
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	req := "GET / HTTP/1.1\r\nHost: " + srv.Addr + "\r\nOrigin: https://127.0.0.1:63118" +
		"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGVzdC1jb25uLWs=\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake: %v %v", resp, err)
	}
	if err := writeMaskedClientTextFrame(conn, "afirma://selectcert?op=selectcert"); err != nil {
		t.Fatal(err)
	}
	if got, err := readServerTextFrame(br); err == nil {
		t.Fatalf("un mensaje sin idsession debía cerrar el canal; respuesta %q", got)
	}
}
