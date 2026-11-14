// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

const (
	avisoRESTCofirma = domain.AvisoCofirmaSinOriginal
	avisoRESTCRL     = domain.AvisoCRLNoAplicable
)

type firmaRESTConAvisos struct{ avisos []string }

func (f firmaRESTConAvisos) Execute(ctx context.Context, cmd application.SignCommand) (application.SignResult, error) {
	res, err := signUseCaseMock{}.Execute(ctx, cmd)
	res.Warnings = f.avisos
	return res, err
}

type loteRESTConAvisos struct{ porTrabajo [][]string }

func (f loteRESTConAvisos) Execute(ctx context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	res, err := (&batchSignUseCaseMock{}).Execute(ctx, cmd)
	for i := range res.Results {
		if i < len(f.porTrabajo) {
			res.Results[i].Warnings = f.porTrabajo[i]
		}
	}
	return res, err
}

// Lote 17b (3d): el puente REST de Qt no recibía los avisos del motor al
// firmar. La respuesta de /sign y la de /sign-batch los llevan ahora, como
// el IPC; una firma sin avisos no lleva el campo (contrato anterior).
func TestSignRESTDevuelveLosAvisosDelMotor(t *testing.T) {
	cuerpo := map[string]any{
		"name":           "firma.csig",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "application/pkcs7-signature",
		"format":         "CAdES",
		"action":         "cosign",
		"certificate_id": "cert-1",
	}
	h := nuevoAdaptadorPruebas(firmaRESTConAvisos{avisos: []string{avisoRESTCofirma, avisoRESTCRL}}, nil, nil).Routes()
	rr := doJSON(t, h, http.MethodPost, "/sign", cuerpo)
	if rr.Code != http.StatusOK {
		t.Fatalf("código %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resp.Warnings, []string{avisoRESTCofirma, avisoRESTCRL}) {
		t.Fatalf("warnings = %q", resp.Warnings)
	}

	h = nuevoAdaptadorPruebas(firmaRESTConAvisos{}, nil, nil).Routes()
	rr = doJSON(t, h, http.MethodPost, "/sign", cuerpo)
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &campos); err != nil {
		t.Fatal(err)
	}
	if _, ok := campos["warnings"]; ok {
		t.Fatalf("una firma sin avisos no lleva warnings: %s", rr.Body.String())
	}
}

func TestSignBatchRESTDevuelveLosAvisosPorDocumento(t *testing.T) {
	item := func(nombre string) map[string]any {
		return map[string]any{"name": nombre, "content_base64": base64.StdEncoding.EncodeToString([]byte("abc"))}
	}
	cuerpo := map[string]any{
		"certificate_id": "cert-1",
		"format":         "CAdES",
		"action":         "cosign",
		"items":          []map[string]any{item("a.csig"), item("b.txt"), item("c.csig")},
	}
	lote := loteRESTConAvisos{porTrabajo: [][]string{{avisoRESTCofirma}, nil, {avisoRESTCRL, avisoRESTCofirma}}}
	h := nuevoAdaptadorPruebas(nil, nil, nil).WithBatchSigner(lote).Routes()
	rr := doJSON(t, h, http.MethodPost, "/sign-batch", cuerpo)
	if rr.Code != http.StatusOK {
		t.Fatalf("código %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Warnings []string `json:"warnings"`
		Results  []struct {
			Warnings []string `json:"warnings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 3 ||
		!slices.Equal(resp.Results[0].Warnings, []string{avisoRESTCofirma}) ||
		len(resp.Results[1].Warnings) != 0 ||
		!slices.Equal(resp.Results[2].Warnings, []string{avisoRESTCRL, avisoRESTCofirma}) {
		t.Fatalf("avisos por documento = %+v", resp.Results)
	}
	if !slices.Equal(resp.Warnings, []string{avisoRESTCofirma, avisoRESTCRL}) {
		t.Fatalf("avisos del lote = %q", resp.Warnings)
	}
}
