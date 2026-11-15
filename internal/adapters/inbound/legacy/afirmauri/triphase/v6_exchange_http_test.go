// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"errors"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestV6IntercambioHTTPSiempreExigePermiso(t *testing.T) {
	for _, method := range []string{"retrieve", "signature", "certificate"} {
		t.Run(method, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, "permitir_trifasica_http")
			excepciones.SetForTest(t, "trifasica_http", false)
			count := 0
			e := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				count++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK")), Header: http.Header{}}, nil
			})})
			session := sesionTest("http://servidor.example/rt", "http://servidor.example/st", "")
			run := func(ctx context.Context) error {
				switch method {
				case "retrieve":
					_, err := e.RetrieveRaw(ctx, session)
					return err
				case "signature":
					return e.UploadSignature(ctx, session, []byte("cert"), []byte("sig"))
				default:
					return e.UploadCertificate(ctx, session, []byte("cert"), nil)
				}
			}
			ctx, warnings := domain.ConAvisosOperacion(context.Background())
			var block *excepciones.Block
			if err := run(ctx); !errors.As(err, &block) || block.ID != "trifasica_http" || count != 0 {
				t.Fatalf("sin permiso: peticiones=%d err=%v", count, err)
			}
			excepciones.SetForTest(t, "trifasica_http", true)
			if err := run(ctx); err != nil || count < 1 || !slices.Contains(warnings.Claves(), excepciones.Warning("trifasica_http")) {
				t.Fatalf("con permiso: peticiones=%d err=%v avisos=%v", count, err, warnings.Claves())
			}
		})
	}
}
