// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package intermediatehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/limits"
	"grxfirma/internal/domain"
)

func TestTransporte_Upload_SendWait_Cancel_Retrieve(t *testing.T) {
	t.Parallel()

	var vistas []capturaPeticion
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm fallo: %v", err)
		}
		vistas = append(vistas, capturaPeticion{
			metodo: r.Method,
			ruta:   r.URL.Path,
			query:  r.URL.RawQuery,
			form:   r.Form.Encode(),
		})

		switch r.Form.Get("op") {
		case "get":
			_, _ = w.Write([]byte("PETICION-PENDIENTE"))
		case "put", "cancel":
			_, _ = w.Write([]byte("OK"))
		default:
			http.Error(w, "ERR", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	sesion := domain.ExchangeSession{
		RequestID:        "req-001",
		SessionKey:       "clave-sesion",
		UploadEndpoint:   server.URL + "/StorageService",
		RetrieveEndpoint: server.URL + "/RetrieveService",
		State:            domain.SessionActive,
	}

	transporte := New(&http.Client{Timeout: 2 * time.Second})
	ctx := context.Background()

	if err := transporte.Upload(ctx, sesion, []byte("FIRMA")); err != nil {
		t.Fatalf("Upload fallo: %v", err)
	}
	if err := transporte.SendWait(ctx, sesion); err != nil {
		t.Fatalf("SendWait fallo: %v", err)
	}
	datos, err := transporte.Retrieve(ctx, sesion)
	if err != nil {
		t.Fatalf("Retrieve fallo: %v", err)
	}
	if string(datos) != "PETICION-PENDIENTE" {
		t.Fatalf("datos retrieve inesperados: %q", string(datos))
	}
	if err := transporte.Cancel(ctx, sesion); err != nil {
		t.Fatalf("Cancel fallo: %v", err)
	}

	if len(vistas) != 4 {
		t.Fatalf("se esperaban 4 peticiones, hubo %d", len(vistas))
	}
	assertFormContains(t, vistas[0].form, url.Values{
		"op":  {"put"},
		"v":   {"1_0"},
		"id":  {"req-001"},
		"dat": {"FIRMA"},
	})
	assertFormContains(t, vistas[1].form, url.Values{
		"op":  {"put"},
		"dat": {"#WAIT"},
		"id":  {"req-001"},
	})
	if vistas[2].metodo != http.MethodGet || !strings.Contains(vistas[2].query, "op=get") {
		t.Fatalf("Retrieve no uso GET con op=get: metodo=%s query=%q", vistas[2].metodo, vistas[2].query)
	}
	assertFormContains(t, vistas[3].form, url.Values{
		"op": {"cancel"},
		"id": {"req-001"},
	})
	// La clave de sesión cifra el intercambio: nunca debe viajar al servidor
	// intermedio (ni en el cuerpo ni en la query), igual que en AutoFirma Java.
	for i, vista := range vistas {
		formulario, _ := url.ParseQuery(vista.form)
		consulta, _ := url.ParseQuery(vista.query)
		if formulario.Has("key") || consulta.Has("key") {
			t.Fatalf("la petición %d envió la clave de sesión: form=%v query=%q", i, vista.form, vista.query)
		}
	}
}

func TestTransporte_RespetaContextoCancelado(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	transporte := New(&http.Client{Timeout: time.Second})
	sesion := domain.ExchangeSession{
		RequestID:        "req-002",
		UploadEndpoint:   server.URL + "/StorageService",
		RetrieveEndpoint: server.URL + "/RetrieveService",
		State:            domain.SessionActive,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := transporte.SendWait(ctx, sesion); err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}

func TestTransporte_RechazaHTTPNoLocalYCredenciales(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		upload   string
		retrieve string
	}{
		{
			name:     "http_publico",
			upload:   "http://example.com/StorageService",
			retrieve: "https://example.com/RetrieveService",
		},
		{
			name:     "credenciales",
			upload:   "https://usuario:secreto@example.com/StorageService",
			retrieve: "https://example.com/RetrieveService",
		},
		{
			name:     "fragmento",
			upload:   "https://example.com/StorageService#secreto",
			retrieve: "https://example.com/RetrieveService",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			transporte := New(&http.Client{Timeout: time.Second})
			err := transporte.SendWait(context.Background(), domain.ExchangeSession{
				RequestID:        "req-segura",
				UploadEndpoint:   tt.upload,
				RetrieveEndpoint: tt.retrieve,
				State:            domain.SessionActive,
			})
			if err == nil {
				t.Fatal("se esperaba el rechazo del endpoint no seguro")
			}
		})
	}
}

func TestTransporte_LimitaSubidaYDescarga(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()

	configured := limits.Default()
	configured.MaxPayloadBytes = 4
	transporte := newWithLimits(server.Client(), configured)
	sesion := domain.ExchangeSession{
		RequestID:        "req-limites",
		UploadEndpoint:   server.URL + "/StorageService",
		RetrieveEndpoint: server.URL + "/RetrieveService",
		State:            domain.SessionActive,
	}

	if err := transporte.Upload(context.Background(), sesion, []byte("12345")); err == nil {
		t.Fatal("se esperaba rechazo de la subida sobredimensionada")
	}
	if requests.Load() != 0 {
		t.Fatal("la subida rechazada no debe alcanzar la red")
	}

	_, err := transporte.Retrieve(context.Background(), sesion)
	var oversized *limits.ErrPayloadExcedido
	if !errors.As(err, &oversized) {
		t.Fatalf("se esperaba ErrPayloadExcedido, se obtuvo %v", err)
	}
}

func TestTransporte_BloqueaRedireccionAOtroOrigen(t *testing.T) {
	t.Parallel()

	var sinkRequests atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sinkRequests.Add(1)
		_, _ = w.Write([]byte("NO-DEBE-LLEGAR"))
	}))
	defer sink.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, sink.URL+"/captura", http.StatusFound)
	}))
	defer source.Close()

	transporte := New(source.Client())
	_, err := transporte.Retrieve(context.Background(), domain.ExchangeSession{
		RequestID:        "req-redirect",
		SessionKey:       "clave-que-no-debe-filtrarse",
		UploadEndpoint:   source.URL + "/StorageService",
		RetrieveEndpoint: source.URL + "/RetrieveService",
		State:            domain.SessionActive,
	})
	if err == nil || !strings.Contains(err.Error(), "otro origen") {
		t.Fatalf("se esperaba rechazo de redireccion entre origenes, se obtuvo %v", err)
	}
	if sinkRequests.Load() != 0 {
		t.Fatal("el segundo origen no debe recibir la peticion ni la clave de sesion")
	}
}

func TestTransporte_PermiteRedireccionRelativaMismoOrigen(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte("DOCUMENTO"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	transporte := New(server.Client())
	data, err := transporte.Retrieve(context.Background(), domain.ExchangeSession{
		RequestID:        "req-redirect-local",
		UploadEndpoint:   server.URL + "/StorageService",
		RetrieveEndpoint: server.URL + "/RetrieveService",
		State:            domain.SessionActive,
	})
	if err != nil {
		t.Fatalf("la redireccion relativa segura fallo: %v", err)
	}
	if string(data) != "DOCUMENTO" {
		t.Fatalf("respuesta inesperada: %q", data)
	}
}

func TestTransporte_NoMutaClienteYAplicaTimeoutSeguro(t *testing.T) {
	t.Parallel()

	original := &http.Client{}
	transporte := New(original)
	if original.Timeout != 0 || original.CheckRedirect != nil {
		t.Fatal("New no debe mutar el cliente entregado por el llamador")
	}
	if transporte.client.Timeout <= 0 || transporte.client.CheckRedirect == nil {
		t.Fatal("el transporte debe aplicar timeout y politica de redireccion")
	}
}

func TestTransporte_NoExponeCuerpoDeErrorRemoto(t *testing.T) {
	t.Parallel()

	const secret = "TOKEN-REMOTO-SENSIBLE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secret, http.StatusInternalServerError)
	}))
	defer server.Close()

	transporte := New(server.Client())
	_, err := transporte.Retrieve(context.Background(), domain.ExchangeSession{
		RequestID:        "req-error",
		UploadEndpoint:   server.URL + "/StorageService",
		RetrieveEndpoint: server.URL + "/RetrieveService",
		State:            domain.SessionActive,
	})
	if err == nil {
		t.Fatal("se esperaba error HTTP")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("el error local no debe reproducir el cuerpo remoto")
	}
}

type capturaPeticion struct {
	metodo string
	ruta   string
	query  string
	form   string
}

func assertFormContains(t *testing.T, encoded string, expected url.Values) {
	t.Helper()
	valores, err := url.ParseQuery(encoded)
	if err != nil {
		t.Fatalf("formulario invalido %q: %v", encoded, err)
	}
	for key, want := range expected {
		got := valores[key]
		if len(got) != len(want) {
			t.Fatalf("valor inesperado para %s: got=%v want=%v", key, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("valor inesperado para %s: got=%v want=%v", key, got, want)
			}
		}
	}
}
