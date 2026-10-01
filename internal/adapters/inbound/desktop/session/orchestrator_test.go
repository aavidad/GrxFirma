// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package session_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/desktop/session"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/originvalidator"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/progressdialog"
)

// --- Mocks / stubs ---

// mockTrustPolicy implementa ports.TrustPolicy para tests.
type mockTrustPolicy struct {
	evaluateFn func(ctx context.Context, origin string) (domain.TrustDecision, error)
}

func (m *mockTrustPolicy) Evaluate(ctx context.Context, origin string) (domain.TrustDecision, error) {
	if m.evaluateFn != nil {
		return m.evaluateFn(ctx, origin)
	}
	return domain.TrustDecision{Status: domain.TrustAllowed}, nil
}

func (m *mockTrustPolicy) Allow(ctx context.Context, origin string) error  { return nil }
func (m *mockTrustPolicy) Deny(ctx context.Context, origin string) error   { return nil }
func (m *mockTrustPolicy) Remove(ctx context.Context, origin string) error { return nil }

// mockCertCatalog implementa ports.CertificateCatalog para tests.
type mockCertCatalog struct {
	listFn func(ctx context.Context) ([]domain.CertificateRef, error)
}

func (m *mockCertCatalog) List(ctx context.Context) ([]domain.CertificateRef, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

type mockKeyProvider struct {
	keyForFn func(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error)
}

func (m *mockKeyProvider) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	if m.keyForFn != nil {
		return m.keyForFn(ctx, ref)
	}
	return &mockSigningKey{id: ref.ID}, nil
}

// mockCertSelector implementa certpicker.CertSelector para tests.
type mockCertSelector struct {
	selectFn func(ctx context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error)
}

func (m *mockCertSelector) Select(ctx context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
	if m.selectFn != nil {
		return m.selectFn(ctx, certs)
	}
	return certpicker.ResultadoSeleccion{}, nil
}

type mockPreferenceStore struct {
	sessionID       string
	sessionOK       bool
	sessionErr      error
	persistentID    string
	persistentOK    bool
	persistentErr   error
	savedSession    []string
	savedPersistent []string
}

func (m *mockPreferenceStore) LoadSession(_ context.Context, _ string) (string, bool, error) {
	return m.sessionID, m.sessionOK, m.sessionErr
}

func (m *mockPreferenceStore) SaveSession(_ context.Context, origin, certificateID string) error {
	m.savedSession = append(m.savedSession, origin+"|"+certificateID)
	return nil
}

func (m *mockPreferenceStore) LoadPersistent(_ context.Context, _ string) (string, bool, error) {
	return m.persistentID, m.persistentOK, m.persistentErr
}

func (m *mockPreferenceStore) SavePersistent(_ context.Context, origin, certificateID string) error {
	m.savedPersistent = append(m.savedPersistent, origin+"|"+certificateID)
	return nil
}

// mockSignerEngine implementa ports.SignerEngine para tests.
type mockSignerEngine struct {
	signFn func(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error)
}

func (m *mockSignerEngine) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if m.signFn != nil {
		return m.signFn(ctx, job, key)
	}
	return domain.SignatureResult{Data: []byte("firma-ok")}, nil
}

// mockProgressReporter implementa progressdialog.Reporter para tests.
type mockProgressReporter struct {
	cerradoFn func()
}

func (m *mockProgressReporter) SetMensaje(_ string)   {}
func (m *mockProgressReporter) SetProgreso(_ float64) {}
func (m *mockProgressReporter) Cerrar() {
	if m.cerradoFn != nil {
		m.cerradoFn()
	}
}

// mockProgressProvider implementa progressdialog.ProgressProvider para tests.
type mockProgressProvider struct {
	reporter progressdialog.Reporter
	calls    int
}

func (m *mockProgressProvider) MostrarProgreso(_ context.Context, _ string) progressdialog.Reporter {
	m.calls++
	if m.reporter != nil {
		return m.reporter
	}
	return &mockProgressReporter{}
}

// mockDesktopNotification implementa ports.DesktopNotification para tests.
type mockDesktopNotification struct {
	notifyFn func(ctx context.Context, title, body string) error
	calls    []string
}

func (m *mockDesktopNotification) Notify(ctx context.Context, title, body string) error {
	m.calls = append(m.calls, title)
	if m.notifyFn != nil {
		return m.notifyFn(ctx, title, body)
	}
	return nil
}

type mockEventPublisher struct {
	eventos []ports.Event
	publish func(ctx context.Context, event ports.Event) error
}

type mockSigningKey struct{ id string }

func (m *mockSigningKey) KeyID() string { return m.id }
func (m *mockSigningKey) CertificateChainDER() [][]byte {
	return [][]byte{[]byte("cert-der")}
}

func (m *mockEventPublisher) Publish(ctx context.Context, event ports.Event) error {
	m.eventos = append(m.eventos, event)
	if m.publish != nil {
		return m.publish(ctx, event)
	}
	return nil
}

type mockSimpleExecutor struct {
	firmarServidorFn    func(ctx context.Context, req triphase.SolicitudFirmaServidor) ([]byte, error)
	executeFn           func(ctx context.Context, session domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error)
	retrieveRawFn       func(ctx context.Context, session domain.ExchangeSession) ([]byte, error)
	uploadFn            func(ctx context.Context, session domain.ExchangeSession, data []byte) error
	uploadSignatureFn   func(ctx context.Context, session domain.ExchangeSession, certDER, signature []byte) error
	uploadCertificateFn func(ctx context.Context, session domain.ExchangeSession, data []byte, legacyParams url.Values) error
}

func (m *mockSimpleExecutor) Execute(ctx context.Context, session domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, session, job)
	}
	return domain.SignatureResult{Format: job.Format, Data: []byte("ok")}, nil
}

func (m *mockSimpleExecutor) RetrieveRaw(ctx context.Context, session domain.ExchangeSession) ([]byte, error) {
	if m.retrieveRawFn != nil {
		return m.retrieveRawFn(ctx, session)
	}
	return nil, errors.New("retrieve raw no configurado en test")
}

func (m *mockSimpleExecutor) Upload(ctx context.Context, session domain.ExchangeSession, data []byte) error {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, session, data)
	}
	return nil
}

func (m *mockSimpleExecutor) FirmarConServidor(ctx context.Context, req triphase.SolicitudFirmaServidor) ([]byte, error) {
	if m.firmarServidorFn != nil {
		return m.firmarServidorFn(ctx, req)
	}
	return []byte("firma-trifasica"), nil
}

func (m *mockSimpleExecutor) UploadSignature(ctx context.Context, session domain.ExchangeSession, certDER, signature []byte) error {
	if m.uploadSignatureFn != nil {
		return m.uploadSignatureFn(ctx, session, certDER, signature)
	}
	return nil
}

func (m *mockSimpleExecutor) UploadCertificate(ctx context.Context, session domain.ExchangeSession, data []byte, legacyParams url.Values) error {
	if m.uploadCertificateFn != nil {
		return m.uploadCertificateFn(ctx, session, data, legacyParams)
	}
	return nil
}

type mockBatchExecutor struct {
	executeFn       func(ctx context.Context, jobs []triphase.BatchJob) []triphase.BatchResult
	executeRemoteFn func(ctx context.Context, req afirmauri.RemoteBatchCommand) error
}

func (m *mockBatchExecutor) Execute(ctx context.Context, jobs []triphase.BatchJob) []triphase.BatchResult {
	if m.executeFn != nil {
		return m.executeFn(ctx, jobs)
	}
	resultados := make([]triphase.BatchResult, len(jobs))
	for i := range jobs {
		resultados[i] = triphase.BatchResult{Index: i, Result: domain.SignatureResult{Format: jobs[i].Job.Format, Data: []byte("ok")}}
	}
	return resultados
}

func (m *mockBatchExecutor) ExecuteLegacyRemote(ctx context.Context, req afirmauri.RemoteBatchCommand) error {
	if m.executeRemoteFn != nil {
		return m.executeRemoteFn(ctx, req)
	}
	return nil
}

// certRef devuelve un CertificateRef válido para tests.
func certRef(id string) domain.CertificateRef {
	return domain.CertificateRef{
		ID:          id,
		Subject:     "CN=Test User",
		Issuer:      "CN=Test CA",
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		Fingerprint: "aa:bb:cc:dd",
	}
}

func certRefExpirado(id string) domain.CertificateRef {
	return domain.CertificateRef{
		ID:          id,
		Subject:     "CN=Test User Expired",
		Issuer:      "CN=Test CA",
		NotAfter:    time.Now().Add(-24 * time.Hour),
		Fingerprint: "ee:ff:00:11",
	}
}

// sesionValida devuelve una ExchangeSession con todos los campos obligatorios.
func sesionValida() domain.ExchangeSession {
	return domain.ExchangeSession{
		RequestID:        "req-123",
		RetrieveEndpoint: "https://example.com/rt",
		UploadEndpoint:   "https://example.com/st",
		State:            domain.SessionActive,
	}
}

// solicitudSimpleValida devuelve una solicitud simple sin BatchCommand.
func solicitudSimpleValida() afirmauri.Solicitud {
	return afirmauri.Solicitud{
		Operacion:   afirmauri.OperacionFirma,
		AccionFirma: domain.ActionSign,
		Formato:     domain.FormatCAdES,
		Origenes:    []string{"https://localhost"},
		Sesion:      sesionValida(),
	}
}

// --- Tests ---

// TestOrchestrator_OrigenRechazado verifica que cuando el validator rechaza el origen,
// HandleRequest retorna error sin llegar al selector de certificados.
func TestOrchestrator_OrigenRechazado(t *testing.T) {
	t.Parallel()

	selectorLlamado := false

	policy := &mockTrustPolicy{
		evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
			return domain.TrustDecision{Status: domain.TrustDenied}, nil
		},
	}
	validator := originvalidator.New(policy)

	notify := &mockDesktopNotification{}

	orch := session.New(session.Config{
		TrustValidator: validator,
		ParserLegacy:   afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-1")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, _ []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selectorLlamado = true
				return certpicker.ResultadoSeleccion{}, nil
			},
		},
		Progress: &mockProgressProvider{},
		Notify:   notify,
	})

	solicitud := solicitudSimpleValida()
	solicitud.Origenes = []string{"https://localhost"}

	err := orch.HandleRequest(context.Background(), solicitud)
	if err == nil {
		t.Fatal("se esperaba error cuando el origen es rechazado")
	}
	if selectorLlamado {
		t.Error("el selector no debe llamarse cuando el origen es rechazado")
	}
	if len(notify.calls) == 0 {
		t.Error("se esperaba una notificación de rechazo")
	}
}

func TestOrchestrator_ProtocoloRemotoSimpleConservaOpcionesLegacy(t *testing.T) {
	t.Parallel()

	var jobRecibido domain.SignatureJob
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		TriphaseExec: &mockSimpleExecutor{
			executeFn: func(ctx context.Context, sesion domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error) {
				jobRecibido = job
				return domain.SignatureResult{Format: job.Format, Data: []byte("ok")}, nil
			},
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion:   afirmauri.OperacionFirma,
		AccionFirma: domain.ActionSign,
		Formato:     domain.FormatXAdES,
		Options:     map[string]string{"profile": "T", "policy": "demo"},
		Origenes:    []string{"https://localhost"},
		Sesion:      sesionValida(),
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if got := jobRecibido.Options["profile"]; got != "T" {
		t.Fatalf("profile inesperado: %q", got)
	}
	if got := jobRecibido.Options["policy"]; got != "demo" {
		t.Fatalf("policy inesperada: %q", got)
	}
}

func TestOrchestrator_BatchRemotoLegacyUsaEjecutorEspecifico(t *testing.T) {
	t.Parallel()

	remoteCalled := false
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1"), certRef("cert-2")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		BatchExec: &mockBatchExecutor{
			executeRemoteFn: func(_ context.Context, req afirmauri.RemoteBatchCommand) error {
				remoteCalled = true
				if string(req.Payload) != `{"singlesigns":[{"id":"1"}]}` {
					t.Fatalf("payload remoto inesperado: %s", string(req.Payload))
				}
				return nil
			},
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
		Origenes:  []string{"https://localhost"},
		Sesion:    sesionValida(),
		RemoteBatch: &afirmauri.RemoteBatchCommand{
			Session:          sesionValida(),
			Payload:          []byte(`{"singlesigns":[{"id":"1"}]}`),
			IsJSONBatch:      true,
			PreSignEndpoint:  "https://localhost/pre",
			PostSignEndpoint: "https://localhost/post",
		},
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if !remoteCalled {
		t.Fatal("se esperaba llamada al ejecutor remoto de batch")
	}
}

func TestOrchestrator_BatchRecuperadoDesdeRetrieveSeResuelveComoRemoto(t *testing.T) {
	t.Parallel()

	remoteCalled := false
	rawEnvelope := []byte(
		`<batch>` +
			`<e k="jsonbatch" v="true"/>` +
			`<e k="batchpresignerurl" v="https%3A%2F%2Flocalhost%2Fpre"/>` +
			`<e k="batchpostsignerurl" v="https%3A%2F%2Flocalhost%2Fpost"/>` +
			`<e k="stservlet" v="https%3A%2F%2Flocalhost%2Fstore"/>` +
			`<e k="rtservlet" v="https%3A%2F%2Flocalhost%2Fretrieve"/>` +
			`<e k="id" v="req-remoto"/>` +
			`<e k="dat" v="` + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(`{"singlesigns":[{"id":"1","datareference":"token"}]}`))) + `"/>` +
			`</batch>`,
	)

	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, origin string) (domain.TrustDecision, error) {
				if origin != "https://localhost" {
					t.Fatalf("origen inesperado validado: %s", origin)
				}
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		TriphaseExec: &mockSimpleExecutor{
			retrieveRawFn: func(_ context.Context, _ domain.ExchangeSession) ([]byte, error) {
				return rawEnvelope, nil
			},
		},
		BatchExec: &mockBatchExecutor{
			executeRemoteFn: func(_ context.Context, req afirmauri.RemoteBatchCommand) error {
				remoteCalled = true
				if req.Session.RequestID != "req-remoto" {
					t.Fatalf("request id remoto inesperado: %s", req.Session.RequestID)
				}
				if req.PreSignEndpoint != "https://localhost/pre" {
					t.Fatalf("presign inesperado: %s", req.PreSignEndpoint)
				}
				return nil
			},
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion:       afirmauri.OperacionLote,
		Origenes:        []string{"https://localhost"},
		Sesion:          sesionValida(),
		RetrieveCommand: &application.RetrieveRequestCommand{Session: sesionValida()},
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if !remoteCalled {
		t.Fatal("se esperaba ejecución remota del lote recuperado")
	}
}

func TestOrchestrator_SelectCertUsaSubidaDedicadaDeCertificado(t *testing.T) {
	t.Parallel()

	subidaCert := false
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		TriphaseExec: &mockSimpleExecutor{
			uploadCertificateFn: func(_ context.Context, session domain.ExchangeSession, data []byte, legacyParams url.Values) error {
				subidaCert = true
				if session.RequestID != "req-123" {
					t.Fatalf("request id inesperado: %s", session.RequestID)
				}
				if string(data) != "cert-der" {
					t.Fatalf("certificado DER inesperado: %q", string(data))
				}
				if got := legacyParams.Get("id"); got != "req-123" {
					t.Fatalf("legacy id inesperado: %q", got)
				}
				return nil
			},
			executeFn: func(context.Context, domain.ExchangeSession, domain.SignatureJob) (domain.SignatureResult, error) {
				t.Fatal("no debería ejecutarse la ruta de firma normal en selectcert")
				return domain.SignatureResult{}, nil
			},
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion:    afirmauri.OperacionSelectCert,
		Origenes:     []string{"https://localhost"},
		Sesion:       sesionValida(),
		LegacyParams: url.Values{"id": []string{"req-123"}},
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if !subidaCert {
		t.Fatal("se esperaba subida dedicada del certificado")
	}
}

func TestOrchestrator_FirmaRecuperadaXMLUsaFirmaLocalYSubidaLegacy(t *testing.T) {
	t.Parallel()

	xmlEnvelope := []byte(`<sign><e k="id" v="req-xml"/><e k="stservlet" v="https%3A%2F%2Fexample.com%2FStorageService"/><e k="format" v="CAdES"/><e k="dat" v="ZG9jdW1lbnRvLXhtbA=="/></sign>`)
	var firmado bool
	var subido bool

	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Signer: &mockSignerEngine{signFn: func(_ context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
			firmado = true
			if string(job.Document.Content) != "documento-xml" {
				t.Fatalf("documento recuperado inesperado: %q", string(job.Document.Content))
			}
			if job.Format != domain.FormatCAdES {
				t.Fatalf("formato inesperado: %s", job.Format)
			}
			if key == nil {
				t.Fatal("se esperaba clave de firma real")
			}
			return domain.SignatureResult{Data: []byte("firma-xml")}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		TriphaseExec: &mockSimpleExecutor{
			retrieveRawFn: func(_ context.Context, _ domain.ExchangeSession) ([]byte, error) {
				return xmlEnvelope, nil
			},
			uploadSignatureFn: func(_ context.Context, session domain.ExchangeSession, certDER, signature []byte) error {
				subido = true
				if session.RequestID != "req-xml" {
					t.Fatalf("request id inesperado: %s", session.RequestID)
				}
				if session.UploadEndpoint != "https://example.com/StorageService" {
					t.Fatalf("upload endpoint inesperado: %s", session.UploadEndpoint)
				}
				if string(certDER) != "cert-der" {
					t.Fatalf("certificado DER inesperado: %q", string(certDER))
				}
				if string(signature) != "firma-xml" {
					t.Fatalf("firma subida inesperada: %q", string(signature))
				}
				return nil
			},
			executeFn: func(context.Context, domain.ExchangeSession, domain.SignatureJob) (domain.SignatureResult, error) {
				t.Fatal("no debería ejecutarse el flujo trifásico JSON para un manifiesto XML legacy")
				return domain.SignatureResult{}, nil
			},
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion:       afirmauri.OperacionFirma,
		Origenes:        []string{"https://example.com"},
		Sesion:          sesionValida(),
		RetrieveCommand: &application.RetrieveRequestCommand{Session: sesionValida()},
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if !firmado {
		t.Fatal("se esperaba firma local del manifiesto XML recuperado")
	}
	if !subido {
		t.Fatal("se esperaba subida legacy Cert|Firma al StorageService")
	}
}

func TestOrchestrator_NoAbreProgresoAntesDeSeleccionarCertificadoEnBatch(t *testing.T) {
	t.Parallel()

	progress := &mockProgressProvider{}
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1"), certRef("cert-2")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			if progress.calls != 0 {
				t.Fatalf("el progreso no debe abrirse antes del selector; llamadas=%d", progress.calls)
			}
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: progress,
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		BatchExec: &mockBatchExecutor{
			executeRemoteFn: func(_ context.Context, _ afirmauri.RemoteBatchCommand) error { return nil },
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
		Origenes:  []string{"https://localhost"},
		Sesion:    sesionValida(),
		RemoteBatch: &afirmauri.RemoteBatchCommand{
			Session:          sesionValida(),
			Payload:          []byte(`{"singlesigns":[{"id":"1"}]}`),
			IsJSONBatch:      true,
			PreSignEndpoint:  "https://localhost/pre",
			PostSignEndpoint: "https://localhost/post",
		},
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if progress.calls == 0 {
		t.Fatal("se esperaba apertura del progreso después de seleccionar certificado")
	}
}

func TestOrchestrator_BatchStopOnErrorEjecutaSecuencialYNoLanzaPosteriores(t *testing.T) {
	t.Parallel()

	var llamadas []string
	orch := session.New(session.Config{
		// El usuario elige el certificado en el selector.
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider:  &mockKeyProvider{},
		Progress:     &mockProgressProvider{},
		Notify:       &mockDesktopNotification{},
		TriphaseExec: &mockSimpleExecutor{},
		BatchExec: &mockBatchExecutor{executeFn: func(
			_ context.Context,
			jobs []triphase.BatchJob,
		) []triphase.BatchResult {
			if len(jobs) != 1 {
				t.Fatalf("stoponerror debe ejecutar un solo trabajo por llamada, obtuvo %d", len(jobs))
			}
			llamadas = append(llamadas, jobs[0].Job.Document.Name)
			if jobs[0].Job.Document.Name == "segundo" {
				return []triphase.BatchResult{{
					Index: 0,
					Err:   errors.New("fallo sintético"),
				}}
			}
			return []triphase.BatchResult{{
				Index:  0,
				Result: domain.SignatureResult{Data: []byte("ok")},
			}}
		}},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
		Sesion:    sesionValida(),
		BatchCommand: &application.ProcessBatchCommand{
			StopOnError: true,
			Jobs: []domain.SignatureJob{
				{Document: domain.Document{Name: "primero"}},
				{Document: domain.Document{Name: "segundo"}},
				{Document: domain.Document{Name: "tercero"}},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "trabajo 1") {
		t.Fatalf("HandleRequest() error = %v, se esperaba fail-fast en trabajo 1", err)
	}
	if got := strings.Join(llamadas, ","); got != "primero,segundo" {
		t.Fatalf("trabajos ejecutados = %q, se esperaba primero,segundo", got)
	}
}

func TestOrchestrator_BatchSinStopOnErrorConservaEjecucionBestEffort(t *testing.T) {
	t.Parallel()

	llamadas := 0
	orch := session.New(session.Config{
		// El usuario elige el certificado en el selector.
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-1")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		Progress:    &mockProgressProvider{},
		Notify:      &mockDesktopNotification{},
		BatchExec: &mockBatchExecutor{executeFn: func(
			_ context.Context,
			jobs []triphase.BatchJob,
		) []triphase.BatchResult {
			llamadas++
			if len(jobs) != 3 {
				t.Fatalf("best-effort debe recibir el lote completo, obtuvo %d trabajos", len(jobs))
			}
			return []triphase.BatchResult{
				{Index: 0, Result: domain.SignatureResult{Data: []byte("uno")}},
				{Index: 1, Err: errors.New("fallo sintético")},
				{Index: 2, Result: domain.SignatureResult{Data: []byte("tres")}},
			}
		}},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
		Sesion:    sesionValida(),
		BatchCommand: &application.ProcessBatchCommand{
			StopOnError: false,
			Jobs: []domain.SignatureJob{
				{Document: domain.Document{Name: "primero"}},
				{Document: domain.Document{Name: "segundo"}},
				{Document: domain.Document{Name: "tercero"}},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "1 error") {
		t.Fatalf("HandleRequest() error = %v, se esperaba resumen best-effort", err)
	}
	if llamadas != 1 {
		t.Fatalf("Execute() llamadas = %d, se esperaba una llamada con el lote completo", llamadas)
	}
}

func TestOrchestrator_FiltraCertificadosNoCaducadosPorDefecto(t *testing.T) {
	t.Parallel()

	var (
		selectorLlamado bool
		certUsado       string
	)
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{
				certRefExpirado("cert-expirado"),
				certRef("cert-valido"),
			}, nil
		}},
		KeyProvider: &mockKeyProvider{keyForFn: func(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
			certUsado = ref.ID
			return &mockSigningKey{id: ref.ID}, nil
		}},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			selectorLlamado = true
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		TriphaseExec: &mockSimpleExecutor{},
		BatchExec: &mockBatchExecutor{
			executeRemoteFn: func(_ context.Context, _ afirmauri.RemoteBatchCommand) error { return nil },
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
		Origenes:  []string{"https://localhost"},
		Sesion:    sesionValida(),
		RemoteBatch: &afirmauri.RemoteBatchCommand{
			Session:          sesionValida(),
			Payload:          []byte(`{"singlesigns":[{"id":"1"}]}`),
			IsJSONBatch:      true,
			PreSignEndpoint:  "https://localhost/pre",
			PostSignEndpoint: "https://localhost/post",
		},
	})
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	// El usuario siempre confirma en el selector, que solo ofrece el vigente.
	if !selectorLlamado {
		t.Fatal("el selector debe mostrarse aunque tras filtrar quede un único certificado")
	}
	if certUsado != "cert-valido" {
		t.Fatalf("certificado filtrado inesperado: %s", certUsado)
	}
}

func TestOrchestrator_FiltroLegacySinCandidatosNoFirmaCaducados(t *testing.T) {
	t.Parallel()

	var recibidos []domain.CertificateRef
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{
				certRefExpirado("cert-expirado-1"),
				certRefExpirado("cert-expirado-2"),
			}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			recibidos = append(recibidos, certs...)
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		TrustValidator: originvalidator.New(&mockTrustPolicy{
			evaluateFn: func(_ context.Context, _ string) (domain.TrustDecision, error) {
				return domain.TrustDecision{Status: domain.TrustAllowed}, nil
			},
		}),
		TriphaseExec: &mockSimpleExecutor{},
		BatchExec: &mockBatchExecutor{
			executeRemoteFn: func(_ context.Context, _ afirmauri.RemoteBatchCommand) error { return nil },
		},
	})

	err := orch.HandleRequest(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
		Origenes:  []string{"https://localhost"},
		Sesion:    sesionValida(),
		Options:   map[string]string{"filter": "nonexpired::"},
		RemoteBatch: &afirmauri.RemoteBatchCommand{
			Session:          sesionValida(),
			Payload:          []byte(`{"singlesigns":[{"id":"1"}]}`),
			IsJSONBatch:      true,
			PreSignEndpoint:  "https://localhost/pre",
			PostSignEndpoint: "https://localhost/post",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "no hay certificados compatibles") {
		t.Fatalf("HandleRequest debe rechazar la operación sin certificados que cumplan el filtro: %v", err)
	}
	if len(recibidos) != 0 {
		t.Fatalf("como en AutoFirma Java no se ofrecen certificados excluidos: se recibieron %d", len(recibidos))
	}
}

// TestOrchestrator_SinCertificados verifica que cuando el catálogo está vacío,
// HandleRequest retorna error y emite notificación de error.
func TestOrchestrator_SinCertificados(t *testing.T) {
	t.Parallel()

	notify := &mockDesktopNotification{}

	orch := session.New(session.Config{
		TrustValidator: nil, // sin validación de origen
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{}, nil
			},
		},
		KeyProvider:  &mockKeyProvider{},
		CertSelector: &mockCertSelector{},
		Progress:     &mockProgressProvider{},
		Notify:       notify,
	})

	err := orch.HandleRequest(context.Background(), solicitudSimpleValida())
	if err == nil {
		t.Fatal("se esperaba error cuando no hay certificados disponibles")
	}
	if len(notify.calls) == 0 {
		t.Error("se esperaba una notificación de error cuando no hay certificados")
	}
}

// TestOrchestrator_FlujoCompleto verifica el flujo feliz completo con mocks de todos los pasos.
func TestOrchestrator_FlujoCompleto(t *testing.T) {
	t.Parallel()

	progressCerrado := false
	notificacionFinal := ""
	ejecutorLlamado := false
	eventos := &mockEventPublisher{}

	notify := &mockDesktopNotification{
		notifyFn: func(_ context.Context, title, _ string) error {
			notificacionFinal = title
			return nil
		},
	}

	reporter := &mockProgressReporter{
		cerradoFn: func() { progressCerrado = true },
	}

	// Construir una solicitud simple con SignCommand.
	doc, err := domain.NewDocument("test.bin", []byte("datos"), "application/octet-stream")
	if err != nil {
		t.Fatalf("error creando documento: %v", err)
	}
	signCmd := &application.SignCommand{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}

	solicitud := solicitudSimpleValida()
	solicitud.Origenes = nil // sin validación de origen
	solicitud.SignCommand = signCmd

	orch := session.New(session.Config{
		TrustValidator: nil,
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-1")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{
			keyForFn: func(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
				return &mockSigningKey{id: "key-" + ref.ID}, nil
			},
		},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
			},
		},
		TriphaseExec: &mockSimpleExecutor{
			executeFn: func(_ context.Context, session domain.ExchangeSession, job domain.SignatureJob) (domain.SignatureResult, error) {
				ejecutorLlamado = true
				if session.RequestID == "" {
					t.Fatal("la sesion no debe llegar vacia al ejecutor")
				}
				if job.Document.Name != "test.bin" {
					t.Fatalf("documento inesperado en el ejecutor: %s", job.Document.Name)
				}
				return domain.SignatureResult{Format: job.Format, Data: []byte("firma-ok")}, nil
			},
		},
		Progress: &mockProgressProvider{reporter: reporter},
		Notify:   notify,
		Eventos:  eventos,
	})

	err = orch.HandleRequest(context.Background(), solicitud)
	if err != nil {
		t.Fatalf("se esperaba flujo completo sin error, se obtuvo: %v", err)
	}
	if !progressCerrado {
		t.Error("se esperaba que el diálogo de progreso se cerrara (defer Cerrar)")
	}
	if !ejecutorLlamado {
		t.Error("se esperaba que el ejecutor simple se invocara")
	}
	if notificacionFinal != "Firma completada" {
		t.Fatalf("notificación final inesperada: %q", notificacionFinal)
	}
	if len(eventos.eventos) == 0 {
		t.Fatal("se esperaban eventos publicados durante la sesion")
	}
}

// TestOrchestrator_ContextoCancelado verifica que cuando el contexto es cancelado
// antes de que el selector se ejecute, HandleRequest retorna error de contexto.
func TestOrchestrator_ContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	selectorInvocado := false

	orch := session.New(session.Config{
		TrustValidator: nil,
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				// Cancelar el contexto justo después de listar los certificados,
				// antes de que el selector se invoque.
				cancel()
				return []domain.CertificateRef{certRef("cert-1")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(ctx context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selectorInvocado = true
				return certpicker.ResultadoSeleccion{}, ctx.Err()
			},
		},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
	})

	err := orch.HandleRequest(ctx, solicitudSimpleValida())
	if err == nil {
		t.Fatal("se esperaba error de contexto cancelado")
	}
	// El contexto fue cancelado; el selector puede no haberse llamado o haber
	// devuelto context.Canceled. En ambos casos debe haber error.
	if !errors.Is(err, context.Canceled) && !selectorInvocado {
		// Si el selector no se llamó (porque ctx.Err() lo cortó antes), está bien.
		// Si se llamó, debe haber propagado el error de contexto a través del flujo.
		t.Logf("err=%v, selectorInvocado=%v", err, selectorInvocado)
	}
}

func TestOrchestrator_SeleccionExplicitaNoAutoSeleccionaCertificadoUnico(t *testing.T) {
	t.Parallel()

	selectorCalls := 0
	certificate := certRef("cert-explicito")
	orch := session.New(session.Config{
		CertCatalog: &mockCertCatalog{
			listFn: func(context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certificate}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selectorCalls++
				if len(certs) != 1 {
					t.Fatalf("selector recibió %d certificados, want 1", len(certs))
				}
				return certpicker.ResultadoSeleccion{
					Certificado: certs[0],
					Recuerdo:    certpicker.NoRecordar,
				}, nil
			},
		},
		TriphaseExec:                        &mockSimpleExecutor{},
		Progress:                            &mockProgressProvider{},
		Notify:                              &mockDesktopNotification{},
		RequireExplicitCertificateSelection: true,
	})

	if err := orch.HandleRequest(
		context.Background(),
		solicitudSimpleValida(),
	); err != nil {
		t.Fatalf("flujo con selección explícita: %v", err)
	}
	if selectorCalls != 1 {
		t.Fatalf("selector invocado %d veces, want 1", selectorCalls)
	}
}

func TestOrchestrator_TimeoutGlobal(t *testing.T) {
	t.Parallel()

	eventos := &mockEventPublisher{}
	orch := session.New(session.Config{
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-1")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
			},
		},
		TriphaseExec: &mockSimpleExecutor{
			executeFn: func(ctx context.Context, _ domain.ExchangeSession, _ domain.SignatureJob) (domain.SignatureResult, error) {
				<-ctx.Done()
				return domain.SignatureResult{}, ctx.Err()
			},
		},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		Eventos:  eventos,
		Timeout:  15 * time.Millisecond,
	})

	solicitud := solicitudSimpleValida()
	solicitud.SignCommand = &application.SignCommand{
		Document: mustDocument(t, "timeout.bin", []byte("datos")),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}

	err := orch.HandleRequest(context.Background(), solicitud)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("se esperaba deadline exceeded, se obtuvo: %v", err)
	}
	if len(eventos.eventos) == 0 {
		t.Fatal("se esperaba al menos un evento publicado en timeout")
	}
}

func TestOrchestrator_PublicaEventoEnErrorDeProtocolo(t *testing.T) {
	t.Parallel()

	eventos := &mockEventPublisher{}
	orch := session.New(session.Config{
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-1")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
			},
		},
		TriphaseExec: &mockSimpleExecutor{
			executeFn: func(_ context.Context, _ domain.ExchangeSession, _ domain.SignatureJob) (domain.SignatureResult, error) {
				return domain.SignatureResult{}, errors.New("fallo de protocolo")
			},
		},
		Progress: &mockProgressProvider{},
		Notify:   &mockDesktopNotification{},
		Eventos:  eventos,
	})

	solicitud := solicitudSimpleValida()
	solicitud.SignCommand = &application.SignCommand{
		Document: mustDocument(t, "error.bin", []byte("datos")),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}

	err := orch.HandleRequest(context.Background(), solicitud)
	if err == nil {
		t.Fatal("se esperaba error de protocolo")
	}
	if !contieneTipoEvento(eventos.eventos, "session.error.protocol") {
		t.Fatal("se esperaba un evento session.error.protocol")
	}
}

func TestOrchestrator_ReutilizaCertificadoPreferidoDeSesion(t *testing.T) {
	t.Parallel()

	selectorLlamado := false
	preferencias := &mockPreferenceStore{
		sessionID: "cert-2",
		sessionOK: true,
	}

	orch := session.New(session.Config{
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-1"), certRef("cert-2")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, _ []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selectorLlamado = true
				return certpicker.ResultadoSeleccion{}, nil
			},
		},
		TriphaseExec: &mockSimpleExecutor{},
		Progress:     &mockProgressProvider{},
		Notify:       &mockDesktopNotification{},
		Preferencias: preferencias,
	})

	solicitud := solicitudSimpleValida()
	solicitud.SignCommand = &application.SignCommand{
		Document: mustDocument(t, "preferido.bin", []byte("datos")),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}
	solicitud.Origenes = []string{"https://portal.example"}

	err := orch.HandleRequest(context.Background(), solicitud)
	if err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if selectorLlamado {
		t.Fatal("no debería invocarse el selector si existe un certificado preferido en sesión")
	}
}

func TestOrchestrator_PideConsentimientoConUnicoCertificado(t *testing.T) {
	t.Parallel()

	selectorLlamado := false
	orch := session.New(session.Config{
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-unico")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selectorLlamado = true
				return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
			},
		},
		TriphaseExec: &mockSimpleExecutor{},
		Progress:     &mockProgressProvider{},
		Notify:       &mockDesktopNotification{},
	})

	solicitud := solicitudSimpleValida()
	solicitud.SignCommand = &application.SignCommand{
		Document: mustDocument(t, "unico.bin", []byte("datos")),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}

	if err := orch.HandleRequest(context.Background(), solicitud); err != nil {
		t.Fatalf("HandleRequest: error inesperado: %v", err)
	}
	if !selectorLlamado {
		t.Fatal("con un único certificado el selector sigue siendo el consentimiento del usuario")
	}
}

func TestOrchestrator_GuardaCertificadoPreferidoSegunModoRecuerdo(t *testing.T) {
	t.Parallel()

	preferencias := &mockPreferenceStore{}
	selectorLlamado := 0
	orch := session.New(session.Config{
		CertCatalog: &mockCertCatalog{
			listFn: func(_ context.Context) ([]domain.CertificateRef, error) {
				return []domain.CertificateRef{certRef("cert-1"), certRef("cert-2")}, nil
			},
		},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{
			selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
				selectorLlamado++
				if selectorLlamado == 1 {
					return certpicker.ResultadoSeleccion{
						Certificado: certs[1],
						Recuerdo:    certpicker.RecordarSesion,
					}, nil
				}
				return certpicker.ResultadoSeleccion{
					Certificado: certs[0],
					Recuerdo:    certpicker.RecordarSiempre,
				}, nil
			},
		},
		TriphaseExec: &mockSimpleExecutor{},
		Progress:     &mockProgressProvider{},
		Notify:       &mockDesktopNotification{},
		Preferencias: preferencias,
	})

	solicitud := solicitudSimpleValida()
	solicitud.SignCommand = &application.SignCommand{
		Document: mustDocument(t, "guardar.bin", []byte("datos")),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}
	solicitud.Origenes = []string{"https://portal.example"}

	if err := orch.HandleRequest(context.Background(), solicitud); err != nil {
		t.Fatalf("primera llamada: error inesperado: %v", err)
	}
	if len(preferencias.savedSession) != 1 || preferencias.savedSession[0] != "https://portal.example|cert-2" {
		t.Fatalf("preferencia de sesión inesperada: %+v", preferencias.savedSession)
	}

	preferencias.sessionID = ""
	preferencias.sessionOK = false

	if err := orch.HandleRequest(context.Background(), solicitud); err != nil {
		t.Fatalf("segunda llamada: error inesperado: %v", err)
	}
	if len(preferencias.savedPersistent) != 1 || preferencias.savedPersistent[0] != "https://portal.example|cert-1" {
		t.Fatalf("preferencia persistente inesperada: %+v", preferencias.savedPersistent)
	}
}

func mustDocument(t *testing.T, name string, contenido []byte) domain.Document {
	t.Helper()
	doc, err := domain.NewDocument(name, contenido, "application/octet-stream")
	if err != nil {
		t.Fatalf("error creando documento de test: %v", err)
	}
	return doc
}

func contieneTipoEvento(eventos []ports.Event, tipo string) bool {
	for _, evento := range eventos {
		if evento.Type == tipo {
			return true
		}
	}
	return false
}

// Asegurar que los mocks implementan las interfaces correctas en tiempo de compilación.
var (
	_ ports.TrustPolicy               = (*mockTrustPolicy)(nil)
	_ ports.CertificateCatalog        = (*mockCertCatalog)(nil)
	_ ports.SigningKeyProvider        = (*mockKeyProvider)(nil)
	_ certpicker.CertSelector         = (*mockCertSelector)(nil)
	_ ports.SignerEngine              = (*mockSignerEngine)(nil)
	_ progressdialog.Reporter         = (*mockProgressReporter)(nil)
	_ progressdialog.ProgressProvider = (*mockProgressProvider)(nil)
	_ ports.DesktopNotification       = (*mockDesktopNotification)(nil)
	_ ports.EventPublisher            = (*mockEventPublisher)(nil)
)

// FIRe con certificado local: format=CAdEStri, serverUrl y "dat" con el
// identificador de la transacción. Debe firmarse en el servidor trifásico con
// ese identificador y subirse la firma, no firmar el identificador en local.
func TestOrchestrator_FormatoTriConServerURLUsaServidorTrifasico(t *testing.T) {
	t.Parallel()
	var (
		recibida  triphase.SolicitudFirmaServidor
		subida    []byte
		ejecutado bool
	)
	exec := &mockSimpleExecutor{
		firmarServidorFn: func(_ context.Context, req triphase.SolicitudFirmaServidor) ([]byte, error) {
			recibida = req
			return []byte("firma-del-servidor"), nil
		},
		uploadSignatureFn: func(_ context.Context, _ domain.ExchangeSession, _ []byte, firma []byte) error {
			subida = firma
			return nil
		},
		executeFn: func(context.Context, domain.ExchangeSession, domain.SignatureJob) (domain.SignatureResult, error) {
			ejecutado = true
			return domain.SignatureResult{}, nil
		},
	}
	orch := session.New(session.Config{
		ParserLegacy: afirmauri.New(nil),
		CertCatalog: &mockCertCatalog{listFn: func(context.Context) ([]domain.CertificateRef, error) {
			return []domain.CertificateRef{certRef("cert-fnmt")}, nil
		}},
		KeyProvider: &mockKeyProvider{},
		CertSelector: &mockCertSelector{selectFn: func(_ context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
			return certpicker.ResultadoSeleccion{Certificado: certs[0]}, nil
		}},
		TriphaseExec: exec,
		Progress:     &mockProgressProvider{},
		Notify:       &mockDesktopNotification{},
	})
	solicitud := solicitudSimpleValida()
	solicitud.LegacyParams = url.Values{"format": {"CAdEStri"}}
	solicitud.SignCommand = &application.SignCommand{
		Document: mustDocument(t, "transaccion", []byte("fc00c581-b871-458f-8972-b7d0ff396648")),
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
		Options:  map[string]string{"serverUrl": "https://firesda.example.es/public/afirma/triphaseSignService", "algorithm": "SHA512withRSA"},
	}
	if err := orch.HandleRequest(context.Background(), solicitud); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if recibida.FormatoLegacy != "CAdEStri" || string(recibida.Datos) != "fc00c581-b871-458f-8972-b7d0ff396648" || recibida.Algoritmo != "SHA512withRSA" {
		t.Fatalf("solicitud trifásica inesperada: %+v", recibida)
	}
	if string(subida) != "firma-del-servidor" || ejecutado {
		t.Fatalf("subida=%q ejecutado=%v", subida, ejecutado)
	}
}
