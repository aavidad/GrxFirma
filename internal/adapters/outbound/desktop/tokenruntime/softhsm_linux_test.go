// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux && pkcs11_preview && !production && softhsm_qa

package tokenruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/pdffixture"
)

// Opt-in integration in a private mount/user/network namespace. Never run this
// suite against installed/personal SoftHSM stores: /qa and its /etc are fixtures.
func requireSoftHSMQA(t *testing.T) {
	t.Helper()
	if os.Getenv("GRXFIRMA_SOFTHSM_QA") != "isolated-synthetic" {
		t.Skip("requires isolated synthetic SoftHSM harness")
	}
	if _, err := os.Stat("/qa/config/tokens.json"); err != nil {
		t.Fatal("missing private QA config")
	}
	if _, err := os.Stat("/home"); !os.IsNotExist(err) {
		t.Fatal("QA must not expose any host home")
	}
	if _, err := os.Stat("/var/lib/softhsm"); !os.IsNotExist(err) {
		t.Fatal("QA must not expose system tokens")
	}
}

func TestSoftHSMUninitializedSlotDoesNotBreakCatalog(t *testing.T) {
	requireSoftHSMQA(t)
	runtime := New("/qa/config", nil)
	refs, err := runtime.List(context.Background())
	if err != nil {
		t.Fatalf("standard SoftHSM catalog failed: %v; diagnostic=%v", err, runtime.Diagnostico())
	}
	if len(refs) != 2 {
		t.Fatalf("expected exactly two synthetic identities, got %d", len(refs))
	}
	for _, ref := range refs {
		if ref.HasSigningKey || !ref.SigningKeyNeedsUnlock {
			t.Fatal("catalogue claimed private key access without login")
		}
	}
	t.Logf("synthetic certificate count=%d", len(refs))
}

func TestSoftHSMMemoryLockFailureBeforePIN(t *testing.T) {
	requireSoftHSMQA(t)
	const childEnvironment = "GRXFIRMA_SOFTHSM_QA_MEMLOCK_CHILD"
	if os.Getenv(childEnvironment) != "isolated-limit-zero" {
		var before, after unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &before); err != nil {
			t.Fatal(err)
		}
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestSoftHSMMemoryLockFailureBeforePIN$", "-test.count=1", "-test.v")
		// Re-exec only this test inside the already required private namespace.
		// No arbitrary inherited environment, driver override or secret is used.
		command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "GORACE=atexit_sleep_ms=0", "GRXFIRMA_SOFTHSM_QA=isolated-synthetic", childEnvironment + "=isolated-limit-zero"}
		command.Dir = "/"
		output, runErr := command.CombinedOutput()
		if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &after); err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatal("subprocess changed the principal test memory limit")
		}
		if runErr != nil {
			t.Fatalf("isolated memory-limit test failed: %v\n%s", runErr, output)
		}
		t.Log("real worker rejected unavailable locked memory; PIN calls=0, signature bytes=0, principal limit unchanged")
		return
	}

	// Irreversible lowering is confined to this dedicated test process. The
	// real worker inherits it through exec; no host/parent limit is changed.
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		t.Fatal(err)
	}
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_MEMLOCK, &limit); err != nil || limit.Cur != 0 || limit.Max != 0 {
		t.Fatal("dedicated subprocess did not enforce zero locked-memory limit")
	}
	var prompts atomic.Int32
	runtime := New("/qa/config", func(context.Context, domain.CertificateRef, pkcs11worker.PINMode) ([]byte, error) {
		prompts.Add(1)
		return nil, errors.New("memory denial must precede any PIN prompt")
	})
	ref := softHSMRef(t, runtime, "RSA")
	uc := application.NuevoSignDocumentUseCase(runtime, runtime, desktopsigner.NuevoMotorFirmaGo(nil), softHSMApproval(func(context.Context, string) (bool, error) { return true, nil }), nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := uc.Ejecutar(ctx, application.SignCommand{
		Document: domain.Document{Name: "qa-memory.txt", MIMEType: "text/plain", Content: []byte("synthetic locked-memory failure QA")},
		Format:   domain.FormatCAdES, Action: domain.ActionSign, CertificateID: ref.ID,
	})
	if !errors.Is(err, pkcs11worker.ErrPINMemoryUnavailable) || prompts.Load() != 0 || len(result.Result.Data) != 0 {
		t.Fatalf("memory failure contract: err=%v PIN calls=%d signature bytes=%d", err, prompts.Load(), len(result.Result.Data))
	}
}

type softHSMApproval func(context.Context, string) (bool, error)

func (f softHSMApproval) Request(ctx context.Context, message string) (bool, error) {
	return f(ctx, message)
}

func softHSMRef(t *testing.T, runtime *Runtime, algorithm string) domain.CertificateRef {
	t.Helper()
	refs, err := runtime.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if strings.Contains(ref.Subject, "AF2 Synthetic "+algorithm+" QA ONLY") {
			return ref
		}
	}
	t.Fatalf("missing synthetic %s identity", algorithm)
	return domain.CertificateRef{}
}

// The real application use case resolves metadata, obtains approval, describes
// an opaque key and calls the isolated worker. No local private key is used by
// this test executable. Software-token integration is not hardware certification.
func TestSoftHSMApplicationFormatMatrix(t *testing.T) {
	requireSoftHSMQA(t)
	for _, algorithm := range []string{"RSA", "EC"} {
		for _, fixture := range softHSMDocuments(t) {
			t.Run(algorithm+"/"+string(fixture.format), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				var approvals, prompts atomic.Int32
				var borrowed []byte
				var chosen domain.CertificateRef
				runtime := New("/qa/config", func(_ context.Context, ref domain.CertificateRef, mode pkcs11worker.PINMode) ([]byte, error) {
					prompts.Add(1)
					if approvals.Load() != 1 || ref.ID != chosen.ID || ref.Fingerprint != chosen.Fingerprint || mode != (pkcs11worker.PINMode{}) {
						return nil, errors.New("unexpected QA identity, approval order or PIN mode")
					}
					borrowed = []byte("648291") // synthetic SoftHSM fixture only
					return borrowed, nil
				})
				chosen = softHSMRef(t, runtime, algorithm)
				if prompts.Load() != 0 {
					t.Fatal("catalogue requested PIN")
				}
				approval := softHSMApproval(func(context.Context, string) (bool, error) {
					approvals.Add(1)
					return true, nil
				})
				uc := application.NuevoSignDocumentUseCase(runtime, runtime, desktopsigner.NuevoMotorFirmaGo(nil), approval, nil, nil)
				result, err := uc.Ejecutar(ctx, application.SignCommand{
					Document: fixture.document, Format: fixture.format, Action: domain.ActionSign, CertificateID: chosen.ID,
				})
				// Existing XML engines are RSA-only. This is an explicit negative
				// capability test, not a successful EC signature or skipped test.
				if algorithm == "EC" && fixture.format != domain.FormatCAdES && fixture.format != domain.FormatPAdES {
					if err == nil || !strings.Contains(err.Error(), "RSA") || len(result.Result.Data) != 0 || prompts.Load() != 0 {
						t.Fatalf("unsupported EC XML must fail before PIN: err=%v prompts=%d", err, prompts.Load())
					}
					t.Log("unsupported EC/XML rejected before PIN; not signing support")
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if approvals.Load() != 1 || prompts.Load() != 1 || len(borrowed) == 0 || !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
					t.Fatalf("approval/PIN/zeroing contract failed: approvals=%d prompts=%d", approvals.Load(), prompts.Load())
				}
				if len(result.Result.Data) == 0 || result.CertificateUsed.Fingerprint != chosen.Fingerprint || len(result.CertificateChainDER) != 1 {
					t.Fatal("missing signed output or wrong synthetic identity")
				}
				var verified domain.VerificationResult
				var signers []domain.CertificateRef
				if fixture.format == domain.FormatCAdES {
					verified, signers, err = commonsigner.NewCAdESVerifier().VerifyDetachedCMS(ctx, result.Result.Data, fixture.document.Content)
				} else {
					verified, signers, err = fixture.verifier.Verify(ctx, domain.Document{Name: fixture.output, MIMEType: fixture.document.MIMEType, Content: result.Result.Data}, domain.CertificateChain{})
				}
				if err != nil || verified.Integrity.Status != domain.VerificationStatusValid || len(signers) != 1 {
					t.Fatalf("signed output verification failed: err=%v result=%+v", err, verified)
				}
				if verified.Trust.Status == domain.VerificationStatusValid {
					t.Fatal("self-signed QA identity must not acquire public trust")
				}
				if signers[0].Fingerprint != chosen.Fingerprint {
					t.Fatal("verified signer differs from selected certificate")
				}
				directory := filepath.Join("/qa/results", strings.ToLower(algorithm))
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, fixture.output), result.Result.Data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "original-"+fixture.document.Name), fixture.document.Content, 0600); err != nil {
					t.Fatal(err)
				}
				t.Logf("bytes=%d integrity=%s trust=%s approval=1 PIN=1 cleared=true", len(result.Result.Data), verified.Integrity.Status, verified.Trust.Status)
			})
		}
	}
}

func TestSoftHSMApplicationCancellationAndWrongPIN(t *testing.T) {
	requireSoftHSMQA(t)
	for _, scenario := range []string{"approval-denied", "PIN-cancelled", "wrong-PIN", "context-cancelled", "valid-after-errors"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var prompts atomic.Int32
			var borrowed []byte
			runtime := New("/qa/config", func(context.Context, domain.CertificateRef, pkcs11worker.PINMode) ([]byte, error) {
				prompts.Add(1)
				if scenario == "PIN-cancelled" {
					return nil, pkcs11worker.ErrPINCancelled
				}
				if scenario == "context-cancelled" {
					cancel()
					return nil, context.Canceled
				}
				borrowed = []byte("648291")
				if scenario == "wrong-PIN" {
					borrowed[0] = '0'
				}
				return borrowed, nil
			})
			ref := softHSMRef(t, runtime, "RSA")
			approval := softHSMApproval(func(context.Context, string) (bool, error) { return scenario != "approval-denied", nil })
			uc := application.NuevoSignDocumentUseCase(runtime, runtime, desktopsigner.NuevoMotorFirmaGo(nil), approval, nil, nil)
			result, err := uc.Ejecutar(ctx, application.SignCommand{Document: domain.Document{Name: "qa.txt", MIMEType: "text/plain", Content: []byte("synthetic cancellation QA")}, Format: domain.FormatCAdES, Action: domain.ActionSign, CertificateID: ref.ID})
			if !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
				t.Fatal("PIN buffer not cleared on completion")
			}
			if scenario == "valid-after-errors" {
				if err != nil || len(result.Result.Data) == 0 || prompts.Load() != 1 {
					t.Fatalf("fresh operation did not recover: %v", err)
				}
				return
			}
			if err == nil || len(result.Result.Data) != 0 {
				t.Fatal("failed/cancelled operation returned a signature")
			}
			wantedPrompts := int32(1)
			if scenario == "approval-denied" {
				wantedPrompts = 0
			}
			if prompts.Load() != wantedPrompts {
				t.Fatalf("unexpected PIN retry/count: %d", prompts.Load())
			}
			if scenario == "wrong-PIN" {
				var operation *pkcs11worker.OperationError
				if !errors.As(err, &operation) || operation.Code != "pin_incorrect" {
					t.Fatalf("wrong PIN lost fixed protocol code: %v", err)
				}
			}
			if scenario == "context-cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("context cancellation lost: %v", err)
			}
			t.Logf("negative case bounded, PIN calls=%d, no signature", prompts.Load())
		})
	}
}

type softHSMForbiddenServices struct{ calls atomic.Int32 }

func (s *softHSMForbiddenServices) RequestTimestamp(context.Context, []byte, crypto.Hash) ([]byte, error) {
	s.calls.Add(1)
	return nil, errors.New("QA must not request a TSA")
}

func (s *softHSMForbiddenServices) Fetch(context.Context, *x509.Certificate, *x509.Certificate) (ports.RevocationEvidence, error) {
	s.calls.Add(1)
	return ports.RevocationEvidence{}, errors.New("QA must not request revocation")
}

func TestSoftHSMLongTermMissingChainRejectsBeforePINAndNetwork(t *testing.T) {
	requireSoftHSMQA(t)
	for _, profile := range []string{"LT", "LTA"} {
		t.Run(profile, func(t *testing.T) {
			var prompts atomic.Int32
			runtime := New("/qa/config", func(context.Context, domain.CertificateRef, pkcs11worker.PINMode) ([]byte, error) {
				prompts.Add(1)
				return nil, errors.New("missing chain must fail before PIN")
			})
			ref := softHSMRef(t, runtime, "RSA")
			services := &softHSMForbiddenServices{}
			motor := desktopsigner.NuevoMotorFirmaGo(nil).WithTimestampAuthority(services).WithRevocationProvider(services)
			uc := application.NuevoSignDocumentUseCase(runtime, runtime, motor, softHSMApproval(func(context.Context, string) (bool, error) { return true, nil }), nil, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result, err := uc.Ejecutar(ctx, application.SignCommand{Document: domain.Document{Name: "qa.txt", MIMEType: "text/plain", Content: []byte("synthetic preflight only")}, Format: domain.FormatCAdES, Action: domain.ActionSign, CertificateID: ref.ID, Options: map[string]string{"profile": profile}})
			if err == nil || !strings.Contains(err.Error(), "cadena de certificación vacía") || len(result.Result.Data) != 0 || prompts.Load() != 0 || services.calls.Load() != 0 {
				t.Fatalf("preflight failed: err=%v PIN=%d services=%d", err, prompts.Load(), services.calls.Load())
			}
		})
	}
}

type softHSMDocument struct {
	format   domain.SignatureFormat
	document domain.Document
	output   string
	verifier ports.VerifierEngine
}

func softHSMDocuments(t *testing.T) []softHSMDocument {
	t.Helper()
	text := domain.Document{Name: "qa.txt", MIMEType: "text/plain", Content: []byte("AF2 synthetic token integration only\n")}
	xml := domain.Document{Name: "qa.xml", MIMEType: "application/xml", Content: []byte(`<qa>synthetic document only</qa>`)}
	factura := domain.Document{Name: "qa-factura.xml", MIMEType: "application/xml", Content: []byte(`<Facturae xmlns="http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"><FileHeader xmlns=""></FileHeader><Parties xmlns=""></Parties><Invoices xmlns=""></Invoices></Facturae>`)}
	odf := softHSMZip(t, map[string]string{
		"mimetype":              "application/vnd.oasis.opendocument.text",
		"META-INF/manifest.xml": `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`,
		"content.xml":           `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`,
	})
	ooxml := softHSMZip(t, map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"docProps/app.xml":    `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"/>`,
		"docProps/core.xml":   `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"/>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
	})
	return []softHSMDocument{
		{domain.FormatCAdES, text, "signed.p7s", nil},
		{domain.FormatPAdES, domain.Document{Name: "qa.pdf", MIMEType: "application/pdf", Content: pdffixture.Minimal()}, "signed.pdf", commonsigner.NewPAdESVerifier()},
		{domain.FormatXAdES, xml, "xades.xml", commonsigner.NewXAdESVerifier()},
		{"XMLdSig", xml, "xmldsig.xml", commonsigner.NewXMLDSigVerifier()},
		{"FacturaE", factura, "factura.xml", commonsigner.NewFacturaEVerifier()},
		{"ASiC-XAdES", text, "signed.asice", commonsigner.NewASiCXAdESVerifier()},
		{"ODF", domain.Document{Name: "qa.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: odf}, "signed.odt", commonsigner.NewODFVerifier()},
		{"OOXML", domain.Document{Name: "qa.docx", MIMEType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Content: ooxml}, "signed.docx", commonsigner.NewOOXMLVerifier()},
	}
}

func softHSMZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "mimetype" {
			return true
		}
		if names[j] == "mimetype" {
			return false
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(files[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
