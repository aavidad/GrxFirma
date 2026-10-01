// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	nativehost "grxfirma/internal/adapters/inbound/desktop/nativehost"
)

func TestEnvBool(t *testing.T) {
	t.Setenv("GRXFIRMA_TEST_BOOL", "sí")
	if !envBool("GRXFIRMA_TEST_BOOL", false) {
		t.Fatal("esperaba true para sí")
	}
	t.Setenv("GRXFIRMA_TEST_BOOL", "off")
	if envBool("GRXFIRMA_TEST_BOOL", true) {
		t.Fatal("esperaba false para off")
	}
	t.Setenv("GRXFIRMA_TEST_BOOL", "valor-no-reconocido")
	if !envBool("GRXFIRMA_TEST_BOOL", true) {
		t.Fatal("esperaba el valor por defecto ante entrada no reconocida")
	}
}

func TestAprobacionSistema_AutoApproveExplicito(t *testing.T) {
	aprobador := &aprobacionSistema{autoApprove: true}
	ok, err := aprobador.Request(context.Background(), "firmar")
	if err != nil {
		t.Fatalf("Request devolvió error: %v", err)
	}
	if !ok {
		t.Fatal("esperaba aprobación cuando autoApprove está activado explícitamente")
	}
}

func TestServeNativeMessage_ElTimeoutEmpiezaTrasLaLectura(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	adaptador := nativehost.New(nil, nil, nil, nil)
	var output bytes.Buffer

	done := make(chan error, 1)
	go func() {
		done <- serveNativeMessage(
			context.Background(),
			adaptador,
			"chrome-extension://"+officialChromiumExtensionID+"/",
			reader,
			&output,
			20*time.Millisecond,
		)
	}()

	// Una extension puede mantener el host vivo sin enviar solicitudes. Esa espera
	// no debe consumir el presupuesto de la operacion siguiente.
	time.Sleep(40 * time.Millisecond)
	if err := nativehost.WriteMessage(writer, []byte(`{"requestId":"idle","action":"ping"}`)); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	_ = writer.Close()
	if err := <-done; err != nil {
		t.Fatalf("serveNativeMessage() error = %v", err)
	}

	payload, err := nativehost.ReadMessage(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if !bytes.Contains(payload, []byte(`"success":true`)) {
		t.Fatalf("respuesta inesperada: %s", payload)
	}
}
