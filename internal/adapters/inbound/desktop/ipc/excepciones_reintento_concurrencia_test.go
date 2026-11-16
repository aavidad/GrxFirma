// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// Dos reintentos simultáneos con el mismo identificador y la misma conexión:
// solo uno llega a firmar y devuelve la firma; el otro sale caducado. La
// entrada se retira del mapa bajo el cerrojo antes de despachar.
func TestUnsafeRetryConcurrenteFirmaUnaSolaVez(t *testing.T) {
	m, ctx, p, engine := retryFixture(t)
	blocked := m.despachar(ctx, p)
	if blocked.RetryID == "" {
		t.Fatalf("sin oferta: %+v", blocked)
	}
	if r := activateRetry(t, m, ctx, blocked.RetryID); !r.OK {
		t.Fatalf("activación: %+v", r)
	}
	raw, _ := json.Marshal(map[string]string{"retryId": blocked.RetryID})
	const intentos = 8
	resultados := make([]respuesta, intentos)
	var wg sync.WaitGroup
	inicio := make(chan struct{})
	for i := range intentos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			resultados[i] = m.despacharConTimeout(ctx, peticion{Action: "unsafe_operation_retry", Params: append(json.RawMessage(nil), raw...)})
		}()
	}
	close(inicio)
	wg.Wait()
	ok := 0
	for _, r := range resultados {
		if r.OK {
			ok++
		}
	}
	if ok != 1 || engine.calls != 2 {
		t.Fatalf("reintentos con éxito=%d llamadas al motor=%d; se esperaba 1 y 2", ok, engine.calls)
	}
}

// La firma devuelta por un reintento no aparece en los registros.
func TestUnsafeRetryNoEscribeLaFirmaEnRegistros(t *testing.T) {
	var registro bytes.Buffer
	previo := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelDebug})))
	previoLog, previoFlags := log.Writer(), log.Flags()
	log.SetOutput(&registro)
	t.Cleanup(func() {
		slog.SetDefault(previo)
		log.SetOutput(previoLog)
		log.SetFlags(previoFlags)
	})

	m, ctx, p, _ := retryFixture(t)
	blocked := m.despachar(ctx, p)
	if r := activateRetry(t, m, ctx, blocked.RetryID); !r.OK {
		t.Fatalf("activación: %+v", r)
	}
	raw, _ := json.Marshal(map[string]string{"retryId": blocked.RetryID})
	r := m.despacharConTimeout(ctx, peticion{Action: "unsafe_operation_retry", Params: raw})
	datos, ok := r.Data.(resultadoFirma)
	if !r.OK || !ok || datos.SignatureB64 == "" {
		t.Fatalf("reintento sin firma: %+v", r)
	}
	firma := base64.StdEncoding.EncodeToString([]byte("signed"))
	if datos.SignatureB64 != firma {
		t.Fatalf("firma inesperada %q", datos.SignatureB64)
	}
	if strings.Contains(registro.String(), firma) || strings.Contains(registro.String(), "signed") {
		t.Fatalf("la firma aparece en los registros: %s", registro.String())
	}
}

// El plazo del reintento empieza cuando se ofrece, no cuando empezó la
// operación original, y coincide con el temporizador que lo borra.
func TestUnsafeRetryPlazoEmpiezaAlOfrecerlo(t *testing.T) {
	m, ctx, p, _ := retryFixture(t)
	r := m.prepareUnsafeRetry(ctx, p)
	if r == nil {
		t.Fatal("sin reintento")
	}
	r.expires = time.Now().Add(-time.Minute) // la operación original tardó más que el plazo
	antes := time.Now()
	token := m.storeUnsafeRetry(ctx, r, "rsa_corta")
	if token == "" {
		t.Fatal("sin token")
	}
	m.unsafeRetryMu.Lock()
	expira := m.unsafeRetries[token].expires
	m.unsafeRetryMu.Unlock()
	if expira.Before(antes.Add(unsafeRetryTTL)) || expira.After(time.Now().Add(unsafeRetryTTL)) {
		t.Fatalf("plazo %v, se esperaba %v desde el aviso", expira.Sub(antes), unsafeRetryTTL)
	}
	if !m.retryMatches(ctx, token, "rsa_corta") {
		t.Fatal("el reintento recién ofrecido sale caducado")
	}
}
