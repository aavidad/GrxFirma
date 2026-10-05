// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

func esperarCancelacion(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("la operación no se canceló al cerrar el portal la conexión")
	}
}

// Regresión 0.0.117: el portal dejaba de esperar y el editor del sello y
// grxfirma-afirmauri seguían abiertos hasta agotar su plazo de 5 minutos.
func TestWatchPeerCloseCancelaSiElPortalCierraLaConexion(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	ctx, stop := watchPeerClose(context.Background(), server, bufio.NewReader(server))
	defer stop()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	esperarCancelacion(t, ctx)
}

func TestWatchPeerCloseCancelaConMarcoDeCierre(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	ctx, stop := watchPeerClose(context.Background(), server, bufio.NewReader(server))
	defer stop()
	go func() { _, _ = client.Write([]byte{0x88, 0x80, 1, 2, 3, 4}) }()
	esperarCancelacion(t, ctx)
}

func TestWatchPeerCloseNoConsumeOtrosMensajes(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	reader := bufio.NewReader(server)
	ctx, stop := watchPeerClose(context.Background(), server, reader)
	go func() { _, _ = client.Write([]byte{0x81}) }()
	time.Sleep(50 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("un mensaje de texto no debe cancelar la operación")
	}
	stop()
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	first, err := reader.ReadByte()
	if err != nil || first != 0x81 {
		t.Fatalf("el bucle principal debe recibir el byte intacto: %x %v", first, err)
	}
}

func TestWatchPeerCloseSeDetieneSinCancelarYPermiteSeguirLeyendo(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	reader := bufio.NewReader(server)
	ctx, stop := watchPeerClose(context.Background(), server, reader)
	time.Sleep(20 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("sin cierre del portal no debe cancelarse la operación")
	}
	stop()
	go func() { _, _ = client.Write([]byte{0x89}) }()
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	first, err := reader.ReadByte()
	if err != nil || first != 0x89 {
		t.Fatalf("tras parar la vigilancia la conexión debe seguir legible: %x %v", first, err)
	}
}
