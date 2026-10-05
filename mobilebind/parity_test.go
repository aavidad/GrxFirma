// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2
package mobilebind

import (
	"context"
	"encoding/base64"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"strings"
	"testing"
)

func TestMobileCoAndCounterSignReuseDesktopEngine(t *testing.T) {
	for _, format := range []string{"cades", "xades"} {
		t.Run(format, func(t *testing.T) {
			facade := newAndroidFacadeForTest(t)
			p12 := ephemeralRSAPKCS12(t, "paridad")
			defer zeroBytes(p12)
			id := importPKCS12(t, facade, p12, "paridad")
			content, name, mime := []byte("contenido original"), "original.txt", "text/plain"
			if format == "xades" {
				content, name, mime = []byte("<documento>original</documento>"), "original.xml", "application/xml"
			}
			req := signRequest{Name: name, MIMEType: mime, Format: format, CertificateID: id, Action: "sign"}
			for _, action := range []string{"sign", "cosign", "countersign"} {
				req.Action = action
				req.ContentBase64 = base64.StdEncoding.EncodeToString(content)
				raw, err := facade.SignJSON(mustJSON(t, req))
				if err != nil {
					t.Fatalf("%s: %v", action, err)
				}
				var signed signResponse
				decodeResponse(t, raw, &signed)
				content, err = base64.StdEncoding.DecodeString(signed.SignedContentBase64)
				if err != nil {
					t.Fatal(err)
				}
				inspection, err := facade.InspectSignatureJSON(mustJSON(t, verifyRequest{Name: req.Name, MIMEType: req.MIMEType,
					ContentBase64: signed.SignedContentBase64}))
				if err != nil {
					t.Fatal(err)
				}
				var found struct {
					HasSignature bool `json:"has_signature"`
				}
				decodeResponse(t, inspection, &found)
				if !found.HasSignature {
					t.Fatal("no propone cofirma para documento firmado")
				}
				req.MIMEType = "application/pkcs7-signature"
				if format == "xades" {
					req.MIMEType = "application/xml"
				}
				if action == "countersign" && format == "xades" && !strings.Contains(string(content), "CounterSignature") {
					t.Fatal("sin contrafirma XAdES")
				}
			}
			original := ""
			if format == "cades" {
				original = base64.StdEncoding.EncodeToString([]byte("contenido original"))
			}
			raw, err := facade.VerifyJSON(mustJSON(t, verifyRequest{Name: req.Name, MIMEType: req.MIMEType,
				ContentBase64: base64.StdEncoding.EncodeToString(content), OriginalBase64: original}))
			if err != nil {
				t.Fatal(err)
			}
			var report verifyResponse
			decodeResponse(t, raw, &report)
			if report.IntegrityStatus != "valid" || len(report.Signers) < 2 {
				t.Fatalf("informe inesperado: %+v", report)
			}
		})
	}
}

func TestMobileSigningProfilesAndTSAValidation(t *testing.T) {
	for _, profile := range []string{"baseline", "t", "lt", "lta"} {
		if err := validateSigningOptions("cades", "cosign", map[string]string{"profile": profile, "tsaURL": "http://tsa.example/rfc3161"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tsa := range []string{"https://tsa.example/rfc3161", "http://tsa.example:8080/time"} {
		if err := validateSigningOptions("pades", "sign", map[string]string{"profile": "t", "tsaURL": tsa}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tsa := range []string{"file:///tmp/tsa", "https://user:pass@tsa.example", "https://tsa.example/#fragment", "https://tsa.example/#", "//tsa.example", "https:///missing", "https://tsa.example/\n"} {
		if err := validateSigningOptions("cades", "sign", map[string]string{"profile": "t", "tsaURL": tsa}); err == nil {
			t.Fatalf("TSA aceptada: %q", tsa)
		}
	}
	for _, options := range []map[string]string{{"profile": "t"}, {"profile": "lt"}, {"profile": "lta"}, {"profile": "unknown"}, {"level": "lta"}, {"TSAURL": "http://tsa.example"}} {
		if err := validateSigningOptions("cades", "sign", options); err == nil {
			t.Fatalf("opciones aceptadas: %v", options)
		}
	}
	for _, format := range []string{"pades", "xades"} {
		if err := validateSigningOptions(format, "sign", map[string]string{"profile": "lta", "tsaURL": "https://tsa.example"}); err == nil {
			t.Fatal("degradacion silenciosa LTA")
		}
	}
	if err := validateSigningOptions("pades", "countersign", nil); err == nil {
		t.Fatal("PAdES acepto contrafirma")
	}
}

func TestMobileContractDeclaresParityCapabilities(t *testing.T) {
	raw, err := buildMobileContract("android", true)
	if err != nil {
		t.Fatal(err)
	}
	var contract mobileContract
	decodeResponse(t, raw, &contract)
	if len(contract.Signing.Actions) != 3 || len(contract.Signing.ProfilesByFormat["CAdES"]) != 4 || contract.EngineVersion == "" {
		t.Fatalf("contrato incompleto: %+v", contract)
	}
	if len(contract.Signing.ActionsByFormat["PAdES"]) != 2 {
		t.Fatal("contrafirma PAdES anunciada")
	}
}

type captureSigningCommand struct {
	command application.SignCommand
	calls   int
}

func (s *captureSigningCommand) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	s.command, s.calls = cmd, s.calls+1
	return application.SignResult{Result: domain.SignatureResult{Data: []byte("signed"), Format: cmd.Format}}, nil
}

func TestFacadePassesOperationProfileAndTSABeforeSigning(t *testing.T) {
	service := &captureSigningCommand{}
	facade := newFacade(service, nil, nil)
	req := signRequest{Name: "test.txt", MIMEType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString([]byte("original")),
		CertificateID: "id", Action: "countersign", Format: "cades", Options: map[string]string{"profile": "lta", "tsaURL": "http://tsa.example/rfc3161"}}
	if _, err := facade.SignJSON(mustJSON(t, req)); err != nil {
		t.Fatal(err)
	}
	if service.command.Action != domain.ActionCounterSign || service.command.Options["profile"] != "lta" || service.command.Options["tsaURL"] != req.Options["tsaURL"] {
		t.Fatal("contrato de firma no propagado")
	}
	req.Options["tsaURL"] = "https://user:password@tsa.example"
	if _, err := facade.SignJSON(mustJSON(t, req)); err == nil || service.calls != 1 {
		t.Fatal("TSA invalida llego a la identidad")
	}
}

func TestMutablePasswordImportCleansBuffersOnAllOutcomes(t *testing.T) {
	for _, valid := range []bool{true, false} {
		facade := newAndroidFacadeForTest(t)
		data := ephemeralRSAPKCS12(t, "prueba")
		password := []byte("prueba")
		if !valid {
			password = []byte("incorrecta")
		}
		_, err := facade.ImportCertificateSecretBytesJSON(data, password)
		if (err == nil) != valid {
			t.Fatalf("resultado de importacion inesperado: %v", err)
		}
		for _, buf := range [][]byte{data, password} {
			for _, b := range buf {
				if b != 0 {
					t.Fatal("buffer secreto retenido")
				}
			}
		}
	}
	var facade *Facade
	data, password := []byte("invalid"), []byte("password")
	_, _ = facade.ImportCertificateSecretBytesJSON(data, password)
	for _, buf := range [][]byte{data, password} {
		for _, b := range buf {
			if b != 0 {
				t.Fatal("buffer retenido en error de inicializacion")
			}
		}
	}
}

func TestInspectionDoesNotClaimUnsignedDocumentsAreSigned(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	for _, content := range []string{"documento sin firma", "<documento>sin firma</documento>", "%PDF-invalido"} {
		raw, err := facade.InspectSignatureJSON(mustJSON(t, verifyRequest{Name: "documento.bin", MIMEType: "application/octet-stream", ContentBase64: base64.StdEncoding.EncodeToString([]byte(content))}))
		if err != nil {
			t.Fatal(err)
		}
		var found struct {
			HasSignature bool `json:"has_signature"`
		}
		decodeResponse(t, raw, &found)
		if found.HasSignature {
			t.Fatal("se propuso cofirma sin firma existente")
		}
	}
}
