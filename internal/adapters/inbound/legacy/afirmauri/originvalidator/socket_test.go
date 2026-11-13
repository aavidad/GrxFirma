// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package originvalidator_test

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/domain"
)

// Lote 13c (A-03): por el canal socket, un destino http de intranet vuelve a
// admitirse si la política lo autoriza; Validate (camino directo) no cambia.
func TestValidateSocket_HTTPDeIntranetAutorizado(t *testing.T) {
	const intranet = "http://intranet.example:8080"
	v := originvalidator.New(&mockPolicy{decisions: map[string]domain.TrustStatus{intranet: domain.TrustAllowed}})
	if err := v.ValidateSocket(context.Background(), intranet); err != nil {
		t.Fatalf("socket: http de intranet autorizado debía admitirse: %v", err)
	}
	if err := v.Validate(context.Background(), intranet); !errors.Is(err, originvalidator.ErrHTTPSoloLocalhost) {
		t.Fatalf("directo: debía seguir rechazándose por formato, err = %v", err)
	}
	sinCifrar, err := originvalidator.ValidarFormatoSocket(intranet)
	if err != nil || !sinCifrar {
		t.Fatalf("ValidarFormatoSocket = %v, %v", sinCifrar, err)
	}
	if sinCifrar, _ := originvalidator.ValidarFormatoSocket("http://127.0.0.1:8080"); sinCifrar {
		t.Fatal("loopback no cuenta como destino sin cifrar")
	}
}

// Lo que no se relaja: sin autorización de la política, con credenciales o
// con ruta, el destino http sigue rechazado.
func TestValidateSocket_HTTPNoAutorizadoSigueRechazado(t *testing.T) {
	v := originvalidator.New(&mockPolicy{decisions: map[string]domain.TrustStatus{
		"http://intranet.example:8080": domain.TrustDenied,
	}})
	for _, origen := range []string{
		"http://intranet.example:8080",      // denegado
		"http://otro.example",               // pendiente (sin decisión)
		"http://u:p@intranet.example:8080",  // credenciales
		"http://intranet.example:8080/ruta", // ruta
		"ftp://intranet.example",            // esquema
	} {
		if err := v.ValidateSocket(context.Background(), origen); err == nil {
			t.Fatalf("%s debía rechazarse", origen)
		}
	}
}
