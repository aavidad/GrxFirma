// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmauri

import (
	"net/url"
	"testing"

	"grxfirma/internal/domain"
)

func TestResolveRetrievedSign_XMLActualizaSesionYDocumento(t *testing.T) {
	t.Parallel()

	raw := []byte(`<sign><e k="id" v="req-xml"/><e k="stservlet" v="https%3A%2F%2Fexample.com%2FStorageService"/><e k="format" v="XAdES"/><e k="dat" v="PGRvYz5vazwvZG9jPg=="/></sign>`)
	cmd, solicitud, ok, err := ResolveRetrievedSign(raw, Solicitud{
		Operacion:   OperacionFirma,
		AccionFirma: domain.ActionSign,
		Formato:     domain.FormatCAdES,
		Sesion: domain.ExchangeSession{
			RequestID:        "req-original",
			RetrieveEndpoint: "https://example.com/RetrieveService",
			UploadEndpoint:   "https://example.com/RetrieveService",
			State:            domain.SessionActive,
		},
		LegacyParams: url.Values{},
	})
	if err != nil {
		t.Fatalf("ResolveRetrievedSign() error = %v", err)
	}
	if !ok {
		t.Fatal("se esperaba detección de manifiesto XML legado")
	}
	if solicitud.Sesion.RequestID != "req-xml" {
		t.Fatalf("request id inesperado: %s", solicitud.Sesion.RequestID)
	}
	if solicitud.Sesion.UploadEndpoint != "https://example.com/StorageService" {
		t.Fatalf("upload endpoint inesperado: %s", solicitud.Sesion.UploadEndpoint)
	}
	if cmd.Format != domain.FormatXAdES {
		t.Fatalf("formato inesperado: %s", cmd.Format)
	}
	if got := string(cmd.Document.Content); got != "<doc>ok</doc>" {
		t.Fatalf("contenido inesperado: %q", got)
	}
}
