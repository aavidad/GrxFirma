// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type idempotentSignUseCase struct {
	calls atomic.Int32
}

func (m *idempotentSignUseCase) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	m.calls.Add(1)
	return application.SignResult{
		Result: domain.SignatureResult{
			Format:    cmd.Format,
			Data:      []byte("firma"),
			Algorithm: "mock",
		},
		CertificateUsed: domain.CertificateRef{ID: cmd.CertificateID},
	}, nil
}

func TestSignRequestID_RechazaReplayYPersisteSoloElHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mock := &idempotentSignUseCase{}
	adaptador := New(mock, nil, nil).WithConfigDir(dir)
	body := validIdempotentSignBody("safari-req-001")

	first := executeSignRequest(t, adaptador, body)
	if first.Code != http.StatusOK {
		t.Fatalf("primera firma: status=%d body=%s", first.Code, first.Body.String())
	}
	var response signResponse
	if err := json.Unmarshal(first.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta JSON: %v", err)
	}
	if response.RequestID != "safari-req-001" {
		t.Fatalf("request_id respuesta = %q", response.RequestID)
	}
	second := executeSignRequest(t, adaptador, body)
	if second.Code != http.StatusConflict {
		t.Fatalf("replay mismo proceso: status=%d body=%s", second.Code, second.Body.String())
	}

	restarted := New(&idempotentSignUseCase{}, nil, nil).WithConfigDir(dir)
	afterRestart := executeSignRequest(t, restarted, body)
	if afterRestart.Code != http.StatusConflict {
		t.Fatalf("replay tras reinicio: status=%d body=%s", afterRestart.Code, afterRestart.Body.String())
	}
	if got := mock.calls.Load(); got != 1 {
		t.Fatalf("Execute se invocó %d veces, want 1", got)
	}

	registryPath := filepath.Join(dir, signRequestRegistryFilename)
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("leer registro: %v", err)
	}
	if bytes.Contains(registry, []byte("safari-req-001")) {
		t.Fatal("el registro no debe persistir el request_id en claro")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(registryPath)
		if err != nil {
			t.Fatalf("stat registro: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("permisos registro = %#o, want 0600", got)
		}
	}
}

func TestSignRequestID_RechazaIdentificadoresNoValidos(t *testing.T) {
	t.Parallel()
	mock := &idempotentSignUseCase{}
	adaptador := New(mock, nil, nil)
	for _, requestID := range []string{"../../escape", "espacio no", "   ", string(bytes.Repeat([]byte{'a'}, 129))} {
		body := validIdempotentSignBody(requestID)
		response := executeSignRequest(t, adaptador, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("request_id %q: status=%d body=%s", requestID, response.Code, response.Body.String())
		}
	}
	if got := mock.calls.Load(); got != 0 {
		t.Fatalf("Execute se invocó %d veces con IDs inválidos", got)
	}
}

func TestSignRequestID_ReservaEsAtomica(t *testing.T) {
	t.Parallel()
	adaptador := New(&idempotentSignUseCase{}, nil, nil)
	const workers = 24
	var accepted atomic.Int32
	var replays atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := adaptador.reserveSignRequestID("concurrent-request")
			switch {
			case err == nil:
				accepted.Add(1)
			case errors.Is(err, errSignRequestReplay):
				replays.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 || replays.Load() != workers-1 || unexpected.Load() != 0 {
		t.Fatalf("accepted=%d replays=%d unexpected=%d", accepted.Load(), replays.Load(), unexpected.Load())
	}
}

func TestSignRequestID_DescartaEntradasExpiradas(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	digest := sha256Hex("expired-request")
	persisted := persistedSignRequests{
		Version: signRequestRegistryVersion,
		Entries: map[string]time.Time{digest: time.Now().Add(-time.Hour)},
	}
	data, err := json.Marshal(persisted)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, signRequestRegistryFilename), data, 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	if err := adaptador.reserveSignRequestID("expired-request"); err != nil {
		t.Fatalf("una entrada expirada debe poder reutilizarse: %v", err)
	}
}

func TestSignRequestID_RegistroCorruptoFallaCerrado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registryPath := filepath.Join(dir, signRequestRegistryFilename)
	if err := os.WriteFile(registryPath, []byte(`{"version":1,"entries":`), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	if err := adaptador.reserveSignRequestID("new-request"); err == nil {
		t.Fatal("un registro corrupto debe impedir la firma")
	}
	if len(adaptador.signRequests) != 0 {
		t.Fatalf("el registro en memoria debe permanecer vacío: %#v", adaptador.signRequests)
	}
}

func validIdempotentSignBody(requestID string) map[string]any {
	return map[string]any{
		"request_id":     requestID,
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
		"certificate_id": "cert-1",
	}
}

func executeSignRequest(t *testing.T, adaptador *Adaptador, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/sign", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(response, request)
	return response
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
