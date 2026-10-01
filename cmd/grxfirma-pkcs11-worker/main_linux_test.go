// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && cgo

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11store"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/security/signingpolicy"
)

func TestClassifyErrorDoesNotExposeDriverMessages(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{pkcs11store.ErrPINIncorrect, "pin_incorrect"},
		{pkcs11store.ErrPINLocked, "pin_locked"},
		{pkcs11store.ErrPINExpired, "pin_expired"},
		{pkcs11store.ErrPINUnavailable, "pin_cancelled"},
		{pkcs11store.ErrTokenUnavailable, "device_removed"},
		{pkcs11store.ErrModuloNoDisponible, "driver_unavailable"},
		{pkcs11store.ErrCGONoDisponible, "driver_unavailable"},
		{pkcs11store.ErrMechanismUnsupported, "unsupported_algorithm"},
		{pkcs11store.ErrIdentityNotFound, "key_unavailable"},
		{pkcs11store.ErrAmbiguousIdentity, "key_unavailable"},
		{signingpolicy.ErrCertificateUnsuitable, "unsuitable_certificate"},
		{errors.New("synthetic driver secret"), "driver_failed"},
	} {
		if code := classifyError(fmt.Errorf("sensitive details: %w", tc.err)); code != tc.code {
			t.Fatalf("got %q want %q", code, tc.code)
		}
	}
}

func TestNewBackendRejectsUncontrolledPaths(t *testing.T) {
	for _, path := range []string{"", "relative.so", "/nonexistent/grxfirma-qa-driver.so"} {
		backend, err := newBackend(path, nil)
		if backend != nil || !errors.Is(err, pkcs11store.ErrModuloNoDisponible) {
			t.Fatalf("backend=%T error=%v", backend, err)
		}
	}
}

func TestAuxiliaryExecutablePrivateProtocol(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the worker intentionally rejects privileged execution")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "grxfirma-pkcs11-worker")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", executable, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v: %s", err, output)
	}
	// Not a .so: the strict loader must report unavailable, not an empty
	// successful catalogue. No real token or personally installed module used.
	module := filepath.Join(directory, "synthetic-unloadable.so")
	if err := os.WriteFile(module, []byte("not a native module"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"invalid", "list"} {
		t.Run(operation, func(t *testing.T) {
			command := exec.CommandContext(ctx, executable)
			command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
			input, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
			request := pkcs11worker.Request{Version: 1, ID: strings.Repeat("a", 32), Operation: operation, ModulePath: module}
			encoded, _ := json.Marshal(request)
			if err := pkcs11worker.WriteFrame(input, pkcs11worker.FrameRequest, encoded, pkcs11worker.MaxRequestBytes); err != nil {
				t.Fatal(err)
			}
			_ = input.Close()
			kind, payload, err := pkcs11worker.ReadFrame(output, pkcs11worker.MaxResponseBytes)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var response pkcs11worker.Response
			if err := json.Unmarshal(payload, &response); err != nil {
				t.Fatal(err)
			}
			if kind != pkcs11worker.FrameResponse || response.ID != request.ID {
				t.Fatal("unexpected protocol response")
			}
			want := "invalid_request"
			if operation == "list" {
				want = "driver_unavailable"
			}
			if response.Code != want {
				t.Fatalf("got %q want %q", response.Code, want)
			}
			if err := command.Wait(); err != nil {
				t.Fatalf("exit: %v", err)
			}
			if stderr.Len() != 0 {
				t.Fatal("auxiliary leaked diagnostics")
			}
		})
	}
}
