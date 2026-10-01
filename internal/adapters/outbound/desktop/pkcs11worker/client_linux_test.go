// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/testsupport/pkcs11sandbox"
)

// A fresh invocation without testing arguments acts only as a synthetic pipe
// peer. No driver, real token, shell or private certificate is used.
func TestMain(m *testing.M) {
	if len(os.Args) == 1 {
		os.Exit(clientFixture())
	}
	os.Exit(m.Run())
}

func clientFixture() int {
	_, payload, err := ReadFrame(os.Stdin, MaxRequestBytes)
	var request Request
	if err != nil || decodeJSON(payload, &request) != nil {
		return 2
	}
	response := Response{Version: ProtocolVersion, ID: request.ID, Code: "ok"}
	mode := strings.TrimSuffix(filepath.Base(request.ModulePath), ".so")
	switch mode {
	case "hang":
		time.Sleep(10 * time.Second)
	case "oversize":
		var header [5]byte
		header[0] = FrameResponse
		binary.BigEndian.PutUint32(header[1:], MaxResponseBytes+1)
		_, _ = os.Stdout.Write(header[:])
		return 0
	case "wrongid":
		response.ID = strings.Repeat("f", 32)
	case "rawerror":
		response.Code = "driver secret must never escape"
	case "error":
		response.Code = "pin_locked"
	case "memoryerror":
		response.Code = "pin_memory_unavailable"
	case "pin", "pinpad", "pinlist", "manypins":
		count := 1
		if mode == "manypins" {
			count = 3
		}
		for i := 0; i < count; i++ {
			challenge := PINChallenge{Version: ProtocolVersion, ID: request.ID, ProtectedAuthenticationPath: mode == "pinpad"}
			if err := writeJSON(os.Stdout, FramePINChallenge, challenge, 256); err != nil {
				return 3
			}
			kind, pin, err := ReadFrame(os.Stdin, MaxPINBytes+1)
			if err != nil || kind != FramePINReply {
				return 4
			}
			want := []byte{1, 'Q', 0, 255, 'A'}
			if mode == "pinpad" {
				want = []byte{1}
			}
			if !bytes.Equal(pin, want) {
				clear(pin)
				return 5
			}
			clear(pin)
		}
		response.Signature = []byte{1, 2, 3}
	case "env":
		for _, name := range []string{"LD_PRELOAD", "GRXFIRMA_TEST_SECRET", "HOME"} {
			if os.Getenv(name) != "" {
				return 6
			}
		}
	}
	if err := writeJSON(os.Stdout, FrameResponse, response, MaxResponseBytes); err != nil {
		return 7
	}
	if mode == "trailing" {
		_, _ = os.Stdout.Write([]byte{1})
	}
	if mode == "crash" {
		return 8
	}
	return 0
}

func fixtureClient(t *testing.T, mode string) Client {
	t.Helper()
	pkcs11sandbox.Require(t)
	return fixtureClientFiles(t, mode)
}

// File-only fixtures retain negative validation coverage without bwrap.
func fixtureClientFiles(t *testing.T, mode string) Client {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	// Go's build/cache directories may inherit a group-writable umask. Copy
	// the synthetic peer into this explicitly private installation fixture.
	original, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	executable = filepath.Join(directory, "test-peer")
	destination, err := os.OpenFile(executable, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(destination, original)
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy: %v; close: %v", copyErr, closeErr)
	}
	module := filepath.Join(directory, mode+".so")
	if err := os.WriteFile(module, []byte("synthetic module marker, not loadable"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{module, executable} {
		if _, err := ValidateModulePath(path); err != nil {
			t.Fatalf("fixture path %q: %v", path, err)
		}
	}
	return Client{Executable: executable, ModulePath: module, Timeout: 3 * time.Second}
}

func clientSignRequest() Request {
	return Request{Operation: "sign", CertificateID: "synthetic", Fingerprint: strings.Repeat("a", 64), Hash: "sha256", Digest: make([]byte, 32)}
}

func TestClientPrivateProcessAndResponse(t *testing.T) {
	t.Setenv("GRXFIRMA_TEST_SECRET", "synthetic value excluded from child")
	for _, mode := range []string{"list", "env", "trailing", "crash", "wrongid", "oversize", "rawerror", "error"} {
		t.Run(mode, func(t *testing.T) {
			client := fixtureClient(t, mode)
			response, err := client.Execute(context.Background(), Request{Operation: "list"})
			if mode == "list" || mode == "env" {
				if err != nil || response.Code != "ok" {
					t.Fatalf("response=%+v error=%v", response, err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid helper result")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("raw helper text exposed")
			}
			if mode == "error" {
				var status *OperationError
				if !errors.As(err, &status) || status.Code != "pin_locked" {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestClientPINOwnershipAndLimits(t *testing.T) {
	for _, mode := range []string{"pin", "pinpad", "pinlist", "manypins"} {
		t.Run(mode, func(t *testing.T) {
			client := fixtureClient(t, mode)
			calls := 0
			var buffers [][]byte
			client.PINSource = func(_ context.Context, pinMode PINMode) ([]byte, error) {
				calls++
				if pinMode.ProtectedAuthenticationPath {
					return nil, nil
				}
				pin := []byte{'Q', 0, 255, 'A'}
				buffers = append(buffers, pin)
				return pin, nil
			}
			request := clientSignRequest()
			if mode == "pinlist" {
				request = Request{Operation: "list"}
			}
			response, err := client.Execute(context.Background(), request)
			if mode == "pin" || mode == "pinpad" {
				if err != nil || len(response.Signature) != 3 || calls != 1 {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			} else if !errors.Is(err, ErrProtocol) {
				t.Fatal(err)
			}
			if (mode == "pinlist" && calls != 0) || (mode == "manypins" && calls != 2) {
				t.Fatal("unwanted PIN prompt")
			}
			for _, pin := range buffers {
				if !bytes.Equal(pin, make([]byte, len(pin))) {
					t.Fatal("PIN not cleared")
				}
			}
		})
	}
}

func TestClientCancellationAndTimeout(t *testing.T) {
	client := fixtureClient(t, "hang")
	client.Timeout = 100 * time.Millisecond
	started := time.Now()
	_, err := client.Execute(context.Background(), Request{Operation: "list"})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout not enforced: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Execute(ctx, Request{Operation: "list"})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestClientRejectsConfigurationOverride(t *testing.T) {
	client := fixtureClientFiles(t, "list")
	_, err := client.Execute(context.Background(), Request{Operation: "list", ModulePath: "/untrusted/from/web.so"})
	if !errors.Is(err, ErrProtocol) {
		t.Fatal(err)
	}
	client.Executable = client.ModulePath // regular but not executable
	_, err = client.Execute(context.Background(), Request{Operation: "list"})
	if !errors.Is(err, ErrHelperUnavailable) {
		t.Fatal(err)
	}
}

func TestRequestPINCancellationDoesNotWaitForPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	pin := []byte("synthetic PIN")
	result := make(chan error, 1)
	go func() {
		_, err := requestPIN(ctx, func(context.Context, PINMode) ([]byte, error) { close(entered); <-release; return pin, nil }, PINMode{})
		result <- err
	}()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	// Do not inspect pin concurrently with the late cleanup under -race.
}

func TestRequestPINPanicIsSanitized(t *testing.T) {
	_, err := requestPIN(context.Background(), func(context.Context, PINMode) ([]byte, error) { panic("secret") }, PINMode{})
	if !errors.Is(err, ErrPINCancelled) {
		t.Fatal(err)
	}
}

func TestClientPINLocalFailurePreservesOnlyFixedCause(t *testing.T) {
	for _, fixed := range []error{ErrPINUnavailable, ErrPINPromptFailed, ErrPINMemoryUnavailable, context.DeadlineExceeded} {
		t.Run(fixed.Error(), func(t *testing.T) {
			client := fixtureClient(t, "pin")
			pin := []byte("synthetic PIN to erase")
			client.PINSource = func(context.Context, PINMode) ([]byte, error) {
				return pin, fmt.Errorf("untrusted secret text: %w", fixed)
			}
			_, err := client.Execute(context.Background(), clientSignRequest())
			if !errors.Is(err, fixed) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error not sanitized: %v", err)
			}
			if !bytes.Equal(pin, make([]byte, len(pin))) {
				t.Fatal("PIN not erased")
			}
		})
	}
}
