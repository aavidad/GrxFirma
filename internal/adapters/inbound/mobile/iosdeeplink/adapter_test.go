// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package iosdeeplink

import (
	"context"
	"encoding/base64"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
)

func TestHandle_LegacyAfirmaURIRemota(t *testing.T) {
	adaptador := New(afirmauri.New(trustStub{estado: domain.TrustAllowed}))

	solicitud, err := adaptador.Handle(context.Background(),
		"afirma://batch?fileId=req-2&retrieveServlet=https%3A%2F%2Fretrieve.example%2FRetrieveService&storageServlet=https%3A%2F%2Fstore.example%2FStorageService")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.RetrieveCommand == nil {
		t.Fatal("se esperaba RetrieveCommand")
	}
	if solicitud.Origen != "ios-deeplink" {
		t.Fatalf("origen inesperado: %s", solicitud.Origen)
	}
}

func TestHandle_DeepLinkPropioDeFirma(t *testing.T) {
	adaptador := New(afirmauri.New(trustStub{estado: domain.TrustAllowed}))
	raw := "grxfirma://sign?name=demo.xml&mime=application%2Fxml&format=XAdES&dat=" +
		base64.StdEncoding.EncodeToString([]byte("<demo/>"))

	solicitud, err := adaptador.Handle(context.Background(), raw)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if solicitud.SignCommand == nil {
		t.Fatal("se esperaba SignCommand")
	}
	if solicitud.Formato != domain.FormatXAdES {
		t.Fatalf("formato inesperado: %s", solicitud.Formato)
	}
}

type trustStub struct {
	estado domain.TrustStatus
}

func (t trustStub) Evaluate(_ context.Context, origin string) (domain.TrustDecision, error) {
	return domain.TrustDecision{Origin: origin, Status: t.estado}, nil
}
func (t trustStub) Allow(context.Context, string) error  { return nil }
func (t trustStub) Deny(context.Context, string) error   { return nil }
func (t trustStub) Remove(context.Context, string) error { return nil }
