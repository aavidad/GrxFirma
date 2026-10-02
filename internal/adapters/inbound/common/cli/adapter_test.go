// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/inbound/common/secretinput"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/adapters/outbound/desktop/truststore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"software.sslmate.com/src/go-pkcs12"
)

func TestCLITLSUsaCAUnicaGestionada(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDir := filepath.Join(home, ".config", "grxfirma")
	operations := []string{"generar-certificados-tls", "estado-confianza-tls"}
	if localtlstrust.ManagedTrustLifecycleSupported() {
		operations = append(operations, "instalar-confianza-tls")
	}
	for _, operation := range operations {
		var stdout, stderr strings.Builder
		adapter := New(nil, nil).WithConfigDir(configDir)
		adapter.Stdout, adapter.Stderr = &stdout, &stderr
		if code := adapter.Run(context.Background(), []string{"-operacion", operation, "-salida-json"}); code != 0 {
			t.Fatalf("%s: code=%d stderr=%s", operation, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), resttls.ManagedLocalhostPrefix) {
			t.Fatalf("%s no usa el material compartido: %s", operation, stdout.String())
		}
	}
	dir := filepath.Join(configDir, "tls")
	rootFile := filepath.Join(dir, resttls.ManagedLocalhostPrefix+"-root.crt.pem")
	data, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("CA sin PEM")
	}
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !localtlstrust.IsManagedLocalCA(root) {
		t.Fatalf("CA no gestionada: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rest-localhost-root.crt.pem")); !os.IsNotExist(err) {
		t.Fatalf("la CLI generó una segunda CA: %v", err)
	}
}

func TestCLITLSLimpiarPreservaFicherosAjenos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDir := filepath.Join(home, ".config", "grxfirma")
	adapter := New(nil, nil).WithConfigDir(configDir)
	var stdout, stderr strings.Builder
	adapter.Stdout, adapter.Stderr = &stdout, &stderr
	if code := adapter.Run(context.Background(), []string{"-operacion", "generar-certificados-tls"}); code != 0 {
		t.Fatalf("generar: %d: %s", code, stderr.String())
	}
	tlsDir := filepath.Join(configDir, "tls")
	foreign := filepath.Join(tlsDir, "ajeno.txt")
	if err := os.WriteFile(foreign, []byte("conservar"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := adapter.Run(context.Background(), []string{"-operacion", "limpiar-almacen-tls"})
	if !localtlstrust.ManagedTrustLifecycleSupported() {
		if code == 0 {
			t.Fatal("la plataforma sin retirada aceptó limpiar el almacén")
		}
		if data, err := os.ReadFile(foreign); err != nil || string(data) != "conservar" {
			t.Fatalf("fichero ajeno alterado: %v", err)
		}
		return
	}
	if code != 0 {
		t.Fatalf("limpiar: %d: %s", code, stderr.String())
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "conservar" {
		t.Fatalf("fichero ajeno: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tlsDir, resttls.ManagedLocalhostPrefix+"-root.crt.pem")); !os.IsNotExist(err) {
		t.Fatalf("CA gestionada conservada: %v", err)
	}
}

type signMock struct {
	last application.SignCommand
}

func (m *signMock) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	m.last = cmd
	return application.SignResult{
		Result: domain.SignatureResult{
			Format: cmd.Format,
			Data:   []byte("firmado"),
		},
		CertificateUsed: domain.CertificateRef{ID: cmd.CertificateID},
	}, nil
}

type batchMock struct {
	last application.ProcessBatchCommand
}

func (m *batchMock) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	m.last = cmd
	return application.BatchResult{
		Results: []application.SignResult{},
		Errores: map[int]error{},
	}, nil
}

type dominiosMock struct {
	llamadas []application.ManageTrustedDomainCommand
}

func (m *dominiosMock) Execute(_ context.Context, cmd application.ManageTrustedDomainCommand) (application.ManageTrustedDomainResult, error) {
	m.llamadas = append(m.llamadas, cmd)
	return application.ManageTrustedDomainResult(cmd), nil
}

type dirHashCheckMock struct{}

func (dirHashCheckMock) Execute(_ context.Context, _ application.CheckDirectoryHashManifestCommand) (application.CheckDirectoryHashManifestResult, error) {
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

type hashCreateMock struct{}

func (hashCreateMock) Execute(_ context.Context, cmd application.CreateHashCommand) (application.CreateHashResult, error) {
	return application.CreateHashResult{
		Algorithm: "SHA-256",
		Format:    cmd.Format,
		Digest:    []byte{0x61, 0x62, 0x63},
		Encoded:   "616263",
	}, nil
}

type dirHashReportCodecMock struct{}

func (dirHashReportCodecMock) EncodeReport(context.Context, domain.DirectoryHashCheckReport) ([]byte, error) {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><entries hashAlgorithm="SHA-256" recursive="true"><not_matching_hash><entry name="dos.txt"></entry></not_matching_hash></entries>`), nil
}

type verifyMock struct{}

func (verifyMock) Execute(_ context.Context, cmd application.VerifyCommand) (application.VerifyResult, error) {
	return application.VerifyResult{
		Verification: domain.VerificationResult{
			Valid:    true,
			Reason:   "firma valida",
			Details:  []string{"integridad comprobada"},
			Format:   "PAdES",
			Coverage: "full",
			Integrity: domain.VerificationAspect{
				Status:  domain.VerificationStatusValid,
				Reason:  "integridad comprobada",
				Details: []string{"integridad comprobada"},
			},
			Certificate: domain.VerificationAspect{
				Status:  domain.VerificationStatusWarning,
				Reason:  "revocacion no disponible",
				Details: []string{"sin OCSP"},
			},
			Trust: domain.VerificationAspect{
				Status:  domain.VerificationStatusValid,
				Reason:  "cadena valida",
				Details: []string{"anclaje local"},
			},
			Warnings: []string{"sin OCSP"},
			Evidence: []domain.VerificationEvidence{{
				Type:    "signingCertificate",
				Summary: "CN=Ana",
			}},
		},
		Firmantes: []domain.CertificateRef{{
			ID:          "cert-1",
			Subject:     "CN=Ana",
			Issuer:      "CN=FNMT",
			Fingerprint: "fp1",
		}},
	}, nil
}

func TestRunFirma_ParseaArgumentosYEjecutaCasoDeUso(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "entrada.pdf")
	if err := os.WriteFile(entrada, []byte("contenido"), 0600); err != nil {
		t.Fatal(err)
	}
	salida := filepath.Join(tmp, "salida.csig")
	firmar := &signMock{}
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(firmar, nil)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-entrada", entrada,
		"-salida", salida,
		"-formato", "CAdES",
		"-accion", "sign",
		"-certificado", "cert-001",
		"-opcion", "level=T",
		"-opcion", "policy=demo",
	})
	if rc != 0 {
		t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
	}
	if firmar.last.CertificateID != "cert-001" {
		t.Fatalf("certificado inesperado: %q", firmar.last.CertificateID)
	}
	if firmar.last.Document.Name != "entrada.pdf" {
		t.Fatalf("nombre de documento inesperado: %q", firmar.last.Document.Name)
	}
	if firmar.last.Options["level"] != "T" {
		t.Fatalf("opcion level inesperada: %#v", firmar.last.Options)
	}
	if firmar.last.Options["policy"] != "demo" {
		t.Fatalf("opcion policy inesperada: %#v", firmar.last.Options)
	}
	if _, err := os.Stat(salida); err != nil {
		t.Fatalf("se esperaba fichero de salida: %v", err)
	}
}

func TestRunFirma_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "entrada.pdf")
	salida := filepath.Join(tmp, "salida.csig")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(&signMock{}, nil).WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-entrada", entrada,
		"-salida", salida,
		"-formato", "CAdES",
		"-accion", "sign",
		"-certificado", "cert-001",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Signing completed successfully.",
		"Operation=sign",
		"Format=CAdES",
		"Certificate=cert-001",
		"Output: " + salida,
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

type protectAndSignCLIMock struct {
	last application.ProtectAndSignCommand
}

type protectCLIMock struct {
	last application.ProtectCommand
}

func (m *protectCLIMock) Execute(_ context.Context, cmd application.ProtectCommand) (application.ProtectResult, error) {
	m.last = cmd
	protectedDoc, err := domain.NewDocument(cmd.Document.Name+".encrypted.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
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

type unprotectCLIMock struct {
	called bool
}

type protectionRecipientsEmptyMock struct{}

func (protectionRecipientsEmptyMock) Resolve(context.Context, []string) ([]domain.ProtectionRecipient, error) {
	return nil, nil
}

func (protectionRecipientsEmptyMock) List(context.Context) ([]domain.ProtectionRecipient, error) {
	return nil, nil
}

func (m *unprotectCLIMock) Execute(_ context.Context, _ application.UnprotectCommand) (application.UnprotectResult, error) {
	m.called = true
	doc, err := domain.NewDocument("secreto.txt", []byte("claro"), "text/plain")
	if err != nil {
		return application.UnprotectResult{}, err
	}
	return application.UnprotectResult{
		Unprotected: domain.UnprotectedPayload{
			Document:    doc,
			Profile:     domain.ProtectionProfileCompat,
			RecipientID: "recipient-1",
		},
	}, nil
}

func (m *protectAndSignCLIMock) Execute(_ context.Context, cmd application.ProtectAndSignCommand) (application.ProtectAndSignResult, error) {
	m.last = cmd
	protectedDoc, err := domain.NewDocument(cmd.Document.Name+".signedenveloped.p7m", []byte("cms"), domain.MIMETypeProtectedCMS)
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

func TestRunLote_ParseaManifiestoYEjecutaCasoDeUso(t *testing.T) {
	tmp := t.TempDir()
	documento := filepath.Join(tmp, "uno.xml")
	manifest := filepath.Join(tmp, "lote.json")
	if err := os.WriteFile(documento, []byte("<x/>"), 0600); err != nil {
		t.Fatal(err)
	}
	manifestContent, err := json.Marshal([]elementoLoteCLI{{
		Ruta:     documento,
		Formato:  "XAdES",
		Accion:   "sign",
		TipoMIME: "application/xml",
	}})
	if err != nil {
		t.Fatalf("serializando manifiesto: %v", err)
	}
	if err := os.WriteFile(manifest, manifestContent, 0600); err != nil {
		t.Fatal(err)
	}
	lote := &batchMock{}
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, lote)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-lote", manifest,
	})
	if rc != 0 {
		t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
	}
	if len(lote.last.Jobs) != 1 {
		t.Fatalf("se esperaba 1 trabajo, se obtuvo %d", len(lote.last.Jobs))
	}
	if lote.last.Jobs[0].Format != domain.FormatXAdES {
		t.Fatalf("formato inesperado: %s", lote.last.Jobs[0].Format)
	}
}

func TestRunLote_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	documento := filepath.Join(tmp, "uno.xml")
	manifest := filepath.Join(tmp, "lote.json")
	if err := os.WriteFile(documento, []byte("<x/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestContent, err := json.Marshal([]elementoLoteCLI{{
		Ruta:     documento,
		Formato:  "XAdES",
		Accion:   "sign",
		TipoMIME: "application/xml",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, manifestContent, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, &batchMock{}).WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{"-lote", manifest})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{"Batch results:", "Total=0", "Errors=0"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestInferirFormatoFirma_SoportaOfficeYXMLDSig(t *testing.T) {
	casos := map[string]string{
		"/tmp/doc.pdf":   "PAdES",
		"/tmp/doc.xml":   "XAdES",
		"/tmp/doc.dsig":  "XMLdSig",
		"/tmp/doc.odt":   "ODF",
		"/tmp/doc.docx":  "OOXML",
		"/tmp/doc.asics": "ASiC-XAdES",
		"/tmp/doc.bin":   "CAdES",
	}
	for entrada, want := range casos {
		if got := inferirFormatoFirma("AUTO", entrada); got != want {
			t.Fatalf("inferirFormatoFirma(AUTO, %q) = %q, want %q", entrada, got, want)
		}
	}
}

func TestConstruirOpcionesFirmaCompat_RespetaPageAllExistente(t *testing.T) {
	cfg := configCLI{
		compatEstrica: true,
		selloVisible:  true,
		selloPagina:   2,
		selloX:        0.62,
		selloY:        0.04,
		selloW:        0.34,
		selloH:        0.12,
		opciones: map[string]string{
			"page": "all",
		},
	}

	got := construirOpcionesFirmaCompat(cfg, "PAdES")
	if got["page"] != "all" {
		t.Fatalf("page = %q, want all", got["page"])
	}
	if got["visibleSeal"] != "true" {
		t.Fatalf("visibleSeal perdido: %#v", got)
	}
}

func TestConstruirOpcionesFirmaCompat_UsaRangoPaginasExplicito(t *testing.T) {
	cfg := configCLI{
		selloVisible: true,
		selloPagina:  2,
		selloPaginas: "1,3-5",
		selloX:       0.62,
		selloY:       0.04,
		selloW:       0.34,
		selloH:       0.12,
	}

	got := construirOpcionesFirmaCompat(cfg, "PAdES")
	if got["page"] != "1,3-5" {
		t.Fatalf("page = %q, want 1,3-5", got["page"])
	}
}

func TestConstruirOpcionesFirmaCompat_NormalizaTodasLasPaginas(t *testing.T) {
	cfg := configCLI{
		selloVisible: true,
		selloPaginas: "todas",
		selloX:       0.62,
		selloY:       0.04,
		selloW:       0.34,
		selloH:       0.12,
	}

	got := construirOpcionesFirmaCompat(cfg, "PAdES")
	if got["page"] != "all" {
		t.Fatalf("page = %q, want all", got["page"])
	}
}

func TestConstruirOpcionesFirmaCompat_AplicaMetadatosPAdES(t *testing.T) {
	cfg := configCLI{
		qrSello:        "https://verifica.ejemplo/",
		motivoFirma:    "Aprobación interna",
		ubicacionFirma: "Granada",
		contactoFirma:  "contacto@example.invalid",
	}

	got := construirOpcionesFirmaCompat(cfg, "PAdES")
	if got["qrContent"] != "https://verifica.ejemplo/" {
		t.Fatalf("qrContent = %q", got["qrContent"])
	}
	if got["reason"] != "Aprobación interna" {
		t.Fatalf("reason = %q", got["reason"])
	}
	if got["location"] != "Granada" {
		t.Fatalf("location = %q", got["location"])
	}
	if got["contactInfo"] != "contacto@example.invalid" {
		t.Fatalf("contactInfo = %q", got["contactInfo"])
	}
}

func TestRunComprobarHashDirectorio_GeneraInforme(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "datos")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	hashPath := filepath.Join(tmp, "datos.hashfiles")
	if err := os.WriteFile(hashPath, []byte("<entries/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(tmp, "salida.hashreport")
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr
	adaptador.ComprobarHashDir = dirHashCheckMock{}
	adaptador.InformeHashDir = dirHashReportCodecMock{}

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "comprobar-hash",
		"-entrada", dir,
		"-fichero-hash", hashPath,
		"-salida", reportPath,
	})
	if rc != 2 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("se esperaba informe en disco: %v", err)
	}
	if !strings.Contains(string(data), "not_matching_hash") {
		t.Fatalf("informe inesperado: %s", string(data))
	}
	if !strings.Contains(stdout.String(), "Informe: "+reportPath) {
		t.Fatalf("salida sin referencia al informe: %s", stdout.String())
	}
}

func TestRunProteccion_AceptaAliasesEncryptedDataSinDestinatarios(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}
	secretB64 := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, container := range []string{
		"cms-encrypted-data",
		"CMS_ENCRYPTED_DATA",
		"cms encrypted data",
		"encrypted-data",
	} {
		t.Run(container, func(t *testing.T) {
			mock := &protectCLIMock{}
			var stdout strings.Builder
			var stderr strings.Builder
			adaptador := New(nil, nil).WithProteccion(mock, nil, nil)
			adaptador.Stdout = &stdout
			adaptador.Stderr = &stderr
			adaptador.Stdin = strings.NewReader(secretB64 + "\n")

			rc := adaptador.Run(context.Background(), []string{
				"-operacion", "proteger",
				"-entrada", entrada,
				"-perfil-proteccion", "compat",
				"-contenedor-proteccion", container,
				"-clave-proteccion-stdin",
				"-no-guardar",
			})
			if rc != 0 {
				t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
			}
			if got := mock.last.Options["container"]; got != container {
				t.Fatalf("container inesperado: %q", got)
			}
			if len(mock.last.RecipientIDs) != 0 {
				t.Fatalf("destinatarios inesperados: %#v", mock.last.RecipientIDs)
			}
		})
	}
}

func TestRunDesproteccion_RechazaPayloadSobreLimiteAntesDelCasoDeUso(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.encrypted.p7m")
	if err := os.WriteFile(entrada, []byte("demasiado"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := &unprotectCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).WithProteccion(nil, mock, nil)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr
	adaptador.Limits.MaxPayloadBytes = 4

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "desproteger",
		"-entrada", entrada,
		"-no-guardar",
	})
	if rc == 0 {
		t.Fatalf("se esperaba error, stdout=%s", stdout.String())
	}
	if mock.called {
		t.Fatal("el caso de uso no debe ejecutarse con un payload sobre el limite")
	}
	if !strings.Contains(stderr.String(), "supera el limite") {
		t.Fatalf("stderr inesperado: %s", stderr.String())
	}
}

func TestRunProteccion_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := &protectCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).
		WithProteccion(mock, nil, nil).
		WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "proteger",
		"-entrada", entrada,
		"-destinatario", "recipient-1",
		"-perfil-proteccion", "compat",
		"-no-guardar",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Protection completed successfully.",
		"Profile=compat",
		"Container=json",
		"Recipients=1",
		"Base64: Y21z",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestRunDesproteccion_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.encrypted.p7m")
	if err := os.WriteFile(entrada, []byte("cms"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := &unprotectCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).
		WithProteccion(nil, mock, nil).
		WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "desproteger",
		"-entrada", entrada,
		"-no-guardar",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Unprotection completed successfully.",
		"Profile=compat",
		"Recipient=recipient-1",
		"Base64: Y2xhcm8=",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestRunProteccionFirmada_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := &protectAndSignCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).
		WithProteccionFirmada(mock).
		WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "proteger-firmando",
		"-entrada", entrada,
		"-destinatario", "recipient-1",
		"-id-certificado", "certificate-1",
		"-perfil-proteccion", "compat",
		"-no-guardar",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Protection completed successfully.",
		"Profile=compat",
		"Container=SignedAndEnvelopedData",
		"Recipients=1",
		"Certificate=certificate-1",
		"Base64: Y21z",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestRunDestinatariosProteccionVacios_TextoHumanoEnIngles(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).
		WithProteccion(nil, nil, protectionRecipientsEmptyMock{}).
		WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "listar-destinatarios-proteccion",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(got, "No recipients loaded for this profile.") ||
		!strings.Contains(got, "authorized P12/PFX") ||
		!strings.Contains(got, "high profile") {
		t.Fatalf("salida sin guía accionable: %q", got)
	}
	if strings.Contains(got, "autorizado") || strings.Contains(got, "perfil alto") {
		t.Fatalf("la salida inglesa mezcla texto español: %q", got)
	}
}

func TestRunProteccionFirmada_AceptaAliasLegacyDeSignedAndEnvelopedData(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := &protectAndSignCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil).WithProteccionFirmada(mock)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "proteger-firmando",
		"-entrada", entrada,
		"-destinatario", "dest-1",
		"-id-certificado", "cert-firma-1",
		"-perfil-proteccion", "compat",
		"-contenedor-proteccion", "signed-and-enveloped-data",
	})
	if rc != 0 {
		t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
	}
	if got := mock.last.Options["container"]; got != "signedandenvelopeddata" {
		t.Fatalf("container inesperado: %q", got)
	}
}

func TestRunProteccionFirmada_RechazaAuthEnvelopedDataConErrorClaro(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := &protectAndSignCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil).WithProteccionFirmada(mock)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "proteger-firmando",
		"-entrada", entrada,
		"-destinatario", "dest-1",
		"-id-certificado", "cert-firma-1",
		"-perfil-proteccion", "compat",
		"-contenedor-proteccion", "authenvelopeddata",
	})
	if rc == 0 {
		t.Fatalf("se esperaba error, stdout=%s", stdout.String())
	}
	if mock.last.Document.Name != "" {
		t.Fatalf("no se esperaba ejecutar el caso de uso: %#v", mock.last)
	}
	if !strings.Contains(stderr.String(), "AuthEnvelopedData") {
		t.Fatalf("stderr inesperado: %s", stderr.String())
	}
}

func TestRunProteccionFirmada_RechazaEnvelopedDataConErrorClaro(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "secreto.txt")
	if err := os.WriteFile(entrada, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock := &protectAndSignCLIMock{}
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil).WithProteccionFirmada(mock)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "proteger-firmando",
		"-entrada", entrada,
		"-destinatario", "dest-1",
		"-id-certificado", "cert-firma-1",
		"-perfil-proteccion", "compat",
		"-contenedor-proteccion", "cms",
	})
	if rc == 0 {
		t.Fatalf("se esperaba error, stdout=%s", stdout.String())
	}
	if mock.last.Document.Name != "" {
		t.Fatalf("no se esperaba ejecutar el caso de uso: %#v", mock.last)
	}
	if !strings.Contains(stderr.String(), "EnvelopedData") {
		t.Fatalf("stderr inesperado: %s", stderr.String())
	}
}

func TestRunAyudaCLI_ExplicaSemanticaCMSDeProteccionFirmada(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{"-ayuda-cli"})
	if rc != 0 {
		t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "SignedAndEnvelopedData") {
		t.Fatalf("la ayuda no menciona SignedAndEnvelopedData: %s", out)
	}
	if !strings.Contains(out, "AuthEnvelopedData") {
		t.Fatalf("la ayuda no menciona AuthEnvelopedData: %s", out)
	}
	if !strings.Contains(out, "proteger-firmando") {
		t.Fatalf("la ayuda no menciona proteger-firmando: %s", out)
	}
	if !strings.Contains(out, "P12/PFX autorizado") ||
		!strings.Contains(out, "no se anuncian para cifrado compat") ||
		!strings.Contains(out, "capacidad real de desprotección RSA") {
		t.Fatalf("la ayuda no explica el requisito fail-closed de compat: %s", out)
	}
}

func TestRunAyudaCLI_EnumeraTodosLosFormatos(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil)
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	if rc := adaptador.Run(context.Background(), []string{"-ayuda-cli"}); rc != 0 {
		t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
	}
	const formatos = "auto|pades|cades|xades|xmldsig|odf|ooxml|facturae|asic-xades"
	if !strings.Contains(stdout.String(), "-formato "+formatos) {
		t.Fatalf("la ayuda CLI no enumera todos los formatos públicos: %s", stdout.String())
	}
}

func TestRunAyudaCLI_LocalizaCabeceras(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	adaptador := New(nil, nil).WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	if rc := adaptador.Run(context.Background(), []string{"-ayuda-cli"}); rc != 0 {
		t.Fatalf("se esperaba codigo 0, se obtuvo %d, stderr=%s", rc, stderr.String())
	}
	for _, esperado := range []string{
		"grxfirma CLI mode",
		"Option summary:",
		"Examples:",
	} {
		if !strings.Contains(stdout.String(), esperado) {
			t.Errorf("la ayuda localizada no contiene %q: %s", esperado, stdout.String())
		}
	}
	for _, castellano := range []string{
		"Modo CLI de grxfirma",
		"Resumen de opciones:",
		"Ejemplos:",
	} {
		if strings.Contains(stdout.String(), castellano) {
			t.Errorf("la cabecera conserva el texto castellano %q: %s", castellano, stdout.String())
		}
	}
}

func TestRunErroresBase_RespetaLocalizador(t *testing.T) {
	t.Run("sin operación", func(t *testing.T) {
		adaptador := New(nil, nil).WithLocalizador(localizador.Para("en"))
		if got := adaptador.t("cli.error.no_operation", "no se ha indicado ninguna operación CLI"); got != "no CLI operation was specified" {
			t.Fatalf("mensaje localizado inesperado: %q", got)
		}
	})

	t.Run("operación no soportada", func(t *testing.T) {
		var stdout strings.Builder
		var stderr strings.Builder
		adaptador := New(nil, nil).WithLocalizador(localizador.Para("en"))
		adaptador.Stdout = &stdout
		adaptador.Stderr = &stderr

		if rc := adaptador.Run(context.Background(), []string{"-operacion", "desconocida", "-entrada", "no-se-lee"}); rc == 0 {
			t.Fatalf("se esperaba error, stderr=%s", stderr.String())
		}
		if got := strings.TrimSpace(stderr.String()); got != "error: unsupported CLI operation: desconocida" {
			t.Fatalf("error localizado inesperado: %q", got)
		}
	})
}

func TestRunVerificar_JSONExponeResultadoRico(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firma.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7\nfirma"), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil).WithVerificador(verifyMock{})
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "verificar",
		"-entrada", entrada,
		"-salida-json",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	resultado, ok := payload["resultado"].(map[string]any)
	if !ok {
		t.Fatalf("resultado inesperado: %#v", payload["resultado"])
	}
	if resultado["formato"] != "PAdES" {
		t.Fatalf("resultado.formato inesperado: %#v", resultado["formato"])
	}
	if resultado["cobertura"] != "full" {
		t.Fatalf("resultado.cobertura inesperada: %#v", resultado["cobertura"])
	}
	integridad, ok := resultado["integridad"].(map[string]any)
	if !ok || integridad["estado"] != "valid" {
		t.Fatalf("integridad inesperada: %#v", resultado["integridad"])
	}
	advertencias, ok := resultado["advertencias"].([]any)
	if !ok || len(advertencias) != 1 {
		t.Fatalf("advertencias inesperadas: %#v", resultado["advertencias"])
	}
}

func TestRunVerificar_TextoHumanoMuestraFormatoCoberturaYAdvertencias(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firma.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7\nfirma"), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil).WithVerificador(verifyMock{})
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "verificar",
		"-entrada", entrada,
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Formato: PAdES") {
		t.Fatalf("salida sin formato: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Cobertura: full") {
		t.Fatalf("salida sin cobertura: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Advertencias:") || !strings.Contains(stdout.String(), "sin OCSP") {
		t.Fatalf("salida sin advertencias: %s", stdout.String())
	}
}

func TestRunVerificar_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "firma.pdf")
	if err := os.WriteFile(entrada, []byte("%PDF-1.7\nfirma"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).
		WithVerificador(verifyMock{}).
		WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "verificar",
		"-entrada", entrada,
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Valid signature.",
		"Format: PAdES",
		"Coverage: full",
		"Warnings:",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestParsearArgs_AceptaSelloPaginas(t *testing.T) {
	cfg, err := parsearArgs([]string{
		"-entrada", "demo.pdf",
		"-sello-visible",
		"-sello-paginas", "1,3-5",
	})
	if err != nil {
		t.Fatalf("parsearArgs fallo: %v", err)
	}
	if cfg.selloPaginas != "1,3-5" {
		t.Fatalf("selloPaginas = %q, want 1,3-5", cfg.selloPaginas)
	}
}

func TestParsearArgs_RechazaSecretosConValorEnArgv(t *testing.T) {
	for _, args := range [][]string{
		{"-operacion", "importar-p12", "-contrasena-p12", "secreto"},
		{"-operacion", "proteger", "-clave-proteccion-b64", "secreto"},
	} {
		if _, err := parsearArgs(args); err == nil || !strings.Contains(secretinput.UserMessage(err, nil), "CWE-214") {
			t.Errorf("parsearArgs(%q) error = %v", args, err)
		}
	}
}

func TestParsearArgs_RechazaOpcionesSensiblesSinReflejarValor(t *testing.T) {
	const secret = "secreto-opcion-no-reflejado"
	for _, args := range [][]string{
		{"-operacion", "firmar", "-opcion", "password=" + secret},
		{"-operacion", "firmar", "-opcion=secret_b64=" + secret},
		{"-operacion", "firmar", "--opcion=authorization=Bearer " + secret},
	} {
		_, err := parsearArgs(args)
		if err == nil {
			t.Errorf("parsearArgs(%q) aceptó una opción sensible", args)
			continue
		}
		if strings.Contains(err.Error(), secret) || strings.Contains(secretinput.UserMessage(err, nil), secret) {
			t.Errorf("parsearArgs(%q) reflejó el secreto: %v", args, err)
		}
	}
}

func TestRunCrearHash_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	entrada := filepath.Join(tmp, "documento.bin")
	if err := os.WriteFile(entrada, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).
		WithHashes(hashCreateMock{}, nil).
		WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "crear-hash",
		"-entrada", entrada,
		"-formato-hash", "hex",
		"-no-guardar",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Hash generated successfully.",
		"Algorithm=SHA-256",
		"Format=hex",
		"Hash: 616263",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestRunComprobarHashDirectorio_TextoHumanoEnIngles(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "datos")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	hashPath := filepath.Join(tmp, "datos.hashfiles")
	if err := os.WriteFile(hashPath, []byte("<entries/>"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	var stderr strings.Builder
	adaptador := New(nil, nil).WithLocalizador(localizador.Para("en"))
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr
	adaptador.ComprobarHashDir = dirHashCheckMock{}

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "comprobar-hash",
		"-entrada", dir,
		"-fichero-hash", hashPath,
	})
	if rc != 2 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	for _, expected := range []string{
		"Directory differs from the manifest.",
		"Algorithm=SHA-256",
		"Not matching: dos.txt",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Errorf("salida inglesa sin %q: %s", expected, stdout.String())
		}
	}
}

func TestOpcionesCLI_SetRechazaClaveSensible(t *testing.T) {
	opts := opcionesCLI{}
	err := opts.Set("private_key=valor")
	var inputErr *secretinput.Error
	if !errors.As(err, &inputErr) || inputErr.Code != secretinput.CodeSensitiveOption {
		t.Fatalf("Set() error = %#v", err)
	}
}

func TestParsearArgs_AceptaFuentesStdinSinSecreto(t *testing.T) {
	cfg, err := parsearArgs([]string{
		"-operacion", "importar-p12",
		"-contrasena-p12-stdin",
		"-clave-proteccion-stdin",
	})
	if err != nil {
		t.Fatalf("parsearArgs() error = %v", err)
	}
	if !cfg.contrasenaP12Stdin || !cfg.secretProteccionStdin {
		t.Fatalf("fuentes stdin no conservadas: %+v", cfg)
	}
}

func TestParsearArgs_NormalizaTodasLasPaginas(t *testing.T) {
	cfg, err := parsearArgs([]string{
		"-entrada", "demo.pdf",
		"-sello-visible",
		"-sello-paginas", "todas",
	})
	if err != nil {
		t.Fatalf("parsearArgs fallo: %v", err)
	}
	if cfg.selloPaginas != "all" {
		t.Fatalf("selloPaginas = %q, want all", cfg.selloPaginas)
	}
}

func TestParsearArgs_RechazaSelloPaginasInvalido(t *testing.T) {
	_, err := parsearArgs([]string{
		"-entrada", "demo.pdf",
		"-sello-visible",
		"-sello-paginas", "1,a",
	})
	if err == nil {
		t.Fatal("se esperaba error por seleccion de paginas invalida")
	}
	if !strings.Contains(err.Error(), "seleccion de paginas invalida") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestParsearArgs_AceptaMetadatosPAdES(t *testing.T) {
	cfg, err := parsearArgs([]string{
		"-entrada", "demo.pdf",
		"-sello-qr", "https://verifica.ejemplo/",
		"-motivo-firma", "Aprobación interna",
		"-ubicacion-firma", "Granada",
		"-contacto-firma", "contacto@example.invalid",
	})
	if err != nil {
		t.Fatalf("parsearArgs fallo: %v", err)
	}
	if cfg.qrSello != "https://verifica.ejemplo/" {
		t.Fatalf("qrSello = %q", cfg.qrSello)
	}
	if cfg.motivoFirma != "Aprobación interna" {
		t.Fatalf("motivoFirma = %q", cfg.motivoFirma)
	}
	if cfg.ubicacionFirma != "Granada" {
		t.Fatalf("ubicacionFirma = %q", cfg.ubicacionFirma)
	}
	if cfg.contactoFirma != "contacto@example.invalid" {
		t.Fatalf("contactoFirma = %q", cfg.contactoFirma)
	}
}

func TestRunFallaSinEntradaNiLote(t *testing.T) {
	adaptador := New(nil, nil)
	var stderr strings.Builder
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), nil)
	if rc != 1 {
		t.Fatalf("se esperaba codigo 1, se obtuvo %d", rc)
	}
	if !strings.Contains(stderr.String(), "debe indicar -entrada o -lote") {
		t.Fatalf("mensaje de error inesperado: %s", stderr.String())
	}
}

func TestRunFirma_FallaConOpcionInvalida(t *testing.T) {
	adaptador := New(&signMock{}, nil)
	var stderr strings.Builder
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-entrada", "dummy.pdf",
		"-opcion", "sin-igual",
	})
	if rc != 1 {
		t.Fatalf("se esperaba codigo 1, se obtuvo %d", rc)
	}
	if !strings.Contains(stderr.String(), "clave=valor") {
		t.Fatalf("mensaje de error inesperado: %s", stderr.String())
	}
}

func TestLeerDominiosConfiados_NoReapareceSemillaTrasBorrado(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(truststore.SeedMarkerPath(tmp), []byte("seeded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tmp, "trusted-domains.compatibility-v1.seeded"),
		[]byte("seeded-compatibility\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "trusted-domains.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp

	dominios, err := adaptador.leerDominiosConfiados(context.Background())
	if err != nil {
		t.Fatalf("leerDominiosConfiados fallo: %v", err)
	}
	if len(dominios) != 0 {
		t.Fatalf("no deberia reaparecer la semilla tras borrado, dominios=%#v", dominios)
	}
}

func TestImportarDominiosIniciales_UsaGestionDominios(t *testing.T) {
	mock := &dominiosMock{}
	adaptador := New(nil, nil)
	adaptador.GestionDominios = mock

	importados, err := adaptador.importarDominiosIniciales(context.Background())
	if err != nil {
		t.Fatalf("importarDominiosIniciales fallo: %v", err)
	}
	if importados != len(truststore.DefaultTrustedOrigins()) {
		t.Fatalf("importados=%d, esperados=%d", importados, len(truststore.DefaultTrustedOrigins()))
	}
	if len(mock.llamadas) != len(truststore.DefaultTrustedOrigins()) {
		t.Fatalf("llamadas=%d, esperadas=%d", len(mock.llamadas), len(truststore.DefaultTrustedOrigins()))
	}
	for _, llamada := range mock.llamadas {
		if llamada.Action != application.TrustActionAllow {
			t.Fatalf("accion inesperada: %s", llamada.Action)
		}
	}
}

func TestEjecutarDominiosListar_JSONVacioNoPregunta(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(truststore.SeedMarkerPath(tmp), []byte("seeded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tmp, "trusted-domains.compatibility-v1.seeded"),
		[]byte("seeded-compatibility\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "trusted-domains.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	adaptador.Stdin = strings.NewReader("s\n")
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.ejecutarDominiosListar(context.Background(), configCLI{json: true})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	var payload struct {
		Exito    bool              `json:"exito"`
		Dominios []json.RawMessage `json:"dominios"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	if len(payload.Dominios) != 0 {
		t.Fatalf("se esperaban 0 dominios, salida=%s", stdout.String())
	}
}

func TestRunImportarP12_GuardaEnDirectorioDelPerfil(t *testing.T) {
	tmp := t.TempDir()
	p12Path := filepath.Join(tmp, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12CLI(t, "CLI Prueba", "valor-prueba"), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = filepath.Join(tmp, "config")
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr
	adaptador.Stdin = strings.NewReader("valor-prueba\n")

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "importar-p12",
		"-fichero-p12", p12Path,
		"-contrasena-p12-stdin",
		"-salida-json",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	destino := filepath.Join(adaptador.ConfigDir, "pkcs12", "prueba.p12")
	if _, err := os.Stat(destino); err != nil {
		t.Fatalf("se esperaba P12 importado en %s: %v", destino, err)
	}
}

func TestRunImportarP12_EntornoCompatSeConsume(t *testing.T) {
	tmp := t.TempDir()
	p12Path := filepath.Join(tmp, "compat.p12")
	if err := os.WriteFile(p12Path, generarP12CLI(t, "CLI Compat", "valor-entorno"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envPKCS12PasswordCLI, "valor-entorno")

	adaptador := New(nil, nil)
	adaptador.ConfigDir = filepath.Join(tmp, "config")
	adaptador.Stdout = io.Discard
	var stderr strings.Builder
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "importar-p12",
		"-fichero-p12", p12Path,
		"-salida-json",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	if _, present := os.LookupEnv(envPKCS12PasswordCLI); present {
		t.Fatalf("%s siguió disponible para procesos hijos", envPKCS12PasswordCLI)
	}
}

func TestRun_ConsumeTodosLosSecretosDelEntornoAntesDeAyuda(t *testing.T) {
	names := []string{
		envPKCS12PasswordCLI,
		envProtectionSecret,
		"GRXFIRMA_REST_TOKEN",
	}
	for _, name := range names {
		t.Setenv(name, "secreto-"+name)
	}
	adaptador := New(nil, nil)
	adaptador.Stdout = io.Discard
	adaptador.Stderr = io.Discard
	if rc := adaptador.Run(context.Background(), []string{"-ayuda"}); rc != 0 {
		t.Fatalf("Run(-ayuda) = %d", rc)
	}
	for _, name := range names {
		if _, present := os.LookupEnv(name); present {
			t.Fatalf("%s siguió disponible antes de resolver la operación", name)
		}
	}
}

func TestRunImportarP12_RecibeCompatYaRetiradaDelEntorno(t *testing.T) {
	tmp := t.TempDir()
	p12Path := filepath.Join(tmp, "transferida.p12")
	if err := os.WriteFile(p12Path, generarP12CLI(t, "CLI Transferida", "valor-transferido"), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil).WithCompatibilitySecrets("valor-transferido", "")
	adaptador.ConfigDir = filepath.Join(tmp, "config")
	adaptador.Stdout = io.Discard
	adaptador.Stderr = io.Discard

	rc := adaptador.Run(context.Background(), []string{
		"-operacion", "importar-p12",
		"-fichero-p12", p12Path,
	})
	if rc != 0 {
		t.Fatalf("código inesperado: %d", rc)
	}
	if adaptador.passwordP12Compat != "" {
		t.Fatal("el adaptador conservó la contraseña compatible después de consumirla")
	}
}

func TestRunLimpiarAutoseleccionCert_BorraPersistenteYSesion(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://portal":"cert-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://portal":"cert-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "limpiar-autoseleccion-cert",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	if _, err := os.Stat(persistente); !os.IsNotExist(err) {
		t.Fatalf("se esperaba borrar el fichero persistente, stat err=%v", err)
	}
	if _, err := os.Stat(sesion); !os.IsNotExist(err) {
		t.Fatalf("se esperaba borrar el fichero de sesión, stat err=%v", err)
	}
	if !strings.Contains(stdout.String(), "Autoselección de certificado limpiada") {
		t.Fatalf("salida inesperada: %s", stdout.String())
	}
}

func TestRunListarAutoseleccionCert_MuestraPersistenteYSesion(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://persistente":"cert-persistente"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://sesion":"cert-sesion"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "listar-autoseleccion-cert",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	salida := stdout.String()
	if !strings.Contains(salida, "Persistente") || !strings.Contains(salida, "https://persistente -> cert-persistente") {
		t.Fatalf("salida persistente inesperada: %s", salida)
	}
	if !strings.Contains(salida, "Sesión") || !strings.Contains(salida, "https://sesion -> cert-sesion") {
		t.Fatalf("salida de sesión inesperada: %s", salida)
	}
}

func TestRunListarAutoseleccionCert_FiltraPorDominio(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://uno":"cert-uno","https://dos":"cert-dos"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://dos":"cert-sesion-dos"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "listar-autoseleccion-cert",
		"-dominio", "https://dos",
		"-salida-json",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	salida := stdout.String()
	if !strings.Contains(salida, "\"dominio_filtrado\": \"https://dos\"") {
		t.Fatalf("json sin dominio filtrado: %s", salida)
	}
	if !strings.Contains(salida, "\"https://dos\": \"cert-dos\"") || !strings.Contains(salida, "\"https://dos\": \"cert-sesion-dos\"") {
		t.Fatalf("json sin entradas filtradas esperadas: %s", salida)
	}
	if strings.Contains(salida, "https://uno") {
		t.Fatalf("el filtro no deberia incluir otros dominios: %s", salida)
	}
}

func TestRunEliminarAutoseleccionCert_BorraSoloElDominioPedido(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://uno":"cert-uno","https://dos":"cert-dos"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://dos":"cert-sesion-dos","https://tres":"cert-sesion-tres"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "eliminar-autoseleccion-cert",
		"-dominio", "https://dos",
		"-salida-json",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}

	datosPersistentes, err := os.ReadFile(persistente)
	if err != nil {
		t.Fatalf("leyendo persistente: %v", err)
	}
	if strings.Contains(string(datosPersistentes), "https://dos") || !strings.Contains(string(datosPersistentes), "https://uno") {
		t.Fatalf("persistente inesperado: %s", string(datosPersistentes))
	}

	datosSesion, err := os.ReadFile(sesion)
	if err != nil {
		t.Fatalf("leyendo sesion: %v", err)
	}
	if strings.Contains(string(datosSesion), "https://dos") || !strings.Contains(string(datosSesion), "https://tres") {
		t.Fatalf("sesion inesperada: %s", string(datosSesion))
	}

	salida := stdout.String()
	if !strings.Contains(salida, "\"dominio\": \"https://dos\"") || !strings.Contains(salida, "\"eliminado_persistente\": true") || !strings.Contains(salida, "\"eliminado_sesion\": true") {
		t.Fatalf("json inesperado: %s", salida)
	}
}

func TestRunResetearAutoseleccionCert_ConfirmaYBorraTodo(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://uno":"cert-uno"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://dos":"cert-dos"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	adaptador.Stdin = strings.NewReader("s\n")
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "resetear-autoseleccion-cert",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	if _, err := os.Stat(persistente); !os.IsNotExist(err) {
		t.Fatalf("se esperaba borrar el fichero persistente, stat err=%v", err)
	}
	if _, err := os.Stat(sesion); !os.IsNotExist(err) {
		t.Fatalf("se esperaba borrar el fichero de sesión, stat err=%v", err)
	}
	if !strings.Contains(stdout.String(), "¿Continuar? [s/N]:") {
		t.Fatalf("faltó confirmación en salida: %s", stdout.String())
	}
}

func TestRunResetearAutoseleccionCert_CancelaSinBorrar(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://uno":"cert-uno"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://dos":"cert-dos"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	adaptador.Stdin = strings.NewReader("n\n")
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "resetear-autoseleccion-cert",
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}
	if _, err := os.Stat(persistente); err != nil {
		t.Fatalf("persistente no debería borrarse: %v", err)
	}
	if _, err := os.Stat(sesion); err != nil {
		t.Fatalf("sesión no debería borrarse: %v", err)
	}
	if !strings.Contains(stdout.String(), "Operación cancelada.") {
		t.Fatalf("salida inesperada: %s", stdout.String())
	}
}

func TestRunExportarAutoseleccionCert_GeneraFicheroJSON(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")
	if err := os.WriteFile(persistente, []byte(`{"https://persistente":"cert-persistente"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sesion, []byte(`{"https://sesion":"cert-sesion"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	destino := filepath.Join(tmp, "exportacion", "autoseleccion.json")

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "exportar-autoseleccion-cert",
		"-fichero-autoseleccion", destino,
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}

	data, err := os.ReadFile(destino)
	if err != nil {
		t.Fatalf("no se pudo leer la exportación: %v", err)
	}
	var payload struct {
		Persistente map[string]string `json:"persistente"`
		Sesion      map[string]string `json:"sesion"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json exportado inválido: %v", err)
	}
	if payload.Persistente["https://persistente"] != "cert-persistente" {
		t.Fatalf("persistente exportado inesperado: %#v", payload.Persistente)
	}
	if payload.Sesion["https://sesion"] != "cert-sesion" {
		t.Fatalf("sesión exportada inesperada: %#v", payload.Sesion)
	}
}

func TestRunImportarAutoseleccionCert_RestauraPersistenteYSesion(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	origen := filepath.Join(tmp, "autoseleccion.json")
	if err := os.WriteFile(origen, []byte(`{
  "persistente": {
    "https://persistente": "cert-persistente"
  },
  "sesion": {
    "https://sesion": "cert-sesion"
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	adaptador := New(nil, nil)
	adaptador.ConfigDir = tmp
	var stdout strings.Builder
	var stderr strings.Builder
	adaptador.Stdout = &stdout
	adaptador.Stderr = &stderr

	rc := adaptador.Run(context.Background(), []string{
		"-modo-cli",
		"-operacion", "importar-autoseleccion-cert",
		"-fichero-autoseleccion", origen,
	})
	if rc != 0 {
		t.Fatalf("codigo inesperado: %d stderr=%s", rc, stderr.String())
	}

	persistente := filepath.Join(tmp, "preferred-certificates.json")
	sesion := filepath.Join(runtimeDir, "grxfirma", "preferred-certificates-session.json")

	dataPersistente, err := os.ReadFile(persistente)
	if err != nil {
		t.Fatalf("no se pudo leer persistencia importada: %v", err)
	}
	dataSesion, err := os.ReadFile(sesion)
	if err != nil {
		t.Fatalf("no se pudo leer sesión importada: %v", err)
	}
	if !strings.Contains(string(dataPersistente), "https://persistente") {
		t.Fatalf("persistencia importada inesperada: %s", string(dataPersistente))
	}
	if !strings.Contains(string(dataSesion), "https://sesion") {
		t.Fatalf("sesión importada inesperada: %s", string(dataSesion))
	}
}

func generarP12CLI(t *testing.T, commonName, password string) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	data, err := pkcs12.Modern.Encode(priv, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode: %v", err)
	}
	return data
}
