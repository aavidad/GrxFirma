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
	"sync/atomic"
	"testing"
	"time"

	restcfg "grxfirma/internal/adapters/outbound/common/config"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/application"
)

// A-07 y lote 13a: si el lanzamiento trajo idsession, un mensaje sin él o
// con otro valor cierra el canal y no llega al adaptador.
func TestSesionTemporal_MensajeSinIdsessionCierraElCanal(t *testing.T) {
	for _, mensaje := range []string{
		"afirma://selectcert?op=selectcert",
		"afirma://selectcert?op=selectcert&idsession=s2",
	} {
		manejador := &manejadorCaptura{}
		conn, br := conectarSesionTemporal(t, &SessionHooks{ExpectedSessionID: "s1"}, manejador)
		if err := writeMaskedClientTextFrame(conn, mensaje); err != nil {
			t.Fatal(err)
		}
		if got, err := readServerTextFrame(br); err == nil {
			t.Fatalf("%q debía cerrar el canal; respuesta %q", mensaje, got)
		}
		if manejador.llamadas.Load() != 0 {
			t.Fatalf("%q no debía llegar al adaptador", mensaje)
		}
	}
}

// Lote 13a (retrocompatibilidad): una sede antigua que lanza
// afirma://websocket sin idsession sigue funcionando; el canal se atiende y
// la operación lleva la marca de sesión no identificada para el diálogo.
func TestSesionTemporal_LanzamientoSinIdsessionSeAtiendeConAviso(t *testing.T) {
	manejador := &manejadorCaptura{}
	conn, br := conectarSesionTemporal(t, &SessionHooks{}, manejador)
	if err := writeMaskedClientTextFrame(conn, "afirma://selectcert?op=selectcert"); err != nil {
		t.Fatal(err)
	}
	got, err := readServerTextFrame(br)
	if err != nil {
		t.Fatalf("una sede sin idsession debía recibir respuesta: %v", err)
	}
	if got != "OK" {
		t.Fatalf("respuesta = %q", got)
	}
	if manejador.llamadas.Load() != 1 || !manejador.sinIdentificar.Load() {
		t.Fatalf("la operación debía llegar marcada como sesión no identificada (llamadas=%d)", manejador.llamadas.Load())
	}
}

// Con idsession correcto la operación llega sin la marca de aviso.
func TestSesionTemporal_IdsessionCorrectoSinAviso(t *testing.T) {
	manejador := &manejadorCaptura{}
	conn, br := conectarSesionTemporal(t, &SessionHooks{ExpectedSessionID: "s1"}, manejador)
	if err := writeMaskedClientTextFrame(conn, "afirma://selectcert?op=selectcert&idsession=s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := readServerTextFrame(br); err != nil {
		t.Fatalf("con idsession correcto debía responder: %v", err)
	}
	if manejador.llamadas.Load() != 1 || manejador.sinIdentificar.Load() {
		t.Fatal("con idsession correcto no debía marcarse la sesión como no identificada")
	}
}

type manejadorCaptura struct {
	llamadas       atomic.Int32
	sinIdentificar atomic.Bool
}

func (m *manejadorCaptura) HandleLegacy(ctx context.Context, _ string, solicitud afirmauri.Solicitud) (Resultado, bool, error) {
	m.llamadas.Add(1)
	m.sinIdentificar.Store(application.SesionNoIdentificada(ctx))
	return Resultado{Tipo: "afirmauri", Texto: "OK", Operacion: solicitud.Operacion}, true, nil
}

func conectarSesionTemporal(t *testing.T, hooks *SessionHooks, manejador LegacyMessageHandler) (net.Conn, *bufio.Reader) {
	t.Helper()
	cfg := restcfg.Default()
	cfg.WebsocketHabilitado = true
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv, err := startTLSServerOnPortsWithHooksAndDependencies(
		ctx, cfg, New(afirmauri.New(nil), nil).WithLegacyHandler(manejador), t.TempDir(), nil, hooks,
		tlsServerDependencies{ensureTrusted: func(context.Context, string) error { return nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Addr, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) // #nosec G402 -- servidor de prueba local
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
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
	return conn, br
}
