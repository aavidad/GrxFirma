// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func TestEscucharIPCWindows_IdaYVuelta(t *testing.T) {
	ruta := fmt.Sprintf(`\\.\pipe\grxfirma_ipc_test_%d_%d`, os.Getpid(), time.Now().UnixNano())
	ln, err := escucharIPC(ruta)
	if err != nil {
		t.Fatalf("escucharIPC: %v", err)
	}
	defer ln.Close()

	servidorTerminado := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			servidorTerminado <- err
			return
		}
		defer conn.Close()
		linea, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			servidorTerminado <- err
			return
		}
		if linea != "ping\n" {
			servidorTerminado <- fmt.Errorf("peticion inesperada: %q", linea)
			return
		}
		_, err = io.WriteString(conn, "pong\n")
		servidorTerminado <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, ruta)
	if err != nil {
		t.Fatalf("DialPipeContext: %v", err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "ping\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	respuesta, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if respuesta != "pong\n" {
		t.Fatalf("respuesta=%q, want %q", respuesta, "pong\n")
	}
	if err := <-servidorTerminado; err != nil {
		t.Fatalf("servidor: %v", err)
	}
}

func TestServidorWindows_VinculacionPIDRechazaHijoYAdmiteProcesoEsperado(
	t *testing.T,
) {
	ruta := fmt.Sprintf(
		`\\.\pipe\grxfirma_ipc_pid_%d_%d`,
		os.Getpid(),
		time.Now().UnixNano(),
	)
	srv := New(&Manejador{})
	if err := srv.VincularPIDFrontend(uint32(os.Getpid())); err != nil {
		t.Fatalf("VincularPIDFrontend: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	servidorTerminado := make(chan error, 1)
	go func() {
		servidorTerminado <- srv.Escuchar(ctx, ruta)
	}()
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

	esperarListenerWindows(t, srv)

	clienteHijo := exec.Command(
		os.Args[0],
		"-test.run=^TestServidorWindows_ClienteHijoRechazado$",
	)
	clienteHijo.Env = append(
		os.Environ(),
		"GRXFIRMA_TEST_CHILD_PIPE="+ruta,
	)
	if output, err := clienteHijo.CombinedOutput(); err != nil {
		t.Fatalf(
			"el cliente hijo no observo el rechazo esperado: %v\n%s",
			err,
			output,
		)
	}

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dialCancel()
	conn, err := winio.DialPipeContext(dialCtx, ruta)
	if err != nil {
		t.Fatalf("DialPipeContext proceso esperado: %v", err)
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
	if !response.OK ||
		response.Protocol != desktopIPCProtocolV1 ||
		response.RequestID != request.RequestID {
		t.Fatalf("respuesta del proceso esperado inesperada: %#v", response)
	}
}

func TestServidorWindows_ClienteHijoRechazado(t *testing.T) {
	ruta := os.Getenv("GRXFIRMA_TEST_CHILD_PIPE")
	if ruta == "" {
		t.Skip("helper exclusivo del subproceso de prueba")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, ruta)
	if err != nil {
		t.Fatalf("DialPipeContext helper: %v", err)
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

func esperarListenerWindows(t *testing.T, srv *Servidor) {
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

func TestEscucharIPCWindows_RechazaRutasNoLocales(t *testing.T) {
	for _, ruta := range []string{
		`C:\Temp\grxfirma.sock`,
		`\\servidor\pipe\grxfirma`,
		`\\.\pipe\`,
		`\\.\pipe\subdirectorio\grxfirma`,
		"\\\\.\\pipe\\grxfirma\ninyectado",
		`\\.\pipe\` + strings.Repeat("a", 300),
	} {
		if _, err := escucharIPC(ruta); err == nil {
			t.Fatalf("escucharIPC(%q) debio rechazar la ruta", ruta)
		}
	}
}

func TestDescriptorSeguridadNamedPipe_ProtegeLaDACL(t *testing.T) {
	descriptor, err := descriptorSeguridadNamedPipe()
	if err != nil {
		t.Fatalf("descriptorSeguridadNamedPipe: %v", err)
	}
	if !strings.Contains(descriptor, "D:P") {
		t.Fatal("el descriptor debe proteger la DACL frente a herencia")
	}
	if !strings.Contains(descriptor, "(A;;RC;;;OW)") {
		t.Fatal("OWNER_RIGHTS debe suprimir los permisos implicitos amplios del propietario")
	}

	grupos, err := windows.GetCurrentProcessToken().GetTokenGroups()
	if err != nil {
		t.Fatalf("GetTokenGroups: %v", err)
	}
	var sidLogon string
	for _, grupo := range grupos.AllGroups() {
		if grupo.Attributes&windows.SE_GROUP_LOGON_ID == windows.SE_GROUP_LOGON_ID {
			sidLogon = grupo.Sid.String()
			break
		}
	}
	if sidLogon == "" || !strings.Contains(descriptor, "(A;;0x12019f;;;"+sidLogon+")") {
		t.Fatal("el descriptor debe autorizar explicitamente la sesion de logon actual")
	}
	if strings.Contains(descriptor, ";;;SY)") {
		t.Fatal("el descriptor no debe autorizar procesos SYSTEM")
	}
	if strings.Contains(descriptor, ";;;WD)") || strings.Contains(descriptor, ";;;AU)") ||
		strings.Contains(descriptor, ";;;BA)") {
		t.Fatal("el descriptor no debe autorizar grupos amplios")
	}
}
