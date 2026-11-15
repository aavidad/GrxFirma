// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestV7SelectCertHTTPSNoNecesitaInstalarCAMantieneLaExistente(t *testing.T) {
	previous, previousPresent := ensureStartupLocalTLSTrust, presentStartupTLSTrustFailure
	previousNotice := getStartupTLSTrustNotice()
	t.Cleanup(func() {
		ensureStartupLocalTLSTrust = previous
		presentStartupTLSTrustFailure = previousPresent
		setStartupTLSTrustNotice(previousNotice)
	})
	presentStartupTLSTrustFailure = func(string) { t.Error("selectcert no debe mostrar un diálogo de instalación de CA") }
	for _, policy := range []string{"apagada", "permitida", "prohibida", "ilegible"} {
		t.Run(policy, func(t *testing.T) {
			home := t.TempDir()
			setTestUserHome(t, home)
			dir := testProtocolConfigDir(home)
			tlsDir := filepath.Join(dir, "tls")
			if err := os.MkdirAll(tlsDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost-root.crt.pem"), []byte("CA ya instalada de prueba"), 0600); err != nil {
				t.Fatal(err)
			}
			before := snapshotHomeV6(t, tlsDir)
			machinepolicy.UnsetForTest(t, "permitir_ca_instalacion_automatica")
			excepciones.SetForTest(t, "ca_instalacion_automatica", policy != "apagada")
			if policy == "prohibida" {
				machinepolicy.SetForTest(t, "permitir_ca_instalacion_automatica", false)
			}
			if policy == "ilegible" {
				machinepolicy.ErrorForTest(t, "permitir_ca_instalacion_automatica", errors.New("ilegible"))
			}
			calls := 0
			ensureStartupLocalTLSTrust = func(context.Context, string) (bool, error) { calls++; return false, errors.New("no se necesita CA") }
			posts := 0
			client := &http.Client{Transport: runtimeProxyRoundTrip(func(r *http.Request) (*http.Response, error) {
				posts++
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.URL.Scheme != "https" || !r.Form.Has("dat") {
					t.Error("subida de certificado no esperada", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("OK"))}, nil
			})}
			var output bytes.Buffer
			code := runConDependencias(context.Background(), []string{"afirma://selectcert?id=uno&stservlet=https%3A%2F%2Fportal.example%2Fst"}, &output, func(ctx context.Context, _ io.Writer, _ string) int {
				if err := triphase.New(client).UploadCertificate(ctx, domain.ExchangeSession{RequestID: "uno", UploadEndpoint: "https://portal.example/st"}, []byte("cert-der"), nil); err != nil {
					t.Error(err)
					return 1
				}
				return 0
			})
			if code != 0 || calls != 0 || posts != 1 || strings.Contains(output.String(), "ca_instalacion_automatica") {
				t.Fatalf("selectcert innecesariamente depende de CA: code=%d ca=%d posts=%d salida=%s", code, calls, posts, output.String())
			}
			if !reflect.DeepEqual(before, snapshotHomeV6(t, tlsDir)) {
				t.Fatal("se cambió la CA existente")
			}
		})
	}
}

func TestV7SelectCertHTTP_SinPermisoDevuelveTrifasicaNoCA(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	for _, id := range []string{"ca_instalacion_automatica", "trifasica_http"} {
		e, _ := excepciones.Find(id)
		machinepolicy.UnsetForTest(t, e.PolicyKey)
		excepciones.SetForTest(t, id, false)
	}
	posts := 0
	client := &http.Client{Transport: runtimeProxyRoundTrip(func(*http.Request) (*http.Response, error) {
		posts++
		return nil, errors.New("no debe salir la petición")
	})}
	var operationErr error
	var output bytes.Buffer
	code := runConDependencias(context.Background(), []string{"afirma://selectcert?id=uno&stservlet=http%3A%2F%2Fportal.example%2Fst"}, &output, func(ctx context.Context, _ io.Writer, _ string) int {
		operationErr = triphase.New(client).UploadCertificate(ctx, domain.ExchangeSession{RequestID: "uno", UploadEndpoint: "http://portal.example/st"}, []byte("cert-der"), nil)
		if operationErr != nil {
			return 1
		}
		return 0
	})
	var block *excepciones.Block
	if code != 1 || !errors.As(operationErr, &block) || block.ID != "trifasica_http" || posts != 0 {
		t.Fatalf("bloqueo incorrecto: code=%d err=%v posts=%d", code, operationErr, posts)
	}
	t.Logf("bloqueo real=%s peticiones=%d", block.ID, posts)
	if strings.Contains(output.String(), "ca_instalacion_automatica") {
		t.Fatal("se atribuye a la CA el bloqueo de HTTP", output.String())
	}
}
