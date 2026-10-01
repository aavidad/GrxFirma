// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type stubProtegerFirmandoIPC struct {
	result application.ProtectAndSignResult
	err    error
	cmd    application.ProtectAndSignCommand
}

func (s *stubProtegerFirmandoIPC) Execute(_ context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

func TestCertificateIDPreferente_SePropagaEnOperacionesDeFirma(t *testing.T) {
	t.Parallel()

	t.Run("sign", func(t *testing.T) {
		input := writeIPCSelectionInput(t, "documento.pdf", []byte("%PDF-1.7"))
		useCase := &stubFirmar{result: application.SignResult{
			Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("firma")},
		}}
		handler := handlerWithSelectionCache(useCase)
		response := handler.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
			InputPath:          input,
			CertificateID:      "cert-preferred",
			CertificateIndex:   0,
			Format:             "pades",
			Action:             "sign",
			ReturnSignatureB64: true,
		}))
		if !response.OK {
			t.Fatalf("sign error = %s", response.Error)
		}
		if useCase.cmd.CertificateID != "cert-preferred" {
			t.Fatalf("CertificateID = %q; want cert-preferred", useCase.cmd.CertificateID)
		}
	})

	t.Run("sign_multicosign", func(t *testing.T) {
		input := writeIPCSelectionInput(t, "documento.pdf", []byte("%PDF-1.7"))
		useCase := &stubMultiCofirmar{result: application.SignResult{
			Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("firma")},
		}}
		handler := handlerWithSelectionCache(nil)
		handler.MultiCofirmar = useCase
		response := handler.despachar(context.Background(), peticionJSON(t, "sign_multicosign", paramsFirma{
			InputPath:          input,
			CertificateID:      "cert-preferred",
			CertificateIndex:   0,
			Format:             "pades",
			Action:             "sign",
			ReturnSignatureB64: true,
		}))
		if !response.OK {
			t.Fatalf("sign_multicosign error = %s", response.Error)
		}
		if useCase.cmd.PrimaryCertificateID != "cert-preferred" {
			t.Fatalf("PrimaryCertificateID = %q; want cert-preferred", useCase.cmd.PrimaryCertificateID)
		}
	})

	t.Run("sign_batch", func(t *testing.T) {
		input := writeIPCSelectionInput(t, "documento.pdf", []byte("%PDF-1.7"))
		useCase := &stubProcesarLote{result: application.BatchResult{
			Results: []application.SignResult{{
				Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("firma")},
			}},
		}}
		handler := handlerWithSelectionCache(nil)
		handler.ProcesarLote = useCase
		response := handler.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths:       []string{input},
			OutputDir:        t.TempDir(),
			CertificateID:    "cert-preferred",
			CertificateIndex: 0,
			Format:           "pades",
			Action:           "sign",
		}))
		if !response.OK {
			t.Fatalf("sign_batch error = %s", response.Error)
		}
		if useCase.cmd.CertificateID != "cert-preferred" {
			t.Fatalf("CertificateID = %q; want cert-preferred", useCase.cmd.CertificateID)
		}
	})

	t.Run("protect_sign", func(t *testing.T) {
		input := writeIPCSelectionInput(t, "documento.txt", []byte("contenido"))
		protected, err := domain.NewDocument("documento.signedenveloped.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
		if err != nil {
			t.Fatal(err)
		}
		useCase := &stubProtegerFirmandoIPC{result: application.ProtectAndSignResult{
			Protected: domain.ProtectedPayload{
				Document: protected,
				Profile:  domain.ProtectionProfileCompat,
			},
			CertificateUsed: domain.CertificateRef{ID: "cert-preferred"},
		}}
		handler := handlerWithSelectionCache(nil)
		handler.ProtegerFirmando = useCase
		response := handler.despachar(context.Background(), peticionJSON(t, "protect_sign", paramsProtection{
			InputPath:        input,
			CertificateID:    "cert-preferred",
			CertificateIndex: 0,
			Profile:          "compat",
			SaveToDisk:       false,
		}))
		if !response.OK {
			t.Fatalf("protect_sign error = %s", response.Error)
		}
		if useCase.cmd.CertificateID != "cert-preferred" {
			t.Fatalf("CertificateID = %q; want cert-preferred", useCase.cmd.CertificateID)
		}
	})
}

func TestResolverCertIDPreferido_ConservaFallbackLegacyYRechazaIDDesconocido(t *testing.T) {
	t.Parallel()

	handler := handlerWithSelectionCache(nil)
	got, err := handler.resolverCertIDPreferido(context.Background(), "", 0)
	if err != nil || got != "cert-index" {
		t.Fatalf("fallback legacy = %q, %v; want cert-index, nil", got, err)
	}
	got, err = handler.resolverCertIDPreferido(context.Background(), " cert-preferred ", 0)
	if err != nil || got != "cert-preferred" {
		t.Fatalf("selección preferida = %q, %v; want cert-preferred, nil", got, err)
	}
	if got, err = handler.resolverCertIDPreferido(context.Background(), "cert-desconocido", 0); err == nil || got != "" {
		t.Fatalf("ID desconocido = %q, %v; want vacío y error", got, err)
	}
}

func handlerWithSelectionCache(signUseCase SignDocumentUseCase) *Manejador {
	handler := &Manejador{Firmar: signUseCase}
	handler.setUltimosCerts([]domain.CertificateRef{
		{ID: "cert-index", Subject: "CN=Índice"},
		{ID: "cert-preferred", Subject: "CN=Preferido"},
	})
	return handler
}

func writeIPCSelectionInput(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}
