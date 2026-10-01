// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type cancellationResult[C, R any] struct {
	cancel context.CancelFunc
	result R
	calls  int
}

func (u *cancellationResult[C, R]) Execute(context.Context, C) (R, error) {
	u.calls++
	u.cancel()
	return u.result, nil
}

func TestHandlerCancelledNativeSuccessNeverCommits(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"sign", "sign_multicosign", "protect", "protect_sign", "unprotect"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "input.pdf"), filepath.Join(dir, "output.bin")
			if err := os.WriteFile(input, []byte("%PDF-synthetic"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(output, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m := &Manejador{}
			m.setUltimosCerts([]domain.CertificateRef{{ID: "synthetic-id-no-key"}})
			var calls *int
			signResult := application.SignResult{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("replacement")}}
			doc, err := domain.NewDocument("protected.afp", []byte("replacement"), "application/octet-stream")
			if err != nil {
				t.Fatal(err)
			}
			protected := domain.ProtectedPayload{Document: doc, Profile: domain.ProtectionProfileCompat}
			var params any = paramsFirma{InputPath: input, OutputPath: output, Format: "pades", Action: "sign"}
			switch action {
			case "sign":
				u := &cancellationResult[application.SignCommand, application.SignResult]{cancel: cancel, result: signResult}
				m.Firmar, calls = u, &u.calls
			case "sign_multicosign":
				u := &cancellationResult[application.MultiCoSignCommand, application.SignResult]{cancel: cancel, result: signResult}
				m.MultiCofirmar, calls = u, &u.calls
			case "protect":
				u := &cancellationResult[application.ProtectCommand, application.ProtectResult]{cancel: cancel, result: application.ProtectResult{Protected: protected}}
				m.Proteger, calls = u, &u.calls
			case "protect_sign":
				u := &cancellationResult[application.ProtectAndSignCommand, application.ProtectAndSignResult]{cancel: cancel, result: application.ProtectAndSignResult{Protected: protected}}
				m.ProtegerFirmando, calls = u, &u.calls
			case "unprotect":
				u := &cancellationResult[application.UnprotectCommand, application.UnprotectResult]{cancel: cancel, result: application.UnprotectResult{Unprotected: domain.UnprotectedPayload{Document: doc}}}
				m.Desproteger, calls = u, &u.calls
			}
			if action == "protect" || action == "protect_sign" || action == "unprotect" {
				params = paramsProtection{InputPath: input, OutputPath: output, CertificateID: "synthetic-id-no-key", Profile: "compat", SaveToDisk: true, Overwrite: "force"}
			}
			resp := m.despachar(ctx, peticionJSON(t, action, params))
			if *calls != 1 {
				t.Fatalf("native double was not called: %s", resp.Error)
			}
			if resp.OK || resp.Error != context.Canceled.Error() {
				t.Fatalf("cancelled success: %#v", resp)
			}
			data, err := os.ReadFile(output)
			if err != nil || string(data) != "original" {
				t.Fatalf("destination changed: %q, %v", data, err)
			}
		})
	}
}

type readDeadlineRecordingConn struct {
	net.Conn
	active  atomic.Bool
	changes chan time.Time
}

func (c *readDeadlineRecordingConn) SetReadDeadline(deadline time.Time) error {
	c.active.Store(!deadline.IsZero())
	c.changes <- deadline
	return c.Conn.SetReadDeadline(deadline)
}

func TestServidorReadDeadlineDisabledDuringDispatchAndRearmedAfterResponse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(input, []byte("%PDF-synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	signer := &disconnectBlockedSigner{started: make(chan context.Context, 1), release: make(chan struct{})}
	server, client := net.Pipe()
	defer client.Close()
	record := &readDeadlineRecordingConn{Conn: server, changes: make(chan time.Time, 16)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		New(&Manejador{Firmar: signer}).servirConexion(context.Background(), record)
	}()
	defer func() {
		client.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("server remained alive")
		}
	}()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	if err := json.NewEncoder(client).Encode(peticionJSON(t, "sign", paramsFirma{InputPath: input, Format: "pades", ReturnSignatureB64: true})); err != nil {
		t.Fatal(err)
	}
	var operation context.Context
	select {
	case operation = <-signer.started:
	case <-time.After(3 * time.Second):
		close(signer.release)
		t.Fatal("sign did not start")
	}
	// No inactivity timer exists during the native call. Only its operation
	// timeout remains; waiting for a next request cannot expire this connection.
	if record.active.Load() || operation.Err() != nil {
		t.Error("idle deadline active during dispatch")
	}
	close(signer.release)
	var response respuesta
	if err := json.NewDecoder(client).Decode(&response); err != nil || !response.OK {
		t.Fatalf("response: %#v, %v", response, err)
	}
	for i := 0; i < 3; i++ {
		select {
		case d := <-record.changes:
			if (i == 1) != d.IsZero() {
				t.Fatalf("deadline transition %d: %v", i, d)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing deadline transition")
		}
	}
}

func TestServidorPersistentPipelinedRequestsRemainOrdered(t *testing.T) {
	t.Parallel()
	server, client := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); New(&Manejador{}).servirConexion(context.Background(), server) }()
	defer func() { client.Close(); <-done }()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	writeDone := make(chan error, 1)
	go func() {
		encoder := json.NewEncoder(client)
		for _, id := range []string{"first", "second", "third", "fourth"} {
			if err := encoder.Encode(peticion{Action: "ping", RequestID: id}); err != nil {
				writeDone <- err
				return
			}
		}
		writeDone <- nil
	}()
	decoder := json.NewDecoder(client)
	for _, id := range []string{"first", "second", "third", "fourth"} {
		var resp respuesta
		if err := decoder.Decode(&resp); err != nil || !resp.OK || resp.RequestID != id {
			t.Fatalf("response %s: %#v, %v", id, resp, err)
		}
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
}

// Emula una llamada nativa que no se puede interrumpir: observa el contexto,
// pero devuelve éxito después de que el test libera la operación.
type disconnectBlockedSigner struct {
	started chan context.Context
	release chan struct{}
}

func (s *disconnectBlockedSigner) Execute(ctx context.Context, _ application.SignCommand) (application.SignResult, error) {
	s.started <- ctx
	<-s.release
	return application.SignResult{Result: domain.SignatureResult{
		Format: domain.FormatPAdES, Data: []byte("synthetic-result-no-key"),
	}}, nil
}

func TestServidorDisconnectCancelsBlockedSignWithoutCommit(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name                string
		existing, halfClose bool
	}{
		{name: "new-output"},
		{name: "existing-output", existing: true},
		{name: "half-close-is-session-end", halfClose: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			existing := scenario.existing
			dir := t.TempDir()
			input, output := filepath.Join(dir, "input.pdf"), filepath.Join(dir, "output.pdf")
			if err := os.WriteFile(input, []byte("%PDF-synthetic-input"), 0o600); err != nil {
				t.Fatal(err)
			}
			original := []byte("preserve-existing-output")
			if existing {
				if err := os.WriteFile(output, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			signer := &disconnectBlockedSigner{started: make(chan context.Context, 1), release: make(chan struct{})}
			server, client := net.Pipe()
			if scenario.halfClose {
				server.Close()
				client.Close()
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				client, err = net.Dial("tcp4", listener.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				server, err = listener.Accept()
				if err != nil {
					t.Fatal(err)
				}
			}
			defer client.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				New(&Manejador{Firmar: signer}).servirConexion(context.Background(), server)
			}()
			defer func() {
				close(signer.release)
				client.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("server did not terminate")
				}
				data, err := os.ReadFile(output)
				if existing {
					if err != nil || string(data) != string(original) {
						t.Errorf("existing destination changed: %q, %v", data, err)
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Errorf("cancelled operation published output: %q, %v", data, err)
				}
			}()
			request := peticionJSON(t, "sign", paramsFirma{InputPath: input, OutputPath: output, Format: "pades"})
			if err := json.NewEncoder(client).Encode(request); err != nil {
				t.Fatal(err)
			}
			var operation context.Context
			select {
			case operation = <-signer.started:
			case <-time.After(3 * time.Second):
				t.Fatal("sign did not start")
			}
			if scenario.halfClose {
				if err := client.(*net.TCPConn).CloseWrite(); err != nil {
					t.Fatal(err)
				}
			} else {
				client.Close()
			}
			select {
			case <-operation.Done():
				if !errors.Is(operation.Err(), context.Canceled) {
					t.Fatalf("unexpected cancellation: %v", operation.Err())
				}
			case <-time.After(time.Second):
				t.Fatal("client disconnected but operation context remained active")
			}
		})
	}
}
