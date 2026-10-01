// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestReviewResponseRejectsCrossOperationAndUntrustedFields(t *testing.T) {
	id := strings.Repeat("a", 32)
	entry := CatalogEntry{Certificate: domain.CertificateRef{ID: "synthetic", Fingerprint: strings.Repeat("b", 64)}}
	for _, tc := range []struct {
		name, operation string
		mutate          func(*Response)
	}{
		{"version", "list", func(r *Response) { r.Version++ }},
		{"id", "list", func(r *Response) { r.ID = strings.Repeat("b", 32) }},
		{"raw_code", "list", func(r *Response) { r.Code = "synthetic driver secret" }},
		{"error_with_data", "list", func(r *Response) { r.Code = "pin_locked"; r.Signature = []byte{1} }},
		{"list_signature", "list", func(r *Response) { r.Signature = []byte{1} }},
		{"list_chain", "list", func(r *Response) { r.ChainDER = [][]byte{{1}} }},
		{"duplicate_ids", "list", func(r *Response) { r.Certificates = []CatalogEntry{entry, entry} }},
		{"large_catalog", "list", func(r *Response) { r.Certificates = make([]CatalogEntry, MaxCertificates+1) }},
		{"large_subject", "list", func(r *Response) {
			e := entry
			e.Certificate.Subject = strings.Repeat("x", 4097)
			r.Certificates = []CatalogEntry{e}
		}},
		{"describe_empty", "describe", func(*Response) {}},
		{"describe_large_der", "describe", func(r *Response) { r.ChainDER = [][]byte{make([]byte, MaxCertificateBytes+1)} }},
		{"describe_signature", "describe", func(r *Response) { r.ChainDER = [][]byte{{1}}; r.Signature = []byte{1} }},
		{"sign_empty", "sign", func(*Response) {}},
		{"sign_large", "sign", func(r *Response) { r.Signature = make([]byte, MaxSignatureBytes+1) }},
		{"sign_catalog", "sign", func(r *Response) { r.Signature = []byte{1}; r.Certificates = []CatalogEntry{entry} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := Response{Version: ProtocolVersion, ID: id, Code: "ok"}
			tc.mutate(&response)
			if !errors.Is(validateResponse(Request{ID: id, Operation: tc.operation}, response), ErrProtocol) {
				t.Fatal("respuesta no confiable admitida")
			}
		})
	}
}

func TestReviewClientClearsPINOnEverySourceFailure(t *testing.T) {
	for _, mode := range []string{"error_with_buffer", "too_large", "pinpad_with_pin"} {
		t.Run(mode, func(t *testing.T) {
			fixture := "pin"
			if mode == "pinpad_with_pin" {
				fixture = "pinpad"
			}
			client := fixtureClient(t, fixture)
			pin := []byte("synthetic PIN")
			if mode == "too_large" {
				pin = bytes.Repeat([]byte{42}, MaxPINBytes+1)
			}
			client.PINSource = func(context.Context, PINMode) ([]byte, error) {
				if mode == "error_with_buffer" {
					return pin, errors.New("synthetic private source error")
				}
				return pin, nil
			}
			_, err := client.Execute(context.Background(), clientSignRequest())
			want := ErrProtocol
			if mode == "error_with_buffer" {
				want = ErrPINCancelled
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v", err)
			}
			if !bytes.Equal(pin, make([]byte, len(pin))) {
				t.Fatal("buffer PIN conservado tras fallo")
			}
		})
	}
}

func TestReviewDescriptionRejectsInvalidDERAndIdentitySubstitution(t *testing.T) {
	request, _, key := fixture(t)
	request.Operation, request.Hash, request.Digest = "describe", "", nil
	valid := Response{Version: ProtocolVersion, ID: request.ID, Code: "ok", ChainDER: key.chain}
	if err := validateResponse(request, valid); err != nil {
		t.Fatalf("descripción sintética válida rechazada: %v", err)
	}
	for _, mode := range []string{"invalid_leaf", "invalid_intermediate", "wrong_fingerprint"} {
		t.Run(mode, func(t *testing.T) {
			r := request
			response := valid
			switch mode {
			case "invalid_leaf":
				response.ChainDER = [][]byte{{1, 2, 3}}
			case "invalid_intermediate":
				response.ChainDER = [][]byte{key.chain[0], {1, 2, 3}}
			case "wrong_fingerprint":
				r.Fingerprint = strings.Repeat("f", 64)
			}
			if !errors.Is(validateResponse(r, response), ErrProtocol) {
				t.Fatal("DER/identidad del auxiliar no validado")
			}
		})
	}
}

func TestReviewClientBoundsNonCooperativePINSource(t *testing.T) {
	client := fixtureClient(t, "pin")
	client.Timeout = 250 * time.Millisecond
	release := make(chan struct{})
	returned := make(chan struct{})
	client.PINSource = func(context.Context, PINMode) ([]byte, error) {
		defer close(returned)
		<-release
		return []byte("synthetic late PIN"), nil
	}
	started := time.Now()
	_, err := client.Execute(context.Background(), clientSignRequest())
	close(release)
	// El callback no coopera, pero la prueba lo libera para no dejar gorutinas.
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("callback no terminó")
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 2*time.Second {
		t.Fatalf("deadline del proceso/PIN no respetado: %v", err)
	}
}
