// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type verificadorLimiteClasicoV2 struct {
	motivo       domain.MotivoDictamen
	estado       domain.EstadoDictamen
	sinFirmantes bool
}

func (v verificadorLimiteClasicoV2) Execute(_ context.Context, _ application.VerifyCommand) (application.VerifyResult, error) {
	f := domain.DictamenFirmante{CertificadoHuellaSHA256: "sha256-test", Cadena: domain.AspectoDictamen{Estado: domain.CadenaValida}, Certificado: domain.AspectoDictamen{Estado: domain.CertificadoVigente}, Revocacion: domain.AspectoDictamen{Estado: domain.RevocacionVigente}, SelloTiempo: domain.AspectoDictamen{Estado: domain.SelloNoPresente}}
	d := domain.DictamenVerificacion{Integridad: domain.AspectoDictamen{Estado: domain.IntegridadValida}, VinculoOriginal: domain.AspectoDictamen{Estado: domain.VinculoNoAportado}, Firmantes: []domain.DictamenFirmante{f}}.Componer()
	if v.sinFirmantes {
		d.Firmantes = nil
	}
	d.Estado = domain.EstadoDictamenIndeterminada
	if v.estado != "" {
		d.Estado = v.estado
	}
	d.Motivo = v.motivo
	return application.VerifyResult{Verification: domain.VerificationResult{Valid: false, Reason: string(v.motivo), Integrity: domain.VerificationAspect{Status: domain.VerificationStatusValid}}, Dictamen: &d}, nil
}

func TestVerifyV2NoRehabilitaBloqueoClasico(t *testing.T) {
	fixture := filepath.Join("..", "..", "..", "..", "..", "testdata", "dictamen-v2", "01_una_firma", "firmado.pdf")
	pdf, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, motivo := range []domain.MotivoDictamen{domain.MotivoDictamenResultadoClasicoNoValido, domain.MotivoDictamenAlgoritmoHeredado, domain.MotivoDictamenClaveRSACorta} {
		t.Run(string(motivo), func(t *testing.T) {
			a := New(nil, verificadorLimiteClasicoV2{motivo: motivo}, nil).WithBearerToken("test-token")
			body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(pdf), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
			req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			a.RoutesSoloVerificacion().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			var got verifyV2Response
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Valid || got.Dictamen.Estado != string(domain.EstadoDictamenIndeterminada) || got.Dictamen.Motivo != string(motivo) {
				t.Fatalf("REST v2 rehabilitó firma clásica: valid=%v estado=%s motivo=%s", got.Valid, got.Dictamen.Estado, got.Dictamen.Motivo)
			}
		})
	}
	t.Run("clasico_no_valido_aunque_v1_diga_valida", func(t *testing.T) {
		a := New(nil, verificadorLimiteClasicoV2{motivo: domain.MotivoDictamenVerificada, estado: domain.EstadoDictamenValida}, nil).WithBearerToken("test-token")
		body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(pdf), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
		req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		a.RoutesSoloVerificacion().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
		var got verifyV2Response
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Valid || got.Dictamen.Estado != "indeterminada" || got.Dictamen.Motivo != string(domain.MotivoDictamenResultadoClasicoNoValido) {
			t.Fatalf("v2 supera clásico: %+v", got.Dictamen)
		}
	})

	t.Run("v1_no_valida_con_aspectos_favorables", func(t *testing.T) {
		a := New(nil, verificadorLimiteClasicoV2{motivo: domain.MotivoDictamenIntegridadNoValida, estado: domain.EstadoDictamenNoValida}, nil).WithBearerToken("test-token")
		body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(pdf), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
		req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		a.RoutesSoloVerificacion().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
		var got verifyV2Response
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Valid || got.Dictamen.Estado != "no_valida" || got.Dictamen.Motivo != string(domain.MotivoDictamenIntegridadNoValida) {
			t.Fatalf("v2 supera v1 no válida: %+v", got.Dictamen)
		}
	})

	t.Run("v1_no_valida_sin_firmante_expuesto", func(t *testing.T) {
		a := New(nil, verificadorLimiteClasicoV2{motivo: domain.MotivoDictamenIntegridadNoValida, estado: domain.EstadoDictamenNoValida, sinFirmantes: true}, nil).WithBearerToken("test-token")
		body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(pdf), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
		req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		a.RoutesSoloVerificacion().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
		var got verifyV2Response
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Valid || got.Dictamen.Estado != "no_valida" || got.Dictamen.Motivo != string(domain.MotivoDictamenIntegridadNoValida) {
			t.Fatalf("sin firmante, v2 supera no válida: %+v", got.Dictamen)
		}
	})

}
