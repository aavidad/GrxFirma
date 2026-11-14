// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"io"
	"net/http"
	"net/http/httptest"

	"grxfirma/internal/adapters/inbound/common/rest"
)

const tokenPruebasREST = "token-pruebas-rest"

func nuevoAdaptadorPruebas(firmar rest.SignDocumentUseCase, verificar rest.VerifySignatureUseCase, seleccionar rest.SelectCertificateUseCase) *rest.Adaptador {
	return rest.New(firmar, verificar, seleccionar).WithBearerToken(tokenPruebasREST)
}

func nuevaPeticionPruebas(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer "+tokenPruebasREST)
	return req
}
