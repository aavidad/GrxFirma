// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/domain"
)

// ---- Helpers para batch tests -----------------------------------------------

// servidorExitosoOK devuelve un httptest.Server que simula el flujo trifásico completo
// con éxito. Acepta múltiples peticiones concurrentes.
func servidorExitosoOK(t *testing.T) *httptest.Server {
	t.Helper()
	docB64 := base64.StdEncoding.EncodeToString([]byte("documento-batch"))
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash-batch"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			w.Header().Set("Content-Type", "application/json")
			rr := retrieveResponse{
				Dat:       docB64,
				Op:        "sign",
				Format:    "CAdES",
				Algorithm: "SHA256withRSA",
			}
			b, _ := json.Marshal(rr)
			w.Write(b)

		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				w.Header().Set("Content-Type", "application/json")
				pr := presignResponse{Sign: hashB64}
				b, _ := json.Marshal(pr)
				w.Write(b)
			} else {
				w.Write([]byte("resultado-firma"))
			}

		default:
			http.Error(w, "no esperado", http.StatusNotFound)
		}
	}))
	return srv
}

// batchJobParaServidor construye un BatchJob apuntando al servidor dado.
func batchJobParaServidor(srvURL string) BatchJob {
	doc, _ := domain.NewDocument("doc.txt", []byte("contenido"), "text/plain")
	return BatchJob{
		Job: domain.SignatureJob{
			Document: doc,
			Format:   domain.FormatCAdES,
			Action:   domain.ActionSign,
		},
		Session: domain.ExchangeSession{
			RequestID:        "req-batch-001",
			RetrieveEndpoint: srvURL,
			UploadEndpoint:   srvURL,
			State:            domain.SessionActive,
		},
	}
}

// ---- Tests ------------------------------------------------------------------

// TestBatch_TodosOK verifica que cuando todos los jobs tienen éxito,
// el batch devuelve resultados OK para cada uno.
func TestBatch_TodosOK(t *testing.T) {
	srv := servidorExitosoOK(t)
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)

	const numJobs = 5
	jobs := make([]BatchJob, numJobs)
	for i := range jobs {
		jobs[i] = batchJobParaServidor(srv.URL)
		// Cada sesión necesita un RequestID único (aunque el mock no lo valide,
		// es buena práctica para evitar colisiones en batches reales).
		jobs[i].Session.RequestID = fmt.Sprintf("req-%d", i)
	}

	ctx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "batch-test"})
	resultados := batch.Execute(ctx, jobs)

	if len(resultados) != numJobs {
		t.Fatalf("esperados %d resultados, obtenidos %d", numJobs, len(resultados))
	}
	for _, r := range resultados {
		if r.Err != nil {
			t.Errorf("job %d: error inesperado: %v", r.Index, r.Err)
		}
		if r.Result.Format != domain.FormatCAdES {
			t.Errorf("job %d: formato incorrecto: %s", r.Index, r.Result.Format)
		}
	}
}

// TestBatch_UnFallo_OtrosSiguen verifica que cuando un job falla (servidor retorna 500),
// los demás jobs completan correctamente (modo best-effort).
func TestBatch_UnFallo_OtrosSiguen(t *testing.T) {
	// Índice del job que fallará.
	const indiceError = 1

	docB64 := base64.StdEncoding.EncodeToString([]byte("doc"))
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))

	// Servidor que falla solo cuando el id contiene "falla".
	srvFallo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "falla") {
			http.Error(w, "error forzado", http.StatusInternalServerError)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			rr := retrieveResponse{Dat: docB64, Op: "sign", Format: "CAdES", Algorithm: "SHA256withRSA"}
			b, _ := json.Marshal(rr)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				pr := presignResponse{Sign: hashB64}
				b, _ := json.Marshal(pr)
				w.Header().Set("Content-Type", "application/json")
				w.Write(b)
			} else {
				w.Write([]byte("resultado"))
			}
		}
	}))
	defer srvFallo.Close()

	executor := New(srvFallo.Client())
	// Usar concurrencia 1 para que el orden sea determinista en la verificación.
	batch := NewBatchWithConcurrency(executor, 1)

	const numJobs = 3
	jobs := make([]BatchJob, numJobs)
	for i := range jobs {
		jobs[i] = batchJobParaServidor(srvFallo.URL)
		jobs[i].Session.RequestID = fmt.Sprintf("req-%d", i)
	}
	// El job con indiceError apunta a un RequestID que activa el error en el servidor.
	jobs[indiceError].Session.RequestID = "falla-req"

	ctx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "batch-test"})
	resultados := batch.Execute(ctx, jobs)

	if len(resultados) != numJobs {
		t.Fatalf("esperados %d resultados, obtenidos %d", numJobs, len(resultados))
	}

	// El job en indiceError debe haber fallado.
	if resultados[indiceError].Err == nil {
		t.Errorf("job %d: debería haber fallado pero no lo hizo", indiceError)
	}

	// Los demás deben haber completado sin error.
	for i, r := range resultados {
		if i == indiceError {
			continue
		}
		if r.Err != nil {
			t.Errorf("job %d: error inesperado: %v", i, r.Err)
		}
	}
}

// TestBatch_ContextoCancelado verifica que cuando el contexto está cancelado
// antes de ejecutar, todos los jobs retornan ctx.Err().
func TestBatch_ContextoCancelado(t *testing.T) {
	srv := servidorExitosoOK(t)
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar inmediatamente

	jobs := []BatchJob{
		batchJobParaServidor(srv.URL),
		batchJobParaServidor(srv.URL),
		batchJobParaServidor(srv.URL),
	}
	for i := range jobs {
		jobs[i].Session.RequestID = fmt.Sprintf("req-%d", i)
	}

	resultados := batch.Execute(ctx, jobs)

	if len(resultados) != len(jobs) {
		t.Fatalf("esperados %d resultados, obtenidos %d", len(jobs), len(resultados))
	}

	for _, r := range resultados {
		if r.Err == nil {
			t.Errorf("job %d: debería haber retornado error de contexto cancelado", r.Index)
		}
	}
}

// TestBatch_ConcurrenciaMaxima verifica que nunca se ejecutan más goroutines
// simultáneas que el límite maxConc configurado.
func TestBatch_ConcurrenciaMaxima(t *testing.T) {
	const maxConc = 2
	const numJobs = 6

	var (
		concActual int64
		concMax    int64
	)

	// Barrera que bloquea cada handler de retrieve hasta que se libere.
	// Se cierra después de lanzar el batch, permitiendo que todos los handlers avancen.
	salir := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id=") {
			actual := atomic.AddInt64(&concActual, 1)
			// Actualizar el máximo de concurrencia observado.
			for {
				viejo := atomic.LoadInt64(&concMax)
				if actual <= viejo {
					break
				}
				if atomic.CompareAndSwapInt64(&concMax, viejo, actual) {
					break
				}
			}

			// Esperar señal de salida antes de responder.
			<-salir

			atomic.AddInt64(&concActual, -1)

			docB64 := base64.StdEncoding.EncodeToString([]byte("doc"))
			rr := retrieveResponse{Dat: docB64, Op: "sign", Format: "CAdES", Algorithm: "SHA256withRSA"}
			b, _ := json.Marshal(rr)
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}

		// Para las demás fases, responder inmediatamente.
		switch {
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))
				pr := presignResponse{Sign: hashB64}
				b, _ := json.Marshal(pr)
				w.Header().Set("Content-Type", "application/json")
				w.Write(b)
			} else {
				w.Write([]byte("resultado"))
			}
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatchWithConcurrency(executor, maxConc)

	jobs := make([]BatchJob, numJobs)
	for i := range jobs {
		jobs[i] = batchJobParaServidor(srv.URL)
		jobs[i].Session.RequestID = fmt.Sprintf("req-%d", i)
	}

	batchCtx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "conc-test"})
	// Lanzar el batch en una goroutine separada.
	done := make(chan []BatchResult, 1)
	go func() {
		done <- batch.Execute(batchCtx, jobs)
	}()

	// Liberar la barrera para que todos los handlers puedan responder.
	close(salir)

	resultados := <-done

	if len(resultados) != numJobs {
		t.Fatalf("esperados %d resultados, obtenidos %d", numJobs, len(resultados))
	}

	// La concurrencia máxima observada no debe superar maxConc.
	if concMax > int64(maxConc) {
		t.Errorf("concurrencia máxima superada: permitida %d, alcanzada %d", maxConc, concMax)
	}
}
