// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/application"
)

type proteccionRESTConAvisos struct{ avisos []string }

func (m proteccionRESTConAvisos) Execute(ctx context.Context, cmd application.ProtectCommand) (application.ProtectResult, error) {
	res, err := (&protectUseCaseMock{}).Execute(ctx, cmd)
	res.Warnings = m.avisos
	return res, err
}

type exportacionRESTConAvisos struct{ avisos []string }

func (m exportacionRESTConAvisos) Execute(ctx context.Context, cmd application.ExportProtectionRecipientCommand) (application.ExportProtectionRecipientResult, error) {
	res, err := (exportProtectionRecipientUseCaseMock{}).Execute(ctx, cmd)
	res.Warnings = m.avisos
	return res, err
}

func TestProteccionREST_AvisosOpcionalesYTraducidos(t *testing.T) {
	for _, idioma := range []string{"es", "en", "fr"} {
		for _, conAvisos := range []bool{false, true} {
			var avisos []string
			if conAvisos {
				avisos = []string{protector.AvisoProteccionAltoSinAlmacen}
			}
			a := nuevoAdaptadorPruebas(nil, nil, nil).WithProtection(proteccionRESTConAvisos{avisos}, nil, nil).WithIntercambioProteccion(exportacionRESTConAvisos{avisos}, nil)
			for _, operacion := range []struct{ method, path, body string }{
				{http.MethodPost, "/protect", `{"name":"acta.txt","content_base64":"YQ==","profile":"alto","recipient_ids":["propio"]}`},
				{http.MethodGet, "/protection/recipient/export?id=propio", ""},
			} {
				req := nuevaPeticionPruebas(operacion.method, operacion.path, bytes.NewBufferString(operacion.body))
				q := req.URL.Query()
				q.Set("lang", idioma)
				req.URL.RawQuery = q.Encode()
				rr := httptest.NewRecorder()
				a.Routes().ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					t.Fatalf("%s: código %d: %s", operacion.path, rr.Code, rr.Body.String())
				}
				var campos map[string]json.RawMessage
				if err := json.Unmarshal(rr.Body.Bytes(), &campos); err != nil {
					t.Fatal(err)
				}
				raw, present := campos["warnings"]
				if present != conAvisos {
					t.Fatalf("%s: warnings presente=%v: %s", operacion.path, present, rr.Body.String())
				}
				if conAvisos {
					var got []string
					if err := json.Unmarshal(raw, &got); err != nil {
						t.Fatal(err)
					}
					if want := []string{localizador.Para(idioma).T(protector.AvisoProteccionAltoSinAlmacen)}; !slices.Equal(got, want) {
						t.Fatalf("%s (%s): warnings=%q; want %q", operacion.path, idioma, got, want)
					}
				}
			}
		}
	}
}
