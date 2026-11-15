// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// Lote 39a: REST no ofrece activar lo inseguro, pero tampoco puede dar por
// buena una verificación que quedó sin evaluar por un bloqueo. Responde 200,
// como hasta ahora (los clientes ya leen valid:false), sin «ok»:true y con el
// motivo en «error». Una verificación normal conserva ok:true.
func TestV39RESTVerificacionBloqueadaNoEsExito(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	for name, activo := range map[string]bool{"bloqueada": false, "con ajuste": true} {
		t.Run(name, func(t *testing.T) {
			excepciones.SetForTest(t, "sha1_leer", activo)
			adaptador := nuevoAdaptadorPruebas(nil, verificadorSHA1RESTV38{firmante: "CN=Firmante de 2012"}, nil).WithConfigDir(t.TempDir())
			rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/verify", map[string]any{
				"name": "firma.csig", "mime_type": "application/pkcs7-signature",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("firma")),
			})
			if rr.Code != http.StatusOK {
				t.Fatalf("código %d: %s", rr.Code, rr.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if !activo {
				if body["ok"] == true || body["valid"] != false {
					t.Fatalf("un bloqueo no es un éxito: %s", rr.Body.String())
				}
				if msg, _ := body["error"].(string); strings.TrimSpace(msg) == "" {
					t.Fatalf("falta el motivo del bloqueo en error: %s", rr.Body.String())
				}
				return
			}
			if body["ok"] != true {
				t.Fatalf("una verificación sin bloqueo sigue con ok:true: %s", rr.Body.String())
			}
			if _, hay := body["error"]; hay {
				t.Fatalf("sin bloqueo no hay error: %s", rr.Body.String())
			}
		})
	}
}
