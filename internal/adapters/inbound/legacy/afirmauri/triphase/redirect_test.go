// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"grxfirma/internal/domain"
)

func servidoresRedireccion(t *testing.T, codigo int) (*httptest.Server, *httptest.Server, *atomic.Int32) {
	t.Helper()
	var visitas atomic.Int32
	destino := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		visitas.Add(1)
		_, _ = w.Write([]byte("OK"))
	}))
	t.Cleanup(destino.Close)
	origen := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destino.URL+"/robado", codigo)
	}))
	t.Cleanup(origen.Close)
	return origen, destino, &visitas
}

// A-04: una redirección a otro origen no se sigue (el origen validado es el
// del endpoint, no el del destino de la redirección).
func TestRetrieveRaw_RechazaRedireccionAOtroOrigen(t *testing.T) {
	origen, _, visitas := servidoresRedireccion(t, http.StatusFound)
	e := New(origen.Client())
	_, err := e.RetrieveRaw(context.Background(), domain.ExchangeSession{
		RequestID: "r", RetrieveEndpoint: origen.URL + "/rt", UploadEndpoint: origen.URL + "/st", State: domain.SessionActive,
	})
	if err == nil {
		t.Fatal("se esperaba error por la redirección a otro origen")
	}
	if n := visitas.Load(); n != 0 {
		t.Fatalf("el destino de la redirección recibió %d peticiones", n)
	}
}

func TestUploadSignature_RechazaRedireccion307AOtroOrigen(t *testing.T) {
	origen, _, visitas := servidoresRedireccion(t, http.StatusTemporaryRedirect)
	e := New(origen.Client())
	_ = e.UploadSignature(context.Background(), domain.ExchangeSession{
		RequestID: "r", RetrieveEndpoint: origen.URL + "/rt", UploadEndpoint: origen.URL + "/st", State: domain.SessionActive,
	}, []byte("cert"), []byte("firma"))
	if n := visitas.Load(); n != 0 {
		t.Fatalf("el destino de la redirección recibió %d peticiones con la firma", n)
	}
}

func TestPoliticaRedireccion(t *testing.T) {
	politica := politicaRedireccionTrifasica(nil)
	pedir := func(raw string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		return r
	}
	via := []*http.Request{pedir("https://a.example/x")}
	if err := politica(pedir("https://a.example/y"), via); err != nil {
		t.Fatalf("misma origen debe admitirse: %v", err)
	}
	for _, destino := range []string{"https://b.example/y", "http://a.example/y", "https://a.example:8443/y"} {
		if err := politica(pedir(destino), via); err == nil {
			t.Fatalf("redirección a %s debía rechazarse", destino)
		}
	}
	largo := make([]*http.Request, maxRedireccionesTrifasicas)
	for i := range largo {
		largo[i] = pedir("https://a.example/x")
	}
	if err := politica(pedir("https://a.example/y"), largo); err == nil {
		t.Fatal("demasiadas redirecciones debían rechazarse")
	}
}
