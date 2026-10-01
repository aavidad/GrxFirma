// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRetiraConfianzaTLSGestionadaAntesDeProcesarProtocolos(t *testing.T) {
	original := removeManagedLocalTLSTrust
	t.Cleanup(func() {
		removeManagedLocalTLSTrust = original
	})

	calls := 0
	var receivedPath string
	removeManagedLocalTLSTrust = func(_ context.Context, certFile string) error {
		calls++
		receivedPath = certFile
		return nil
	}
	protocolCalls := 0
	var stderr bytes.Buffer
	code := runConDependencias(
		context.Background(),
		[]string{removeManagedLocalTLSTrustFlag},
		&stderr,
		func(context.Context, io.Writer, string) int {
			protocolCalls++
			return 0
		},
	)
	if code != 0 {
		t.Fatalf("runConDependencias() code = %d stderr=%s", code, stderr.String())
	}
	if calls != 1 {
		t.Fatalf("removeManagedLocalTLSTrust calls = %d, want 1", calls)
	}
	if protocolCalls != 0 {
		t.Fatalf("protocolCalls = %d, want 0", protocolCalls)
	}
	wantSuffix := filepath.Join(
		".config",
		"grxfirma",
		"tls",
		"websocket-localhost-root.crt.pem",
	)
	if !strings.HasSuffix(filepath.Clean(receivedPath), wantSuffix) {
		t.Fatalf("ruta CA = %q, want suffix %q", receivedPath, wantSuffix)
	}
}

func TestRunRetiradaSilenciosaNoInvocaAlmacenProtegido(t *testing.T) {
	originalRemove, originalRetire := removeManagedLocalTLSTrust, retireManagedLocalTLSKey
	t.Cleanup(func() { removeManagedLocalTLSTrust, retireManagedLocalTLSKey = originalRemove, originalRetire })
	removeManagedLocalTLSTrust = func(context.Context, string) error {
		t.Fatal("la retirada silenciosa no debe tocar ROOT")
		return nil
	}
	called := false
	retireManagedLocalTLSKey = func(_ context.Context, certFile string) error {
		called = true
		if filepath.Base(certFile) != "websocket-localhost-root.crt.pem" {
			t.Fatalf("ruta CA inesperada: %s", certFile)
		}
		return nil
	}
	var stderr bytes.Buffer
	if code := runConDependencias(context.Background(), []string{removeManagedLocalTLSTrustFlag, noPromptLocalTLSTrustFlag}, &stderr, nil); code != 0 || !called {
		t.Fatalf("retirada silenciosa: code=%d called=%t stderr=%s", code, called, stderr.String())
	}
}

func TestExtraerFlagsServidor(t *testing.T) {
	t.Parallel()

	cfg, resto, err := extraerFlagsServidor([]string{"--server", "--server-modo", "websocket", "afirma://sign?id=1"})
	if err != nil {
		t.Fatalf("extraerFlagsServidor() error = %v", err)
	}
	if !cfg.habilitado {
		t.Fatal("se esperaba modo servidor habilitado")
	}
	if cfg.modo != "websocket" {
		t.Fatalf("modo = %q, want websocket", cfg.modo)
	}
	if len(resto) != 1 || resto[0] != "afirma://sign?id=1" {
		t.Fatalf("resto = %v, want [afirma://sign?id=1]", resto)
	}
}

func TestRun_URIProtocoloConArgumentosAdicionalesSeRechaza(t *testing.T) {
	var stderr bytes.Buffer
	llamado := false
	code := runConDependencias(context.Background(), []string{"--server", "afirma://sign?id=1"}, &stderr,
		func(context.Context, io.Writer, string) int { llamado = true; return 0 })
	if code == 0 || llamado {
		t.Fatalf("una URI afirma:// con flags adicionales no debe procesarse (code=%d, llamado=%v)", code, llamado)
	}
	if !invocacionProtocolo([]string{"AFIRMA://x"}) || invocacionProtocolo([]string{"--version"}) {
		t.Fatal("detección de invocación de protocolo incorrecta")
	}
}
