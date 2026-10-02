// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nativehost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/limits"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type catalogMock struct {
	certs []domain.CertificateRef
	err   error
}

func (m catalogMock) List(context.Context) ([]domain.CertificateRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.certs, nil
}

type signMock struct {
	result   application.SignResult
	err      error
	captured *application.SignCommand
}

func (m signMock) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	if m.captured != nil {
		*m.captured = cmd
	}
	if m.err != nil {
		return application.SignResult{}, m.err
	}
	return m.result, nil
}

type verifyMock struct {
	result application.VerifyResult
	err    error
}

func (m verifyMock) Execute(_ context.Context, cmd application.VerifyCommand) (application.VerifyResult, error) {
	if cmd.OriginalDocument != nil && string(cmd.OriginalDocument.Content) != "original" {
		return application.VerifyResult{}, errors.New("originalData no se propagó correctamente")
	}
	if m.err != nil {
		return application.VerifyResult{}, m.err
	}
	return m.result, nil
}

type trustMock struct {
	allowed bool
	err     error
}

func (m trustMock) Evaluate(context.Context, string) (domain.TrustDecision, error) {
	if m.err != nil {
		return domain.TrustDecision{}, m.err
	}
	status := domain.TrustDenied
	if m.allowed {
		status = domain.TrustAllowed
	}
	return domain.TrustDecision{Origin: "caller", Status: status}, nil
}

func (m trustMock) Allow(context.Context, string) error  { return nil }
func (m trustMock) Deny(context.Context, string) error   { return nil }
func (m trustMock) Remove(context.Context, string) error { return nil }

func TestReadWriteMessageRoundTrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	original := []byte(`{"requestId":"1","action":"ping"}`)
	if err := WriteMessage(&buf, original); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	payload, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if !bytes.Equal(payload, original) {
		t.Fatalf("payload = %q, want %q", payload, original)
	}
}

func TestReadMessageRechazaLongitudExcesiva(t *testing.T) {
	t.Parallel()

	// Cabecera de 4 bytes con una longitud enorme y sin cuerpo: sin límite,
	// ReadMessage intentaría reservar ~4 GiB (DoS por agotamiento de memoria).
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint32(maxMessageSize+1))

	if _, err := ReadMessage(&buf); err == nil {
		t.Fatal("ReadMessage() debía rechazar una longitud por encima del máximo")
	}
}

func TestProcessPing(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil, nil)
	payload := []byte(`{"requestId":1,"action":"ping"}`)
	responses, err := adapter.Process(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("responses len = %d, want 1", len(responses))
	}

	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success || resp.RequestID != "1" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestProcessGetCertificates(t *testing.T) {
	t.Parallel()

	adapter := New(catalogMock{certs: []domain.CertificateRef{{
		ID:          "cert-1",
		Subject:     "CN=Alice",
		Issuer:      "CN=CA",
		NotAfter:    time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		Fingerprint: "abc123",
	}}}, nil, nil, nil)

	responses, err := adapter.Process(context.Background(), "", []byte(`{"requestId":"2","action":"getCertificates"}`))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success || len(resp.Certificates) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Certificates[0].ID != "cert-1" {
		t.Fatalf("certificate id = %q, want cert-1", resp.Certificates[0].ID)
	}
}

func TestProcessSignChunksLargeSignatures(t *testing.T) {
	t.Parallel()

	signature := bytes.Repeat([]byte("a"), chunkSize)
	signature = append(signature, bytes.Repeat([]byte("b"), 64)...)
	adapter := New(nil, signMock{result: application.SignResult{
		Result: domain.SignatureResult{
			Format:    domain.FormatCAdES,
			Algorithm: "SHA256withRSA",
			Data:      signature,
		},
		CertificateUsed: domain.CertificateRef{ID: "cert-1"},
	}}, nil, nil)

	req := request{
		RequestID:     "3",
		Action:        "sign",
		Name:          "demo.txt",
		MIMEType:      "text/plain",
		Format:        "cades",
		CertificateID: "cert-1",
		Data:          base64.StdEncoding.EncodeToString([]byte("hola")),
	}
	payload, _ := json.Marshal(req)
	responses, err := adapter.Process(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(responses) != 2 {
		t.Fatalf("responses len = %d, want 2", len(responses))
	}
}

func TestProcessSignPropagaCallerNoFalseableYOrigenNormalizado(t *testing.T) {
	t.Parallel()

	var captured application.SignCommand
	adapter := New(nil, signMock{
		result: application.SignResult{
			Result: domain.SignatureResult{
				Format: domain.FormatCAdES,
				Data:   []byte("firma"),
			},
		},
		captured: &captured,
	}, nil, nil)
	adapter.RequireCaller = true
	const caller = "chrome-extension://pkefjandjcgdmhoonmhnllikibobijgg/"
	payload := []byte(`{
		"requestId":"ctx-1",
		"action":"sign",
		"name":"demo.txt",
		"mimeType":"text/plain",
		"format":"cades",
		"certificateId":"cert-1",
		"data":"aG9sYQ==",
		"requesterApplication":"falseada",
		"requesterOrigin":"https://sede.example.test/ruta"
	}`)
	responses, err := adapter.Process(context.Background(), caller, payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success {
		t.Fatalf("resp = %+v", resp)
	}
	if captured.RequesterApplication != caller {
		t.Fatalf("RequesterApplication = %q, want caller fuera de banda", captured.RequesterApplication)
	}
	if captured.RequesterOrigin != "https://sede.example.test" {
		t.Fatalf("RequesterOrigin = %q", captured.RequesterOrigin)
	}
}

func TestProcessRequireCallerRechazaRequestIDAusenteYReplay(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil, nil)
	adapter.RequireCaller = true
	const caller = "chrome-extension://pkefjandjcgdmhoonmhnllikibobijgg/"

	assertError := func(payload string, expected string) response {
		t.Helper()
		responses, err := adapter.Process(context.Background(), caller, []byte(payload))
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		var resp response
		if err := json.Unmarshal(responses[0], &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if resp.Success || !strings.Contains(resp.Error, expected) {
			t.Fatalf("resp = %+v, want error %q", resp, expected)
		}
		return resp
	}

	invalid := assertError(`{"action":"ping"}`, "identificador seguro")
	if !strings.Contains(invalid.Error, "reinstale") {
		t.Fatalf("el rechazo no explica cómo recuperarse: %q", invalid.Error)
	}
	first, err := adapter.Process(
		context.Background(),
		caller,
		[]byte(`{"requestId":"replay-1","action":"ping"}`),
	)
	if err != nil {
		t.Fatalf("primer Process() error = %v", err)
	}
	var firstResp response
	if err := json.Unmarshal(first[0], &firstResp); err != nil || !firstResp.Success {
		t.Fatalf("primera respuesta = %+v, %v", firstResp, err)
	}
	replayed := assertError(`{"requestId":"replay-1","action":"ping"}`, "repetida")
	if !strings.Contains(replayed.Error, "reinicie el navegador") {
		t.Fatalf("el replay no explica cómo recuperarse: %q", replayed.Error)
	}
}

func TestNormalizeRequesterOriginFallaCerrado(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"http://sede.example.test",
		"javascript:alert(1)",
		"https://usuario:clave@sede.example.test",
		"https://sede.example.test/?query=1",
		"https://sede.example.test/#fragmento",
	} {
		if got := normalizeRequesterOrigin(raw); got != "" {
			t.Errorf("normalizeRequesterOrigin(%q) = %q, want vacío", raw, got)
		}
	}
}

func TestProcessSignRechazaPayloadDecodificadoExcesivo(t *testing.T) {
	t.Parallel()

	adapter := New(nil, signMock{}, nil, nil).WithLimits(limits.Limits{MaxPayloadBytes: 4})
	req := request{
		RequestID: "3b",
		Action:    "sign",
		Name:      "demo.txt",
		MIMEType:  "text/plain",
		Format:    "cades",
		Data:      base64.StdEncoding.EncodeToString([]byte("demasiado")),
	}
	payload, _ := json.Marshal(req)
	responses, err := adapter.Process(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Success || resp.Error == "" {
		t.Fatalf("resp = %+v, want payload limit error", resp)
	}
}

func TestProcessVerify(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, verifyMock{result: application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:       true,
			Reason:      "ok",
			Details:     []string{"cadena valida"},
			Certificate: domain.VerificationAspect{Status: domain.VerificationStatusUnknown},
		},
		Firmantes: []domain.CertificateRef{{ID: "cert-1"}},
	}}, nil)

	req := request{
		RequestID:     "4",
		Action:        "verify",
		Name:          "firma.p7s",
		MIMEType:      "application/pkcs7-signature",
		SignatureData: base64.StdEncoding.EncodeToString([]byte("firma")),
	}
	payload, _ := json.Marshal(req)
	responses, err := adapter.Process(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success || resp.Result == nil || !resp.Result.Valid {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.Result.Signers) != 1 || resp.Result.Signers[0] != "cert-1" {
		t.Fatalf("signers = %+v", resp.Result.Signers)
	}
	if resp.Result.CertificateStatus != "unknown" {
		t.Fatalf("certificateStatus = %q", resp.Result.CertificateStatus)
	}
}

func TestProcessVerifyDetachedConOriginalData(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, verifyMock{result: application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:   true,
			Reason:  "ok",
			Details: []string{"detached valido"},
		},
		Firmantes: []domain.CertificateRef{{ID: "cert-1"}},
	}}, nil)

	req := request{
		RequestID:     "4b",
		Action:        "verify",
		Name:          "firma.csig",
		MIMEType:      "application/pkcs7-signature",
		SignatureData: base64.StdEncoding.EncodeToString([]byte("firma")),
		OriginalData:  base64.StdEncoding.EncodeToString([]byte("original")),
	}
	payload, _ := json.Marshal(req)
	responses, err := adapter.Process(context.Background(), "", payload)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success || resp.Result == nil || !resp.Result.Valid {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestProcessRejectsUntrustedCaller(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil, trustMock{allowed: false})
	responses, err := adapter.Process(context.Background(), "ext-no", []byte(`{"requestId":"5","action":"ping"}`))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	var resp response
	if err := json.Unmarshal(responses[0], &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Success {
		t.Fatalf("resp.Success = true, want false")
	}
}

func TestServeOnce(t *testing.T) {
	t.Parallel()

	adapter := New(nil, nil, nil, nil)
	var input bytes.Buffer
	if err := WriteMessage(&input, []byte(`{"requestId":"6","action":"ping"}`)); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	var output bytes.Buffer
	if err := adapter.ServeOnce(context.Background(), "", &input, &output); err != nil {
		t.Fatalf("ServeOnce() error = %v", err)
	}

	payload, err := ReadMessage(&output)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	var resp response
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestProcessCatalogError(t *testing.T) {
	t.Parallel()

	adapter := New(catalogMock{err: errors.New("boom")}, nil, nil, nil)
	responses, err := adapter.Process(context.Background(), "", []byte(`{"requestId":"7","action":"getCertificates"}`))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	var resp response
	_ = json.Unmarshal(responses[0], &resp)
	if resp.Error == "" {
		t.Fatalf("resp = %+v, want error", resp)
	}
}
