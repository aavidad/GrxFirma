// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"io"
	"net/http"
	"net/http/httptest"
)

// tokenPruebasREST es la credencial de las pruebas: authorize falla cerrado
// sin credencial configurada (B-04).
const tokenPruebasREST = "token-pruebas-rest"

func nuevoAdaptadorPruebas(firmar SignDocumentUseCase, verificar VerifySignatureUseCase, seleccionar SelectCertificateUseCase) *Adaptador {
	return New(firmar, verificar, seleccionar).WithBearerToken(tokenPruebasREST)
}

func nuevaPeticionPruebas(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer "+tokenPruebasREST)
	return req
}
