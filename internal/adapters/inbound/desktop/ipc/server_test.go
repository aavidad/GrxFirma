// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/observability/correlation"
)

type correlationCapturingCatalog struct {
	calls atomic.Int32
	seen  chan correlation.Correlation
}

func (c *correlationCapturingCatalog) List(ctx context.Context) ([]domain.CertificateRef, error) {
	c.calls.Add(1)
	value, ok := correlation.From(ctx)
	if ok && c.seen != nil {
		c.seen <- value
	}
	return nil, nil
}

func TestEffectiveIPCTraceID_UsaTraceIDExplicito(t *testing.T) {
	t.Parallel()
	got := effectiveIPCTraceID(peticion{
		RequestID: "req-123",
		TraceID:   "trace-abc",
	})
	if got != "trace-abc" {
		t.Fatalf("traceID=%q, want %q", got, "trace-abc")
	}
}

func TestEffectiveIPCTraceID_FallbackARequestID(t *testing.T) {
	t.Parallel()
	got := effectiveIPCTraceID(peticion{
		RequestID: "req-123",
	})
	if got != "req-123" {
		t.Fatalf("traceID=%q, want %q", got, "req-123")
	}
}

func TestSensitiveIPCAction_IncluyeClavesTransitoriasDeProteccion(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"protect", "protect_sign", "unprotect", " PROTECT "} {
		if !isSensitiveIPCAction(action) {
			t.Fatalf("isSensitiveIPCAction(%q) = false; want true", action)
		}
	}
	if !isSensitiveIPCAction("certificate_export_public") {
		t.Fatal("la exportación debe borrar sus parámetros IPC de la memoria")
	}
	if isSensitiveIPCAction("protection_recipients") {
		t.Fatal("el catálogo sin claves no debe marcarse como acción sensible")
	}
}

func TestTimeoutIPC_DistingueAdmisionDeInactividadDelFrontend(t *testing.T) {
	t.Parallel()
	if timeoutPrimeraLectura != 60*time.Second {
		t.Fatalf("timeout inicial = %s; want 60s", timeoutPrimeraLectura)
	}
	if timeoutInactividad < 15*time.Minute {
		t.Fatalf(
			"timeout de inactividad = %s; no cubre una selección humana normal",
			timeoutInactividad,
		)
	}
	if timeoutInactividad <= timeoutPrimeraLectura {
		t.Fatalf(
			"timeout de inactividad = %s; debe superar al inicial %s",
			timeoutInactividad,
			timeoutPrimeraLectura,
		)
	}
}

func TestServidorServirConexion_EcoRequestIDYTraceID(t *testing.T) {
	t.Parallel()

	srv := New(&Manejador{})
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	req := peticion{
		RequestID: "req-123",
		TraceID:   "trace-abc",
		Action:    "ping",
	}
	if err := json.NewEncoder(clientConn).Encode(req); err != nil {
		t.Fatalf("Encode(req): %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp respuesta
	if err := json.NewDecoder(bufio.NewReader(clientConn)).Decode(&resp); err != nil {
		t.Fatalf("Decode(resp): %v", err)
	}

	if !resp.OK || resp.Action != "ping" {
		t.Fatalf("respuesta inesperada: %#v", resp)
	}
	if resp.RequestID != "req-123" {
		t.Fatalf("requestId=%q, want %q", resp.RequestID, "req-123")
	}
	if resp.TraceID != "trace-abc" {
		t.Fatalf("traceId=%q, want %q", resp.TraceID, "trace-abc")
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_FallbackTraceIDARequestID(t *testing.T) {
	t.Parallel()

	srv := New(&Manejador{})
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	req := peticion{
		RequestID: "req-456",
		Action:    "ping",
	}
	if err := json.NewEncoder(clientConn).Encode(req); err != nil {
		t.Fatalf("Encode(req): %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp respuesta
	if err := json.NewDecoder(bufio.NewReader(clientConn)).Decode(&resp); err != nil {
		t.Fatalf("Decode(resp): %v", err)
	}

	if resp.RequestID != "req-456" {
		t.Fatalf("requestId=%q, want %q", resp.RequestID, "req-456")
	}
	if resp.TraceID != "req-456" {
		t.Fatalf("traceId=%q, want fallback %q", resp.TraceID, "req-456")
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_PropagaReferenciaOpacaAntesDelDispatch(t *testing.T) {
	t.Parallel()

	catalog := &correlationCapturingCatalog{
		seen: make(chan correlation.Correlation, 1),
	}
	srv := New(&Manejador{Catalogo: catalog})
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	const (
		requestID = "req-context-123"
		traceID   = "trace-context-456"
	)
	if err := json.NewEncoder(clientConn).Encode(peticion{
		RequestID: requestID,
		TraceID:   traceID,
		Action:    "certificates",
	}); err != nil {
		t.Fatalf("Encode(req): %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp respuesta
	if err := json.NewDecoder(bufio.NewReader(clientConn)).Decode(&resp); err != nil {
		t.Fatalf("Decode(resp): %v", err)
	}
	if !resp.OK {
		t.Fatalf("respuesta inesperada: %#v", resp)
	}

	select {
	case value := <-catalog.seen:
		if value.Reference() == "" {
			t.Fatal("el dispatch recibio una referencia vacia")
		}
		if strings.Contains(value.Reference(), requestID) || strings.Contains(value.Reference(), traceID) {
			t.Fatalf("el dispatch recibio un ID bruto: %q", value.Reference())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("el catalogo no recibio correlacion en su contexto")
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_ClienteLegacyRecibeCorrelacionInterna(t *testing.T) {
	t.Parallel()

	catalog := &correlationCapturingCatalog{
		seen: make(chan correlation.Correlation, 1),
	}
	srv := New(&Manejador{Catalogo: catalog})
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	if _, err := clientConn.Write([]byte("{\"action\":\"certificates\"}\n")); err != nil {
		t.Fatalf("Write(req): %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp respuesta
	if err := json.NewDecoder(bufio.NewReader(clientConn)).Decode(&resp); err != nil {
		t.Fatalf("Decode(resp): %v", err)
	}
	if !resp.OK || resp.RequestID != "" || resp.TraceID != "" {
		t.Fatalf("respuesta legacy inesperada: %#v", resp)
	}

	select {
	case value := <-catalog.seen:
		if !strings.HasPrefix(value.Reference(), "corr-sha256-") {
			t.Fatalf("referencia interna = %q", value.Reference())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("el cliente legacy no genero correlacion interna")
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_RechazaIDsInvalidosAntesDelDispatch(t *testing.T) {
	t.Parallel()

	catalog := &correlationCapturingCatalog{}
	srv := New(&Manejador{Catalogo: catalog})
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	invalidRequests := []map[string]any{
		{"requestId": "", "action": "certificates"},
		{"requestId": nil, "action": "certificates"},
		{"RequestID": "", "action": "certificates"},
		{"requestId": "req\ncontrol", "action": "certificates"},
		{"requestId": "req-valido", "traceId": "trace\tcontrol", "action": "certificates"},
		{"requestId": "traza-ñ", "action": "certificates"},
		{"requestId": strings.Repeat("x", 129), "action": "certificates"},
	}
	reader := bufio.NewReader(clientConn)
	for _, req := range invalidRequests {
		if err := json.NewEncoder(clientConn).Encode(req); err != nil {
			t.Fatalf("Encode(%#v): %v", req, err)
		}

		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var resp respuesta
		if err := json.NewDecoder(reader).Decode(&resp); err != nil {
			t.Fatalf("Decode(resp para %#v): %v", req, err)
		}
		if resp.OK || resp.Action != "" ||
			resp.RequestID != "" || resp.TraceID != "" ||
			resp.Error != "identificadores de correlacion invalidos" {
			t.Fatalf("respuesta invalida para %#v: %#v", req, resp)
		}
		encodedResp, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Marshal(resp): %v", err)
		}
		for _, field := range []string{"requestId", "traceId"} {
			if raw, ok := req[field].(string); ok && raw != "" && strings.Contains(string(encodedResp), raw) {
				t.Fatalf("la respuesta refleja el %s rechazado", field)
			}
		}
	}
	if got := catalog.calls.Load(); got != 0 {
		t.Fatalf("dispatches con IDs invalidos = %d; want 0", got)
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorCerrar_CierraConexionesActivas(t *testing.T) {
	t.Parallel()

	srv := New(&Manejador{})
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	if !srv.registrarConexion(serverConn) {
		t.Fatal("no se pudo registrar la conexion de prueba")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	srv.Cerrar()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("la conexion activa no termino al cerrar el servidor")
	}

	if srv.registrarConexion(clientConn) {
		t.Fatal("un servidor cerrado no debe aceptar conexiones nuevas")
	}
}

func TestServidorOcupado_NoRetieneConexionRechazada(t *testing.T) {
	t.Parallel()

	srv := New(&Manejador{})
	for i := 0; i < maxConexionesSimultaneas; i++ {
		srv.semaforo <- struct{}{}
	}

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	if !srv.registrarConexion(serverConn) {
		t.Fatal("no se pudo registrar la conexion de prueba")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp respuesta
	if err := json.NewDecoder(clientConn).Decode(&resp); err != nil {
		t.Fatalf("Decode(respuesta ocupado): %v", err)
	}
	<-done

	srv.mu.Lock()
	conexiones := len(srv.conexiones)
	srv.mu.Unlock()
	if conexiones != 0 {
		t.Fatalf("conexiones retenidas=%d, want 0", conexiones)
	}
}

func TestServidorEscuchar_NoReabreTrasCerrar(t *testing.T) {
	t.Parallel()

	ruta := filepath.Join(t.TempDir(), "grxfirma-ipc.sock")
	if runtime.GOOS == "windows" {
		ruta = fmt.Sprintf(`\\.\pipe\grxfirma_ipc_closed_%d`, os.Getpid())
	}

	srv := New(&Manejador{})
	srv.Cerrar()
	if err := srv.Escuchar(context.Background(), ruta); err != nil {
		t.Fatalf("Escuchar tras Cerrar: %v", err)
	}
	if srv.listener != nil {
		t.Fatal("un servidor cerrado no debe conservar un listener nuevo")
	}
}

func TestNuevoServidor_InicializaEstadoDeConexiones(t *testing.T) {
	t.Parallel()

	srv := NuevoServidor(
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		"",
	)
	if srv.conexiones == nil {
		t.Fatal("NuevoServidor debe inicializar el registro de conexiones")
	}
	if srv.semaforo == nil {
		t.Fatal("NuevoServidor debe inicializar el limite de conexiones")
	}
}
