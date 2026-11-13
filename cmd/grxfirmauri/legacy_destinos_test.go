// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// A-03: un lote recuperado por el canal socket no puede mandar la cadena del
// certificado ni las firmas a un pre/post que la política no permite.
func TestLegacyWebSocketHandler_LoteRecuperadoConPresignerNoPermitido(t *testing.T) {
	var peticionesEvil atomic.Int32
	evil := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		peticionesEvil.Add(1)
	}))
	defer evil.Close()

	lote := base64.StdEncoding.EncodeToString([]byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"d1","datareference":"x"}]}`))
	sobre := `<batch><e k="jsonbatch" v="true"/><e k="batchpresignerurl" v="` + evil.URL + `/pre"/><e k="batchpostsignerurl" v="` + evil.URL + `/post"/><e k="dat" v="` + lote + `"/></batch>`
	rt := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sobre))
	}))
	defer rt.Close()

	rechazo := errors.New("el origen no esta autorizado")
	ejecutor := triphase.New(rt.Client()).ConValidadorDestino(func(_ context.Context, origen string) error {
		if origen == evil.URL {
			return rechazo
		}
		return nil
	})
	handler := &legacyWebSocketHandler{retrieve: ejecutor, batchRemote: triphase.NewBatch(ejecutor)}
	sesion := domain.ExchangeSession{RequestID: "r1", RetrieveEndpoint: rt.URL + "/rt", UploadEndpoint: rt.URL + "/st", State: domain.SessionActive}
	_, err := handler.handleBatch(context.Background(), afirmauri.Solicitud{
		Operacion:       afirmauri.OperacionLote,
		Sesion:          sesion,
		RetrieveCommand: &application.RetrieveRequestCommand{Session: sesion},
	})
	if err == nil || !strings.Contains(err.Error(), "no esta autorizado") {
		t.Fatalf("err = %v; se esperaba rechazo del presigner", err)
	}
	if n := peticionesEvil.Load(); n != 0 {
		t.Fatalf("el servidor no permitido recibió %d peticiones", n)
	}
}
