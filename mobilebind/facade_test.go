// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/mobile/androidintent"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type signServiceMock struct {
	result application.SignResult
	err    error
}

func (m signServiceMock) Execute(context.Context, application.SignCommand) (application.SignResult, error) {
	return m.result, m.err
}

type verifyServiceMock struct {
	result application.VerifyResult
	err    error
}

func (m verifyServiceMock) Execute(context.Context, application.VerifyCommand) (application.VerifyResult, error) {
	return m.result, m.err
}

type verifyServiceDetachedMock struct {
	result application.VerifyResult
	err    error
}

func (m verifyServiceDetachedMock) Execute(_ context.Context, cmd application.VerifyCommand) (application.VerifyResult, error) {
	if cmd.OriginalDocument == nil || string(cmd.OriginalDocument.Content) != "original" {
		return application.VerifyResult{}, errors.New("original_content_base64 no se propagó correctamente")
	}
	return m.result, m.err
}

type selectServiceMock struct {
	result application.SelectCertificateResult
	err    error
}

func (m selectServiceMock) Execute(context.Context, application.SelectCertificateCommand) (application.SelectCertificateResult, error) {
	return m.result, m.err
}

type processBatchServiceMock struct {
	result application.BatchResult
	err    error
}

func (m processBatchServiceMock) Execute(context.Context, application.ProcessBatchCommand) (application.BatchResult, error) {
	return m.result, m.err
}

type processBatchCaptureMock struct {
	result  application.BatchResult
	err     error
	lastCmd application.ProcessBatchCommand
}

func (m *processBatchCaptureMock) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	m.lastCmd = cmd
	return m.result, m.err
}

type retrieveServiceMock struct {
	result application.RetrieveRequestResult
	err    error
}

func (m retrieveServiceMock) Execute(context.Context, application.RetrieveRequestCommand) (application.RetrieveRequestResult, error) {
	return m.result, m.err
}

type uploadServiceMock struct {
	err error
}

func (m uploadServiceMock) Execute(context.Context, application.UploadResultCommand) error {
	return m.err
}

type importServiceMock struct {
	result application.ImportCertificateResult
	err    error
}

func (m importServiceMock) Execute(context.Context, application.ImportCertificateCommand) (application.ImportCertificateResult, error) {
	return m.result, m.err
}

type profileServiceMock struct {
	result application.ResolvePlatformProfileResult
	err    error
}

func (m profileServiceMock) Execute(context.Context, application.ResolvePlatformProfileCommand) (application.ResolvePlatformProfileResult, error) {
	return m.result, m.err
}

func TestFacadeSignJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(signServiceMock{result: application.SignResult{
		Result: domain.SignatureResult{
			Format:    domain.FormatCAdES,
			Algorithm: "SHA256withRSA",
			Data:      []byte("firma"),
		},
		CertificateUsed: domain.CertificateRef{ID: "cert-1"},
	}}, nil, nil)

	payload := `{"name":"demo.txt","content_base64":"` + base64.StdEncoding.EncodeToString([]byte("hola")) + `","mime_type":"text/plain","format":"CAdES","action":"sign","certificate_id":"cert-1"}`
	out, err := facade.SignJSON(payload)
	if err != nil {
		t.Fatalf("SignJSON() error = %v", err)
	}
	var resp signResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.CertificateID != "cert-1" || resp.SignedContentBase64 == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFacadeVerifyJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, verifyServiceMock{result: application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:       true,
			Reason:      "ok",
			Details:     []string{"cadena valida"},
			Certificate: domain.VerificationAspect{Status: domain.VerificationStatusUnknown},
			Format:      string(domain.FormatPAdES),
			Coverage:    "full",
			SignerSummaries: []domain.VerificationSignerSummary{{
				ID:          "cert-1",
				Subject:     "CN=Alice",
				Issuer:      "CN=CA",
				Fingerprint: "abc123",
			}},
			Warnings: []string{"warning-demo"},
		},
		Firmantes: []domain.CertificateRef{{ID: "cert-1"}},
	}}, nil)

	payload := `{"name":"firma.p7s","content_base64":"` + base64.StdEncoding.EncodeToString([]byte("firma")) + `","mime_type":"application/pkcs7-signature"}`
	out, err := facade.VerifyJSON(payload)
	if err != nil {
		t.Fatalf("VerifyJSON() error = %v", err)
	}
	var resp verifyResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Valid || len(resp.Signers) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Format != string(domain.FormatPAdES) || resp.Coverage != "full" {
		t.Fatalf("metadatos de verificación inesperados: %+v", resp)
	}
	if resp.CertificateStatus != "unknown" {
		t.Fatalf("certificate_status = %q", resp.CertificateStatus)
	}
	if len(resp.SignerSummaries) != 1 || resp.SignerSummaries[0].Subject != "CN=Alice" {
		t.Fatalf("signer_summaries inesperado: %+v", resp)
	}
	if len(resp.Warnings) != 2 || resp.Warnings[0] != "warning-demo" {
		t.Fatalf("warnings inesperados: %+v", resp)
	}
}

func TestFacadeVerifyJSONDetached(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, verifyServiceDetachedMock{result: application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:    true,
			Reason:   "ok",
			Details:  []string{"detached valido"},
			Format:   string(domain.FormatCAdES),
			Coverage: "detached",
		},
		Firmantes: []domain.CertificateRef{{ID: "cert-1"}},
	}}, nil)

	payload := `{"name":"firma.p7s","content_base64":"` + base64.StdEncoding.EncodeToString([]byte("firma")) + `","mime_type":"application/pkcs7-signature","original_content_base64":"` + base64.StdEncoding.EncodeToString([]byte("original")) + `"}`
	out, err := facade.VerifyJSON(payload)
	if err != nil {
		t.Fatalf("VerifyJSON() detached error = %v", err)
	}
	var resp verifyResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Valid || len(resp.Signers) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Coverage != "detached" {
		t.Fatalf("coverage detached inesperada: %+v", resp)
	}
}

func TestFacadeSelectCertificateJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, selectServiceMock{result: application.SelectCertificateResult{
		Selection: domain.CertificateSelection{
			Certificate: domain.CertificateRef{
				ID:          "cert-2",
				Subject:     "CN=Alice",
				Issuer:      "CN=CA",
				Fingerprint: "abc123",
			},
			Confirmed: true,
		},
	}})

	out, err := facade.SelectCertificateJSON(`{"subject_filter":"Alice","solo_no_caducados":true}`)
	if err != nil {
		t.Fatalf("SelectCertificateJSON() error = %v", err)
	}
	var resp selectCertificateResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.CertificateID != "cert-2" || !resp.Confirmed {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFacadeProcessBatchJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withBatchService(processBatchServiceMock{
		result: application.BatchResult{
			Results: []application.SignResult{{
				Result: domain.SignatureResult{
					Format:    domain.FormatPAdES,
					Algorithm: "SHA256withRSA",
					Data:      []byte("pdf-firmado"),
				},
				CertificateUsed: domain.CertificateRef{ID: "cert-batch"},
			}},
		},
	})

	payload := `{"items":[{"name":"doc.pdf","content_base64":"` + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")) + `","mime_type":"application/pdf","format":"PAdES","action":"sign"}],"session":{"request_id":"req-1","upload_endpoint":"https://up","retrieve_endpoint":"https://rt"}}`
	out, err := facade.processBatchJSON(payload)
	if err != nil {
		t.Fatalf("ProcessBatchJSON() error = %v", err)
	}
	var resp processBatchResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.OK || len(resp.Results) != 1 || resp.Results[0].CertificateID != "cert-batch" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFacadeProcessBatchJSONPropagatesCertificateID(t *testing.T) {
	t.Parallel()

	batch := &processBatchCaptureMock{
		result: application.BatchResult{
			Results: []application.SignResult{{
				Result: domain.SignatureResult{
					Format:    domain.FormatPAdES,
					Algorithm: "SHA256withRSA",
					Data:      []byte("pdf-firmado"),
				},
				CertificateUsed: domain.CertificateRef{ID: "cert-batch"},
			}},
		},
	}
	facade := newFacade(nil, nil, nil).withBatchService(batch)

	payload := `{"certificate_id":" cert-lote ","items":[{"name":"doc.pdf","content_base64":"` + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4")) + `","mime_type":"application/pdf","format":"PAdES","action":"sign"}]}`
	if _, err := facade.processBatchJSON(payload); err != nil {
		t.Fatalf("ProcessBatchJSON() error = %v", err)
	}
	if batch.lastCmd.CertificateID != "cert-lote" {
		t.Fatalf("certificate_id no propagado: %+v", batch.lastCmd)
	}
}

func TestFacadeProcessBatchJSONMergesGlobalOptionsAndItemOverrides(t *testing.T) {
	t.Parallel()

	batch := &processBatchCaptureMock{result: application.BatchResult{
		Results: []application.SignResult{
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("a")}},
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("b")}},
		},
	}}
	facade := newFacade(nil, nil, nil).withBatchService(batch)
	content := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4"))
	payload := `{
		"options":{"visibleSeal":"true","visibleSealRectW":"220","page":"all"},
		"items":[
			{"name":"global.pdf","content_base64":"` + content + `","mime_type":"application/pdf","format":"PAdES","action":"sign"},
			{"name":"rango.pdf","content_base64":"` + content + `","mime_type":"application/pdf","format":"PAdES","action":"sign","options":{"page":"1,3-5"}}
		]
	}`

	if _, err := facade.processBatchJSON(payload); err != nil {
		t.Fatalf("ProcessBatchJSON() error = %v", err)
	}
	if got := batch.lastCmd.Jobs[0].Options["page"]; got != "all" {
		t.Fatalf("page global = %q, want all", got)
	}
	if got := batch.lastCmd.Jobs[1].Options["page"]; got != "1,3-5" {
		t.Fatalf("page override = %q, want 1,3-5", got)
	}
	if got := batch.lastCmd.Jobs[1].Options["visibleSealRectW"]; got != "220" {
		t.Fatalf("rectW heredado = %q, want 220", got)
	}
}

func TestFacadeTranslateAndroidIntentJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withAndroidIntentAdapter(androidintent.New(nil))
	payload := `{"action":"android.intent.action.SEND","descriptor":"contrato.pdf","payload_base64":"` + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n")) + `"}`
	out, err := facade.TranslateAndroidIntentJSON(payload)
	if err != nil {
		t.Fatalf("TranslateAndroidIntentJSON() error = %v", err)
	}
	var resp androidIntentResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Type != "firma" || resp.DocumentName != "contrato.pdf" || resp.Format != string(domain.FormatPAdES) {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.SignatureAction != string(domain.ActionSign) {
		t.Fatalf("signature action inesperada: %+v", resp)
	}
}

func TestFacadeTranslateAndroidIntentJSONSendMultiple(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withAndroidIntentAdapter(androidintent.New(nil))
	payload := `{"action":"android.intent.action.SEND_MULTIPLE","items":[` +
		`{"descriptor":"contrato.pdf","payload_base64":"` + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n")) + `"},` +
		`{"descriptor":"factura.xml","payload_base64":"` + base64.StdEncoding.EncodeToString([]byte("<root/>")) + `"}` +
		`]}`
	out, err := facade.TranslateAndroidIntentJSON(payload)
	if err != nil {
		t.Fatalf("TranslateAndroidIntentJSON() send_multiple error = %v", err)
	}
	var resp androidIntentResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Type != "lote" || resp.JobCount != 2 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.ViewActionID != "firmar_lote" {
		t.Fatalf("accion visual inesperada: %+v", resp)
	}
}

func TestFacadeTranslateAndroidIntentJSONSendMultipleRequiresItems(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withAndroidIntentAdapter(androidintent.New(nil))
	_, err := facade.TranslateAndroidIntentJSON(`{"action":"android.intent.action.SEND_MULTIPLE"}`)
	if err == nil {
		t.Fatal("se esperaba error por items ausentes")
	}
	if err.Error() != "items es obligatorio para send_multiple" {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestFacadeTranslateAndroidIntentJSONRequiresPayload(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withAndroidIntentAdapter(androidintent.New(nil))
	_, err := facade.TranslateAndroidIntentJSON(`{"action":"android.intent.action.SEND"}`)
	if err == nil {
		t.Fatal("se esperaba error por payload ausente")
	}
	if err.Error() != "payload_base64 o text es obligatorio" {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestFacadeRetrieveRequestJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withRetrieveService(retrieveServiceMock{
		result: application.RetrieveRequestResult{
			Session: domain.ExchangeSession{
				RequestID:        "req-2",
				SessionKey:       "sek",
				UploadEndpoint:   "https://up",
				RetrieveEndpoint: "https://rt",
				State:            domain.SessionActive,
			},
			Data: []byte("payload"),
		},
	})

	out, err := facade.retrieveRequestJSON(`{"session":{"request_id":"req-2","upload_endpoint":"https://up","retrieve_endpoint":"https://rt"}}`)
	if err != nil {
		t.Fatalf("RetrieveRequestJSON() error = %v", err)
	}
	var resp retrieveResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.RequestID != "req-2" || resp.DataBase64 == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFacadeUploadResultJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withUploadService(uploadServiceMock{})
	out, err := facade.uploadResultJSON(`{"session":{"request_id":"req-3","upload_endpoint":"https://up","retrieve_endpoint":"https://rt"},"data_base64":"` + base64.StdEncoding.EncodeToString([]byte("resultado")) + `"}`)
	if err != nil {
		t.Fatalf("UploadResultJSON() error = %v", err)
	}
	var resp uploadResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.OK {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFacadeImportCertificateJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withImportService(importServiceMock{
		result: application.ImportCertificateResult{
			Certificate: domain.CertificateRef{
				ID:          "cert-import",
				Subject:     "CN=Mobile",
				Issuer:      "CN=CA",
				Fingerprint: "fp-mobile",
			},
		},
	})

	out, err := facade.ImportCertificateJSON(`{"data_base64":"` + base64.StdEncoding.EncodeToString([]byte("P12")) + `","password":"secret"}`)
	if err != nil {
		t.Fatalf("ImportCertificateJSON() error = %v", err)
	}
	var resp importCertificateResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.CertificateID != "cert-import" || resp.Fingerprint != "fp-mobile" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestFacadeImportCertificateBytesJSONCleansMutableInput(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withImportService(importServiceMock{
		result: application.ImportCertificateResult{
			Certificate: domain.CertificateRef{
				ID:          "cert-direct",
				Subject:     "CN=Mobile",
				Issuer:      "CN=CA",
				Fingerprint: "fp-direct",
			},
		},
	})
	data := []byte("P12-directo")
	out, err := facade.ImportCertificateBytesJSON(data, "secret")
	if err != nil {
		t.Fatalf("ImportCertificateBytesJSON() error = %v", err)
	}
	var resp importCertificateResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.CertificateID != "cert-direct" {
		t.Fatalf("resp = %+v", resp)
	}
	for index, value := range data {
		if value != 0 {
			t.Fatalf("data[%d] no fue limpiado", index)
		}
	}
}

func TestFacadeMapsCertificateImportErrorsWithoutLeakingCause(t *testing.T) {
	t.Parallel()

	const secret = "causa-interna-con-secreto"
	tests := []struct {
		name     string
		sentinel error
		want     string
	}{
		{
			name:     "password or old PKCS12",
			sentinel: errMobilePKCS12Decode,
			want:     mobilePKCS12DecodeMessage,
		},
		{
			name:     "certificate validity",
			sentinel: errMobileCertificateNotCurrent,
			want:     mobileCertificateNotCurrentMessage,
		},
		{
			name:     "unsupported signing identity",
			sentinel: errMobileSigningIdentityUnsupported,
			want:     mobileSigningIdentityUnsupportedMessage,
		},
		{
			name:     "unknown failure",
			sentinel: errors.New("error desconocido"),
			want:     mobileCertificateImportFallbackMessage,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			facade := newFacade(nil, nil, nil).withImportService(importServiceMock{
				err: errors.Join(test.sentinel, errors.New(secret)),
			})
			data := []byte("PKCS12")

			_, err := facade.ImportCertificateBytesJSON(data, "password")

			if err == nil {
				t.Fatal("se esperaba error de importación")
			}
			if err.Error() != test.want {
				t.Fatalf("mensaje = %q; esperado %q", err.Error(), test.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("el mensaje filtró la causa interna: %q", err)
			}
		})
	}
}

func TestFacadeResolvePlatformProfileJSON(t *testing.T) {
	t.Parallel()

	facade := newFacade(nil, nil, nil).withProfileService(profileServiceMock{
		result: application.ResolvePlatformProfileResult{
			Profile: ports.CapabilityProfile{
				HasSecureStorage:    true,
				HasMobileDeepLink:   true,
				HasDocumentPicker:   true,
				HasTemporaryStorage: true,
			},
		},
	})

	out, err := facade.ResolvePlatformProfileJSON()
	if err != nil {
		t.Fatalf("ResolvePlatformProfileJSON() error = %v", err)
	}
	var resp platformProfileResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.HasSecureStorage || !resp.HasMobileDeepLink {
		t.Fatalf("resp = %+v", resp)
	}
}
