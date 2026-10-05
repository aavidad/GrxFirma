// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"testing"
	"time"

	legacyws "grxfirma/internal/adapters/inbound/legacy/websocket"
)

func TestPortalCompletionDeliveredClosesServiceAfterGrace(t *testing.T) {
	now := time.Unix(100, 0)
	completion := newPortalCompletion(func() time.Time { return now })
	completion.delivered("signature")
	var displayed []string
	completion.onDelivered = func(kind string) { displayed = append(displayed, kind) }
	completion.display()
	completion.display()
	if len(displayed) != 1 || displayed[0] != "signature" {
		t.Fatalf("la entrega debe ocultarse de inmediato una sola vez: %v", displayed)
	}
	for remaining := 5; remaining > 0; remaining-- {
		kind, seconds, closeNow, failed := completion.snapshot()
		if kind != "signature" || seconds != remaining || closeNow || failed {
			t.Fatalf("plazo interno en %d s: %q, %d, %t, %t", remaining, kind, seconds, closeNow, failed)
		}
		completion.display()
		now = now.Add(time.Second)
	}
	if len(displayed) != 1 {
		t.Fatalf("la espera repitió el aviso de entrega: %v", displayed)
	}
	closed := false
	if !completion.expire(func() { closed = true }) || !closed {
		t.Fatal("la entrega no cerró el servicio al vencer los cinco segundos")
	}
}

func TestPortalCompletionNewRequestCancelsAndRestarts(t *testing.T) {
	now := time.Unix(100, 0)
	completion := newPortalCompletion(func() time.Time { return now })
	var displayed []string
	completion.onDelivered = func(kind string) { displayed = append(displayed, kind) }
	completion.delivered("signature")
	completion.display()
	now = now.Add(3 * time.Second)
	completion.received()
	completion.display()
	now = now.Add(3 * time.Second)
	if completion.expire(nil) {
		t.Fatal("la cuenta anterior cerró durante una nueva operación")
	}
	completion.delivered("certificate")
	completion.display()
	kind, seconds, _, _ := completion.snapshot()
	if kind != "certificate" || seconds != 5 {
		t.Fatalf("nueva cuenta: %q, %d s", kind, seconds)
	}
	if len(displayed) != 2 || displayed[0] != "signature" || displayed[1] != "certificate" {
		t.Fatalf("avisos de entrega incorrectos: %v", displayed)
	}
	now = now.Add(5 * time.Second)
	if !completion.expire(nil) {
		t.Fatal("la segunda entrega no cerró al vencer")
	}
}

// Tras un fallo o una cancelación el aviso queda a la vista y se cierra solo
// al vencer su plazo. En 0.0.118 seguía abierto hasta pulsar «Cerrar».
func TestPortalCompletionFailureClosesAfterNotice(t *testing.T) {
	now := time.Unix(100, 0)
	completion := newPortalCompletion(func() time.Time { return now })
	var displayed []string
	completion.onDelivered = func(kind string) { displayed = append(displayed, kind) }
	completion.delivered("signature")
	completion.failure()
	completion.display()
	if len(displayed) != 0 {
		t.Fatalf("el fallo se anunció como entrega: %v", displayed)
	}
	now = now.Add(portalFailureCloseDelay - time.Second)
	if completion.expire(nil) {
		t.Fatal("el aviso de fallo se cerró antes de poder leerlo")
	}
	kind, seconds, closeNow, failed := completion.snapshot()
	if kind != "" || seconds != 1 || closeNow || !failed {
		t.Fatalf("estado del aviso: %q, %d, %t, %t", kind, seconds, closeNow, failed)
	}
	completion.display()
	if len(displayed) != 0 {
		t.Fatalf("el aviso de fallo se ocultó: %v", displayed)
	}
	now = now.Add(time.Second)
	closed := false
	if !completion.expire(func() { closed = true }) || !closed {
		t.Fatal("el aviso de fallo no se cerró al vencer el plazo")
	}
}

func TestPortalCompletionNewRequestStopsFailureNotice(t *testing.T) {
	now := time.Unix(100, 0)
	completion := newPortalCompletion(func() time.Time { return now })
	completion.failure()
	now = now.Add(3 * time.Second)
	completion.received()
	now = now.Add(portalFailureCloseDelay)
	if completion.expire(nil) {
		t.Fatal("el plazo del fallo anterior cerró una operación nueva")
	}
	if _, _, _, failed := completion.snapshot(); failed {
		t.Fatal("la operación nueva heredó el fallo anterior")
	}
}

func TestPortalCancelledResult(t *testing.T) {
	for _, value := range []string{"CANCEL", " cancel ", "Cancel"} {
		if !portalCancelledResult(legacyws.Resultado{Tipo: "firma", Texto: value}) {
			t.Errorf("%q no se reconoció como cancelación", value)
		}
	}
	for _, value := range []string{"", "SAF_01: error", "ERR-01", "BASE64", "CANCELADO"} {
		if portalCancelledResult(legacyws.Resultado{Tipo: "firma", Texto: value}) {
			t.Errorf("%q se tomó por cancelación", value)
		}
	}
}

func TestPortalCompletionDeliverySerializedWithNewRequest(t *testing.T) {
	completion := newPortalCompletion(time.Now)
	completion.delivered("signature")
	entered := make(chan struct{})
	release := make(chan struct{})
	displayDone := make(chan struct{})
	completion.onDelivered = func(string) {
		close(entered)
		<-release
	}
	go func() {
		completion.display()
		close(displayDone)
	}()
	<-entered
	receiveStarted := make(chan struct{})
	receiveDone := make(chan struct{})
	go func() {
		close(receiveStarted)
		completion.received()
		close(receiveDone)
	}()
	<-receiveStarted
	select {
	case <-receiveDone:
		close(release)
		t.Fatal("la nueva operación se procesó antes de terminar el aviso anterior")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-displayDone
	<-receiveDone
	if kind, _, _, _ := completion.snapshot(); kind != "" {
		t.Fatalf("la nueva operación no limpió la entrega anterior: %q", kind)
	}
}

func TestSuccessfulPortalResultRejectsCancellationAndErrors(t *testing.T) {
	for _, value := range []string{"CANCEL", "SAF_01: error", "ERR-01", "error al firmar", ""} {
		if successfulPortalResult(legacyws.Resultado{Tipo: "firma", Texto: value}, "firma") {
			t.Errorf("resultado %q considerado entrega correcta", value)
		}
	}
	if !successfulPortalResult(legacyws.Resultado{Tipo: "firma", Texto: "BASE64"}, "firma") {
		t.Fatal("la respuesta válida no se reconoció")
	}
}
