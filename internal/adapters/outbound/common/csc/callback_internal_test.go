// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallbackRechazaHostRutaYMetodoAjenos(t *testing.T) {
	c := &Cliente{opc: Opciones{TextoCallback: "ok"}}
	var recibidos []resultadoCallback
	h := c.manejadorCallback("127.0.0.1:5555", "estado", func(r resultadoCallback) { recibidos = append(recibidos, r) })
	casos := []struct {
		metodo, destino, host string
		esperado              int
	}{
		{http.MethodGet, "/otra?state=estado&code=x", "127.0.0.1:5555", http.StatusNotFound},
		{http.MethodPost, "/callback?state=estado&code=x", "127.0.0.1:5555", http.StatusNotFound},
		{http.MethodGet, "/callback?state=estado&code=x", "malicioso.example:5555", http.StatusBadRequest},
	}
	for _, caso := range casos {
		req := httptest.NewRequest(caso.metodo, caso.destino, nil)
		req.Host = caso.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != caso.esperado {
			t.Fatalf("%s %s host=%s: %d, se esperaba %d", caso.metodo, caso.destino, caso.host, w.Code, caso.esperado)
		}
	}
	if len(recibidos) != 0 {
		t.Fatalf("ninguna de estas peticiones debe entregar un código: %d", len(recibidos))
	}
	req := httptest.NewRequest(http.MethodGet, "/callback?state=estado&code=abc", nil)
	req.Host = "127.0.0.1:5555"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || len(recibidos) != 1 || recibidos[0].codigo != "abc" {
		t.Fatalf("callback válido: %d %+v", w.Code, recibidos)
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("faltan cabeceras de protección: %v", w.Header())
	}
}

func TestCadenaJSONSinCopia(t *testing.T) {
	for entrada, esperado := range map[string]string{`"abc.DEF-123"`: "abc.DEF-123", `"a\/b"`: "a/b"} {
		got, err := cadenaJSONSinCopia([]byte(entrada))
		if err != nil || string(got) != esperado {
			t.Fatalf("%s: %q %v", entrada, got, err)
		}
	}
	for _, mala := range []string{`""`, `123`, `"a`, "\"a\x01b\""} {
		if _, err := cadenaJSONSinCopia([]byte(mala)); err == nil {
			t.Fatalf("%q debió rechazarse", mala)
		}
	}
}
