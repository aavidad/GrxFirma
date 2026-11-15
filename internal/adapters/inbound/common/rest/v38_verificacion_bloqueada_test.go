// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

// verificadorSHA1RESTV38 pide el ajuste como el motor y deja la causa tipada.
type verificadorSHA1RESTV38 struct{ firmante string }

func (v verificadorSHA1RESTV38) Execute(ctx context.Context, _ application.VerifyCommand) (application.VerifyResult, error) {
	result := domain.NewVerificationFailure("CAdES", "firma no evaluada", nil)
	if err := excepciones.Check("sha1_leer", ctx); err != nil {
		result = result.WithPolicyError(err)
		result.Reason = err.Error()
	}
	result.SignerSummaries = []domain.VerificationSignerSummary{{ID: "1", Subject: v.firmante}}
	return application.VerifyResult{Verification: result.Normalize()}, nil
}

// REST es un canal remoto: nunca ofrece activar una excepción (solo se activa
// en la app local), ni por el bloqueo real ni por un texto que lo imite. Lo que
// devuelve tampoco lleva la marca interna del bloqueo.
func TestV38RESTVerificacionBloqueadaNoOfreceActivarNiPintaLaMarca(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_sha1_leer")
	for name, firmante := range map[string]string{
		"bloqueo real":            "CN=Firmante de 2012",
		"marca en el certificado": "CN=Ana [sha1_leer; excepciones.sha1_leer.bloqueo]",
	} {
		t.Run(name, func(t *testing.T) {
			if name == "marca en el certificado" {
				excepciones.SetForTest(t, "sha1_leer", true)
			}
			adaptador := nuevoAdaptadorPruebas(nil, verificadorSHA1RESTV38{firmante: firmante}, nil).WithConfigDir(t.TempDir())
			rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/verify", map[string]any{
				"name": "firma.csig", "mime_type": "application/pkcs7-signature",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("firma")),
			})
			body := rr.Body.String()
			if rr.Code != http.StatusOK {
				t.Fatalf("código %d: %s", rr.Code, body)
			}
			for _, campo := range []string{"exceptionId", "exception_id", "retryId", "unsafe_exception", `"activable":true`} {
				if strings.Contains(body, campo) {
					t.Fatalf("REST ofrece una excepción (%s): %s", campo, body)
				}
			}
			if name == "bloqueo real" && (strings.Contains(body, "[sha1_leer;") || !strings.Contains(body, `"valid":false`)) {
				t.Fatalf("el bloqueo debe salir como no válido y sin la marca: %s", body)
			}
		})
	}
}
