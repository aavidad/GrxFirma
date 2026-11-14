// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux || darwin

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// D-03: en Linux y macOS el servidor vinculado a un PID rechaza a otro proceso
// del mismo usuario (un hijo del test) y atiende al proceso esperado. Es la
// misma prueba que TestServidorWindows_VinculacionPIDRechazaHijoYAdmiteProcesoEsperado.
func TestServidorUnix_VinculacionPIDRechazaHijoYAdmiteProcesoEsperado(t *testing.T) {
	if !SoportaVinculacionPIDFrontend() {
		t.Fatal("Linux y macOS deben obtener del núcleo el PID del peer")
	}
	// Ruta corta: macOS limita sun_path a 104 bytes.
	dir, err := os.MkdirTemp("", "gx")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)
	ruta := filepath.Join(dir, fmt.Sprintf("p%d.sock", os.Getpid()))

	srv := New(&Manejador{})
	if err := srv.VincularPIDFrontend(uint32(os.Getpid())); err != nil {
		t.Fatalf("VincularPIDFrontend: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	servidorTerminado := make(chan error, 1)
	go func() { servidorTerminado <- srv.Escuchar(ctx, ruta) }()
	defer func() {
		cancel()
		srv.Cerrar()
		select {
		case err := <-servidorTerminado:
			if err != nil {
				t.Errorf("Servidor.Escuchar: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("el servidor IPC no termino al cancelar")
		}
	}()
	esperarListenerUnix(t, srv)

	clienteHijo := exec.Command(os.Args[0], "-test.run=^TestServidorUnix_ClienteHijoRechazado$")
	clienteHijo.Env = append(os.Environ(), "GRXFIRMA_TEST_CHILD_SOCKET="+ruta)
	if output, err := clienteHijo.CombinedOutput(); err != nil {
		t.Fatalf("el cliente hijo no observo el rechazo esperado: %v\n%s", err, output)
	}

	conn, err := net.DialTimeout("unix", ruta, 3*time.Second)
	if err != nil {
		t.Fatalf("Dial proceso esperado: %v", err)
	}
	defer conn.Close()
	request := peticion{
		Protocol:  desktopIPCProtocolV1,
		RequestID: "req-parent",
		TraceID:   "trace-parent",
		Action:    "hello",
		Params:    json.RawMessage(`{}`),
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		t.Fatalf("Encode proceso esperado: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var response respuesta
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("Decode proceso esperado: %v", err)
	}
	if !response.OK || response.RequestID != request.RequestID {
		t.Fatalf("respuesta del proceso esperado inesperada: %#v", response)
	}
}

func TestServidorUnix_ClienteHijoRechazado(t *testing.T) {
	ruta := os.Getenv("GRXFIRMA_TEST_CHILD_SOCKET")
	if ruta == "" {
		t.Skip("helper exclusivo del subproceso de prueba")
	}
	conn, err := net.DialTimeout("unix", ruta, 3*time.Second)
	if err != nil {
		t.Fatalf("Dial helper: %v", err)
	}
	defer conn.Close()
	if _, err := io.WriteString(
		conn,
		`{"protocol":"desktop-ipc-v1","requestId":"req-child","traceId":"trace-child","action":"hello","params":{}}`+"\n",
	); err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := bufio.NewReader(conn).ReadByte(); err == nil {
		t.Fatal("un proceso hijo con PID distinto recibio respuesta del servidor")
	}
}

func esperarListenerUnix(t *testing.T, srv *Servidor) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		listo := srv.listener != nil
		srv.mu.Unlock()
		if listo {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("el listener IPC no quedo listo a tiempo")
}
