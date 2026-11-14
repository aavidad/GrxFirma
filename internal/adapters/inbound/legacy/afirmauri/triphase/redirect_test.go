// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"errors"
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
	politica := politicaRedireccionTrifasica(nil, nil)
	pedir := func(raw string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		return r
	}
	via := []*http.Request{pedir("https://a.example/x")}
	if err := politica(pedir("https://a.example/y"), via); err != nil {
		t.Fatalf("misma origen debe admitirse: %v", err)
	}
	// Sin validador, otro origen se rechaza (falla cerrado).
	for _, destino := range []string{"https://b.example/y", "http://a.example/y", "https://a.example:8443/y", "ftp://a.example/y"} {
		if err := politica(pedir(destino), via); err == nil {
			t.Fatalf("redirección a %s debía rechazarse", destino)
		}
	}
	// Lote 13c: vuelve el máximo de saltos de net/http (10).
	seis := make([]*http.Request, 6)
	for i := range seis {
		seis[i] = pedir("https://a.example/x")
	}
	if err := politica(pedir("https://a.example/y"), seis); err != nil {
		t.Fatalf("seis saltos en el mismo origen debían admitirse: %v", err)
	}
	largo := make([]*http.Request, maxRedireccionesTrifasicas)
	for i := range largo {
		largo[i] = pedir("https://a.example/x")
	}
	if err := politica(pedir("https://a.example/y"), largo); err == nil {
		t.Fatal("demasiadas redirecciones debían rechazarse")
	}
}

// Lote 13c (A-04): una redirección a otro origen se sigue si ese origen pasa
// el validador de destino; de https a http nunca, aunque el validador lo
// admita, y lo que el validador rechaza sigue rechazado.
func TestPoliticaRedireccion_OtroOrigenSegunValidador(t *testing.T) {
	var validados []string
	e := New(nil).ConValidadorDestino(func(_ context.Context, origen string) error {
		validados = append(validados, origen)
		if origen == "https://malo.example" {
			return errors.New("no autorizado")
		}
		return nil
	})
	politica := politicaRedireccionTrifasica(nil, e)
	pedir := func(raw string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, raw, nil)
		return r
	}
	via := []*http.Request{pedir("https://a.example/x")}
	if err := politica(pedir("https://sso.example/y"), via); err != nil {
		t.Fatalf("otro origen autorizado debía seguirse: %v", err)
	}
	if err := politica(pedir("https://malo.example/y"), via); err == nil {
		t.Fatal("otro origen no autorizado debía rechazarse")
	}
	if err := politica(pedir("http://sso.example/y"), via); err == nil {
		t.Fatal("de https a http debía rechazarse aunque el validador lo admita")
	}
	if err := politica(pedir("http://intranet.example/z"), []*http.Request{pedir("http://intranet.example/x")}); err != nil {
		t.Fatalf("http dentro del mismo origen http debía seguirse: %v", err)
	}
	if len(validados) != 2 || validados[0] != "https://sso.example" || validados[1] != "https://malo.example" {
		t.Fatalf("orígenes validados = %v", validados)
	}
}

func TestRetrieveRaw_SigueRedireccionAOtroOrigenAutorizado(t *testing.T) {
	origen, destino, visitas := servidoresRedireccion(t, http.StatusFound)
	var validados []string
	e := New(origen.Client()).ConValidadorDestino(func(_ context.Context, o string) error {
		validados = append(validados, o)
		return nil
	})
	_, _ = e.RetrieveRaw(context.Background(), domain.ExchangeSession{
		RequestID: "r", RetrieveEndpoint: origen.URL + "/rt", UploadEndpoint: origen.URL + "/st", State: domain.SessionActive,
	})
	if visitas.Load() == 0 {
		t.Fatal("la redirección a un origen autorizado debía seguirse")
	}
	if len(validados) == 0 || validados[0] != destino.URL {
		t.Fatalf("el destino de la redirección debía pasar el validador: %v", validados)
	}
}

func TestUploadSignature_Redireccion307AOtroOrigenRechazadoPorValidador(t *testing.T) {
	origen, destino, visitas := servidoresRedireccion(t, http.StatusTemporaryRedirect)
	e := New(origen.Client()).ConValidadorDestino(func(_ context.Context, o string) error {
		if o == destino.URL {
			return errors.New("no autorizado")
		}
		return nil
	})
	_ = e.UploadSignature(context.Background(), domain.ExchangeSession{
		RequestID: "r", RetrieveEndpoint: origen.URL + "/rt", UploadEndpoint: origen.URL + "/st", State: domain.SessionActive,
	}, []byte("cert"), []byte("firma"))
	if n := visitas.Load(); n != 0 {
		t.Fatalf("el destino no autorizado recibió %d peticiones con la firma", n)
	}
}
