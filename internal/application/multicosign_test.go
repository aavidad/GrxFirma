// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"testing"

	"grxfirma/internal/domain"
)

type signExecutorStub struct {
	results []SignResult
	err     error
	cmds    []SignCommand
}

func (s *signExecutorStub) Execute(_ context.Context, cmd SignCommand) (SignResult, error) {
	s.cmds = append(s.cmds, cmd)
	if s.err != nil {
		return SignResult{}, s.err
	}
	if len(s.results) == 0 {
		return SignResult{}, nil
	}
	res := s.results[0]
	s.results = s.results[1:]
	return res, nil
}

func TestMultiCoSignUseCase_Execute_Secuencia(t *testing.T) {
	doc, err := domain.NewDocument("doc.pdf", []byte("base"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	exec := &signExecutorStub{
		results: []SignResult{
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("firmado1")}},
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("firmado2")}},
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("firmado3")}},
		},
	}
	uc := NuevoMultiCoSignUseCase(exec)
	result, err := uc.Execute(context.Background(), MultiCoSignCommand{
		Document:                 doc,
		Format:                   domain.FormatPAdES,
		InitialAction:            domain.ActionSign,
		PrimaryCertificateID:     "cert-1",
		AdditionalCertificateIDs: []string{"cert-2", "cert-3"},
		Options: map[string]string{
			"visibleSeal":              "true",
			"visibleSealRectX":         "10",
			"visibleSealImageBase64":   "aW1hZ2Vu",
			"visibleSealSignerSummary": "Firmantes: Principal | Cofirmantes: Otro",
			"qrContent":                "https://verifica",
			"VisibleSealQRContent":     "EXP-2026",
			"reason":                   "Aprobación",
			"strictCompat":             "true",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := string(result.Result.Data); got != "firmado3" {
		t.Fatalf("resultado final = %q, want firmado3", got)
	}
	if len(exec.cmds) != 3 {
		t.Fatalf("len(cmds) = %d, want 3", len(exec.cmds))
	}
	if exec.cmds[0].Action != domain.ActionSign || exec.cmds[1].Action != domain.ActionCoSign || exec.cmds[2].Action != domain.ActionCoSign {
		t.Fatalf("acciones inesperadas: %+v", []domain.SignatureAction{exec.cmds[0].Action, exec.cmds[1].Action, exec.cmds[2].Action})
	}
	if exec.cmds[1].Document.Name != "doc.pdf" || string(exec.cmds[1].Document.Content) != "firmado1" {
		t.Fatalf("documento de segunda firma inesperado: %+v", exec.cmds[1].Document)
	}
	if exec.cmds[1].Options["reason"] != "Aprobación" {
		t.Fatalf("options no visuales no propagadas: %+v", exec.cmds[1].Options)
	}
	if _, ok := exec.cmds[1].Options["visibleSeal"]; ok {
		t.Fatalf("visibleSeal no debe propagarse a cofirmas: %+v", exec.cmds[1].Options)
	}
	if _, ok := exec.cmds[1].Options["qrContent"]; ok {
		t.Fatalf("qrContent no debe propagarse a cofirmas: %+v", exec.cmds[1].Options)
	}
	if _, ok := exec.cmds[1].Options["visibleSealSignerSummary"]; ok {
		t.Fatalf("visibleSealSignerSummary no debe propagarse a cofirmas: %+v", exec.cmds[1].Options)
	}
	if _, ok := exec.cmds[1].Options["visibleSealImageBase64"]; ok {
		t.Fatalf("visibleSealImageBase64 no debe propagarse a cofirmas: %+v", exec.cmds[1].Options)
	}
	if _, ok := exec.cmds[1].Options["VisibleSealQRContent"]; ok {
		t.Fatalf("visibleSealQRContent no debe propagarse a cofirmas: %+v", exec.cmds[1].Options)
	}
}

func TestMultiCoSignUseCase_Execute_RejectsCounterSign(t *testing.T) {
	doc, err := domain.NewDocument("doc.pdf", []byte("base"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	uc := NuevoMultiCoSignUseCase(&signExecutorStub{})
	if _, err := uc.Execute(context.Background(), MultiCoSignCommand{
		Document:             doc,
		Format:               domain.FormatPAdES,
		InitialAction:        domain.ActionCounterSign,
		PrimaryCertificateID: "cert-1",
		AdditionalCertificateIDs: []string{
			"cert-2",
		},
	}); err == nil {
		t.Fatal("expected error for countersign")
	}
}

func TestMultiCoSignUseCase_Execute_RechazaFormatoSinCofirmaReal(t *testing.T) {
	doc, err := domain.NewDocument("doc.bin", []byte("base"), "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	for _, format := range []domain.SignatureFormat{
		domain.FormatCAdES,
		domain.FormatXAdES,
		domain.SignatureFormat("XMLdSig"),
		domain.SignatureFormat("FacturaE"),
		domain.SignatureFormat("ASiC-XAdES"),
	} {
		format := format
		t.Run(string(format), func(t *testing.T) {
			exec := &signExecutorStub{}
			uc := NuevoMultiCoSignUseCase(exec)
			_, err := uc.Execute(context.Background(), MultiCoSignCommand{
				Document:                 doc,
				Format:                   format,
				InitialAction:            domain.ActionSign,
				PrimaryCertificateID:     "cert-1",
				AdditionalCertificateIDs: []string{"cert-2"},
			})
			if err == nil {
				t.Fatalf("Execute() debía rechazar %s", format)
			}
			if len(exec.cmds) != 0 {
				t.Fatalf("el formato %s llegó al motor: %+v", format, exec.cmds)
			}
		})
	}
}

func TestMultiCoSignUseCase_Execute_ExigeCofirmanteDistinto(t *testing.T) {
	doc, err := domain.NewDocument("doc.pdf", []byte("base"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	for _, additional := range [][]string{
		nil,
		{"", "  "},
		{"cert-1"},
		{" cert-1 ", "cert-1"},
	} {
		exec := &signExecutorStub{}
		uc := NuevoMultiCoSignUseCase(exec)
		_, err := uc.Execute(context.Background(), MultiCoSignCommand{
			Document:                 doc,
			Format:                   domain.FormatPAdES,
			InitialAction:            domain.ActionSign,
			PrimaryCertificateID:     " cert-1 ",
			AdditionalCertificateIDs: additional,
		})
		if err == nil {
			t.Fatalf("Execute() debía rechazar cofirmantes %#v", additional)
		}
		if len(exec.cmds) != 0 {
			t.Fatalf("los cofirmantes %#v llegaron al motor: %+v", additional, exec.cmds)
		}
	}
}

func TestSoportaCofirmaMultiple_SoloMotoresConCofirmaReal(t *testing.T) {
	for _, tc := range []struct {
		format domain.SignatureFormat
		want   bool
	}{
		{domain.FormatPAdES, true},
		{domain.SignatureFormat("ODF"), true},
		{domain.SignatureFormat("OOXML"), true},
		{domain.FormatCAdES, false},
		{domain.FormatXAdES, false},
		{domain.SignatureFormat("FacturaE"), false},
	} {
		if got := soportaCofirmaMultiple(tc.format); got != tc.want {
			t.Errorf("soportaCofirmaMultiple(%q) = %t, want %t", tc.format, got, tc.want)
		}
	}
}
