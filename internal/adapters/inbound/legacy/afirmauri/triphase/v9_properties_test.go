// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"encoding/base64"
	"errors"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestV9PropiedadesJavaExigenTSAHTTP(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	for name, raw := range map[string]string{
		"espacio":      "tsaURL http://tsa.invalid/tsr",
		"tabulador":    "tsaURL\t:\thttp://tsa.invalid/tsr",
		"continuacion": "tsa\\\nURL=http://tsa.invalid/tsr",
		"crlf":         "tsa\\\r\n  URL=http://tsa.invalid/tsr",
		"cr":           "mode=implicit\rtsaURL=http://tsa.invalid/tsr",
		"unicode":      `\u0074saURL=http\://tsa.invalid/tsr`,
	} {
		t.Run(name, func(t *testing.T) {
			var block *excepciones.Block
			err := requireTriphaseTSAProperties(context.Background(), base64.StdEncoding.EncodeToString([]byte(raw)))
			if !errors.As(err, &block) || block.ID != "tsa_http" {
				t.Fatalf("Java ve tsaURL HTTP sin bloqueo: %q err=%v", raw, err)
			}
		})
	}
}
func TestV9ServidorCompruebaPropiedadesEnviadasSinReescribir(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	excepciones.SetForTest(t, "tsa_http", false)
	requests := 0
	var sent string
	e := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		data, err := base64.URLEncoding.DecodeString(r.Form.Get("params"))
		if err != nil {
			t.Error(err)
		}
		sent = string(data)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ERR-11:fin"))}, nil
	})}).ConValidadorDestino(validadorQueAdmiteTodo)
	ctx, warnings := domain.ConAvisosOperacion(context.Background())
	ctx = ContextWithSigningKey(ctx, &mockLegacyBatchKey{chain: [][]byte{[]byte("cert-der")}})
	req := SolicitudFirmaServidor{ServerURL: "https://servidor.example/t", FormatoLegacy: "CAdEStri", Datos: []byte("doc"), ExtraParams: map[string]string{"mode": "implicit\rtsaURL http://tsa.invalid/tsr"}}
	_, err := e.FirmarConServidor(ctx, req)
	var block *excepciones.Block
	if !errors.As(err, &block) || block.ID != "tsa_http" || requests != 0 {
		t.Fatalf("propiedades enviadas sin permiso: %v peticiones=%d", err, requests)
	}
	excepciones.SetForTest(t, "tsa_http", true)
	_, err = e.FirmarConServidor(ctx, req)
	if errors.As(err, &block) || requests != 1 {
		t.Fatalf("con permiso: %v peticiones=%d", err, requests)
	}
	if sent != "mode=implicit\rtsaURL http://tsa.invalid/tsr\n" {
		t.Fatalf("se reescribieron propiedades: %q", sent)
	}
	if len(warnings.Claves()) != 1 || warnings.Claves()[0] != excepciones.Warning("tsa_http") {
		t.Fatal(warnings.Claves())
	}
}
