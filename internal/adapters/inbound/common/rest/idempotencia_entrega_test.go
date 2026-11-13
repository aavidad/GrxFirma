// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/application"
)

type firmaQueFallaUnaVez struct {
	mu      sync.Mutex
	llamada int
}

func (f *firmaQueFallaUnaVez) Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error) {
	f.mu.Lock()
	f.llamada++
	n := f.llamada
	f.mu.Unlock()
	if n == 1 {
		return application.SignResult{}, errors.New("PIN cancelado")
	}
	return signUseCaseMock{}.Execute(ctx, cmd)
}

type loteQueFallaUnaVez struct {
	mu      sync.Mutex
	llamada int
}

func (f *loteQueFallaUnaVez) Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	f.mu.Lock()
	f.llamada++
	n := f.llamada
	f.mu.Unlock()
	if n == 1 {
		return application.BatchResult{}, errors.New("sello de tiempo caído")
	}
	return (&batchSignUseCaseMock{}).Execute(ctx, cmd)
}

func cuerpoFirmaConID(t *testing.T, id string) map[string]any {
	t.Helper()
	return map[string]any{
		"request_id":     id,
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
		"certificate_id": "cert-1",
	}
}

// B-03: si la firma falla, el request_id no queda gastado.
func TestSign_RequestIDSeLiberaSiLaFirmaFalla(t *testing.T) {
	h := nuevoAdaptadorPruebas(&firmaQueFallaUnaVez{}, nil, nil).Routes()
	cuerpo := cuerpoFirmaConID(t, "firma-reintento-0001")
	if rr := doJSON(t, h, http.MethodPost, "/sign", cuerpo); rr.Code != http.StatusBadRequest {
		t.Fatalf("primer intento = %d; se esperaba 400", rr.Code)
	}
	if rr := doJSON(t, h, http.MethodPost, "/sign", cuerpo); rr.Code != http.StatusOK {
		t.Fatalf("reintento = %d (%s); se esperaba 200", rr.Code, rr.Body.String())
	}
	if rr := doJSON(t, h, http.MethodPost, "/sign", cuerpo); rr.Code != http.StatusConflict {
		t.Fatalf("tras entregar la firma el request_id debe quedar gastado: %d", rr.Code)
	}
}

func TestSignBatch_RequestIDSeLiberaSiElLoteFalla(t *testing.T) {
	h := nuevoAdaptadorPruebas(nil, nil, nil).WithBatchSigner(&loteQueFallaUnaVez{}).Routes()
	cuerpo := map[string]any{
		"request_id":     "lote-reintento-0001",
		"certificate_id": "cert-1",
		"format":         "CAdES",
		"items": []map[string]any{{
			"name":           "doc.txt",
			"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		}},
	}
	if rr := doJSON(t, h, http.MethodPost, "/sign-batch", cuerpo); rr.Code != http.StatusBadRequest {
		t.Fatalf("primer intento = %d; se esperaba 400", rr.Code)
	}
	if rr := doJSON(t, h, http.MethodPost, "/sign-batch", cuerpo); rr.Code != http.StatusOK {
		t.Fatalf("reintento = %d (%s); se esperaba 200", rr.Code, rr.Body.String())
	}
}

// escritorRoto simula la conexión cerrada por WriteTimeout: la respuesta
// no llega al cliente.
type escritorRoto struct{ *httptest.ResponseRecorder }

func (e escritorRoto) Write([]byte) (int, error) { return 0, errors.New("conexión cerrada") }
func (e escritorRoto) FlushError() error         { return errors.New("conexión cerrada") }

// B-03: si la firma no se puede entregar, el cliente puede reintentar.
func TestSign_RequestIDSeLiberaSiLaRespuestaNoSeEntrega(t *testing.T) {
	h := nuevoAdaptadorPruebas(signUseCaseMock{}, nil, nil).Routes()
	cuerpo := cuerpoFirmaConID(t, "firma-perdida-0001")
	data, _ := json.Marshal(cuerpo)
	req := nuevaPeticionPruebas(http.MethodPost, "/sign", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(escritorRoto{httptest.NewRecorder()}, req)

	if rr := doJSON(t, h, http.MethodPost, "/sign", cuerpo); rr.Code != http.StatusOK {
		t.Fatalf("reintento tras respuesta perdida = %d (%s); se esperaba 200", rr.Code, rr.Body.String())
	}
}

type firmaLenta struct{ espera time.Duration }

func (f firmaLenta) Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error) {
	time.Sleep(f.espera)
	return signUseCaseMock{}.Execute(ctx, cmd)
}

// B-03: una firma más lenta que el WriteTimeout del servidor (PIN, TSA) se
// sigue entregando.
func TestSign_FirmaMasLentaQueWriteTimeoutSeEntrega(t *testing.T) {
	srv := httptest.NewUnstartedServer(nuevoAdaptadorPruebas(firmaLenta{espera: 400 * time.Millisecond}, nil, nil).Routes())
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	data, _ := json.Marshal(cuerpoFirmaConID(t, "firma-lenta-0001"))
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/sign", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenPruebasREST)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("la respuesta de la firma lenta se perdió: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("firma lenta = %d", resp.StatusCode)
	}
}
