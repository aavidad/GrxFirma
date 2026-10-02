// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	restin "grxfirma/internal/adapters/inbound/common/rest"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/verificacionlocal"
	"grxfirma/internal/application"
)

func TestDictamenV2_CorpusSintetico(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "testdata", "dictamen-v2")
	anchors, err := verificacionlocal.CargarAnclas(filepath.Join(root, "pki", "raiz.pem"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, state, reason, secondChange, postChange string
		incomplete                                    bool
	}{
		{"01_una_firma", "valida", "verificada", "", "ninguno", false},
		{"02_dos_firmas", "valida", "verificada", "permitidos", "ninguno", false},
		{"03_pagina_entre_firmas", "no_valida", "cambios_no_permitidos", "no_permitidos", "ninguno", false},
		{"04_pagina_posterior", "no_valida", "cambios_no_permitidos", "permitidos", "no_permitidos", false},
		{"05_docmdp_1", "no_valida", "cambios_no_permitidos", "no_permitidos", "ninguno", false},
		{"06_original_ajeno", "indeterminada", "vinculo_original_no_acreditado", "permitidos", "ninguno", false},
		{"07_byterange_incompleto", "no_valida", "byterange_no_cubre_revision_completa", "", "ninguno", true},
		{"08_certificado_revocado", "no_valida", "certificado_no_valido", "permitidos", "ninguno", false},
		{"09_xref_stream", "indeterminada", "cambios_no_comprobados", "permitidos", "no_comprobados", false},
		{"10_xref_hibrido", "indeterminada", "cambios_no_comprobados", "permitidos", "no_comprobados", false},
		{"11_fieldmdp_all", "no_valida", "cambios_no_permitidos", "no_permitidos", "ninguno", false},
		{"12_dss", "indeterminada", "cambios_no_comprobados", "permitidos", "no_comprobados", false},
		{"13_doctimestamp", "indeterminada", "cambios_no_comprobados", "permitidos", "no_comprobados", false},
		{"14_docmdp_2", "valida", "verificada", "permitidos", "ninguno", false},
		{"15_docmdp_3", "valida", "verificada", "permitidos", "ninguno", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, tc.name)
			original, err := os.ReadFile(filepath.Join(dir, "original.pdf"))
			if err != nil {
				t.Fatal(err)
			}
			signed, err := os.ReadFile(filepath.Join(dir, "firmado.pdf"))
			if err != nil {
				t.Fatal(err)
			}
			expectedBytes, err := os.ReadFile(filepath.Join(dir, "dictamen-esperado.json"))
			if err != nil {
				t.Fatal(err)
			}
			crlName := "crl-buena"
			if tc.name == "08_certificado_revocado" {
				crlName = "crl-b-revocado"
			}
			crl, err := verificacionlocal.NuevoAlmacenCRL(filepath.Join(root, "pki", crlName))
			if err != nil {
				t.Fatal(err)
			}
			uc := application.NuevoVerifySignatureUseCase(anchors, commonsigner.NewMultiVerifierOffline(), nil).ConEvaluador(verificacionlocal.Nuevo(verificacionlocal.Configuracion{CRL: crl}))
			adapter := restin.New(nil, uc, nil).WithBearerToken("token-sintetico-de-corpus")
			body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(signed), "original_content_base64": base64.StdEncoding.EncodeToString(original), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer token-sintetico-de-corpus")
			adapter.RoutesSoloVerificacion().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			var wire struct {
				Dictamen map[string]any `json:"dictamen"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
				t.Fatal(err)
			}
			var expected map[string]any
			if err := json.Unmarshal(expectedBytes, &expected); err != nil {
				t.Fatal(err)
			}
			if wire.Dictamen["estado"] != tc.state || wire.Dictamen["motivo"] != tc.reason {
				t.Fatalf("veredicto: %s", rec.Body.String())
			}
			post := wire.Dictamen["cambiosPosteriores"].(map[string]any)
			if post["estado"] != tc.postChange {
				t.Fatalf("cambios posteriores: %v", post)
			}
			firms := wire.Dictamen["firmas"].([]any)
			if tc.secondChange != "" && (len(firms) < 2 || firms[1].(map[string]any)["cambiosDesdeAnterior"].(map[string]any)["estado"] != tc.secondChange) {
				t.Fatalf("segunda firma: %v", firms)
			}
			if tc.incomplete && (len(firms) != 1 || firms[0].(map[string]any)["cubreDocumentoCompletoHastaAqui"] != false) {
				t.Fatalf("ByteRange incompleto: %v", firms)
			}
			eliminarFechasDinamicas(wire.Dictamen)
			eliminarFechasDinamicas(expected)
			if !reflect.DeepEqual(wire.Dictamen, expected) {
				t.Fatalf("dictamen distinto del esperado: %s", rec.Body.String())
			}
		})
	}
}

func eliminarFechasDinamicas(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "comprobadoEn")
		delete(typed, "fecha")
		for _, nested := range typed {
			eliminarFechasDinamicas(nested)
		}
	case []any:
		for _, nested := range typed {
			eliminarFechasDinamicas(nested)
		}
	}
}

func TestDictamenV2_SinCRLNoEsValida(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "testdata", "dictamen-v2")
	anchors, err := verificacionlocal.CargarAnclas(filepath.Join(root, "pki", "raiz.pem"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "02_dos_firmas")
	original, err := os.ReadFile(filepath.Join(dir, "original.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	signed, err := os.ReadFile(filepath.Join(dir, "firmado.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	uc := application.NuevoVerifySignatureUseCase(anchors, commonsigner.NewMultiVerifierOffline(), nil).ConEvaluador(verificacionlocal.Nuevo(verificacionlocal.Configuracion{}))
	adapter := restin.New(nil, uc, nil).WithBearerToken("token-sintetico-de-corpus")
	body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(signed), "original_content_base64": base64.StdEncoding.EncodeToString(original), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-corpus")
	adapter.RoutesSoloVerificacion().ServeHTTP(rec, req)
	var result struct {
		Valid    bool `json:"valid"`
		Dictamen struct {
			Estado     string `json:"estado"`
			Motivo     string `json:"motivo"`
			Revocacion struct {
				Estado string `json:"estado"`
			} `json:"revocacion"`
		} `json:"dictamen"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.Dictamen.Estado != "indeterminada" || result.Dictamen.Motivo != "revocacion_no_acreditada" || result.Dictamen.Revocacion.Estado != "no_comprobada" {
		t.Fatalf("sin CRL se aceptó la firma: %s", rec.Body.String())
	}
}

func TestDictamenV2_ConservaRegistroDeFirmaNoComprobada(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "testdata", "dictamen-v2")
	anchors, err := verificacionlocal.CargarAnclas(filepath.Join(root, "pki", "raiz.pem"))
	if err != nil {
		t.Fatal(err)
	}
	crl, err := verificacionlocal.NuevoAlmacenCRL(filepath.Join(root, "pki", "crl-buena"))
	if err != nil {
		t.Fatal(err)
	}
	signed, err := os.ReadFile(filepath.Join(root, "01_una_firma", "firmado.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	marker := bytes.Index(signed, []byte("/Contents<"))
	if marker < 0 {
		t.Fatal("falta CMS")
	}
	signed = append([]byte(nil), signed...)
	hexStart := marker + len("/Contents<")
	if string(signed[hexStart:hexStart+4]) != "3082" {
		t.Fatal("CMS DER inesperado")
	}
	derLength, err := strconv.ParseUint(string(signed[hexStart+4:hexStart+8]), 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	spot := hexStart + 8 + int(derLength)*2 - 2
	if spot >= len(signed) || signed[spot] == '>' {
		t.Fatal("CMS demasiado corto")
	}
	if signed[spot] == '0' {
		signed[spot] = '1'
	} else {
		signed[spot] = '0'
	}
	uc := application.NuevoVerifySignatureUseCase(anchors, commonsigner.NewMultiVerifierOffline(), nil).ConEvaluador(verificacionlocal.Nuevo(verificacionlocal.Configuracion{CRL: crl}))
	adapter := restin.New(nil, uc, nil).WithBearerToken("token-sintetico-de-corpus")
	body, _ := json.Marshal(map[string]string{"content_base64": base64.StdEncoding.EncodeToString(signed), "contrato_solicitado": "autofirmav2.dictamen-verificacion.v2"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v2/verify", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-sintetico-de-corpus")
	adapter.RoutesSoloVerificacion().ServeHTTP(rec, req)
	var result struct {
		Dictamen struct {
			Estado string `json:"estado"`
			Motivo string `json:"motivo"`
			Firmas []struct {
				Integridad struct {
					Estado string `json:"estado"`
				} `json:"integridad"`
				CertificadoHuellaSHA256 string `json:"certificadoHuellaSHA256"`
			} `json:"firmas"`
		} `json:"dictamen"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Dictamen.Estado != "indeterminada" || result.Dictamen.Motivo != "firmas_no_comprobadas" || len(result.Dictamen.Firmas) != 1 || result.Dictamen.Firmas[0].Integridad.Estado != "parcial" || result.Dictamen.Firmas[0].CertificadoHuellaSHA256 != "" {
		t.Fatalf("firma corrupta no queda individualizada: %s", rec.Body.String())
	}
}
