// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/domain"
)

// Servidor trifásico simulado con el protocolo de @firma/FIRe: comprueba los
// parámetros de pre y post y que el PK1 es la firma local del PRE.
func TestFirmarConServidor_ProtocoloDeFIRe(t *testing.T) {
	pre := []byte("atributos-firmados-cades")
	docID := []byte("fc00c581-b871-458f-8972-b7d0ff396648")
	var postVisto bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		// Como el servicio real, los parámetros solo se leen del cuerpo.
		if r.URL.RawQuery != "" {
			t.Errorf("los parámetros deben ir en el cuerpo, no en la URL: %s", r.URL.RawQuery)
		}
		r.Form = r.PostForm
		if r.Form.Get("format") != "CAdES" || r.Form.Get("algo") != "SHA512withRSA" || r.Form.Get("cop") != "sign" {
			t.Fatalf("parámetros inesperados: %v", r.Form)
		}
		if r.Form.Get("doc") != base64.URLEncoding.EncodeToString(docID) || r.Form.Get("cert") == "" {
			t.Fatalf("doc/cert inesperados: %v", r.Form)
		}
		switch r.Form.Get("op") {
		case "pre":
			td := `<xml><firmas format="CAdES"><firma Id="1"><param n="PRE">` + base64.StdEncoding.EncodeToString(pre) + `</param></firma></firmas></xml>`
			_, _ = w.Write([]byte(base64.URLEncoding.EncodeToString([]byte(td))))
		case "post":
			postVisto = true
			raw, err := base64.URLEncoding.DecodeString(r.Form.Get("session"))
			if err != nil {
				t.Fatalf("session no es Base64 URL-safe: %v", err)
			}
			var td legacyXMLTriphaseData
			if err := xml.Unmarshal(raw, &td); err != nil {
				t.Fatal(err)
			}
			h := sha512.Sum512(pre)
			esperado := base64.StdEncoding.EncodeToString(append([]byte("pk1:"), h[:]...))
			encontrado := false
			for _, p := range td.Firmas.Firmas[0].Params {
				if p.Name == "PK1" && p.Value == esperado {
					encontrado = true
				}
			}
			if !encontrado {
				t.Fatalf("falta el PK1 correcto: %s", raw)
			}
			_, _ = w.Write([]byte("OK NEWID=" + base64.URLEncoding.EncodeToString([]byte("FIRMA-FINAL"))))
		default:
			t.Fatalf("op inesperada: %q", r.Form.Get("op"))
		}
	}))
	defer srv.Close()

	e := New(srv.Client()).ConValidadorDestino(validadorQueAdmiteTodo)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{chain: [][]byte{[]byte("cert-der")}})
	firma, err := e.FirmarConServidor(ctx, SolicitudFirmaServidor{
		ServerURL: srv.URL + "/public/afirma/triphaseSignService", FormatoLegacy: "CAdEStri",
		Accion: domain.ActionSign, Algoritmo: "SHA512withRSA", Datos: docID,
		ExtraParams: map[string]string{"serverUrl": srv.URL, "precalculatedHashAlgorithm": "SHA-512"},
	})
	if err != nil {
		t.Fatalf("FirmarConServidor: %v", err)
	}
	if string(firma) != "FIRMA-FINAL" || !postVisto {
		t.Fatalf("firma = %q, post=%v", firma, postVisto)
	}
}

func TestFirmarConServidor_ErroresDelServidorYHTTP(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ERR-11:Error en la prefirma"))
	}))
	defer srv.Close()
	e := New(srv.Client()).ConValidadorDestino(validadorQueAdmiteTodo)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{chain: [][]byte{[]byte("c")}})
	_, err := e.FirmarConServidor(ctx, SolicitudFirmaServidor{ServerURL: srv.URL, FormatoLegacy: "XAdEStri", Datos: []byte("d")})
	if err == nil || !strings.Contains(err.Error(), "rechazó la prefirma") {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.FirmarConServidor(ctx, SolicitudFirmaServidor{ServerURL: "http://inseguro.example/x", FormatoLegacy: "CAdEStri", Datos: []byte("d")}); err == nil {
		t.Fatal("un servidor trifásico sin HTTPS debe rechazarse")
	}
}

func validadorQueAdmiteTodo(context.Context, string) error { return nil }

// A-01: el servidor trifásico tiene que pasar la política en el punto de uso
// (los manifiestos recuperados también traen serverUrl).
func TestFirmarConServidor_ValidadorRechazaSinPeticiones(t *testing.T) {
	var peticiones atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		peticiones.Add(1)
		_, _ = w.Write([]byte("ERR-1"))
	}))
	defer srv.Close()
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{chain: [][]byte{[]byte("c")}})
	req := SolicitudFirmaServidor{ServerURL: srv.URL + "/ts", FormatoLegacy: "CAdEStri", Datos: []byte("d")}

	rechazo := errors.New("el origen no esta autorizado")
	var visto string
	e := New(srv.Client()).ConValidadorDestino(func(_ context.Context, origen string) error {
		visto = origen
		return rechazo
	})
	if _, err := e.FirmarConServidor(ctx, req); !errors.Is(err, rechazo) {
		t.Fatalf("err = %v; se esperaba el rechazo del validador", err)
	}
	if visto != srv.URL {
		t.Fatalf("el validador recibió %q; se esperaba el origen %q", visto, srv.URL)
	}
	// Sin validador configurado se falla cerrado.
	if _, err := New(srv.Client()).FirmarConServidor(ctx, req); err == nil {
		t.Fatal("sin validador de destino debía fallar")
	}
	if n := peticiones.Load(); n != 0 {
		t.Fatalf("el servidor recibió %d peticiones; se esperaban 0", n)
	}
}

// A-01: una aprobación es una firma; una prefirma con dos <firma> se rechaza
// sin firmar nada ni llamar a la postfirma.
func TestFirmarConServidor_RechazaVariasFirmasEnLaPrefirma(t *testing.T) {
	var post atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("op") == "post" {
			post.Add(1)
			_, _ = w.Write([]byte("OK NEWID=" + base64.URLEncoding.EncodeToString([]byte("F"))))
			return
		}
		pre := base64.StdEncoding.EncodeToString([]byte("pre"))
		td := `<xml><firmas format="CAdES"><firma Id="1"><param n="PRE">` + pre + `</param></firma><firma Id="2"><param n="PRE">` + pre + `</param></firma></firmas></xml>`
		_, _ = w.Write([]byte(base64.URLEncoding.EncodeToString([]byte(td))))
	}))
	defer srv.Close()
	clave := &mockLegacyBatchPreKey{mockLegacyBatchKey: mockLegacyBatchKey{chain: [][]byte{[]byte("c")}}}
	ctx := ContextWithSigningKey(context.Background(), clave)
	e := New(srv.Client()).ConValidadorDestino(validadorQueAdmiteTodo)
	_, err := e.FirmarConServidor(ctx, SolicitudFirmaServidor{ServerURL: srv.URL, FormatoLegacy: "CAdEStri", Datos: []byte("d")})
	if err == nil {
		t.Fatal("una prefirma con dos <firma> debía rechazarse")
	}
	if len(clave.opts) != 0 {
		t.Fatalf("la clave firmó %d veces; no debía firmar", len(clave.opts))
	}
	if post.Load() != 0 {
		t.Fatal("no debía llamarse a la postfirma")
	}
}
