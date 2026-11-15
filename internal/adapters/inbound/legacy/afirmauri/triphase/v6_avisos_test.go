// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"grxfirma/internal/adapters/outbound/common/localizador"
	avisosglobal "grxfirma/internal/security/avisos"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// Con la excepción activa y sin colector en el contexto (como el orquestador
// y el manejador WebSocket de grxfirmauri), ¿queda algún aviso visible?
func TestV6TrifasicaHTTPPermitidaSinColectorAvisa(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	machinepolicy.UnsetForTest(t, "permitir_trifasica_http")
	excepciones.SetForTest(t, "tsa_http", true)
	excepciones.SetForTest(t, "trifasica_http", true)
	avisosglobal.Reiniciar()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"dat":"ZG9j","format":"CAdES","algorithm":"SHA256withRSA","extraParams":"tsaURL=http://tsa.example.invalid/tsr"}`
		if r.URL.Query().Get("op") != "get" {
			body = "fallo"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	session := sesionTest("http://servidor.example/retrieve", "http://servidor.example/upload", "")
	result, err := New(client).Execute(context.Background(), session, jobTest())
	t.Logf("err=%v avisos_globales=%v", err, avisosglobal.Todos())
	for _, id := range []string{"tsa_http", "trifasica_http"} {
		found := false
		for _, aviso := range avisosglobal.Todos() {
			if aviso.Titulo == localizador.Detectar().T(excepciones.Warning(id)) {
				found = true
			}
		}
		if !found || !slices.Contains(result.Warnings, excepciones.Warning(id)) {
			t.Fatalf("se pierde %s: global=%v resultado=%v", id, found, result.Warnings)
		}
	}
}
