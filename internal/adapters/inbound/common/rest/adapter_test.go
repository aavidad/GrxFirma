// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/outbound/desktop/pdfpreview"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"software.sslmate.com/src/go-pkcs12"
)

func TestRoutes_Sign(t *testing.T) {
	adaptador := rest.New(signUseCaseMock{}, nil, nil)
	body := map[string]any{
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
		"certificate_id": "cert-1",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["certificate_id"] != "cert-1" {
		t.Fatalf("certificate_id inesperado: %#v", resp["certificate_id"])
	}
}

func TestRoutes_SignContent_ResuelveCertificateIndex(t *testing.T) {
	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithCertificateSources(
		catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-contenido",
			Subject:     "CN=Firma REST",
			Issuer:      "CN=CA",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp-contenido",
		}}},
		nil,
	)
	body := map[string]any{
		"name":             "doc.txt",
		"content_base64":   base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":        "text/plain",
		"format":           "CAdES",
		"action":           "sign",
		"certificateIndex": 0,
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respuesta JSON invalida: %v", err)
	}
	if resp["certificate_id"] != "cert-contenido" {
		t.Fatalf("certificate_id inesperado: %#v", resp["certificate_id"])
	}
}

func TestRoutes_Sign_AplicaSubfilterPAdESPersistidoYRespetaOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"padesSubFilter":"adobe"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	firmar := &capturingSignUseCase{}
	adaptador := rest.New(firmar, nil, nil).WithConfigDir(dir)
	body := map[string]any{
		"name":           "doc.pdf",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 prueba")),
		"mime_type":      "application/pdf",
		"format":         "PAdES",
		"action":         "sign",
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma con default PAdES: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if firmar.lastCmd.Options["subfilter"] != "adobe" {
		t.Fatalf("subfilter persistido no llegó al motor: %#v", firmar.lastCmd.Options)
	}

	body["options"] = map[string]string{"SubFilter": "etsi"}
	rr = doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma con override PAdES: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if firmar.lastCmd.Options["SubFilter"] != "etsi" {
		t.Fatalf("el override explícito no prevaleció: %#v", firmar.lastCmd.Options)
	}
	if _, duplicate := firmar.lastCmd.Options["subfilter"]; duplicate {
		t.Fatalf("se filtró una clave duplicada al motor: %#v", firmar.lastCmd.Options)
	}
}

const settingsFacturaEPredeterminadaTest = `{
  "facturaePolicyVersion": "3.1",
  "policyIdentifier": "urn:oid:1.2.3.4",
  "policyIdentifierHash": "Ohixl6upD6av8N7pEvDABhEL6hM=",
  "policyQualifier": "https://www.facturae.gob.es/politica.html",
  "signerClaimedRole": "emisor",
  "signatureProductionCity": "Granada",
  "signatureProductionProvince": "Granada",
  "signatureProductionPostalCode": "18001",
  "signatureProductionCountry": "ES"
}`

func comprobarOpcionesFacturaEREST(t *testing.T, got map[string]string) {
	t.Helper()
	want := map[string]string{
		"facturaePolicyVersion":         "3.1",
		"policyIdentifier":              "urn:oid:1.2.3.4",
		"policyIdentifierHash":          "Ohixl6upD6av8N7pEvDABhEL6hM=",
		"policyQualifier":               "https://www.facturae.gob.es/politica.html",
		"signerClaimedRole":             "emisor",
		"signatureProductionCity":       "Granada",
		"signatureProductionProvince":   "Granada",
		"signatureProductionPostalCode": "18001",
		"signatureProductionCountry":    "ES",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q; want %q; opciones=%#v", key, got[key], value, got)
		}
	}
}

func settingsXAdESPredeterminadaREST(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]string{
		"xadesPolicyIdentifier":              "urn:oid:1.2.3.4.5",
		"xadesPolicyIdentifierHash":          base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"xadesPolicyIdentifierHashAlgorithm": "SHA-256",
		"xadesPolicyQualifier":               "https://sede.example/politica-xades.pdf",
	})
	if err != nil {
		t.Fatalf("json.Marshal(settings XAdES) error = %v", err)
	}
	return data
}

func comprobarOpcionesXAdESREST(t *testing.T, got map[string]string) {
	t.Helper()
	want := map[string]string{
		"policyIdentifier":              "urn:oid:1.2.3.4.5",
		"policyIdentifierHash":          base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"policyIdentifierHashAlgorithm": "http://www.w3.org/2001/04/xmlenc#sha256",
		"policyQualifier":               "https://sede.example/politica-xades.pdf",
	}
	if len(got) != len(want) {
		t.Fatalf("opciones XAdES = %#v; want %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q; want %q; opciones=%#v", key, got[key], value, got)
		}
	}
}

func TestRoutes_Sign_AplicaFacturaEPersistidaYRespetaOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settingsFacturaEPredeterminadaTest), 0o600); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	firmar := &capturingSignUseCase{}
	adaptador := rest.New(firmar, nil, nil).WithConfigDir(dir)
	body := map[string]any{
		"name":           "factura.xml",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("<Facturae/>")),
		"mime_type":      "application/xml",
		"format":         "FacturaE",
		"action":         "sign",
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma con defaults FacturaE: status=%d body=%s", rr.Code, rr.Body.String())
	}
	comprobarOpcionesFacturaEREST(t, firmar.lastCmd.Options)

	body["options"] = map[string]string{
		"PolicyIdentifier":  "urn:oid:9.9",
		"SIGNERCLAIMEDROLE": "receptor",
	}
	rr = doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma con overrides FacturaE: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if firmar.lastCmd.Options["PolicyIdentifier"] != "urn:oid:9.9" ||
		firmar.lastCmd.Options["SIGNERCLAIMEDROLE"] != "receptor" {
		t.Fatalf("los overrides explícitos no prevalecieron: %#v", firmar.lastCmd.Options)
	}
	for _, key := range []string{
		"facturaePolicyVersion",
		"policyIdentifier",
		"policyIdentifierHash",
		"policyQualifier",
		"signerClaimedRole",
	} {
		if _, duplicate := firmar.lastCmd.Options[key]; duplicate {
			t.Fatalf("se mezcló %q con la política/rol explícitos: %#v", key, firmar.lastCmd.Options)
		}
	}
}

func TestRoutes_Sign_AplicaPoliticaXAdESPersistidaYRespetaOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		settingsXAdESPredeterminadaREST(t),
		0o600,
	); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	firmar := &capturingSignUseCase{}
	adaptador := rest.New(firmar, nil, nil).WithConfigDir(dir)
	body := map[string]any{
		"name":           "documento.xml",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("<documento/>")),
		"mime_type":      "application/xml",
		"format":         "XAdES",
		"action":         "sign",
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma con default XAdES: status=%d body=%s", rr.Code, rr.Body.String())
	}
	comprobarOpcionesXAdESREST(t, firmar.lastCmd.Options)

	body["options"] = map[string]string{
		"XAdESPolicyIdentifier": "urn:oid:9.9",
	}
	rr = doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma con override XAdES: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(firmar.lastCmd.Options) != 1 ||
		firmar.lastCmd.Options["XAdESPolicyIdentifier"] != "urn:oid:9.9" {
		t.Fatalf("la política explícita XAdES no prevaleció: %#v", firmar.lastCmd.Options)
	}
}

func TestRoutes_Sign_PreferenciasCorruptasNoBloqueanFirmaNiOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"padesSubFilter":`), 0o600); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	firmar := &capturingSignUseCase{}
	adaptador := rest.New(firmar, nil, nil).WithConfigDir(dir)
	base := map[string]any{
		"name":           "documento.bin",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("contenido")),
		"mime_type":      "application/octet-stream",
		"action":         "sign",
	}

	for _, tc := range []struct {
		name    string
		format  string
		options map[string]string
	}{
		{name: "otro formato", format: "CAdES"},
		{name: "PAdES con override", format: "PAdES", options: map[string]string{"SubFilter": "adobe"}},
		{name: "PAdES usa fallback seguro", format: "PAdES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := make(map[string]any, len(base)+2)
			for key, value := range base {
				body[key] = value
			}
			body["format"] = tc.format
			if tc.options != nil {
				body["options"] = tc.options
			}
			rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if tc.options == nil {
				if _, exists := firmar.lastCmd.Options["subfilter"]; exists {
					t.Fatalf("se inyectó un default desde settings corruptos: %#v", firmar.lastCmd.Options)
				}
			}
		})
	}
}

func TestRoutes_SignByPathCompatQt(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "doc.pdf")
	outputPath := filepath.Join(dir, "firmado.pdf")
	if err := os.WriteFile(inputPath, []byte("abc"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}

	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithFileSystemPaths().WithCertificateSources(
		catalogMockREST{certs: []domain.CertificateRef{{
			ID:            "cert-qt",
			Subject:       "CN=Ana Perez",
			Issuer:        "CN=FNMT",
			NotAfter:      time.Now().Add(time.Hour),
			Fingerprint:   "fp1",
			HasSigningKey: true,
		}}},
		nil,
	)

	body := map[string]any{
		"inputPath":        inputPath,
		"outputPath":       outputPath,
		"certificateIndex": 0,
		"format":           "PAdES",
		"action":           "sign",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("se esperaba firma en disco: %v", err)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("ok inesperado: %#v", resp["ok"])
	}
	if resp["outputPath"] != outputPath {
		t.Fatalf("outputPath inesperado: %#v", resp["outputPath"])
	}
	if resp["certificate_id"] != "cert-qt" {
		t.Fatalf("certificate_id inesperado: %#v", resp["certificate_id"])
	}
}

func TestRoutes_SignBatch(t *testing.T) {
	mock := &batchSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithBatchSigner(mock).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{
			{
				ID:          "cert-lote",
				Subject:     "CN=Lote",
				Issuer:      "CN=FNMT",
				NotAfter:    time.Now().Add(time.Hour),
				Fingerprint: "fp-lote",
			},
		}}, nil)
	body := map[string]any{
		"request_id":       "web-batch-001",
		"certificateIndex": 0,
		"format":           "AUTO",
		"action":           "sign",
		"items": []map[string]any{
			{
				"name":           "uno.pdf",
				"mime_type":      "application/pdf",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\nuno")),
			},
			{
				"name":           "dos.xml",
				"mime_type":      "application/xml",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("<root/>")),
			},
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign-batch", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var batchResponse map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &batchResponse); err != nil {
		t.Fatalf("respuesta de lote invalida: %v", err)
	}
	if batchResponse["request_id"] != "web-batch-001" {
		t.Fatalf("request_id de lote inesperado: %#v", batchResponse["request_id"])
	}
	if mock.lastCmd.CertificateID != "cert-lote" {
		t.Fatalf("certificate_id de lote inesperado: %q", mock.lastCmd.CertificateID)
	}
	if len(mock.lastCmd.Jobs) != 2 {
		t.Fatalf("jobs inesperados: %d", len(mock.lastCmd.Jobs))
	}
	if mock.lastCmd.Jobs[0].Format != domain.FormatPAdES {
		t.Fatalf("formato item 0 inesperado: %s", mock.lastCmd.Jobs[0].Format)
	}
	if mock.lastCmd.Jobs[1].Format != domain.FormatXAdES {
		t.Fatalf("formato item 1 inesperado: %s", mock.lastCmd.Jobs[1].Format)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("ok inesperado: %#v", resp["ok"])
	}
	results, ok := resp["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results inesperados: %#v", resp["results"])
	}
	replay := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign-batch", body)
	if replay.Code != http.StatusConflict {
		t.Fatalf("replay de lote: codigo=%d cuerpo=%s", replay.Code, replay.Body.String())
	}
	if mock.calls != 1 {
		t.Fatalf("Execute de lote invocado %d veces, esperaba 1", mock.calls)
	}
}

func TestRoutes_SignBatch_AplicaDefaultPAdESSoloAlFormatoCorrespondiente(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"padesSubFilter":"adobe"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	mock := &batchSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithBatchSigner(mock).WithConfigDir(dir)
	body := map[string]any{
		"action": "sign",
		"items": []map[string]any{
			{
				"name":           "documento.pdf",
				"mime_type":      "application/pdf",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 prueba")),
			},
			{
				"name":           "documento.xml",
				"mime_type":      "application/xml",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("<doc/>")),
			},
		},
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign-batch", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma por lotes: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(mock.lastCmd.Jobs) != 2 {
		t.Fatalf("jobs = %d; want 2", len(mock.lastCmd.Jobs))
	}
	if mock.lastCmd.Jobs[0].Options["subfilter"] != "adobe" {
		t.Fatalf("el trabajo PAdES no recibió su default: %#v", mock.lastCmd.Jobs[0].Options)
	}
	if _, leaked := mock.lastCmd.Jobs[1].Options["subfilter"]; leaked {
		t.Fatalf("el default PAdES se filtró al trabajo XAdES: %#v", mock.lastCmd.Jobs[1].Options)
	}
}

func TestRoutes_SignBatch_AplicaFacturaEPorDocumentoConMismaPrecedencia(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settingsFacturaEPredeterminadaTest), 0o600); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	mock := &batchSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithBatchSigner(mock).WithConfigDir(dir)
	factura := base64.StdEncoding.EncodeToString([]byte("<Facturae/>"))
	body := map[string]any{
		"format": "FacturaE",
		"action": "sign",
		"items": []map[string]any{
			{
				"name":           "predeterminada.xml",
				"mime_type":      "application/xml",
				"content_base64": factura,
			},
			{
				"name":           "explicita.xml",
				"mime_type":      "application/xml",
				"content_base64": factura,
				"options": map[string]string{
					"PolicyIdentifier":  "urn:oid:9.9",
					"SIGNERCLAIMEDROLE": "receptor",
				},
			},
		},
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign-batch", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma por lotes FacturaE: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(mock.lastCmd.Jobs) != 2 {
		t.Fatalf("jobs FacturaE = %d; want 2", len(mock.lastCmd.Jobs))
	}
	comprobarOpcionesFacturaEREST(t, mock.lastCmd.Jobs[0].Options)

	explicitas := mock.lastCmd.Jobs[1].Options
	if explicitas["PolicyIdentifier"] != "urn:oid:9.9" ||
		explicitas["SIGNERCLAIMEDROLE"] != "receptor" {
		t.Fatalf("overrides por documento perdidos: %#v", explicitas)
	}
	for _, key := range []string{
		"facturaePolicyVersion",
		"policyIdentifier",
		"policyIdentifierHash",
		"policyQualifier",
		"signerClaimedRole",
	} {
		if _, duplicate := explicitas[key]; duplicate {
			t.Fatalf("se mezcló %q en el trabajo con política explícita: %#v", key, explicitas)
		}
	}
	if explicitas["signatureProductionCity"] != "Granada" ||
		explicitas["signatureProductionCountry"] != "ES" {
		t.Fatalf("los metadatos no sobrescritos no conservaron sus defaults: %#v", explicitas)
	}
}

func TestRoutes_SignBatch_AplicaPoliticaXAdESPorDocumentoConMismaPrecedencia(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		settingsXAdESPredeterminadaREST(t),
		0o600,
	); err != nil {
		t.Fatalf("os.WriteFile(settings.json) error = %v", err)
	}
	mock := &batchSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithBatchSigner(mock).WithConfigDir(dir)
	documento := base64.StdEncoding.EncodeToString([]byte("<documento/>"))
	body := map[string]any{
		"format": "XAdES",
		"action": "sign",
		"items": []map[string]any{
			{
				"name":           "predeterminado.xml",
				"mime_type":      "application/xml",
				"content_base64": documento,
			},
			{
				"name":           "explicito.xml",
				"mime_type":      "application/xml",
				"content_base64": documento,
				"options": map[string]string{
					"XAdESPolicyIdentifier": "urn:oid:9.9",
				},
			},
		},
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign-batch", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("firma por lotes XAdES: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(mock.lastCmd.Jobs) != 2 {
		t.Fatalf("jobs XAdES = %d; want 2", len(mock.lastCmd.Jobs))
	}
	comprobarOpcionesXAdESREST(t, mock.lastCmd.Jobs[0].Options)
	if got := mock.lastCmd.Jobs[1].Options; len(got) != 1 ||
		got["XAdESPolicyIdentifier"] != "urn:oid:9.9" {
		t.Fatalf("override XAdES por documento inesperado: %#v", got)
	}
}

func TestRoutes_SignBatch_MezclaPlantillaGlobalYOverridesPorDocumento(t *testing.T) {
	mock := &batchSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithBatchSigner(mock).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-lote",
			Subject:     "CN=Lote",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp-lote",
		}}}, nil)
	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n"))
	body := map[string]any{
		"certificateIndex": 0,
		"format":           "PAdES",
		"action":           "sign",
		"options": map[string]string{
			"visibleSeal":      "true",
			"visibleSealRectX": "36",
			"visibleSealRectY": "36",
			"visibleSealRectW": "220",
			"visibleSealRectH": "70",
			"page":             "all",
		},
		"items": []map[string]any{
			{
				"name":           "global.pdf",
				"mime_type":      "application/pdf",
				"content_base64": pdf,
			},
			{
				"name":           "pagina.pdf",
				"mime_type":      "application/pdf",
				"content_base64": pdf,
				"options": map[string]string{
					"page": "2",
				},
			},
			{
				"name":           "rango.pdf",
				"mime_type":      "application/pdf",
				"content_base64": pdf,
				"options": map[string]string{
					"page":             "1,3-5",
					"visibleSealRectX": "72",
				},
			},
		},
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign-batch", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if got := len(mock.lastCmd.Jobs); got != 3 {
		t.Fatalf("jobs = %d, want 3", got)
	}
	if got := mock.lastCmd.Jobs[0].Options["page"]; got != "all" {
		t.Fatalf("page global = %q, want all", got)
	}
	if got := mock.lastCmd.Jobs[1].Options["page"]; got != "2" {
		t.Fatalf("page del documento = %q, want 2", got)
	}
	if got := mock.lastCmd.Jobs[2].Options["page"]; got != "1,3-5" {
		t.Fatalf("page del rango = %q, want 1,3-5", got)
	}
	if got := mock.lastCmd.Jobs[2].Options["visibleSealRectX"]; got != "72" {
		t.Fatalf("rectX del documento = %q, want 72", got)
	}
	for i, job := range mock.lastCmd.Jobs {
		if got := job.Options["visibleSeal"]; got != "true" {
			t.Fatalf("job %d no heredó el sello global: %q", i, got)
		}
		if got := job.Options["visibleSealRectW"]; got != "220" {
			t.Fatalf("job %d no heredó rectW: %q", i, got)
		}
	}
}

func TestRoutes_Verify(t *testing.T) {
	adaptador := rest.New(nil, verifyUseCaseMock{}, nil)
	body := map[string]any{
		"name":           "firma.csig",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("firma")),
		"mime_type":      "application/pkcs7-signature",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/verify", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Fatalf("valid inesperado: %#v", resp["valid"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result inesperado: %#v", resp["result"])
	}
	if result["format"] != "CAdES" {
		t.Fatalf("result.format inesperado: %#v", result["format"])
	}
	if result["coverage"] != "full" {
		t.Fatalf("result.coverage inesperado: %#v", result["coverage"])
	}
	integrity, ok := result["integrity"].(map[string]any)
	if !ok || integrity["status"] != "valid" {
		t.Fatalf("integrity inesperado: %#v", result["integrity"])
	}
	certificate, ok := result["certificate"].(map[string]any)
	if !ok || certificate["status"] != "warning" {
		t.Fatalf("certificate inesperado: %#v", result["certificate"])
	}
	trust, ok := result["trust"].(map[string]any)
	if !ok || trust["status"] != "valid" {
		t.Fatalf("trust inesperado: %#v", result["trust"])
	}
	warnings, ok := result["warnings"].([]any)
	if !ok || len(warnings) != 1 {
		t.Fatalf("warnings inesperadas: %#v", result["warnings"])
	}
	signerSummaries, ok := result["signerSummaries"].([]any)
	if !ok || len(signerSummaries) != 1 {
		t.Fatalf("signerSummaries inesperados: %#v", result["signerSummaries"])
	}
}

func TestRoutes_VerifyErrorIncluyeDiagnosticoGuiado(t *testing.T) {
	adaptador := rest.New(nil, verifyUseCaseErrorMock{err: errors.New("error accediendo al almacén PKCS#11")}, nil)
	body := map[string]any{
		"name":           "firma.csig",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("firma")),
		"mime_type":      "application/pkcs7-signature",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/verify", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("codigo inesperado: %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	diag, ok := resp["diagnostic"].(map[string]any)
	if !ok {
		t.Fatalf("diagnostic ausente: %#v", resp)
	}
	if diag["category"] != "certificate_store" {
		t.Fatalf("diagnostic.category=%#v, want certificate_store", diag["category"])
	}
}

func TestRoutes_HashCreate(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithHashes(hashCreateUseCaseMock{}, nil)
	body := map[string]any{
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"algorithm":      "SHA-256",
		"format":         "hex",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/hash", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["algorithm"] != "SHA-256" {
		t.Fatalf("algorithm inesperado: %#v", resp["algorithm"])
	}
	if stringValue(resp["hash"]) == "" {
		t.Fatalf("hash vacio: %s", rr.Body.String())
	}
}

func TestRoutes_HashCheck(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithHashes(nil, hashCheckUseCaseMock{})
	body := map[string]any{
		"content_base64":      base64.StdEncoding.EncodeToString([]byte("abc")),
		"hash_content_base64": base64.StdEncoding.EncodeToString([]byte("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015adh")),
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/hash/check", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Fatalf("valid inesperado: %#v", resp["valid"])
	}
}

func TestRoutes_HashPathsRechazanDirectoriosSensibles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		body     map[string]any
	}{
		{
			name:     "crear lee sistema",
			endpoint: "/hash",
			body: map[string]any{
				"inputPath": "/etc/passwd",
				"format":    "hex",
			},
		},
		{
			name:     "crear escribe sistema",
			endpoint: "/hash",
			body: map[string]any{
				"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
				"outputPath":     "/etc/grxfirma.hash",
				"format":         "hex",
			},
		},
		{
			name:     "comprobar lee sistema",
			endpoint: "/hash/check",
			body: map[string]any{
				"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
				"hashPath":       "/etc/passwd",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adaptador := rest.New(nil, nil, nil).
				WithHashes(hashCreateUseCaseMock{}, hashCheckUseCaseMock{}).
				WithFileSystemPaths()
			rr := doJSON(t, adaptador.Routes(), http.MethodPost, test.endpoint, test.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "no valida") {
				t.Fatalf("respuesta sin diagnóstico de ruta: %s", rr.Body.String())
			}
		})
	}
}

func TestRoutes_DirectoryHashCheck_WithReport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "uno.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(dir, "resultado.hashreport")
	adaptador := rest.New(nil, nil, nil).
		WithFileSystemPaths().
		WithDirectoryHashes(nil, dirHashCheckUseCaseMock{}).
		WithDirectoryHashReports(dirHashReportCodecMock{})
	body := map[string]any{
		"inputPath":        dir,
		"hashPath":         filepath.Join(dir, "directorio.hashfiles"),
		"saveReportToDisk": true,
		"reportOutputPath": reportPath,
	}
	if err := os.WriteFile(filepath.Join(dir, "directorio.hashfiles"), []byte("<entries/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/hash/check", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["valid"] != false {
		t.Fatalf("valid inesperado: %#v", resp["valid"])
	}
	if stringValue(resp["report_base64"]) == "" {
		t.Fatalf("report_base64 vacio: %s", rr.Body.String())
	}
	if resp["report_output_path"] != reportPath {
		t.Fatalf("report_output_path inesperado: %#v", resp["report_output_path"])
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("se esperaba informe en disco: %v", err)
	}
}

func TestRoutes_ProtectionRecipients(t *testing.T) {
	authRecipient := generarDestinatarioAuthEnvelopedREST(t, "dest-auth")
	authRecipientWithoutID := authRecipient
	authRecipientWithoutID.ID = ""
	adaptador := rest.New(nil, nil, nil).WithProtection(nil, nil, protectionRecipientsMock{
		recipients: []domain.ProtectionRecipient{authRecipient, authRecipientWithoutID, {
			ID:                     "dest-compat-sin-certificado",
			Label:                  "Compat RSA sin certificado",
			RSAOAEP256PublicKeyDER: []byte{1},
		}, {
			ID:                "dest-2",
			Label:             "PQ local",
			MLKEM768PublicKey: []byte{1},
			X25519PublicKey:   []byte{2},
		}},
	})
	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/protection/recipients", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("ok inesperado: %#v", resp["ok"])
	}
	recipients, ok := resp["recipients"].([]any)
	if !ok || len(recipients) != 4 {
		t.Fatalf("recipients inesperados: %#v", resp["recipients"])
	}
	first, _ := recipients[0].(map[string]any)
	second, _ := recipients[1].(map[string]any)
	third, _ := recipients[2].(map[string]any)
	fourth, _ := recipients[3].(map[string]any)
	profiles := map[string]bool{
		stringValue(first["profile"]):  true,
		stringValue(second["profile"]): true,
		stringValue(third["profile"]):  true,
		stringValue(fourth["profile"]): true,
	}
	if !profiles["compat"] || !profiles["alto"] {
		t.Fatalf("profiles inesperados: %#v %#v %#v %#v", first, second, third, fourth)
	}
	if first["authEnvelopedDataCompatible"] != true ||
		second["authEnvelopedDataCompatible"] != false ||
		third["authEnvelopedDataCompatible"] != false ||
		fourth["authEnvelopedDataCompatible"] != false {
		t.Fatalf("capacidades AuthEnvelopedData inesperadas: %#v", recipients)
	}
}

func TestRoutes_ProtectionRecipientExport(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithIntercambioProteccion(exportProtectionRecipientUseCaseMock{}, nil)
	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/protection/recipient/export?id=dest-2", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("ok inesperado: %#v", resp["ok"])
	}
	if stringValue(resp["filename"]) != "dest-2.afpr.json" {
		t.Fatalf("filename inesperado: %#v", resp["filename"])
	}
	recipient, _ := resp["recipient"].(map[string]any)
	if stringValue(recipient["profile"]) != "alto" {
		t.Fatalf("profile inesperado: %#v", recipient)
	}
}

func TestRoutes_ProtectionRecipientImport(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithIntercambioProteccion(nil, importProtectionRecipientUseCaseMock{})
	body := map[string]any{
		"data_base64": base64.StdEncoding.EncodeToString([]byte(`{"profile":"alto"}`)),
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protection/recipient/import", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("ok inesperado: %#v", resp["ok"])
	}
	recipient, _ := resp["recipient"].(map[string]any)
	if stringValue(recipient["id"]) != "dest-importado" || stringValue(recipient["profile"]) != "alto" {
		t.Fatalf("recipient inesperado: %#v", recipient)
	}
}

func TestRoutes_Protect(t *testing.T) {
	mock := &protectUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithProtection(mock, nil, nil)
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"recipient_ids":  []string{"dest-1"},
		"options": map[string]any{
			"container": "cms",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["profile"] != string(domain.ProtectionProfileCompat) {
		t.Fatalf("profile inesperado: %#v", resp["profile"])
	}
	if resp["recipientCount"] != float64(1) {
		t.Fatalf("recipientCount inesperado: %#v", resp["recipientCount"])
	}
	if mock.lastCmd.Options["container"] != "cms" {
		t.Fatalf("container inesperado: %#v", mock.lastCmd.Options["container"])
	}
}

func TestRoutes_ProtectAuthEnvelopedData(t *testing.T) {
	mock := &protectUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithProtection(mock, nil, nil)
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"recipient_ids":  []string{"dest-auth"},
		"options": map[string]any{
			"container": "authenvelopeddata",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Options["container"] != "authenvelopeddata" {
		t.Fatalf("container inesperado: %#v", mock.lastCmd.Options["container"])
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"documentName":"secreto.txt.authenveloped.p7m"`)) {
		t.Fatalf("nombre AuthEnvelopedData inesperado: %s", rr.Body.String())
	}
}

func TestRoutes_Unprotect(t *testing.T) {
	mock := &unprotectUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithProtection(nil, mock, nil)
	body := map[string]any{
		"name":           "secreto.txt.afp",
		"content_base64": base64.StdEncoding.EncodeToString([]byte(`{"version":"v1"}`)),
		"mime_type":      domain.MIMETypeProtectedEnvelope,
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/unprotect", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["documentName"] != "secreto.txt" {
		t.Fatalf("documentName inesperado: %#v", resp["documentName"])
	}
	if len(mock.lastCmd.Options) != 0 {
		t.Fatalf("options inesperadas: %#v", mock.lastCmd.Options)
	}
}

func TestRoutes_ProtectEncryptedDataWithoutRecipients(t *testing.T) {
	mock := &protectUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithProtection(mock, nil, nil)
	secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"secret_b64":     secret,
		"options": map[string]any{
			"container": "cms-encrypted",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if len(mock.lastCmd.RecipientIDs) != 0 {
		t.Fatalf("recipientIDs inesperados: %#v", mock.lastCmd.RecipientIDs)
	}
	if mock.lastCmd.Options["container"] != "cms-encrypted" {
		t.Fatalf("container inesperado: %#v", mock.lastCmd.Options["container"])
	}
	if mock.lastCmd.Options["secret_b64"] != secret {
		t.Fatalf("secret_b64 inesperado: %#v", mock.lastCmd.Options["secret_b64"])
	}
}

func TestRoutes_ProtectSign(t *testing.T) {
	mock := &protectAndSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithSignedProtection(mock).
		WithProtection(nil, nil, protectionRecipientsMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			Label:                  "Compat RSA",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}}).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-firma-1",
			Subject:     "CN=Ana Perez",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}}}, nil)
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"recipient_ids":  []string{"dest-1"},
		"certificate_id": "cert-firma-1",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect-sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Options["container"] != "signedandenvelopeddata" {
		t.Fatalf("container inesperado: %#v", mock.lastCmd.Options["container"])
	}
	if mock.lastCmd.CertificateID != "cert-firma-1" {
		t.Fatalf("certificateID inesperado: %#v", mock.lastCmd.CertificateID)
	}
	if len(mock.lastCmd.RecipientIDs) != 1 || mock.lastCmd.RecipientIDs[0] != "dest-1" {
		t.Fatalf("recipientIDs inesperados: %#v", mock.lastCmd.RecipientIDs)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["certificate_id"] != "cert-firma-1" {
		t.Fatalf("certificate_id inesperado: %#v", resp["certificate_id"])
	}
	if resp["documentName"] != "secreto.txt.signedenveloped.p7m" {
		t.Fatalf("documentName inesperado: %#v", resp["documentName"])
	}
}

func TestRoutes_ProtectSign_AceptaAliasLegacyDeSignedAndEnvelopedData(t *testing.T) {
	mock := &protectAndSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithSignedProtection(mock).
		WithProtection(nil, nil, protectionRecipientsMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			Label:                  "Compat RSA",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}}).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-firma-1",
			Subject:     "CN=Ana Perez",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}}}, nil)
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"recipient_ids":  []string{"dest-1"},
		"certificate_id": "cert-firma-1",
		"options": map[string]any{
			"container": "signed-and-enveloped-data",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect-sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Options["container"] != "signedandenvelopeddata" {
		t.Fatalf("container inesperado: %#v", mock.lastCmd.Options["container"])
	}
}

func TestRoutes_ProtectSign_RechazaAuthEnvelopedDataConErrorClaro(t *testing.T) {
	mock := &protectAndSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithSignedProtection(mock).
		WithProtection(nil, nil, protectionRecipientsMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			Label:                  "Compat RSA",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}}).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-firma-1",
			Subject:     "CN=Ana Perez",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}}}, nil)
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"recipient_ids":  []string{"dest-1"},
		"certificate_id": "cert-firma-1",
		"options": map[string]any{
			"container": "authenvelopeddata",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect-sign", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Document.Name != "" {
		t.Fatalf("no se esperaba ejecutar el caso de uso: %#v", mock.lastCmd)
	}
	if !strings.Contains(rr.Body.String(), "AuthEnvelopedData") {
		t.Fatalf("cuerpo inesperado: %s", rr.Body.String())
	}
}

func TestRoutes_ProtectSign_RechazaEnvelopedDataConErrorClaro(t *testing.T) {
	mock := &protectAndSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithSignedProtection(mock).
		WithProtection(nil, nil, protectionRecipientsMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			Label:                  "Compat RSA",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}}).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-firma-1",
			Subject:     "CN=Ana Perez",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}}}, nil)
	body := map[string]any{
		"name":           "secreto.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("secreto")),
		"mime_type":      "text/plain",
		"profile":        "compat",
		"recipient_ids":  []string{"dest-1"},
		"certificate_id": "cert-firma-1",
		"options": map[string]any{
			"container": "cms",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect-sign", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Document.Name != "" {
		t.Fatalf("no se esperaba ejecutar el caso de uso: %#v", mock.lastCmd)
	}
	if !strings.Contains(rr.Body.String(), "EnvelopedData") {
		t.Fatalf("cuerpo inesperado: %s", rr.Body.String())
	}
}

func TestRoutes_ProtectSignByPath_AceptaAliasLegacyDeSignedAndEnvelopedData(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(inputPath, []byte("secreto"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := &protectAndSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithFileSystemPaths().
		WithSignedProtection(mock).
		WithProtection(nil, nil, protectionRecipientsMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			Label:                  "Compat RSA",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}}).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-firma-1",
			Subject:     "CN=Ana Perez",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}}}, nil)
	body := map[string]any{
		"inputPath":          inputPath,
		"profile":            "compat",
		"recipient_ids":      []string{"dest-1"},
		"certificate_id":     "cert-firma-1",
		"saveToDisk":         false,
		"returnProtectedB64": true,
		"options": map[string]any{
			"container": "signed-and-enveloped-data",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect-sign", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Options["container"] != "signedandenvelopeddata" {
		t.Fatalf("container inesperado: %#v", mock.lastCmd.Options["container"])
	}
}

func TestRoutes_ProtectSignByPath_RechazaAuthEnvelopedDataConErrorClaro(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(inputPath, []byte("secreto"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := &protectAndSignUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).
		WithFileSystemPaths().
		WithSignedProtection(mock).
		WithProtection(nil, nil, protectionRecipientsMock{recipients: []domain.ProtectionRecipient{{
			ID:                     "dest-1",
			Label:                  "Compat RSA",
			RSAOAEP256PublicKeyDER: []byte("rsa-pub"),
			CertificateDER:         []byte("cert-der"),
		}}}).
		WithCertificateSources(catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-firma-1",
			Subject:     "CN=Ana Perez",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}}}, nil)
	body := map[string]any{
		"inputPath":      inputPath,
		"profile":        "compat",
		"recipient_ids":  []string{"dest-1"},
		"certificate_id": "cert-firma-1",
		"options": map[string]any{
			"container": "authenvelopeddata",
		},
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/protect-sign", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Document.Name != "" {
		t.Fatalf("no se esperaba ejecutar el caso de uso: %#v", mock.lastCmd)
	}
	if !strings.Contains(rr.Body.String(), "AuthEnvelopedData") {
		t.Fatalf("cuerpo inesperado: %s", rr.Body.String())
	}
}

func TestRoutes_UnprotectEncryptedDataWithSecret(t *testing.T) {
	mock := &unprotectUseCaseMock{}
	adaptador := rest.New(nil, nil, nil).WithProtection(nil, mock, nil)
	secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	body := map[string]any{
		"name":           "secreto.txt.encrypted.p7m",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("cms")),
		"mime_type":      domain.MIMETypeProtectedCMS,
		"secret_b64":     secret,
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/unprotect", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if mock.lastCmd.Options["secret_b64"] != secret {
		t.Fatalf("secret_b64 inesperado: %#v", mock.lastCmd.Options["secret_b64"])
	}
}

func TestRoutes_Index(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type inesperado: %q", ct)
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") ||
		!strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("Content-Security-Policy ausente o insegura: %q", csp)
	}
	if got := rr.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy inesperada: %q", got)
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options inesperada: %q", got)
	}
	if got := rr.Header().Get("Permissions-Policy"); !strings.Contains(got, "camera=()") {
		t.Fatalf("Permissions-Policy ausente o insegura: %q", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control inesperada: %q", got)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/openapi.json") || !strings.Contains(body, "/health") ||
		!strings.Contains(body, "/signer") || !strings.Contains(body, "/validator") {
		t.Fatalf("respuesta index inesperada: %s", body)
	}
	if !strings.Contains(body, `id="languageSelect"`) {
		t.Fatalf("index sin selector visible de idioma: %s", body)
	}
	if !strings.Contains(body, `/signer?lang=es`) || !strings.Contains(body, `/validator?lang=es`) ||
		!strings.Contains(body, `/openapi.json?lang=es`) || !strings.Contains(body, `/health?lang=es`) ||
		!strings.Contains(body, `/certificates?lang=es`) {
		t.Fatalf("index sin propagacion visible del locale por enlaces: %s", body)
	}
	if !strings.Contains(body, "Selección de páginas inválida. Usa 1, 1,3-5 o all.") {
		t.Fatalf("index sin validacion visible de rangos de pagina: %s", body)
	}
	if !strings.Contains(body, `id="signFileMeta"`) {
		t.Fatalf("index sin resumen visible de ficheros seleccionados: %s", body)
	}
	if !strings.Contains(body, `id="signDirectory"`) || !strings.Contains(body, `webkitdirectory`) {
		t.Fatalf("index sin seleccion de carpeta para lote: %s", body)
	}
	if !strings.Contains(body, `id="signPlanSummary"`) || !strings.Contains(body, "Plan de firma") {
		t.Fatalf("index sin plan visible de firma: %s", body)
	}
	if !strings.Contains(body, `id="selectedCertificateSummary"`) {
		t.Fatalf("index sin resumen visible del certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, `id="selectedCertificateStatus"`) {
		t.Fatalf("index sin estado visible del certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, `id="selectedCertificateExportSummary"`) {
		t.Fatalf("index sin exportacion visible del certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, `data-copy-selected-certificate-json="1"`) || !strings.Contains(body, `data-copy-selected-certificate-fingerprint="1"`) {
		t.Fatalf("index sin acciones visibles para copiar/exportar certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, `id="rememberConsoleCertificate"`) || !strings.Contains(body, `id="selectedCertificatePreferenceSummary"`) {
		t.Fatalf("index sin preferencia visible para recordar el certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, `id="certificateFilterMeta"`) {
		t.Fatalf("index sin contador visible de certificados filtrados: %s", body)
	}
	if !strings.Contains(body, `id="certificateInventorySummary"`) {
		t.Fatalf("index sin resumen visible del inventario de certificados: %s", body)
	}
	if !strings.Contains(body, `id="certificateFilterSummary"`) {
		t.Fatalf("index sin estado visible de filtros de certificados: %s", body)
	}
	if !strings.Contains(body, `id="certificateUsabilitySummary"`) {
		t.Fatalf("index sin estado visible de usabilidad de certificados: %s", body)
	}
	if !strings.Contains(body, `id="certificateExpirySummary"`) {
		t.Fatalf("index sin resumen visible de caducidad proxima de certificados: %s", body)
	}
	if !strings.Contains(body, `id="rootSignatureReason"`) || !strings.Contains(body, `id="rootSignatureLocation"`) || !strings.Contains(body, `id="rootSignatureContact"`) {
		t.Fatalf("index sin metadatos PAdES visibles: %s", body)
	}
	if !strings.Contains(body, `id="rootSignProfile"`) || !strings.Contains(body, `id="rootTsaUrl"`) {
		t.Fatalf("index sin perfil de firma/TSA visibles: %s", body)
	}
	if !strings.Contains(body, `id="rootPadesSubFilter"`) {
		t.Fatalf("index sin subfiltro PAdES visible: %s", body)
	}
	if !strings.Contains(body, `id="rootFacturaePolicyVersion"`) || !strings.Contains(body, `id="rootFacturaeSignerRole"`) {
		t.Fatalf("index sin bloque visible de politica/rol FacturaE: %s", body)
	}
	if !strings.Contains(body, `id="rootFacturaePolicyIdentifier"`) || !strings.Contains(body, `id="rootFacturaePolicyIdentifierHash"`) || !strings.Contains(body, `id="rootFacturaePolicyQualifier"`) {
		t.Fatalf("index sin bloque visible de identificador/hash/qualifier FacturaE: %s", body)
	}
	if !strings.Contains(body, `id="rootFacturaeSignatureCity"`) || !strings.Contains(body, `id="rootFacturaeSignatureProvince"`) || !strings.Contains(body, `id="rootFacturaeSignaturePostalCode"`) || !strings.Contains(body, `id="rootFacturaeSignatureCountry"`) {
		t.Fatalf("index sin bloque visible de lugar de firma FacturaE: %s", body)
	}
	if !strings.Contains(body, `id="rootSealQRContent"`) {
		t.Fatalf("index sin QR del sello visible: %s", body)
	}
	if !strings.Contains(body, "Verificación automática") {
		t.Fatalf("index sin bloque de verificacion automatica: %s", body)
	}
	if !strings.Contains(body, "Resumen de seguridad") {
		t.Fatalf("index sin resumen visible de postura de seguridad: %s", body)
	}
	if !strings.Contains(body, `id="verifyOriginalSummary"`) {
		t.Fatalf("index sin resumen visible del original de referencia: %s", body)
	}
	if !strings.Contains(body, `id="verifyArtifactSummary"`) {
		t.Fatalf("index sin resumen visible del tipo probable del firmado: %s", body)
	}
	if !strings.Contains(body, `id="verifyPairSummary"`) {
		t.Fatalf("index sin resumen visible de la pareja de validacion: %s", body)
	}
	if !strings.Contains(body, `id="verifyCompatibilitySummary"`) {
		t.Fatalf("index sin resumen visible de compatibilidad de validacion: %s", body)
	}
	if !strings.Contains(body, `id="verifyAlgorithmSummary"`) {
		t.Fatalf("index sin resumen visible del stack probable de firma: %s", body)
	}
	if !strings.Contains(body, "Sin coincidencias registradas.") || !strings.Contains(body, "No hay ficheros con hash distinto.") {
		t.Fatalf("index sin detalle guiado de diferencias para hash de directorio: %s", body)
	}
	if !strings.Contains(body, `id="hashCreatePlanSummary"`) || !strings.Contains(body, `id="hashCheckPlanSummary"`) {
		t.Fatalf("index sin planes visibles de utilidades hash: %s", body)
	}
	if !strings.Contains(body, `id="hashCreateArtifactSummary"`) {
		t.Fatalf("index sin resumen visible del artefacto esperado de huella: %s", body)
	}
	if !strings.Contains(body, `id="hashCheckAlgorithmSummary"`) {
		t.Fatalf("index sin resumen visible del algoritmo de comprobacion de huella: %s", body)
	}
	if !strings.Contains(body, `id="hashCheckArtifactSummary"`) {
		t.Fatalf("index sin resumen visible del artefacto de huella: %s", body)
	}
	if !strings.Contains(body, "Cobertura") || !strings.Contains(body, "Integridad") || !strings.Contains(body, "Confianza") || !strings.Contains(body, "Evidencias") {
		t.Fatalf("index sin bloque rico de verificacion: %s", body)
	}
	if !strings.Contains(body, `id="verifyPlanSummary"`) {
		t.Fatalf("index sin plan visible de validacion: %s", body)
	}
	if !strings.Contains(body, "Proteger / Desproteger") || !strings.Contains(body, `id="protectionRecipients"`) {
		t.Fatalf("index sin consola de proteccion: %s", body)
	}
	if !strings.Contains(body, `id="protectionRecipientSummary"`) {
		t.Fatalf("index sin resumen guiado de destinatarios de proteccion: %s", body)
	}
	if !strings.Contains(body, `id="protectionCatalogSummary"`) {
		t.Fatalf("index sin resumen visible del catalogo de proteccion: %s", body)
	}
	if !strings.Contains(body, `id="protectionContainerPolicySummary"`) {
		t.Fatalf("index sin politica visible del contenedor de proteccion: %s", body)
	}
	if !strings.Contains(body, `<option value="authenvelopeddata">CMS AuthEnvelopedData (.authenveloped.p7m)</option>`) ||
		!strings.Contains(body, "AES-256-GCM y RSA-OAEP-SHA256/MGF1-SHA256") ||
		!strings.Contains(body, "authEnvelopedDataCompatible") {
		t.Fatalf("index sin flujo seguro AuthEnvelopedData: %s", body)
	}
	if !strings.Contains(body, `id="protectionSelectionStatus"`) {
		t.Fatalf("index sin estado visible de seleccion de destinatarios: %s", body)
	}
	if !strings.Contains(body, `id="protectArtifactSummary"`) {
		t.Fatalf("index sin resumen visible del artefacto protegido esperado: %s", body)
	}
	if !strings.Contains(body, `id="protectPlanSummary"`) {
		t.Fatalf("index sin plan visible de proteccion: %s", body)
	}
	if !strings.Contains(body, `id="unprotectPlanSummary"`) {
		t.Fatalf("index sin plan visible de desproteccion: %s", body)
	}
	if !strings.Contains(body, `id="unprotectArtifactSummary"`) {
		t.Fatalf("index sin resumen visible del artefacto protegido: %s", body)
	}
	if !strings.Contains(body, `id="tlsTrustSummary"`) {
		t.Fatalf("index sin resumen visible de estado TLS/trust: %s", body)
	}
	if !strings.Contains(body, `id="proxySecretStoreSummary"`) {
		t.Fatalf("index sin resumen visible del almacen seguro de secretos de proxy: %s", body)
	}
	if !strings.Contains(body, "Modo de proxy en ejecución") {
		t.Fatalf("index sin etiqueta visible de modo de proxy en ejecución: %s", body)
	}
	if !strings.Contains(body, `id="diagnosticsSummary"`) {
		t.Fatalf("index sin resumen visible de diagnostico local: %s", body)
	}
	if !strings.Contains(body, `id="diagnosticsExportSummary"`) {
		t.Fatalf("index sin salida visible para exportar/copiar diagnostico: %s", body)
	}
	if !strings.Contains(body, `data-copy-diagnostics-report="1"`) {
		t.Fatalf("index sin accion visible para copiar diagnostico: %s", body)
	}
	if !strings.Contains(body, "grxfirma.console.preferences") {
		t.Fatalf("index sin persistencia local de preferencias visibles de producto: %s", body)
	}
	if !strings.Contains(body, `id="protectBtn"`) || !strings.Contains(body, `id="unprotectBtn"`) {
		t.Fatalf("index sin acciones de proteger/desproteger: %s", body)
	}
	if !strings.Contains(body, `id="exportProtectionRecipientBtn"`) || !strings.Contains(body, `id="importProtectionRecipientBtn"`) || !strings.Contains(body, `id="importProtectionRecipientFile"`) {
		t.Fatalf("index sin intercambio de destinatarios fuertes: %s", body)
	}
}

func TestRoutes_Signer(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/signer", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type inesperado: %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Firmador web") || !strings.Contains(body, "/sign") || !strings.Contains(body, "Firma visible (PAdES)") {
		t.Fatalf("respuesta signer inesperada: %s", body)
	}
	if !strings.Contains(body, `id="languageSelect"`) {
		t.Fatalf("firmador web sin selector visible de idioma: %s", body)
	}
	if !strings.Contains(body, `href="/?lang=es"`) || !strings.Contains(body, `href="/validator?lang=es"`) ||
		!strings.Contains(body, `href="/?lang=es#hashToolsCard"`) || !strings.Contains(body, `href="/openapi.json?lang=es"`) {
		t.Fatalf("firmador web sin propagacion visible del locale por enlaces: %s", body)
	}
	if !strings.Contains(body, `id="actionSelect"`) || !strings.Contains(body, `<option value="cosign">`) || !strings.Contains(body, `<option value="countersign">`) {
		t.Fatalf("firmador web sin selector guiado de accion/cofirma: %s", body)
	}
	if !strings.Contains(body, `id="signProfile"`) || !strings.Contains(body, `id="tsaUrl"`) {
		t.Fatalf("firmador web sin perfil de firma y TSA visibles: %s", body)
	}
	if !strings.Contains(body, `id="padesSubFilter"`) {
		t.Fatalf("firmador web sin subfiltro PAdES visible: %s", body)
	}
	if !strings.Contains(body, `id="localPathInput"`) || !strings.Contains(body, `id="localOutputPathInput"`) {
		t.Fatalf("firmador web sin preferencias visibles de ruta local/salida: %s", body)
	}
	if !strings.Contains(body, `id="selectionTypeSummary"`) {
		t.Fatalf("firmador web sin resumen visible del tipo de seleccion: %s", body)
	}
	if !strings.Contains(body, `id="certificateInventoryStatus"`) {
		t.Fatalf("firmador web sin resumen visible del inventario de certificados: %s", body)
	}
	if !strings.Contains(body, `id="autoSelectCertificateBtn"`) || !strings.Contains(body, `id="certificateAutoSelectSummary"`) {
		t.Fatalf("firmador web sin autoseleccion visible de certificado: %s", body)
	}
	if !strings.Contains(body, `id="clearSelectedCertificateBtn"`) {
		t.Fatalf("firmador web sin accion visible para quitar la seleccion del certificado: %s", body)
	}
	if !strings.Contains(body, `id="certificateSubjectFilter"`) || !strings.Contains(body, `id="certificateIssuerFilter"`) {
		t.Fatalf("firmador web sin filtros visibles por titular/emisor: %s", body)
	}
	if !strings.Contains(body, `id="onlyUsableCertificates"`) || !strings.Contains(body, `id="onlyValidCertificates"`) {
		t.Fatalf("firmador web sin filtros visibles de usabilidad/caducidad: %s", body)
	}
	if !strings.Contains(body, `id="certificateFilterMeta"`) || !strings.Contains(body, `id="certificateFilterSummary"`) {
		t.Fatalf("firmador web sin resumen visible del filtrado de certificados: %s", body)
	}
	if !strings.Contains(body, `id="rememberSelectedCertificate"`) || !strings.Contains(body, `id="certificatePreferenceSummary"`) {
		t.Fatalf("firmador web sin preferencia visible de recordar certificado: %s", body)
	}
	if !strings.Contains(body, `id="certificateStatus"`) {
		t.Fatalf("firmador web sin estado visible del certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, `id="certificateExpiryStatus"`) {
		t.Fatalf("firmador web sin aviso visible de caducidad del certificado: %s", body)
	}
	if !strings.Contains(body, `id="certificateDetailsSummary"`) {
		t.Fatalf("firmador web sin ficha visible de identidad del certificado: %s", body)
	}
	if !strings.Contains(body, `id="certificateExportSummary"`) {
		t.Fatalf("firmador web sin exportacion visible del certificado seleccionado: %s", body)
	}
	if !strings.Contains(body, "Copiar JSON del certificado") {
		t.Fatalf("firmador web sin utilidad visible para copiar el JSON del certificado: %s", body)
	}
	if !strings.Contains(body, "Copiar huella") {
		t.Fatalf("firmador web sin utilidad visible para copiar la huella del certificado: %s", body)
	}
	if !strings.Contains(body, `id="metadataSummary"`) {
		t.Fatalf("firmador web sin resumen visible de metadatos de firma: %s", body)
	}
	if !strings.Contains(body, `id="metadataApplicabilitySummary"`) {
		t.Fatalf("firmador web sin estado visible de aplicabilidad de metadatos: %s", body)
	}
	if !strings.Contains(body, `id="metadataDeliverySummary"`) {
		t.Fatalf("firmador web sin resumen visible de entrega efectiva de metadatos: %s", body)
	}
	if !strings.Contains(body, `id="signPlanSummary"`) {
		t.Fatalf("firmador web sin plan visible de firma: %s", body)
	}
	if !strings.Contains(body, `id="quickVerifyFileInput"`) || !strings.Contains(body, `id="quickVerifyBtn"`) {
		t.Fatalf("firmador web sin validacion rapida visible: %s", body)
	}
	if !strings.Contains(body, `id="quickVerifySourceSummary"`) || !strings.Contains(body, `id="quickVerifyPlanSummary"`) {
		t.Fatalf("firmador web sin resumen visible de validacion rapida: %s", body)
	}
	if !strings.Contains(body, `id="quickVerifyOriginalFileInput"`) || !strings.Contains(body, `id="quickVerifyOriginalPathInput"`) {
		t.Fatalf("firmador web sin original de referencia visible en verificacion rapida: %s", body)
	}
	if !strings.Contains(body, `id="quickVerifyOriginalSummary"`) || !strings.Contains(body, `id="quickVerifyPairSummary"`) {
		t.Fatalf("firmador web sin resumen visible del emparejamiento de verificacion: %s", body)
	}
	if !strings.Contains(body, `id="quickVerifyResultSummary"`) {
		t.Fatalf("firmador web sin resumen compacto del resultado de verificacion rapida: %s", body)
	}
	if !strings.Contains(body, `id="verificationExportSummary"`) {
		t.Fatalf("firmador web sin exportacion visible del ultimo resultado de verificacion: %s", body)
	}
	if !strings.Contains(body, "Copiar JSON de verificación") {
		t.Fatalf("firmador web sin utilidad visible para copiar el JSON de verificacion: %s", body)
	}
	if !strings.Contains(body, `id="resultArtifactSummary"`) {
		t.Fatalf("firmador web sin resumen visible del ultimo artefacto firmado: %s", body)
	}
	if !strings.Contains(body, `id="enableMultiCosign"`) || !strings.Contains(body, `id="multiCosignSelect"`) {
		t.Fatalf("firmador web sin cofirma multiple guiada visible: %s", body)
	}
	if !strings.Contains(body, `id="multiCosignSummary"`) {
		t.Fatalf("firmador web sin resumen visible de cofirma multiple: %s", body)
	}
	if !strings.Contains(body, `id="multiCosignOutputNotice"`) {
		t.Fatalf("firmador web sin politica visible de salida para cofirma multiple: %s", body)
	}
	if !strings.Contains(body, `id="multiCosignSelectAllBtn"`) || !strings.Contains(body, `id="multiCosignClearBtn"`) {
		t.Fatalf("firmador web sin acciones visibles para gestionar cofirmantes adicionales: %s", body)
	}
	if !strings.Contains(body, `["PAdES", "ODF", "OOXML"].includes(currentFormat())`) {
		t.Fatalf("firmador web ofrece cofirma múltiple en formatos que el motor no puede cofirmar")
	}
	if !strings.Contains(body, `const followingCoSignOptions = stripVisualOptionsForCoSign(payload.options);`) ||
		!strings.Contains(body, `options: followingCoSignOptions`) {
		t.Fatalf("firmador web no reutiliza opciones saneadas en las cofirmas posteriores")
	}
	stripStart := strings.Index(body, `function stripVisualOptionsForCoSign(options)`)
	if stripStart < 0 {
		t.Fatalf("firmador web no contiene el saneado aislado de opciones visuales")
	}
	stripEnd := strings.Index(body[stripStart:], `function fileToBase64(file)`)
	if stripEnd < 0 {
		t.Fatalf("firmador web no contiene el saneado aislado de opciones visuales")
	}
	stripBlock := body[stripStart : stripStart+stripEnd]
	for _, visualOption := range []string{
		`"visibleSealImageBase64"`,
		`"visibleSealSignerSummary"`,
		`"qrContent"`,
		`"visibleSealQRContent"`,
	} {
		if !strings.Contains(stripBlock, visualOption) {
			t.Fatalf("firmador web no elimina la opción visual %s de las cofirmas posteriores", visualOption)
		}
	}
	if !strings.Contains(body, `id="certificateRoleSummary"`) || !strings.Contains(body, "Rol del certificado") {
		t.Fatalf("firmador web sin resumen visible del rol operativo del certificado: %s", body)
	}
	if !strings.Contains(body, `id="operationStrategySummary"`) || !strings.Contains(body, "Estrategia de operación") {
		t.Fatalf("firmador web sin estrategia visible de operacion: %s", body)
	}
	if !strings.Contains(body, `id="verificationPlanSummary"`) {
		t.Fatalf("firmador web sin resumen visible de verificacion posterior: %s", body)
	}
	if !strings.Contains(body, `id="outputDeliverySummary"`) {
		t.Fatalf("firmador web sin resumen visible de persistencia efectiva: %s", body)
	}
	if !strings.Contains(body, `id="proxySecretStoreSummary"`) || !strings.Contains(body, "Almacen seguro de proxy") {
		t.Fatalf("firmador web sin estado visible del almacen seguro de proxy: %s", body)
	}
	if !strings.Contains(body, "Modo de proxy en ejecución") {
		t.Fatalf("firmador web sin etiqueta visible de modo de proxy en ejecución: %s", body)
	}
	if !strings.Contains(body, `id="saveToDisk"`) || !strings.Contains(body, `id="returnB64"`) || !strings.Contains(body, `id="overwriteOutput"`) {
		t.Fatalf("firmador web sin politicas visibles de salida: %s", body)
	}
	if !strings.Contains(body, "/?lang=es#hashToolsCard") {
		t.Fatalf("firmador web sin acceso directo a utilidades de huellas: %s", body)
	}
	if !strings.Contains(body, "/sign-batch") || !strings.Contains(body, `type="file" multiple`) {
		t.Fatalf("firmador web sin soporte de lote/multiple: %s", body)
	}
	if !strings.Contains(body, `id="directoryInput"`) || !strings.Contains(body, `id="pickDirectoryBtn"`) {
		t.Fatalf("firmador web sin seleccion de carpeta: %s", body)
	}
	if !strings.Contains(body, `<option value="AUTO">AUTO</option>`) {
		t.Fatalf("firmador web sin formato AUTO: %s", body)
	}
	if !strings.Contains(body, `<option value="ODF">ODF</option>`) || !strings.Contains(body, `<option value="OOXML">OOXML</option>`) || !strings.Contains(body, `<option value="FacturaE">FacturaE</option>`) || !strings.Contains(body, `<option value="ASiC-XAdES">ASiC-XAdES</option>`) {
		t.Fatalf("firmador web sin formatos ODF/OOXML/FacturaE/ASiC-XAdES: %s", body)
	}
	if !strings.Contains(body, "invalidPageSelection") {
		t.Fatalf("firmador web sin validacion de rangos de pagina: %s", body)
	}
	if !strings.Contains(body, `id="signatureReason"`) || !strings.Contains(body, `id="signatureLocation"`) || !strings.Contains(body, `id="signatureContact"`) {
		t.Fatalf("firmador web sin metadatos PAdES visibles: %s", body)
	}
	if !strings.Contains(body, `id="facturaePolicyVersion"`) || !strings.Contains(body, `id="facturaePolicyIdentifier"`) || !strings.Contains(body, `id="facturaePolicyIdentifierHash"`) || !strings.Contains(body, `id="facturaePolicyQualifier"`) || !strings.Contains(body, `id="facturaeSignerRole"`) {
		t.Fatalf("firmador web sin FacturaE avanzada visible: %s", body)
	}
	if !strings.Contains(body, `id="facturaeSignatureCity"`) || !strings.Contains(body, `id="facturaeSignatureProvince"`) || !strings.Contains(body, `id="facturaeSignaturePostalCode"`) || !strings.Contains(body, `id="facturaeSignatureCountry"`) {
		t.Fatalf("firmador web sin lugar de firma FacturaE visible: %s", body)
	}
	if !strings.Contains(body, `id="signatureQRContent"`) {
		t.Fatalf("firmador web sin QR del sello visible: %s", body)
	}
	if !strings.Contains(body, "verifyAfterSign") || !strings.Contains(body, "verificationSigners") {
		t.Fatalf("firmador web sin bloque de verificacion automatica: %s", body)
	}
	if !strings.Contains(body, "verificationPosture") {
		t.Fatalf("firmador web sin resumen visible de postura de seguridad: %s", body)
	}
	if !strings.Contains(body, "verificationPlanTitle") || !strings.Contains(body, "verificationPlanSign") {
		t.Fatalf("firmador web sin mensajes guiados de verificacion posterior: %s", body)
	}
	if !strings.Contains(body, "signProfileHelp") || !strings.Contains(body, "tsaUrlHelp") {
		t.Fatalf("firmador web sin ayuda guiada de perfil/TSA: %s", body)
	}
	if !strings.Contains(body, "padesSubFilterHelp") {
		t.Fatalf("firmador web sin ayuda guiada de subfiltro PAdES: %s", body)
	}
	if !strings.Contains(body, "verificationCoverage") || !strings.Contains(body, "verificationIntegrity") || !strings.Contains(body, "verificationEvidence") {
		t.Fatalf("firmador web sin bloque rico de verificacion: %s", body)
	}
	incidentStart := strings.Index(body, `function buildSupportIncidentPayload(diagnostics)`)
	incidentEnd := strings.Index(body, `function renderSupportExportSummary(payload)`)
	if incidentStart < 0 || incidentEnd <= incidentStart {
		t.Fatal("firmador web sin constructor acotado de incidencia")
	}
	incidentBlock := body[incidentStart:incidentEnd]
	for _, required := range []string{
		`supportIncidentFileName(ui.localPathInput.value)`,
		`supportIncidentSafeDiagnostics(diagnostics)`,
		`supportIncidentSafeGuidedDiagnostic(state.lastGuidedDiagnostic)`,
		`supportIncidentSafeRequestId(state.lastRequestId)`,
		`verification_report_available`,
		`multi_cosign_count`,
	} {
		if !strings.Contains(incidentBlock, required) {
			t.Fatalf("incidencia web sin saneado requerido %q", required)
		}
	}
	for _, forbidden := range []string{
		`local_path:`,
		`output_path:`,
		`multi_cosign_additional_ids`,
		`verification_json:`,
		`certificate_json:`,
		`cert.id`,
		`cert.subjectName`,
		`cert.issuerName`,
		`diagnostics ||`,
	} {
		if strings.Contains(incidentBlock, forbidden) {
			t.Fatalf("incidencia web conserva dato sensible/no acotado %q", forbidden)
		}
	}
	for _, required := range []string{
		`function newSignRequestId()`,
		`window.crypto.getRandomValues(bytes)`,
		`const requestPayload = { ...payload, request_id: requestId }`,
		`request_id: newSignRequestId()`,
		`state.lastRequestId = supportIncidentSafeRequestId(response.request_id) || payload.request_id`,
		`error.diagnostic = data && data.diagnostic`,
		`state.lastGuidedDiagnostic = error.diagnostic`,
		`function guidedDiagnosticCategoryLabel(value)`,
		`case "local_web_service":`,
		`case "government_afirma":`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("firmador web sin correlacion segura requerida %q", required)
		}
	}
}

func TestRoutes_Validator(t *testing.T) {
	t.Parallel()

	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/validator", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /validator = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content-type inesperado: %q", ct)
	}

	body := rr.Body.String()
	for _, expected := range []string{
		"Validación avanzada",
		`id="signedFile"`,
		`id="originalFile"`,
		`id="verifyResult"`,
		`id="certificateSelect"`,
		`id="validateCertificateBtn"`,
		`id="onlineCheckBtn"`,
		`id="hashInput"`,
		`id="hashReference"`,
		`id="downloadVerifyBtn"`,
		`id="downloadCertificateBtn"`,
		`id="downloadHashBtn"`,
		`"/verify"`,
		`"/certificates?check=1"`,
		`"/certificates/validate"`,
		`"/certificates/online-check"`,
		`"/hash/check"`,
		`href="/signer?lang=es"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("validador sin contrato visible %q", expected)
		}
	}
	for _, securityContract := range []string{
		`const MAX_FILE_BYTES = 64 * 1024 * 1024`,
		`const MAX_REQUEST_RAW_BYTES = 72 * 1024 * 1024`,
		`ensureCombinedFileSize([signed, original])`,
		`ensureCombinedFileSize([input, check ? reference : null])`,
		`validation: validation, onlineRevocation: response`,
		`let certificateRequestGeneration = 0`,
		`requestGeneration !== certificateRequestGeneration`,
		`String(currentCertificate.id || "") !== requestedCertificateID`,
		`onlineRevocation: {status: "unavailable", error: M.requestFailed}`,
		`M.signature + ": "`,
	} {
		if !strings.Contains(body, securityContract) {
			t.Errorf("validador sin contrato funcional/seguridad %q", securityContract)
		}
	}
	if strings.Count(body, `requestGeneration !== certificateRequestGeneration`) < 4 {
		t.Error("validador sin descarte de respuestas obsoletas en validación local y revocación online")
	}
	for _, forbidden := range []string{
		"localStorage",
		"sessionStorage",
		"console.log",
		".innerHTML",
		"document.write",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("validador contiene primitiva no permitida %q", forbidden)
		}
	}
}

func TestRoutes_ValidatorAliasYMetodo(t *testing.T) {
	t.Parallel()

	adaptador := rest.New(nil, nil, nil)
	alias := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(alias, httptest.NewRequest(http.MethodGet, "/validador?lang=es", nil))
	if alias.Code != http.StatusOK || !strings.Contains(alias.Body.String(), "Validación avanzada") {
		t.Fatalf("GET /validador = %d cuerpo=%s", alias.Code, alias.Body.String())
	}

	invalidMethod := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(invalidMethod, httptest.NewRequest(http.MethodPost, "/validator", nil))
	if invalidMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /validator = %d, esperaba %d", invalidMethod.Code, http.StatusMethodNotAllowed)
	}
}

func TestRoutes_ValidatorLocaleQueryRendersSelectedLanguage(t *testing.T) {
	t.Parallel()

	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/validator?lang=en", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /validator?lang=en = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, expected := range []string{
		`<html lang="en">`,
		`<option value="en" selected>`,
		"Advanced validation",
		"Verify signatures, certificates and hashes without sending documents outside this computer.",
		"Certificate validation",
		"Hashes and integrity",
		`href="/signer?lang=en"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("validador inglés sin %q", expected)
		}
	}
	for _, spanish := range []string{
		"Validación avanzada",
		"Selecciona primero los ficheros necesarios.",
		"El fichero supera el tamaño máximo permitido.",
	} {
		if strings.Contains(body, spanish) {
			t.Errorf("validador inglés mantiene texto visible en castellano %q", spanish)
		}
	}
}

func TestRoutes_IndexLocaleQueryRendersSelectedLanguage(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/?lang=en", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<html lang="en">`) {
		t.Fatalf("index sin html lang en ingles: %s", body)
	}
	if !strings.Contains(body, `<option value="en" selected>`) {
		t.Fatalf("index sin selector de idioma marcado en ingles: %s", body)
	}
	if !strings.Contains(body, `>English</option>`) || !strings.Contains(body, `>Spanish</option>`) {
		t.Fatalf("index sin nombres localizados de idiomas en ingles: %s", body)
	}
	if !strings.Contains(body, `>Web signer</a>`) {
		t.Fatalf("index sin acceso al firmador localizado en ingles: %s", body)
	}
	if !strings.Contains(body, "Refresh status") || !strings.Contains(body, "Service status") {
		t.Fatalf("index sin hero/status localizado en ingles: %s", body)
	}
	if !strings.Contains(body, "Diagnostic report") || !strings.Contains(body, "Install local TLS trust") || !strings.Contains(body, "Clear local TLS store") {
		t.Fatalf("index sin panel de diagnostico/TLS localizado en ingles: %s", body)
	}
	if !strings.Contains(body, "The proxy secure store is available for this local backend.") || !strings.Contains(body, "System trust") {
		t.Fatalf("index sin resumen visible de TLS/proxy localizado en ingles: %s", body)
	}
	if !strings.Contains(body, "Hash file path") || !strings.Contains(body, "Report path (.hashreport)") || !strings.Contains(body, "Save report") {
		t.Fatalf("index sin panel hash localizado en ingles: %s", body)
	}
	if strings.Contains(body, "Guardar informe") || strings.Contains(body, "Ruta del fichero de huella") || strings.Contains(body, "Confianza del sistema") {
		t.Fatalf("index mantiene textos visibles funcionales en castellano para lang=en: %s", body)
	}
	if !strings.Contains(body, `/signer?lang=en`) || !strings.Contains(body, `/validator?lang=en`) ||
		!strings.Contains(body, `/openapi.json?lang=en`) {
		t.Fatalf("index sin enlaces renderizados con lang=en: %s", body)
	}
}

func TestRoutes_SignerLocaleQueryRendersSelectedLanguage(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/signer?lang=en", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<html lang="en">`) {
		t.Fatalf("firmador web sin html lang en ingles: %s", body)
	}
	if !strings.Contains(body, `<option value="en" selected>`) {
		t.Fatalf("firmador web sin selector de idioma marcado en ingles: %s", body)
	}
	if !strings.Contains(body, `>English</option>`) || !strings.Contains(body, `>Spanish</option>`) {
		t.Fatalf("firmador web sin nombres localizados de idiomas en ingles: %s", body)
	}
	if !strings.Contains(body, `>Full console</a>`) || !strings.Contains(body, `>Hash tools</a>`) {
		t.Fatalf("firmador web sin enlaces de ayuda/local navigation localizados en ingles: %s", body)
	}
	if !strings.Contains(body, "Certificate and signing") || !strings.Contains(body, "Quick verification") {
		t.Fatalf("firmador web sin bloques visibles clave localizados en ingles: %s", body)
	}
	if !strings.Contains(body, "Local channel between browser and application") ||
		!strings.Contains(body, "Public @firma platform") {
		t.Fatalf("firmador web sin categorias de diagnostico localizadas en ingles: %s", body)
	}
	if !strings.Contains(body, "Advanced FacturaE") || !strings.Contains(body, "FacturaE policy version") || !strings.Contains(body, "Policy qualifier") {
		t.Fatalf("firmador web sin bloque FacturaE localizado en ingles: %s", body)
	}
	if !strings.Contains(body, "No certificate selected.") || !strings.Contains(body, "Certificate preference") {
		t.Fatalf("firmador web sin paneles de certificados localizados en ingles: %s", body)
	}
	if !strings.Contains(body, "Next step") || !strings.Contains(body, "Select a file, a folder or a local path first to prepare the operation.") {
		t.Fatalf("firmador web sin asistente guiado localizado en ingles: %s", body)
	}
	if strings.Contains(body, "Certificado y firma") || strings.Contains(body, "Validación rápida") {
		t.Fatalf("firmador web mantiene textos principales en castellano para lang=en: %s", body)
	}
	if strings.Contains(body, "Opcional. Estos campos solo se aplican cuando el formato efectivo es FacturaE.") || strings.Contains(body, "Sin documento original de referencia.") || strings.Contains(body, "Siguiente paso") {
		t.Fatalf("firmador web mantiene ayudas o estados visibles en castellano para lang=en: %s", body)
	}
	if !strings.Contains(body, `href="/?lang=en"`) || !strings.Contains(body, `href="/validator?lang=en"`) ||
		!strings.Contains(body, `href="/?lang=en#hashToolsCard"`) || !strings.Contains(body, `href="/openapi.json?lang=en"`) {
		t.Fatalf("firmador web sin enlaces renderizados con lang=en: %s", body)
	}
}

func TestRoutes_VerifyByPathCompatQt(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "firma.csig")
	if err := os.WriteFile(inputPath, []byte("firma"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}

	adaptador := rest.New(nil, verifyUseCaseMock{}, nil).WithFileSystemPaths()
	body := map[string]any{
		"inputPath": inputPath,
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/verify", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Fatalf("ok inesperado: %#v", resp["ok"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result inesperado: %#v", resp["result"])
	}
	if result["valid"] != true {
		t.Fatalf("result.valid inesperado: %#v", result["valid"])
	}
}

func TestRoutes_VerifyDetached(t *testing.T) {
	adaptador := rest.New(nil, verifyUseCaseMock{}, nil)
	body := map[string]any{
		"name":                    "firma.csig",
		"content_base64":          base64.StdEncoding.EncodeToString([]byte("firma")),
		"mime_type":               "application/pkcs7-signature",
		"original_content_base64": base64.StdEncoding.EncodeToString([]byte("original")),
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/verify", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
}

func TestRoutes_SelectCertificate(t *testing.T) {
	adaptador := rest.New(nil, nil, selectUseCaseMock{})
	body := map[string]any{
		"subject_filter":    "ana",
		"solo_no_caducados": true,
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/select-certificate", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["certificate_id"] != "cert-ana" {
		t.Fatalf("certificate_id inesperado: %#v", resp["certificate_id"])
	}
}

func TestRoutes_MethodNotAllowed(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/sign", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
}

func TestRoutes_OpenAPI(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"openapi"`)) {
		t.Fatalf("respuesta openapi inesperada: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"/sign-batch"`)) {
		t.Fatalf("faltaba /sign-batch en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`Identificador opcional de idempotencia del lote`)) {
		t.Fatalf("faltaba request_id del lote en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"/hash"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"/hash/check"`)) {
		t.Fatalf("faltaba documentacion de hash en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"/certificates/validate"`)) ||
		!bytes.Contains(rr.Body.Bytes(), []byte(`"/certificates/online-check"`)) {
		t.Fatalf("faltaba documentacion de validacion de certificados en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"page":"all"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`1,3-5`)) {
		t.Fatalf("faltaba documentacion de page=all/rangos en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"reason":"Firma electrónica avanzada"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"contactInfo":"correo@ejemplo.es"`)) {
		t.Fatalf("faltaba documentacion de metadatos PAdES en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"qrContent":"https://verifica.ejemplo/"`)) {
		t.Fatalf("faltaba documentacion de QR del sello en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"FacturaE"`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"ASiC-XAdES"`)) {
		t.Fatalf("faltaba documentacion de formatos FacturaE/ASiC-XAdES en openapi: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"authenvelopeddata"`)) ||
		!bytes.Contains(rr.Body.Bytes(), []byte(`"authEnvelopedDataCompatible"`)) ||
		!bytes.Contains(rr.Body.Bytes(), []byte(`.authenveloped.p7m`)) ||
		!bytes.Contains(rr.Body.Bytes(), []byte(`RSA-OAEP-SHA256/MGF1-SHA256`)) {
		t.Fatalf("faltaba contrato AuthEnvelopedData seguro en openapi: %s", rr.Body.String())
	}
}

func TestRoutes_Health(t *testing.T) {
	adaptador := rest.New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("respuesta health inesperada: %s", rr.Body.String())
	}
}

func TestRoutes_Certificates(t *testing.T) {
	closeCount := 0
	adaptador := rest.New(nil, nil, nil).WithCertificateSources(
		catalogMockREST{certs: []domain.CertificateRef{{
			ID:            "cert-1",
			Subject:       "CN=Ana Perez",
			Issuer:        "CN=FNMT",
			NotAfter:      time.Now().Add(time.Hour),
			Fingerprint:   "fp1",
			HasSigningKey: true,
		}}},
		closingSigningKeyProviderREST{key: &closingSigningKeyREST{closeCount: &closeCount}},
	)

	req := httptest.NewRequest(http.MethodGet, "/certificates", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"subjectName":"CN=Ana Perez"`)) {
		t.Fatalf("respuesta certificates inesperada: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"canSign":true`)) {
		t.Fatalf("se esperaba canSign=true: %s", rr.Body.String())
	}
	if closeCount != 0 {
		t.Fatalf("listar certificados abrió una clave privada, cierres=%d", closeCount)
	}
}

func TestRoutes_CertificateValidate_NoConfiaEnAutofirmadoPresentado(t *testing.T) {
	_, certificatePEM, _ := generarCertificadoAuth(t)
	block, _ := pem.Decode([]byte(certificatePEM))
	if block == nil {
		t.Fatal("fixture PEM inválido")
	}
	adaptador := rest.New(nil, nil, nil).WithCertificateSources(
		catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-autofirmado",
			Subject:     "CN=Auth Test",
			Issuer:      "CN=Auth Test",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fixture",
		}}},
		signingKeyProviderChainREST{chain: [][]byte{block.Bytes}},
	)

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/certificates/validate", map[string]any{
		"certificate_id": "cert-autofirmado",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /certificates/validate = %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		OK                    bool     `json:"ok"`
		Valid                 bool     `json:"valid"`
		TimeValid             bool     `json:"time_valid"`
		DigitalSignatureUsage bool     `json:"digital_signature_usage"`
		Trusted               bool     `json:"trusted"`
		ChainDepth            int      `json:"chain_depth"`
		PresentedChainDepth   int      `json:"presented_chain_depth"`
		Issues                []string `json:"issues"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta JSON inválida: %v", err)
	}
	if !response.OK || response.Valid || response.Trusted || !response.TimeValid || !response.DigitalSignatureUsage {
		t.Fatalf("resultado inesperado: %+v", response)
	}
	if response.ChainDepth != 0 || response.PresentedChainDepth != 1 {
		t.Fatalf("profundidad inesperada: %+v", response)
	}
	if bytes.Contains(rr.Body.Bytes(), []byte(`"trust_error"`)) {
		t.Fatalf("la API no debe exponer el error X.509 crudo: %s", rr.Body.String())
	}
}

func TestRoutes_CertificateValidate_RechazaCadenaVaciaYMetodo(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithCertificateSources(
		catalogMockREST{certs: []domain.CertificateRef{{
			ID:          "cert-sin-cadena",
			Subject:     "CN=Sin cadena",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fixture",
		}}},
		signingKeyProviderChainREST{},
	)

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/certificates/validate", map[string]any{
		"certificate_id": "cert-sin-cadena",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("cadena vacía = %d, esperaba 400; cuerpo=%s", rr.Code, rr.Body.String())
	}
	method := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(method, httptest.NewRequest(http.MethodGet, "/certificates/validate", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /certificates/validate = %d, esperaba 405", method.Code)
	}
}

func TestRoutes_CertificateEndpoints_CierranClaveEnExitoYError(t *testing.T) {
	_, certificatePEM, _ := generarCertificadoAuth(t)
	block, _ := pem.Decode([]byte(certificatePEM))
	if block == nil {
		t.Fatal("fixture PEM inválido")
	}
	onlineChain := generarCadenaCertificadoREST(t)
	tests := []struct {
		name         string
		path         string
		chain        [][]byte
		wantCode     int
		wantResponse string
	}{
		{
			name:         "validacion_resultado",
			path:         "/certificates/validate",
			chain:        [][]byte{block.Bytes},
			wantCode:     http.StatusOK,
			wantResponse: `"trusted":false`,
		},
		{
			name:         "validacion_error",
			path:         "/certificates/validate",
			chain:        nil,
			wantCode:     http.StatusBadRequest,
			wantResponse: `"error"`,
		},
		{
			name:         "online_resultado",
			path:         "/certificates/online-check",
			chain:        onlineChain,
			wantCode:     http.StatusOK,
			wantResponse: `"status":"inconclusive"`,
		},
		{
			name:         "online_error_controlado",
			path:         "/certificates/online-check",
			chain:        [][]byte{[]byte("DER inválido")},
			wantCode:     http.StatusOK,
			wantResponse: `"status":"unavailable"`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			closeCount := 0
			key := &closingSigningKeyREST{
				chain:      test.chain,
				closeCount: &closeCount,
			}
			adaptador := rest.New(nil, nil, nil).WithCertificateSources(
				catalogMockREST{certs: []domain.CertificateRef{{
					ID:          "cert-close",
					Subject:     "CN=Close",
					NotAfter:    time.Now().Add(time.Hour),
					Fingerprint: "fixture",
				}}},
				closingSigningKeyProviderREST{key: key},
			)

			rr := doJSON(t, adaptador.Routes(), http.MethodPost, test.path, map[string]any{
				"certificate_id": "cert-close",
			})
			if rr.Code != test.wantCode {
				t.Fatalf("%s = %d, esperaba %d; cuerpo=%s", test.path, rr.Code, test.wantCode, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), test.wantResponse) {
				t.Fatalf("%s no contiene %q: %s", test.path, test.wantResponse, rr.Body.String())
			}
			if closeCount != 1 {
				t.Fatalf("Close() llamado %d veces, esperaba exactamente una", closeCount)
			}
		})
	}
}

func TestRoutes_CertificateEndpoints_CierranClaveDevueltaConError(t *testing.T) {
	for _, path := range []string{"/certificates/validate", "/certificates/online-check"} {
		path := path
		t.Run(path, func(t *testing.T) {
			closeCount := 0
			adaptador := rest.New(nil, nil, nil).WithCertificateSources(
				catalogMockREST{certs: []domain.CertificateRef{{
					ID:       "cert-error-close",
					Subject:  "CN=Close error",
					NotAfter: time.Now().Add(time.Hour),
				}}},
				closingSigningKeyProviderREST{
					key: &closingSigningKeyREST{closeCount: &closeCount},
					err: errors.New("fallo simulado tras adquirir la clave"),
				},
			)

			rr := doJSON(t, adaptador.Routes(), http.MethodPost, path, map[string]any{
				"certificate_id": "cert-error-close",
			})
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, esperaba 400; cuerpo=%s", path, rr.Code, rr.Body.String())
			}
			if closeCount != 1 {
				t.Fatalf("Close() llamado %d veces, esperaba exactamente una", closeCount)
			}
		})
	}
}

func TestRoutes_CertificatesRespetaBearerSiEstaConfigurado(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).
		WithBearerToken("token-prueba").
		WithCertificateSources(
			catalogMockREST{certs: []domain.CertificateRef{{
				ID:          "cert-1",
				Subject:     "CN=Ana Perez",
				Issuer:      "CN=FNMT",
				NotAfter:    time.Now().Add(time.Hour),
				Fingerprint: "fp1",
			}}},
			signingKeyProviderMockREST{},
		)

	req := httptest.NewRequest(http.MethodGet, "/certificates", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("codigo sin bearer = %d, want 401 cuerpo=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/certificates", nil)
	req.Header.Set("Authorization", "Bearer token-prueba")
	rr = httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo con bearer = %d, want 200 cuerpo=%s", rr.Code, rr.Body.String())
	}
}

func TestRoutes_Settings_GetYPost(t *testing.T) {
	dir := t.TempDir()
	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)

	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/settings", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado en GET inicial: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"settings":{}`)) {
		t.Fatalf("respuesta settings inicial inesperada: %s", rr.Body.String())
	}

	body := map[string]any{
		"expertMode":       true,
		"themeIndex":       2,
		"stickySigner":     true,
		"certsExpiredShow": false,
	}
	rr = doJSON(t, adaptador.Routes(), http.MethodPost, "/settings", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado en POST: %d cuerpo=%s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, adaptador.Routes(), http.MethodGet, "/settings", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado en GET final: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"expertMode":true`)) {
		t.Fatalf("settings no persistidos: %s", rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "qt-settings.json")); err != nil {
		t.Fatalf("se esperaba qt-settings.json: %v", err)
	}
}

func TestRoutes_Settings_RechazaCredencialesProxyEnClaro(t *testing.T) {
	dir := t.TempDir()
	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)

	body := map[string]any{
		"proxyUsername": "usuario",
		"proxyPassword": "secreta",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/settings", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("proxy")) {
		t.Fatalf("error inesperado: %s", rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "qt-settings.json")); !os.IsNotExist(err) {
		t.Fatalf("no debio persistir qt-settings.json, err=%v", err)
	}
}

func TestRoutes_Settings_DescartaCredencialSeguridadUIEnClaro(t *testing.T) {
	dir := t.TempDir()
	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)

	body := map[string]any{
		"expertMode":             true,
		"securityAccessPassword": "secreto-local",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/settings", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado en POST: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("securityAccessPassword")) ||
		bytes.Contains(rr.Body.Bytes(), []byte("secreto-local")) {
		t.Fatalf("la respuesta no debe reflejar la credencial UI: %s", rr.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "qt-settings.json"))
	if err != nil {
		t.Fatalf("leer qt-settings.json: %v", err)
	}
	if bytes.Contains(data, []byte("securityAccessPassword")) ||
		bytes.Contains(data, []byte("secreto-local")) {
		t.Fatalf("qt-settings.json no debe contener la credencial UI: %s", data)
	}
}

func TestRoutes_Settings_GetMigraCredencialSeguridadUIEnClaroHeredada(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "qt-settings.json")
	if err := os.WriteFile(ruta, []byte(`{
  "expertMode": true,
  "securityAccessPassword": "secreto-heredado"
}`), 0o600); err != nil {
		t.Fatalf("preparar qt-settings.json heredado: %v", err)
	}
	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)

	rr := doJSON(t, adaptador.Routes(), http.MethodGet, "/settings", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado en GET: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("securityAccessPassword")) ||
		bytes.Contains(rr.Body.Bytes(), []byte("secreto-heredado")) {
		t.Fatalf("GET no debe devolver la credencial heredada: %s", rr.Body.String())
	}

	data, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer qt-settings.json migrado: %v", err)
	}
	if bytes.Contains(data, []byte("securityAccessPassword")) ||
		bytes.Contains(data, []byte("secreto-heredado")) {
		t.Fatalf("la migración no retiró la credencial del disco: %s", data)
	}
	if !bytes.Contains(data, []byte(`"expertMode": true`)) {
		t.Fatalf("la migración perdió preferencias válidas: %s", data)
	}
}

func TestRoutes_ServiceStatus(t *testing.T) {
	adaptador := rest.New(nil, nil, nil).WithServicio(&serviceManagerMockREST{
		estado: ports.EstadoServicio{
			Instalado:  true,
			Activo:     true,
			Plataforma: "linux",
			Metodo:     "systemd-user",
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/service/status", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"status"`)) {
		t.Fatalf("respuesta service/status inesperada: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"method":"systemd-user"`)) {
		t.Fatalf("method inesperado: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"running":true`)) {
		t.Fatalf("running inesperado: %s", rr.Body.String())
	}
}

func TestRoutes_ServiceInstall(t *testing.T) {
	gestor := &serviceManagerMockREST{}
	adaptador := rest.New(nil, nil, nil).WithServicio(gestor)
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/service/install", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("se esperaba ok=true: %s", rr.Body.String())
	}
	if gestor.installSocket == "" {
		t.Fatal("se esperaba socket por defecto en la instalación del servicio")
	}
}

type pdfPreviewUseCaseMockREST struct {
	result application.PdfPreviewResult
	err    error
	cmd    application.PdfPreviewCommand
	calls  int
	input  []byte
	run    func(application.PdfPreviewCommand)
}

func (m *pdfPreviewUseCaseMockREST) Ejecutar(_ context.Context, cmd application.PdfPreviewCommand) (application.PdfPreviewResult, error) {
	m.cmd = cmd
	m.calls++
	m.input, _ = os.ReadFile(cmd.Ruta)
	if m.run != nil {
		m.run(cmd)
	}
	return m.result, m.err
}

func TestRoutes_PDFPreviewReal(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(inputPath, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}

	preview := &pdfPreviewUseCaseMockREST{result: application.PdfPreviewResult{
		DataB64:      "aW1hZ2VuLXBuZw==",
		Ancho:        612,
		Alto:         792,
		PaginaActual: 2,
		TotalPaginas: 7,
	}}
	adaptador := rest.New(nil, nil, nil).
		WithPDFPreviewPaths().
		WithPDFPreview(preview)
	body := map[string]any{
		"path": inputPath,
		"page": 2,
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/pdf/preview", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta JSON invalida: %v", err)
	}
	if response["ok"] != true ||
		response["imageBase64"] != "aW1hZ2VuLXBuZw==" ||
		response["currentPage"] != float64(2) ||
		response["totalPages"] != float64(7) ||
		response["widthPoints"] != float64(612) ||
		response["heightPoints"] != float64(792) {
		t.Fatalf("respuesta de preview inesperada: %#v", response)
	}
	if response["data"] != response["imageBase64"] ||
		response["width"] != response["widthPoints"] ||
		response["height"] != response["heightPoints"] {
		t.Fatalf("aliases Qt incompatibles: %#v", response)
	}
	if preview.calls != 1 || preview.cmd.Ruta == inputPath || preview.cmd.Pagina != 2 {
		t.Fatalf("comando de preview inesperado: calls=%d cmd=%+v", preview.calls, preview.cmd)
	}
	if string(preview.input) != "%PDF-1.4\n" {
		t.Fatalf("el renderer no recibio la copia segura: %q", preview.input)
	}
	if _, err := os.Stat(preview.cmd.Ruta); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("la copia temporal no se limpio: %v", err)
	}
}

func TestRoutes_PDFPreviewConRendererPdftoppm(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm no disponible")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo no disponible")
	}
	inputPath, err := filepath.Abs("../../../../../test/regression/fixtures/v1/samples/multiple_pages.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("fixture PDF no disponible: %v", err)
	}
	handler := rest.New(nil, nil, nil).
		WithPDFPreviewPaths().
		WithPDFPreview(application.NuevoPdfPreviewUseCase(pdfpreview.New().WithDPI(36))).
		Routes()
	rr := doJSON(t, handler, http.MethodPost, "/pdf/preview", map[string]any{
		"path": inputPath,
		"page": 2,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString(stringValue(response["imageBase64"]))
	if err != nil {
		t.Fatalf("imageBase64 invalido: %v", err)
	}
	if !bytes.HasPrefix(png, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("el renderer no devolvio PNG: %x", png[:min(len(png), 8)])
	}
	if response["currentPage"] != float64(2) || response["totalPages"] != float64(6) {
		t.Fatalf("paginacion real inesperada: %#v", response)
	}
	if response["widthPoints"] == float64(0) || response["heightPoints"] == float64(0) {
		t.Fatalf("dimensiones reales ausentes: %#v", response)
	}
}

func TestRoutes_PDFPreviewPermisoEstrechoNoHabilitaFirmaPorRuta(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(inputPath, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := &pdfPreviewUseCaseMockREST{result: application.PdfPreviewResult{
		DataB64:      "cG5n",
		Ancho:        595.28,
		Alto:         841.89,
		PaginaActual: 1,
		TotalPaginas: 1,
	}}
	handler := rest.New(signUseCaseMock{}, nil, nil).
		WithPDFPreviewPaths().
		WithPDFPreview(preview).
		Routes()

	previewResponse := doJSON(t, handler, http.MethodPost, "/pdf/preview", map[string]any{
		"path": inputPath,
		"page": 1,
	})
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200; body=%s", previewResponse.Code, previewResponse.Body.String())
	}

	signResponse := doJSON(t, handler, http.MethodPost, "/sign", map[string]any{
		"inputPath": inputPath,
		"format":    "PAdES",
	})
	if signResponse.Code != http.StatusForbidden {
		t.Fatalf("sign status = %d, want 403; body=%s", signResponse.Code, signResponse.Body.String())
	}
	if preview.calls != 1 {
		t.Fatalf("llamadas de preview = %d, want 1", preview.calls)
	}
}

func TestRoutes_PDFPreviewPaginaFueraDeRango(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(inputPath, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := &pdfPreviewUseCaseMockREST{result: application.PdfPreviewResult{
		DataB64:      "cG5n",
		Ancho:        595.28,
		Alto:         841.89,
		PaginaActual: 3,
		TotalPaginas: 3,
	}}
	rr := doJSON(t,
		rest.New(nil, nil, nil).WithFileSystemPaths().WithPDFPreview(preview).Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": inputPath, "page": 99},
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["currentPage"] != float64(3) || response["totalPages"] != float64(3) {
		t.Fatalf("paginacion fuera de rango inesperada: %#v", response)
	}
	if preview.cmd.Pagina != 99 {
		t.Fatalf("page no llego al caso de uso: %+v", preview.cmd)
	}
}

func TestRoutes_PDFPreviewRechazaRutasDeshabilitadas(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(inputPath, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := &pdfPreviewUseCaseMockREST{}
	rr := doJSON(t,
		rest.New(nil, nil, nil).WithPDFPreview(preview).Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": inputPath, "page": 1},
	)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
	if preview.calls != 0 {
		t.Fatalf("el renderer no debe ejecutarse con rutas deshabilitadas: %d llamadas", preview.calls)
	}
}

func TestRoutes_PDFPreviewRechazaEscape(t *testing.T) {
	escapePath := "/tmp/../etc/passwd"
	if runtime.GOOS == "windows" {
		windowsDir := os.Getenv("WINDIR")
		if windowsDir == "" {
			windowsDir = `C:\Windows`
		}
		escapePath = filepath.Join(windowsDir, "Temp") + string(filepath.Separator) + ".." + string(filepath.Separator) + "win.ini"
	}
	preview := &pdfPreviewUseCaseMockREST{}
	rr := doJSON(t,
		rest.New(nil, nil, nil).WithFileSystemPaths().WithPDFPreview(preview).Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": escapePath, "page": 1},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if preview.calls != 0 {
		t.Fatalf("el renderer no debe ejecutarse para una ruta de escape: %d llamadas", preview.calls)
	}
}

func TestRoutes_PDFPreviewRechazaSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.pdf")
	link := filepath.Join(dir, "link.pdf")
	if err := os.WriteFile(target, []byte("%PDF-1.7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	preview := &pdfPreviewUseCaseMockREST{}
	rr := doJSON(t,
		rest.New(nil, nil, nil).WithFileSystemPaths().WithPDFPreview(preview).Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": link, "page": 1},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if preview.calls != 0 {
		t.Fatalf("el renderer no debe ejecutarse para un symlink: %d llamadas", preview.calls)
	}
}

func TestRoutes_PDFPreviewRechazaNoRegular(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "directorio.pdf")
	if err := os.Mkdir(inputPath, 0o700); err != nil {
		t.Fatal(err)
	}
	preview := &pdfPreviewUseCaseMockREST{}
	rr := doJSON(t,
		rest.New(nil, nil, nil).WithFileSystemPaths().WithPDFPreview(preview).Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": inputPath, "page": 1},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if preview.calls != 0 {
		t.Fatalf("el renderer no debe ejecutarse para un directorio: %d llamadas", preview.calls)
	}
}

func TestRoutes_PDFPreviewRechazaFicheroExcesivo(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "grande.pdf")
	data := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("x"), 512)...)
	if err := os.WriteFile(inputPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	preview := &pdfPreviewUseCaseMockREST{}
	rr := doJSON(t,
		rest.New(nil, nil, nil).
			WithFileSystemPaths().
			WithPDFPreview(preview).
			WithMaxBodyBytes(512).
			Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": inputPath, "page": 1},
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if preview.calls != 0 {
		t.Fatalf("el renderer no debe ejecutarse para un fichero excesivo: %d llamadas", preview.calls)
	}
}

func TestRoutes_PDFPreviewUsaCopiaPrivadaAnteIntercambio(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "doc.pdf")
	replacementPath := filepath.Join(dir, "replacement.pdf")
	const original = "%PDF-1.7\ncontenido-original"
	if err := os.WriteFile(inputPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacementPath, []byte("%PDF-1.7\ncontenido-sustituido"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := &pdfPreviewUseCaseMockREST{result: application.PdfPreviewResult{
		DataB64:      "cG5n",
		Ancho:        595.28,
		Alto:         841.89,
		PaginaActual: 1,
		TotalPaginas: 1,
	}}
	preview.run = func(cmd application.PdfPreviewCommand) {
		if err := os.Remove(inputPath); err != nil {
			t.Fatalf("Remove(inputPath): %v", err)
		}
		if err := os.Symlink(replacementPath, inputPath); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		rendererInput, err := os.ReadFile(cmd.Ruta)
		if err != nil {
			t.Fatalf("ReadFile(copia privada): %v", err)
		}
		if string(rendererInput) != original {
			t.Fatalf("el renderer observo el fichero intercambiado: %q", rendererInput)
		}
	}
	rr := doJSON(t,
		rest.New(nil, nil, nil).WithFileSystemPaths().WithPDFPreview(preview).Routes(),
		http.MethodPost,
		"/pdf/preview",
		map[string]any{"path": inputPath, "page": 1},
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if string(preview.input) != original {
		t.Fatalf("la entrada del renderer no coincide con los bytes validados: %q", preview.input)
	}
}

func TestRoutes_CertificatesImport(t *testing.T) {
	dir := t.TempDir()
	data := generarP12REST(t, "Import Qt", "secret")

	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)
	body := map[string]any{
		"p12B64":   base64.StdEncoding.EncodeToString(data),
		"password": "secret",
	}
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/certificates/import", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("se esperaba ok=true: %s", rr.Body.String())
	}
	pkcs12Dir := filepath.Join(dir, "pkcs12")
	entries, err := os.ReadDir(pkcs12Dir)
	if err != nil {
		t.Fatalf("os.ReadDir(pkcs12Dir) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("se esperaba 1 P12 importado, got %d", len(entries))
	}
}

func TestRoutes_CertificatesImport_RechazaP12Sobredimensionado(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte{0x7f}, 10*1024*1024+1)
	body := map[string]any{
		"p12B64":   base64.StdEncoding.EncodeToString(data),
		"password": "secret",
	}

	rr := doJSON(t, rest.New(nil, nil, nil).WithConfigDir(dir).Routes(), http.MethodPost, "/certificates/import", body)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "pkcs12")); !os.IsNotExist(err) {
		t.Fatalf("un P12 sobredimensionado no debe persistirse: %v", err)
	}
}

func TestRoutes_TLSTrustStatus(t *testing.T) {
	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost.crt.pem"), []byte("cert"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost.key.pem"), []byte("key"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(key) error = %v", err)
	}

	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)
	req := httptest.NewRequest(http.MethodGet, "/tls/trust-status", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("se esperaba ok=true: %s", rr.Body.String())
	}
	var response struct {
		TLSStore struct {
			State                 string `json:"state"`
			ArtifactCount         int    `json:"artifactCount"`
			CertificateCount      int    `json:"certificateCount"`
			KeyCount              int    `json:"keyCount"`
			LocalCertificateState string `json:"localCertificateState"`
			SystemTrustState      string `json:"systemTrustState"`
		} `json:"tlsStore"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta JSON invalida: %v cuerpo=%s", err, rr.Body.String())
	}
	if response.TLSStore.State != "available" ||
		response.TLSStore.ArtifactCount != 2 ||
		response.TLSStore.CertificateCount != 1 ||
		response.TLSStore.KeyCount != 1 ||
		response.TLSStore.LocalCertificateState != "generated" ||
		response.TLSStore.SystemTrustState != "platform_dependent" {
		t.Fatalf("estado TLS inesperado: %#v", response.TLSStore)
	}
	for _, secreto := range []string{dir, tlsDir, "websocket-localhost.crt.pem", "websocket-localhost.key.pem"} {
		if bytes.Contains(rr.Body.Bytes(), []byte(secreto)) {
			t.Fatalf("la respuesta de diagnóstico filtra %q: %s", secreto, rr.Body.String())
		}
	}
}

func TestRoutes_DiagnosticsReport(t *testing.T) {
	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost.crt.pem"), []byte("cert"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost.key.pem"), []byte("key"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(key) error = %v", err)
	}

	closeCount := 0
	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir).WithCertificateSources(
		catalogMockREST{certs: []domain.CertificateRef{{
			ID:            "cert-1",
			Subject:       "CN=Ana Perez",
			Issuer:        "CN=FNMT",
			NotAfter:      time.Now().Add(time.Hour),
			Fingerprint:   "fp1",
			HasSigningKey: true,
		}}},
		closingSigningKeyProviderREST{key: &closingSigningKeyREST{closeCount: &closeCount}},
	)

	req := httptest.NewRequest(http.MethodGet, "/diagnostics/report", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"certificateCount":1`)) {
		t.Fatalf("certificateCount inesperado: %s", rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"canSignCount":1`)) {
		t.Fatalf("canSignCount inesperado: %s", rr.Body.String())
	}
	var response struct {
		TLSStore struct {
			State                 string `json:"state"`
			ArtifactCount         int    `json:"artifactCount"`
			CertificateCount      int    `json:"certificateCount"`
			KeyCount              int    `json:"keyCount"`
			LocalCertificateState string `json:"localCertificateState"`
			SystemTrustState      string `json:"systemTrustState"`
		} `json:"tlsStore"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta JSON invalida: %v cuerpo=%s", err, rr.Body.String())
	}
	if response.TLSStore.State != "available" ||
		response.TLSStore.ArtifactCount != 2 ||
		response.TLSStore.CertificateCount != 1 ||
		response.TLSStore.KeyCount != 1 ||
		response.TLSStore.LocalCertificateState != "generated" ||
		response.TLSStore.SystemTrustState != "platform_dependent" {
		t.Fatalf("estado TLS inesperado: %#v", response.TLSStore)
	}
	for _, secreto := range []string{dir, tlsDir, "websocket-localhost.crt.pem", "websocket-localhost.key.pem"} {
		if bytes.Contains(rr.Body.Bytes(), []byte(secreto)) {
			t.Fatalf("el informe de diagnóstico filtra %q: %s", secreto, rr.Body.String())
		}
	}
	if closeCount != 0 {
		t.Fatalf("el informe abrió una clave privada, cierres=%d", closeCount)
	}
}

func TestRoutes_DiagnosticsReport_AlmacenNoCreadoCuentaCero(t *testing.T) {
	dir := t.TempDir()
	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)
	req := httptest.NewRequest(http.MethodGet, "/diagnostics/report", nil)
	rr := httptest.NewRecorder()

	adaptador.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		TLSStore struct {
			State         string `json:"state"`
			ArtifactCount int    `json:"artifactCount"`
		} `json:"tlsStore"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("respuesta JSON invalida: %v cuerpo=%s", err, rr.Body.String())
	}
	if response.TLSStore.State != "not_created" || response.TLSStore.ArtifactCount != 0 {
		t.Fatalf("almacén ausente inesperado: %#v", response.TLSStore)
	}
	if bytes.Contains(rr.Body.Bytes(), []byte(dir)) {
		t.Fatalf("el informe de diagnóstico filtra la ruta de configuración: %s", rr.Body.String())
	}
}

func TestRoutes_TLSClearStore(t *testing.T) {
	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost.crt.pem"), []byte("cert"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tlsDir, "websocket-localhost.key.pem"), []byte("key"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(key) error = %v", err)
	}

	adaptador := rest.New(nil, nil, nil).WithConfigDir(dir)
	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/tls/clear-store", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		if !bytes.Contains(rr.Body.Bytes(), []byte(`"ok":false`)) {
			t.Fatalf("la plataforma sin retirada no falló cerrado: %s", rr.Body.String())
		}
		entries, err := os.ReadDir(tlsDir)
		if err != nil {
			t.Fatalf("os.ReadDir(tlsDir) error = %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("la plataforma sin ciclo retiró artefactos: %d", len(entries))
		}
		return
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("se esperaba ok=true: %s", rr.Body.String())
	}
	wantRemoved := `"removed":2`
	if runtime.GOOS == "linux" {
		wantRemoved = `"removed":3`
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(wantRemoved)) {
		t.Fatalf("removed inesperado: %s", rr.Body.String())
	}
	entries, err := os.ReadDir(tlsDir)
	if err != nil {
		t.Fatalf("os.ReadDir(tlsDir) error = %v", err)
	}
	if runtime.GOOS == "linux" {
		allowed := map[string]bool{
			".local-tls-inventory.lock":                                true,
			".websocket-localhost-ca.lock":                             true,
			"websocket-localhost-root.crt.pem.grxfirma-trust.lock":     true,
			"websocket-localhost-root.crt.pem.grxfirma-nss-trust.json": true,
		}
		for _, entry := range entries {
			if !allowed[entry.Name()] {
				t.Fatalf("queda un artefacto TLS inesperado: %s", entry.Name())
			}
		}
		inventoryPath := filepath.Join(tlsDir, "websocket-localhost-root.crt.pem.grxfirma-nss-trust.json")
		if raw, err := os.ReadFile(inventoryPath); err == nil {
			var inventory struct {
				Entries []json.RawMessage `json:"entries"`
			}
			if err := json.Unmarshal(raw, &inventory); err != nil || len(inventory.Entries) != 0 {
				t.Fatalf("inventario NSS no vacío o inválido tras retirar confianza: %v", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("no se pudo leer el inventario NSS: %v", err)
		}
		return
	}
	for _, entry := range entries {
		if entry.Name() != ".websocket-localhost-ca.lock" {
			t.Fatalf("queda un artefacto TLS inesperado: %s", entry.Name())
		}
	}
}

func TestRoutes_BearerOpcional(t *testing.T) {
	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithBearerToken("valor-prueba")
	body := map[string]any{
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
	}

	rr := doJSON(t, adaptador.Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("codigo inesperado sin bearer: %d", rr.Code)
	}

	reqData, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/sign", bytes.NewReader(reqData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valor-prueba")
	rr = httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado con bearer: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
}

func TestRoutes_BearerNoExponeAutenticacionPorCertificado(t *testing.T) {
	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithBearerToken("valor-prueba")
	handler := adaptador.Routes()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/auth/challenge"},
		{method: http.MethodPost, path: "/auth/verify"},
		{method: http.MethodGet, path: "/autenticacion/reto"},
		{method: http.MethodPost, path: "/autenticacion/verificar"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rr.Code, http.StatusNotFound)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var document struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &document); err != nil {
		t.Fatalf("openapi json invalido: %v", err)
	}
	for _, path := range []string{"/auth/challenge", "/auth/verify"} {
		if _, exposed := document.Paths[path]; exposed {
			t.Errorf("OpenAPI Bearer expone el endpoint desactivado %s", path)
		}
	}
}

func TestRoutes_AllowlistCertificadosVaciaNoActivaAutenticacion(t *testing.T) {
	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithCertificateAuth(" , : ", 15*time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/auth/challenge", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("challenge con allowlist vacia = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRoutes_AuthPorCertificado(t *testing.T) {
	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithBearerToken("bearer-alternativo")

	priv, certPEM, fingerprint := generarCertificadoAuth(t)
	adaptador.WithCertificateAuth(fingerprint, 15*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/auth/challenge", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("challenge codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var reto struct {
		ChallengeID  string `json:"challengeId"`
		ChallengeB64 string `json:"challengeB64"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &reto); err != nil {
		t.Fatalf("challenge json invalido: %v", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(reto.ChallengeB64)
	if err != nil {
		t.Fatalf("challenge b64 invalido: %v", err)
	}
	digest := sha256Bytes(nonce)
	firma, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest)
	if err != nil {
		t.Fatalf("firma de reto fallo: %v", err)
	}

	verifyBody := map[string]any{
		"challengeId":    reto.ChallengeID,
		"signatureB64":   base64.StdEncoding.EncodeToString(firma),
		"certificatePEM": certPEM,
	}
	rr = doJSON(t, adaptador.Routes(), http.MethodPost, "/auth/verify", verifyBody)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var verifyResp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &verifyResp)
	token, _ := verifyResp["sessionToken"].(string)
	if token == "" {
		t.Fatalf("sessionToken vacío: %s", rr.Body.String())
	}

	body := map[string]any{
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
	}
	reqData, _ := json.Marshal(body)
	enviarFirma := func() *httptest.ResponseRecorder {
		peticion := httptest.NewRequest(http.MethodPost, "/sign", bytes.NewReader(reqData))
		peticion.Header.Set("Content-Type", "application/json")
		peticion.Header.Set("Authorization", "Bearer "+token)
		respuesta := httptest.NewRecorder()
		adaptador.Routes().ServeHTTP(respuesta, peticion)
		return respuesta
	}
	if rr = enviarFirma(); rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado con sesión cert: %d cuerpo=%s", rr.Code, rr.Body.String())
	}

	_, _, fingerprintDistinto := generarCertificadoAuth(t)
	adaptador.WithCertificateAuth(fingerprintDistinto, 15*time.Minute)
	if rr = enviarFirma(); rr.Code != http.StatusUnauthorized {
		t.Fatalf("sesión tras cambiar allowlist = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	adaptador.WithCertificateAuth("", 15*time.Minute)
	if rr = enviarFirma(); rr.Code != http.StatusUnauthorized {
		t.Fatalf("sesión tras desactivar auth cert = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRoutes_AuthPorCertificadoRechazaFueraDeAllowlist(t *testing.T) {
	_, _, fingerprintPermitido := generarCertificadoAuth(t)
	privNoPermitida, certPEMNoPermitido, _ := generarCertificadoAuth(t)
	adaptador := rest.New(signUseCaseMock{}, nil, nil).
		WithBearerToken("valor-prueba").
		WithCertificateAuth(fingerprintPermitido, 15*time.Minute)

	req := httptest.NewRequest(http.MethodPost, "/auth/challenge", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("challenge codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var reto struct {
		ChallengeID  string `json:"challengeId"`
		ChallengeB64 string `json:"challengeB64"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &reto); err != nil {
		t.Fatalf("challenge json invalido: %v", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(reto.ChallengeB64)
	if err != nil {
		t.Fatalf("challenge b64 invalido: %v", err)
	}
	firma, err := rsa.SignPKCS1v15(rand.Reader, privNoPermitida, crypto.SHA256, sha256Bytes(nonce))
	if err != nil {
		t.Fatalf("firma de reto fallo: %v", err)
	}
	rr = doJSON(t, adaptador.Routes(), http.MethodPost, "/auth/verify", map[string]any{
		"challengeId":    reto.ChallengeID,
		"signatureB64":   base64.StdEncoding.EncodeToString(firma),
		"certificatePEM": certPEMNoPermitido,
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("verify fuera de allowlist = %d, want %d; cuerpo=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestStartTLSServer_ClienteHTTPS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adaptador := rest.New(signUseCaseMock{}, nil, nil).WithBearerToken("valor-prueba")
	srv, err := rest.StartTLSServer(ctx, "127.0.0.1:0", adaptador.Routes(), t.TempDir())
	if err != nil {
		t.Fatalf("StartTLSServer() error = %v", err)
	}

	_, _, wsRootFile, _, err := rest.EnsureBrowserCompatibleLocalhostCertificate(
		filepath.Dir(srv.CertFile), rest.ManagedLocalhostPrefix,
	)
	if err != nil {
		t.Fatalf("certificado WebSocket: %v", err)
	}
	if filepath.Base(srv.CertFile) != rest.ManagedLocalhostPrefix+".crt.pem" {
		t.Fatalf("REST no usa la hoja compartida: %s", srv.CertFile)
	}
	certPEM, err := os.ReadFile(wsRootFile)
	if err != nil {
		t.Fatalf("ReadFile(cert) error = %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("no se pudo cargar el certificado TLS local")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    roots,
				ServerName: "localhost",
			},
		},
	}

	resp, err := client.Get("https://" + srv.Addr + "/openapi.json")
	if err != nil {
		t.Fatalf("GET openapi.json error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status inesperado openapi: %d", resp.StatusCode)
	}

	body := map[string]any{
		"name":           "doc.txt",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("abc")),
		"mime_type":      "text/plain",
		"format":         "CAdES",
		"action":         "sign",
		"certificate_id": "cert-1",
	}
	data, _ := json.Marshal(body)
	unauthorized, _ := http.NewRequest(http.MethodPost, "https://"+srv.Addr+"/sign", bytes.NewReader(data))
	unauthorized.Header.Set("Content-Type", "application/json")
	denied, err := client.Do(unauthorized)
	if err != nil {
		t.Fatalf("POST /sign sin bearer: %v", err)
	}
	denied.Body.Close()
	if denied.StatusCode != http.StatusUnauthorized {
		t.Fatalf("REST TLS sin bearer = %d", denied.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodPost, "https://"+srv.Addr+"/sign", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valor-prueba")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /sign error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status inesperado sign: %d", resp.StatusCode)
	}
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json marshal fallo: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

type signUseCaseMock struct{}

func (signUseCaseMock) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	return application.SignResult{
		Result: domain.SignatureResult{
			Format:    cmd.Format,
			Data:      []byte("firma"),
			Algorithm: "CAdES-BES-Detached",
		},
		CertificateUsed: domain.CertificateRef{
			ID:          cmd.CertificateID,
			Subject:     "CN=Ana",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		},
	}, nil
}

type capturingSignUseCase struct {
	lastCmd application.SignCommand
}

func (m *capturingSignUseCase) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	m.lastCmd = cmd
	return application.SignResult{
		Result: domain.SignatureResult{
			Format:    cmd.Format,
			Data:      []byte("firma"),
			Algorithm: "mock-sign",
		},
		CertificateUsed: domain.CertificateRef{ID: cmd.CertificateID},
	}, nil
}

type batchSignUseCaseMock struct {
	lastCmd application.ProcessBatchCommand
	calls   int
}

func (m *batchSignUseCaseMock) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	m.calls++
	m.lastCmd = cmd
	results := make([]application.SignResult, 0, len(cmd.Jobs))
	for _, job := range cmd.Jobs {
		results = append(results, application.SignResult{
			Result: domain.SignatureResult{
				Format:    job.Format,
				Data:      []byte("firma-" + job.Document.Name),
				Algorithm: "mock-batch",
			},
			CertificateUsed: domain.CertificateRef{
				ID:          cmd.CertificateID,
				Subject:     "CN=Lote",
				Issuer:      "CN=FNMT",
				NotAfter:    time.Now().Add(time.Hour),
				Fingerprint: "fp-lote",
			},
		})
	}
	return application.BatchResult{Results: results, Errores: map[int]error{}}, nil
}

type verifyUseCaseMock struct{}
type verifyUseCaseErrorMock struct{ err error }

type hashCreateUseCaseMock struct{}

func (hashCreateUseCaseMock) Execute(_ context.Context, cmd application.CreateHashCommand) (application.CreateHashResult, error) {
	return application.CreateHashResult{
		Algorithm: "SHA-256",
		Format:    cmd.Format,
		Digest:    []byte{0x61, 0x62, 0x63},
		Encoded:   "616263h",
	}, nil
}

type hashCheckUseCaseMock struct{}

func (hashCheckUseCaseMock) Execute(_ context.Context, cmd application.CheckHashCommand) (application.CheckHashResult, error) {
	return application.CheckHashResult{
		Valid:           true,
		Algorithm:       "SHA-256",
		Format:          cmd.Format,
		ExpectedDigest:  cmd.ExpectedHash,
		ActualDigest:    cmd.ExpectedHash,
		ExpectedEncoded: "616263h",
		ActualEncoded:   "616263h",
	}, nil
}

type dirHashCheckUseCaseMock struct{}

func (dirHashCheckUseCaseMock) Execute(_ context.Context, _ application.CheckDirectoryHashManifestCommand) (application.CheckDirectoryHashManifestResult, error) {
	return application.CheckDirectoryHashManifestResult{
		Valid: false,
		Report: domain.DirectoryHashCheckReport{
			Algorithm:       "SHA-256",
			Recursive:       true,
			MatchingHash:    []string{"uno.txt"},
			NotMatchingHash: []string{"dos.txt"},
		},
	}, nil
}

type dirHashReportCodecMock struct{}

func (dirHashReportCodecMock) EncodeReport(context.Context, domain.DirectoryHashCheckReport) ([]byte, error) {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><entries hashAlgorithm="SHA-256" recursive="true"><not_matching_hash><entry name="dos.txt"></entry></not_matching_hash></entries>`), nil
}

func (verifyUseCaseMock) Execute(_ context.Context, cmd application.VerifyCommand) (application.VerifyResult, error) {
	if cmd.SignedDocument.Name != "firma.csig" {
		return application.VerifyResult{}, errors.New("nombre de documento no preservado")
	}
	if cmd.SignedDocument.MIMEType != "application/pkcs7-signature" {
		return application.VerifyResult{}, errors.New("mime type no preservado")
	}
	if cmd.OriginalDocument != nil && string(cmd.OriginalDocument.Content) != "original" {
		return application.VerifyResult{}, errors.New("original_content_base64 no se propagó correctamente")
	}
	return application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:    true,
			Reason:   "OK",
			Details:  []string{"firma integra"},
			Format:   "CAdES",
			Coverage: "full",
			Integrity: domain.VerificationAspect{
				Status:  domain.VerificationStatusValid,
				Reason:  "integridad comprobada",
				Details: []string{"firma integra"},
			},
			Certificate: domain.VerificationAspect{
				Status:  domain.VerificationStatusWarning,
				Reason:  "certificado sin comprobacion OCSP",
				Details: []string{"revocacion no disponible"},
			},
			Trust: domain.VerificationAspect{
				Status:  domain.VerificationStatusValid,
				Reason:  "cadena de confianza valida",
				Details: []string{"anclaje local encontrado"},
			},
			SignerSummaries: []domain.VerificationSignerSummary{{
				ID:          "cert-1",
				Subject:     "CN=Ana",
				Issuer:      "CN=FNMT",
				Fingerprint: "fp1",
			}},
			Warnings: []string{"revocacion no disponible"},
			Evidence: []domain.VerificationEvidence{{
				Type:    "signingCertificate",
				Summary: "CN=Ana",
			}},
		},
		Firmantes: []domain.CertificateRef{{
			ID:          "cert-1",
			Subject:     "CN=Ana",
			Issuer:      "CN=FNMT",
			NotAfter:    time.Now().Add(time.Hour),
			Fingerprint: "fp1",
		}},
	}, nil
}

func (m verifyUseCaseErrorMock) Execute(_ context.Context, _ application.VerifyCommand) (application.VerifyResult, error) {
	return application.VerifyResult{}, m.err
}

type selectUseCaseMock struct{}

func (selectUseCaseMock) Execute(_ context.Context, _ application.SelectCertificateCommand) (application.SelectCertificateResult, error) {
	return application.SelectCertificateResult{
		Selection: domain.CertificateSelection{
			Certificate: domain.CertificateRef{
				ID:          "cert-ana",
				Subject:     "CN=Ana",
				Issuer:      "CN=FNMT",
				NotAfter:    time.Now().Add(time.Hour),
				Fingerprint: "fp1",
			},
			Confirmed: true,
		},
	}, nil
}

type protectUseCaseMock struct {
	lastCmd application.ProtectCommand
}

func (m *protectUseCaseMock) Execute(_ context.Context, cmd application.ProtectCommand) (application.ProtectResult, error) {
	m.lastCmd = cmd
	name := cmd.Document.Name + ".afp"
	mime := domain.MIMETypeProtectedEnvelope
	switch cmd.Options["container"] {
	case "cms":
		name = cmd.Document.Name + ".enveloped"
		mime = domain.MIMETypeProtectedCMS
	case "authenvelopeddata":
		name = cmd.Document.Name + ".authenveloped.p7m"
		mime = domain.MIMETypeProtectedCMS
	case "cms-encrypted":
		name = cmd.Document.Name + ".encrypted.p7m"
		mime = domain.MIMETypeProtectedCMS
	}
	protectedDoc, err := domain.NewDocument(name, []byte(`{"ok":true}`), mime)
	if err != nil {
		return application.ProtectResult{}, err
	}
	return application.ProtectResult{
		Protected: domain.ProtectedPayload{
			Document:       protectedDoc,
			Profile:        cmd.Profile,
			RecipientCount: len(cmd.RecipientIDs),
		},
	}, nil
}

func generarDestinatarioAuthEnvelopedREST(t *testing.T, id string) domain.ProtectionRecipient {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(98),
		Subject:               pkix.Name{CommonName: "Destinatario AuthEnvelopedData"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey() error = %v", err)
	}
	return domain.ProtectionRecipient{
		ID:                     id,
		Label:                  "Destinatario AuthEnvelopedData",
		RSAOAEP256PublicKeyDER: publicKeyDER,
		CertificateDER:         certificateDER,
	}
}

type protectAndSignUseCaseMock struct {
	lastCmd application.ProtectAndSignCommand
}

func (m *protectAndSignUseCaseMock) Execute(_ context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error) {
	m.lastCmd = cmd
	protectedDoc, err := domain.NewDocument(cmd.Document.Name+".signedenveloped.p7m", []byte(`{"ok":true}`), domain.MIMETypeProtectedCMS)
	if err != nil {
		return application.ProtectAndSignResult{}, err
	}
	return application.ProtectAndSignResult{
		Protected: domain.ProtectedPayload{
			Document:       protectedDoc,
			Profile:        cmd.Profile,
			RecipientCount: len(cmd.RecipientIDs),
		},
		CertificateUsed: domain.CertificateRef{ID: cmd.CertificateID},
	}, nil
}

type unprotectUseCaseMock struct {
	lastCmd application.UnprotectCommand
}

func (m *unprotectUseCaseMock) Execute(_ context.Context, cmd application.UnprotectCommand) (application.UnprotectResult, error) {
	m.lastCmd = cmd
	plainDoc, err := domain.NewDocument("secreto.txt", []byte("secreto"), "text/plain")
	if err != nil {
		return application.UnprotectResult{}, err
	}
	return application.UnprotectResult{
		Unprotected: domain.UnprotectedPayload{
			Document:    plainDoc,
			Profile:     domain.ProtectionProfileCompat,
			RecipientID: "dest-1",
		},
	}, nil
}

type protectionRecipientsMock struct {
	recipients []domain.ProtectionRecipient
}

func (m protectionRecipientsMock) List(context.Context) ([]domain.ProtectionRecipient, error) {
	return append([]domain.ProtectionRecipient(nil), m.recipients...), nil
}

func (m protectionRecipientsMock) Resolve(_ context.Context, recipientIDs []string) ([]domain.ProtectionRecipient, error) {
	if len(recipientIDs) == 0 {
		return append([]domain.ProtectionRecipient(nil), m.recipients...), nil
	}
	out := make([]domain.ProtectionRecipient, 0, len(recipientIDs))
	for _, id := range recipientIDs {
		for _, recipient := range m.recipients {
			if recipient.ID == id {
				out = append(out, recipient)
			}
		}
	}
	return out, nil
}

type exportProtectionRecipientUseCaseMock struct{}

func (exportProtectionRecipientUseCaseMock) Execute(_ context.Context, cmd application.ExportProtectionRecipientCommand) (application.ExportProtectionRecipientResult, error) {
	return application.ExportProtectionRecipientResult{
		Recipient: domain.ProtectionRecipient{
			ID:                cmd.RecipientID,
			Label:             "PQ local",
			MLKEM768PublicKey: []byte{1},
			X25519PublicKey:   []byte{2},
		},
		Data: []byte(`{"profile":"alto"}`),
	}, nil
}

type importProtectionRecipientUseCaseMock struct{}

func (importProtectionRecipientUseCaseMock) Execute(_ context.Context, _ application.ImportProtectionRecipientCommand) (application.ImportProtectionRecipientResult, error) {
	return application.ImportProtectionRecipientResult{
		Recipient: domain.ProtectionRecipient{
			ID:                "dest-importado",
			Label:             "Compartido",
			MLKEM768PublicKey: []byte{1},
			X25519PublicKey:   []byte{2},
		},
	}, nil
}

type catalogMockREST struct {
	certs []domain.CertificateRef
	err   error
}

func (m catalogMockREST) List(context.Context) ([]domain.CertificateRef, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.certs, nil
}

type signingKeyProviderMockREST struct{}

func (signingKeyProviderMockREST) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return signingKeyMockREST{}, nil
}

type signingKeyMockREST struct{}

func (signingKeyMockREST) KeyID() string                 { return "mock-key" }
func (signingKeyMockREST) CertificateChainDER() [][]byte { return nil }

type signingKeyProviderChainREST struct {
	chain [][]byte
}

func (provider signingKeyProviderChainREST) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return signingKeyChainREST(provider), nil
}

type signingKeyChainREST struct {
	chain [][]byte
}

func (signingKeyChainREST) KeyID() string { return "mock-chain-key" }
func (key signingKeyChainREST) CertificateChainDER() [][]byte {
	return key.chain
}

type closingSigningKeyProviderREST struct {
	key ports.SigningKey
	err error
}

func (provider closingSigningKeyProviderREST) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return provider.key, provider.err
}

type closingSigningKeyREST struct {
	chain      [][]byte
	closeCount *int
}

func (*closingSigningKeyREST) KeyID() string { return "mock-closing-key" }
func (key *closingSigningKeyREST) CertificateChainDER() [][]byte {
	return key.chain
}
func (key *closingSigningKeyREST) Close() {
	if key.closeCount != nil {
		(*key.closeCount)++
	}
}

type serviceManagerMockREST struct {
	estado        ports.EstadoServicio
	err           error
	installSocket string
}

func (m *serviceManagerMockREST) Estado(context.Context) (ports.EstadoServicio, error) {
	return m.estado, m.err
}

func (m *serviceManagerMockREST) Instalar(_ context.Context, socket string) error {
	m.installSocket = socket
	return m.err
}

func (m *serviceManagerMockREST) Desinstalar(context.Context) error { return m.err }
func (m *serviceManagerMockREST) Iniciar(context.Context) error     { return m.err }
func (m *serviceManagerMockREST) Detener(context.Context) error     { return m.err }

func generarP12REST(t *testing.T, commonName, password string) []byte {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	data, err := pkcs12.Modern.Encode(priv, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode() error = %v", err)
	}
	return data
}

func generarCadenaCertificadoREST(t *testing.T) [][]byte {
	t.Helper()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey(root) error = %v", err)
	}
	now := time.Now()
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(80),
		Subject:               pkix.Name{CommonName: "CA REST T087"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate(root) error = %v", err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("x509.ParseCertificate(root) error = %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey(leaf) error = %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(81),
		Subject:               pkix.Name{CommonName: "Leaf REST T087"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate(leaf) error = %v", err)
	}
	return [][]byte{leafDER, rootDER}
}

func generarCertificadoAuth(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Auth Test",
			Organization: []string{"Diputacion de Granada"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	sum := x509DigestSHA256(der)
	return priv, string(pemBytes), sum
}

func x509DigestSHA256(der []byte) string {
	sum := sha256Bytes(der)
	return toHex(sum)
}

func sha256Bytes(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func toHex(data []byte) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = hexChars[b>>4]
		out[i*2+1] = hexChars[b&0x0f]
	}
	return string(out)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

// --- Tests de regresión de seguridad ---

// TestRoutes_Sign_RechazaInputPathDeSistema verifica H-01: inputPath apuntando a
// rutas del sistema debe ser rechazado cuando AllowFileSystemPaths=false (defecto).
func TestRoutes_Sign_RechazaInputPathDeSistema(t *testing.T) {
	t.Parallel()

	rutas := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/proc/self/mem",
		"../../../etc/passwd",
		"/usr/bin/sh",
	}

	handler := rest.New(signUseCaseMock{}, nil, nil).Routes()

	for _, ruta := range rutas {
		ruta := ruta
		t.Run(ruta, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"inputPath": ruta,
				"format":    "CAdES",
			})
			req := httptest.NewRequest(http.MethodPost, "/sign", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code == http.StatusOK {
				t.Fatalf("inputPath=%q: se esperaba rechazo (4xx), se obtuvo 200", ruta)
			}
		})
	}
}

func TestRoutes_Sign_RechazaRutaDeSelloSinOperacionesDeFichero(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "seal.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	body := map[string]any{
		"name":           "doc.pdf",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.7")),
		"format":         "PAdES",
		"options": map[string]string{
			"visibleSeal":          "true",
			"visibleSealImagePath": imagePath,
		},
	}
	rr := doJSON(t, rest.New(signUseCaseMock{}, nil, nil).Routes(), http.MethodPost, "/sign", body)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestRoutes_Sign_RutaDeSelloPermitidaRechazaSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.png")
	link := filepath.Join(dir, "seal.png")
	if err := os.WriteFile(target, []byte("png"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	body := map[string]any{
		"name":           "doc.pdf",
		"content_base64": base64.StdEncoding.EncodeToString([]byte("%PDF-1.7")),
		"format":         "PAdES",
		"options": map[string]string{
			"visibleSeal":          "true",
			"visibleSealImagePath": link,
		},
	}
	handler := rest.New(signUseCaseMock{}, nil, nil).WithFileSystemPaths().Routes()
	rr := doJSON(t, handler, http.MethodPost, "/sign", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestRoutes_Sign_RechazaCuerpoExcesivo verifica H-07: cuerpos mayores que MaxBodyBytes
// deben ser rechazados con 413 o 400 sin agotar la RAM del servidor.
func TestRoutes_Sign_RechazaCuerpoExcesivo(t *testing.T) {
	t.Parallel()

	const limite = 512 // límite reducido para el test

	handler := rest.New(signUseCaseMock{}, nil, nil).WithMaxBodyBytes(limite).Routes()

	cuerpo := bytes.Repeat([]byte("x"), limite+1)
	req := httptest.NewRequest(http.MethodPost, "/sign", bytes.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatalf("se esperaba rechazo por cuerpo excesivo (%d bytes), se obtuvo 200", len(cuerpo))
	}
	if rr.Code != http.StatusRequestEntityTooLarge && rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 413 o 400", rr.Code)
	}
}
