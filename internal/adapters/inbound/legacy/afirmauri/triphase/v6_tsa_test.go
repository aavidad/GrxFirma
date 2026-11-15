// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestV6FirmarConServidorTSAHTTPSinAjuste(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	var bodies []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ERR-11:fin")), Header: http.Header{}}, nil
	})}
	e := New(client).ConValidadorDestino(validadorQueAdmiteTodo)
	ctx, avisos := domain.ConAvisosOperacion(context.Background())
	ctx = ContextWithSigningKey(ctx, &mockLegacyBatchKey{chain: [][]byte{[]byte("cert-der")}})
	_, err := e.FirmarConServidor(ctx, SolicitudFirmaServidor{
		ServerURL: "https://servidor.example/triphaseSignService", FormatoLegacy: "CAdEStri",
		Accion: domain.ActionSign, Algoritmo: "SHA256withRSA", Datos: []byte("doc"),
		ExtraParams: map[string]string{"tsaURL": "http://tsa.example.invalid/tsr", "signatureLevel": "T"},
	})
	var block *excepciones.Block
	t.Logf("err=%v bloqueo_tsa=%v peticiones=%d avisos=%v", err, errors.As(err, &block) && block.ID == "tsa_http", len(bodies), avisos.Claves())
	for _, b := range bodies {
		if i := strings.Index(b, "params="); i >= 0 {
			v := b[i+7:]
			if j := strings.Index(v, "&"); j >= 0 {
				v = v[:j]
			}
			raw, _ := base64.URLEncoding.DecodeString(v)
			t.Logf("params enviados al servidor: %q", raw)
		}
	}
	if len(bodies) > 0 {
		t.Errorf("HUECO: la tsaURL http se envió al servidor trifásico sin tsa_http")
	}
}

func TestV6ServidorTSAHTTPDecision(t *testing.T) {
	for _, mode := range []string{"apagado", "permitido", "maquina", "ilegible"} {
		t.Run(mode, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, "permitir_tsa_http")
			excepciones.SetForTest(t, "tsa_http", mode != "apagado")
			if mode == "maquina" {
				machinepolicy.SetForTest(t, "permitir_tsa_http", false)
			}
			if mode == "ilegible" {
				machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
			}
			count := 0
			e := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				count++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ERR-11:fin")), Header: http.Header{}}, nil
			})}).ConValidadorDestino(validadorQueAdmiteTodo)
			ctx, warnings := domain.ConAvisosOperacion(context.Background())
			ctx = ContextWithSigningKey(ctx, &mockLegacyBatchKey{chain: [][]byte{[]byte("cert-der")}})
			_, err := e.FirmarConServidor(ctx, SolicitudFirmaServidor{ServerURL: "https://servidor.example/triphase", FormatoLegacy: "CAdEStri", Datos: []byte("documento"), ExtraParams: map[string]string{"tSaUrL": "http://tsa.example.invalid/tsr"}})
			var block *excepciones.Block
			if mode == "permitido" {
				if errors.As(err, &block) || count != 1 {
					t.Fatalf("permitido: count=%d err=%v", count, err)
				}
				if !slices.Contains(warnings.Claves(), excepciones.Warning("tsa_http")) {
					t.Fatal(warnings.Claves())
				}
			} else if !errors.As(err, &block) || block.ID != "tsa_http" || count != 0 {
				t.Fatalf("bloqueo %s: count=%d err=%v", mode, count, err)
			}
		})
	}
}

func TestV6LoteXMLTSAHTTPSeControlaAntesDePrefirma(t *testing.T) {
	for _, global := range []bool{false, true} {
		for _, mode := range []string{"apagado", "permitido", "maquina", "ilegible"} {
			t.Run(fmt.Sprintf("global=%v/%s", global, mode), func(t *testing.T) {
				machinepolicy.UnsetForTest(t, "permitir_tsa_http")
				excepciones.SetForTest(t, "tsa_http", mode != "apagado")
				if mode == "maquina" {
					machinepolicy.SetForTest(t, "permitir_tsa_http", false)
				}
				if mode == "ilegible" {
					machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
				}
				count := 0
				e := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					count++
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ERR-11:fin")), Header: http.Header{}}, nil
				})}).ConValidadorDestino(validadorQueAdmiteTodo)
				extra := "<extraparams>" + base64.StdEncoding.EncodeToString([]byte("tsaURL=http://tsa.example.invalid/tsr")) + "</extraparams>"
				sign := "<singlesign Id=\"uno\"><datasource>doc</datasource>"
				raw := "<signbatch algorithm=\"SHA256withRSA\">"
				if global {
					raw += extra + sign
				} else {
					raw += sign + extra
				}
				raw += "</singlesign></signbatch>"
				ctx, warnings := domain.ConAvisosOperacion(context.Background())
				ctx = ContextWithSigningKey(ctx, &mockLegacyBatchKey{chain: [][]byte{[]byte("cert-der")}})
				_, err := e.executeLegacyRemoteBatchResult(ctx, afirmauri.RemoteBatchCommand{Payload: []byte(raw), PreSignEndpoint: "https://servidor.example/pre", PostSignEndpoint: "https://servidor.example/post"}, false)
				var block *excepciones.Block
				if mode == "permitido" {
					if errors.As(err, &block) || count == 0 || !slices.Contains(warnings.Claves(), excepciones.Warning("tsa_http")) {
						t.Fatalf("permitido: count=%d err=%v avisos=%v", count, err, warnings.Claves())
					}
				} else if !errors.As(err, &block) || block.ID != "tsa_http" || count != 0 {
					t.Fatalf("bloqueado: count=%d err=%v", count, err)
				}
			})
		}
	}
}

func TestV6TSAPropiedadesNoPierdeAliasesRepetidos(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	raw := "tsaURL=http://tsa.example.invalid/tsr\ntsaurl=https://otra.example/tsr"
	// Java distingue esas claves; la minúscula final no anula la tsaURL del portal.
	xml := []byte("<signbatch><singlesign Id=\"uno\"><extraparams>" + base64.StdEncoding.EncodeToString([]byte(raw)) + "</extraparams></singlesign></signbatch>")
	var block *excepciones.Block
	if err := requireXMLBatchTSAOptions(context.Background(), xml); !errors.As(err, &block) || block.ID != "tsa_http" {
		t.Fatalf("el alias posterior oculta TSA HTTP: %v", err)
	}
}
