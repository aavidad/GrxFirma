// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Tests de caja blanca para Manejador. Se usa package ipc (no ipc_test)
// para poder llamar a los metodos no exportados despachar y despacharConTimeout.
package ipc

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/updatecheck"
	"grxfirma/internal/adapters/outbound/desktop/clockdiagnostic"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ---------------------------------------------------------------------------
// Dobles de prueba
// ---------------------------------------------------------------------------

type stubCatalogo struct {
	certs []domain.CertificateRef
	err   error
}

type stubUpdateChecker struct {
	result  updatecheck.Resultado
	err     error
	version string
	calls   int
}

func (s *stubUpdateChecker) Comprobar(
	_ context.Context,
	versionActual string,
) (updatecheck.Resultado, error) {
	s.calls++
	s.version = versionActual
	return s.result, s.err
}

type formattingLocalizer struct{}

func (formattingLocalizer) T(id string, args ...any) string {
	return fmt.Sprintf(id, args...)
}

func (s *stubCatalogo) List(_ context.Context) ([]domain.CertificateRef, error) {
	return s.certs, s.err
}

type stubImportador struct {
	data             []byte
	received         []byte
	password         string
	receivedPassword string
	err              error
}

func (s *stubImportador) Import(_ context.Context, data []byte, password string) (domain.CertificateRef, error) {
	s.data = data
	s.received = append([]byte(nil), data...)
	s.password = password
	s.receivedPassword = strings.Clone(password)
	return domain.CertificateRef{}, s.err
}

type stubFirmar struct {
	result application.SignResult
	err    error
	cmd    application.SignCommand
}

func (s *stubFirmar) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubMultiCofirmar struct {
	result application.SignResult
	err    error
	cmd    application.MultiCoSignCommand
	cmds   []application.MultiCoSignCommand
}

func (s *stubMultiCofirmar) Execute(_ context.Context, cmd application.MultiCoSignCommand) (application.SignResult, error) {
	s.cmd = cmd
	s.cmds = append(s.cmds, cmd)
	return s.result, s.err
}

type stubVerificar struct {
	result application.VerifyResult
	err    error
	cmd    application.VerifyCommand
}

type stubDestinatariosProteccion struct {
	recipients []domain.ProtectionRecipient
}

func (s stubDestinatariosProteccion) List(context.Context) ([]domain.ProtectionRecipient, error) {
	return append([]domain.ProtectionRecipient(nil), s.recipients...), nil
}

func (s stubDestinatariosProteccion) Resolve(context.Context, []string) ([]domain.ProtectionRecipient, error) {
	return append([]domain.ProtectionRecipient(nil), s.recipients...), nil
}

type stubProteger struct {
	result                    application.ProtectResult
	err                       error
	cmd                       application.ProtectCommand
	symmetricKeyDuringExecute []byte
}

func (s *stubProteger) Execute(_ context.Context, cmd application.ProtectCommand) (application.ProtectResult, error) {
	s.cmd = cmd
	s.symmetricKeyDuringExecute = append(
		[]byte(nil),
		cmd.SymmetricKey...,
	)
	return s.result, s.err
}

type stubDesproteger struct {
	result                    application.UnprotectResult
	err                       error
	cmd                       application.UnprotectCommand
	symmetricKeyDuringExecute []byte
}

func (s *stubDesproteger) Execute(_ context.Context, cmd application.UnprotectCommand) (application.UnprotectResult, error) {
	s.cmd = cmd
	s.symmetricKeyDuringExecute = append(
		[]byte(nil),
		cmd.SymmetricKey...,
	)
	return s.result, s.err
}

func (s *stubVerificar) Execute(_ context.Context, cmd application.VerifyCommand) (application.VerifyResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubCrearHash struct {
	result application.CreateHashResult
	err    error
	cmd    application.CreateHashCommand
}

func (s *stubCrearHash) Execute(_ context.Context, cmd application.CreateHashCommand) (application.CreateHashResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubComprobarHash struct {
	result application.CheckHashResult
	err    error
	cmd    application.CheckHashCommand
}

func (s *stubComprobarHash) Execute(_ context.Context, cmd application.CheckHashCommand) (application.CheckHashResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubCrearHashDir struct {
	result application.CreateDirectoryHashManifestResult
	err    error
	cmd    application.CreateDirectoryHashManifestCommand
}

func (s *stubCrearHashDir) Execute(_ context.Context, cmd application.CreateDirectoryHashManifestCommand) (application.CreateDirectoryHashManifestResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubComprobarHashDir struct {
	result application.CheckDirectoryHashManifestResult
	err    error
	cmd    application.CheckDirectoryHashManifestCommand
}

func (s *stubComprobarHashDir) Execute(_ context.Context, cmd application.CheckDirectoryHashManifestCommand) (application.CheckDirectoryHashManifestResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubHashReportCodec struct {
	data []byte
	err  error
}

func (s *stubHashReportCodec) EncodeReport(context.Context, domain.DirectoryHashCheckReport) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte(nil), s.data...), nil
}

type stubProcesarLote struct {
	result application.BatchResult
	err    error
	cmd    application.ProcessBatchCommand
}

func (s *stubProcesarLote) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type stubServicio struct {
	estado ports.EstadoServicio
	err    error
}

func (s *stubServicio) Estado(_ context.Context) (ports.EstadoServicio, error) {
	return s.estado, s.err
}
func (s *stubServicio) Instalar(_ context.Context, _ string) error { return s.err }
func (s *stubServicio) Desinstalar(_ context.Context) error        { return s.err }
func (s *stubServicio) Iniciar(_ context.Context) error            { return s.err }
func (s *stubServicio) Detener(_ context.Context) error            { return s.err }

type stubSettings struct {
	datos map[string]any
	err   error
}

func (s *stubSettings) Cargar(_ context.Context) (map[string]any, error) {
	if s.datos == nil {
		return map[string]any{}, s.err
	}
	return s.datos, s.err
}
func (s *stubSettings) Guardar(_ context.Context, _ map[string]any) error { return s.err }

type stubTypedSettings struct {
	doc          ports.DocumentoConfiguracionUsuario
	err          error
	usedLoadDoc  bool
	usedSaveDoc  bool
	lastSavedDoc ports.DocumentoConfiguracionUsuario
}

func (s *stubTypedSettings) Cargar(_ context.Context) (map[string]any, error) {
	return s.doc.Mapa(), s.err
}

func (s *stubTypedSettings) Guardar(_ context.Context, _ map[string]any) error { return s.err }

func (s *stubTypedSettings) CargarDocumento(_ context.Context) (ports.DocumentoConfiguracionUsuario, error) {
	s.usedLoadDoc = true
	return s.doc, s.err
}

func (s *stubTypedSettings) GuardarDocumento(_ context.Context, doc ports.DocumentoConfiguracionUsuario) error {
	s.usedSaveDoc = true
	s.lastSavedDoc = doc
	return s.err
}

type stubProxySecretStore struct {
	status      ports.ProxySecretStoreStatus
	err         error
	descriptor  ports.ProxySecretDescriptor
	storeErr    error
	deleteErr   error
	storedRealm string
	stored      ports.ProxySecretMaterial
	deletedIDs  []string
	events      *[]string
}

func (s *stubProxySecretStore) Store(_ context.Context, realm string, material ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	s.storedRealm = realm
	s.stored = ports.ProxySecretMaterial{
		Realm:    material.Realm,
		Username: material.Username,
		Password: append([]byte(nil), material.Password...),
	}
	if s.events != nil {
		*s.events = append(*s.events, "store")
	}
	return s.descriptor, s.storeErr
}

func (s *stubProxySecretStore) Load(context.Context, string) (ports.ProxySecretMaterial, error) {
	return ports.ProxySecretMaterial{}, errors.New("not implemented")
}

func (s *stubProxySecretStore) Delete(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	if s.events != nil {
		*s.events = append(*s.events, "delete:"+id)
	}
	return s.deleteErr
}

func (s *stubProxySecretStore) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	return s.status, s.err
}

type transactionalSettingsStub struct {
	doc       ports.DocumentoConfiguracionUsuario
	loadErr   error
	saveErr   error
	saveCalls int
	events    *[]string
}

func (s *transactionalSettingsStub) Cargar(context.Context) (map[string]any, error) {
	return s.doc.Mapa(), s.loadErr
}

func (s *transactionalSettingsStub) Guardar(_ context.Context, data map[string]any) error {
	return s.GuardarDocumento(context.Background(), ports.DocumentoConfiguracionUsuarioDesdeMapa(data))
}

func (s *transactionalSettingsStub) CargarDocumento(context.Context) (ports.DocumentoConfiguracionUsuario, error) {
	return s.doc, s.loadErr
}

func (s *transactionalSettingsStub) GuardarDocumento(_ context.Context, doc ports.DocumentoConfiguracionUsuario) error {
	s.saveCalls++
	if s.events != nil {
		*s.events = append(*s.events, "save")
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	s.doc = doc
	return nil
}

type stubSigningKey struct {
	id    string
	chain [][]byte
}

func (s stubSigningKey) KeyID() string { return s.id }

func (s stubSigningKey) CertificateChainDER() [][]byte {
	return append([][]byte(nil), s.chain...)
}

type stubClosingSigningKey struct {
	stubSigningKey
	closed int
}

func (s *stubClosingSigningKey) Close() {
	s.closed++
}

type stubClaves struct {
	key ports.SigningKey
	err error
}

func (s *stubClaves) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.key, nil
}

type diagnosticKeyProviderStub struct {
	keys      map[string]ports.SigningKey
	errors    map[string]error
	requested []string
}

type clockDiagnosticProviderStub struct {
	report clockdiagnostic.Report
	calls  int
}

func (s *clockDiagnosticProviderStub) Diagnose(
	context.Context,
) clockdiagnostic.Report {
	s.calls++
	return s.report
}

func (s *diagnosticKeyProviderStub) KeyFor(
	_ context.Context,
	certificate domain.CertificateRef,
) (ports.SigningKey, error) {
	s.requested = append(s.requested, certificate.ID)
	if err := s.errors[certificate.ID]; err != nil {
		return nil, err
	}
	return s.keys[certificate.ID], nil
}

// ---------------------------------------------------------------------------
// Helper: manejador vacio
// ---------------------------------------------------------------------------

func manejadorVacio() *Manejador { return &Manejador{} }

func peticionJSON(t *testing.T, accion string, params any) peticion {
	t.Helper()
	raw, _ := json.Marshal(params)
	return peticion{Action: accion, Params: json.RawMessage(raw)}
}

// ---------------------------------------------------------------------------
// Ping
// ---------------------------------------------------------------------------

func TestDespachar_Ping(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticion{Action: "ping"})
	if !resp.OK {
		t.Errorf("ping: esperaba OK=true, got %v (error: %s)", resp.OK, resp.Error)
	}
}

func TestDespachar_CheckUpdatesDevuelveResultadoInformativo(t *testing.T) {
	t.Parallel()
	checker := &stubUpdateChecker{result: updatecheck.Resultado{
		VersionActual: "0.1.0",
		UltimaVersion: "v0.2.0",
		HayNueva:      true,
		Comparable:    true,
		URL:           updatecheck.OfficialRepositoryURL + "/releases/tag/v0.2.0",
	}}
	m := &Manejador{
		UpdateChecker:  checker,
		CurrentVersion: " 0.1.0 ",
	}
	resp := m.despachar(context.Background(), peticion{Action: "check_updates"})
	if !resp.OK {
		t.Fatalf("check_updates: %s", resp.Error)
	}
	result, ok := resp.Data.(updatecheck.Resultado)
	if !ok || !result.HayNueva || !result.Comparable {
		t.Fatalf("resultado inesperado: %#v", resp.Data)
	}
	if checker.calls != 1 || checker.version != "0.1.0" {
		t.Fatalf("checker calls=%d version=%q", checker.calls, checker.version)
	}
}

func TestDespachar_CheckUpdatesExplicaFalloYNoBloqueaFirma(t *testing.T) {
	t.Parallel()
	checker := &stubUpdateChecker{err: errors.New(
		`proxy https://usuario:secreto@proxy.example rechazó la conexión`,
	)}
	m := &Manejador{
		UpdateChecker:  checker,
		CurrentVersion: "0.1.0",
		Loc:            formattingLocalizer{},
	}
	resp := m.despachar(context.Background(), peticion{Action: "check_updates"})
	if resp.OK {
		t.Fatal("check_updates debía fallar")
	}
	for _, fragment := range []string{
		"No se pudo conectar mediante el proxy",
		"Revise su configuración",
	} {
		if !strings.Contains(resp.Error, fragment) {
			t.Fatalf("el error no contiene %q: %s", fragment, resp.Error)
		}
	}
	if resp.ErrorCode != "update_proxy_unavailable" || resp.Diagnostic == nil || resp.Diagnostic.UserMessage != resp.Error {
		t.Fatalf("código o diagnóstico IPC incorrecto: %+v", resp)
	}
	for _, sensitive := range []string{"usuario", "secreto", "proxy.example"} {
		if strings.Contains(resp.Error, sensitive) {
			t.Fatalf("el error de red filtró %q: %s", sensitive, resp.Error)
		}
	}
}

func TestDespachar_CheckUpdatesSinPublicacionesNoEsError(t *testing.T) {
	checker := &stubUpdateChecker{result: updatecheck.Resultado{
		VersionActual: "0.0.100", Estado: updatecheck.EstadoSinPublicaciones,
	}}
	m := &Manejador{UpdateChecker: checker, CurrentVersion: "0.0.100", Loc: formattingLocalizer{}}
	resp := m.despachar(context.Background(), peticion{Action: "check_updates"})
	result, ok := resp.Data.(updatecheck.Resultado)
	if !resp.OK || !ok || result.HayNueva || result.Estado != updatecheck.EstadoSinPublicaciones ||
		result.Mensaje != "Todavía no hay versiones publicadas en el canal oficial." {
		t.Fatalf("sin publicaciones = %+v", resp)
	}
}

func TestDespachar_ClockDiagnostics_UsaProveedorInternoSinParametros(
	t *testing.T,
) {
	t.Parallel()
	provider := &clockDiagnosticProviderStub{
		report: clockdiagnostic.Report{
			ThresholdSeconds: 5,
			Steps: []clockdiagnostic.Step{
				{
					Code:   "local_clock",
					Status: "unknown",
				},
				{
					Code:   "remote_clock",
					Status: "unknown",
				},
				{
					Code:   "government_afirma",
					Status: "unknown",
				},
			},
		},
	}
	m := &Manejador{ClockDiagnostics: provider}

	resp := m.despachar(
		context.Background(),
		peticion{
			Action: "clock_diagnostics",
			Params: json.RawMessage(
				`{"url":"https://127.0.0.1:8443/secreto"}`,
			),
		},
	)

	if !resp.OK || resp.Action != "clock_diagnostics" {
		t.Fatalf("respuesta inesperada: %#v", resp)
	}
	report, ok := resp.Data.(clockdiagnostic.Report)
	if !ok || report.ThresholdSeconds != 5 || len(report.Steps) != 3 {
		t.Fatalf("informe inesperado: %#v", resp.Data)
	}
	if provider.calls != 1 {
		t.Fatalf("Diagnose calls=%d, want 1", provider.calls)
	}
	serialized, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal(resp): %v", err)
	}
	if strings.Contains(string(serialized), "127.0.0.1") ||
		strings.Contains(string(serialized), "secreto") {
		t.Fatalf("la acción reflejó parámetros arbitrarios: %s", serialized)
	}
}

// ---------------------------------------------------------------------------
// Accion desconocida
// ---------------------------------------------------------------------------

func TestDespachar_AccionDesconocida(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticion{Action: "no_existe"})
	if resp.OK {
		t.Error("accion desconocida: esperaba OK=false")
	}
	if resp.Diagnostic == nil || resp.Diagnostic.Category != string(application.GuidedDiagnosticAppLocal) {
		t.Fatalf("diagnostic=%#v, want app_local", resp.Diagnostic)
	}
}

func TestProtectionRecipients_ExponeCompatibilidadAuthEnvelopedData(t *testing.T) {
	recipient := generarDestinatarioAuthEnvelopedIPC(t, "dest-auth")
	recipientWithoutID := recipient
	recipientWithoutID.ID = ""
	handler := &Manejador{
		Destinatarios: stubDestinatariosProteccion{recipients: []domain.ProtectionRecipient{
			recipient,
			recipientWithoutID,
			{ID: "dest-sin-certificado", RSAOAEP256PublicKeyDER: []byte{1}},
		}},
	}
	response := handler.handleProtectionRecipients(context.Background())
	if !response.OK {
		t.Fatalf("handleProtectionRecipients() error = %s", response.Error)
	}
	payload, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(payload), `"authEnvelopedDataCompatible":true`) ||
		!strings.Contains(string(payload), `"authEnvelopedDataCompatible":false`) {
		t.Fatalf("capacidades AuthEnvelopedData inesperadas: %s", payload)
	}
	var decoded struct {
		Recipients []destinatarioProteccionJSON `json:"recipients"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(decoded.Recipients) != 3 || decoded.Recipients[1].AuthEnvelopedDataCompatible {
		t.Fatalf("un destinatario sin ID no debe anunciar compatibilidad: %#v", decoded.Recipients)
	}
}

func TestProtectEncryptedDataIPC_PropagaSecretoTransitorioSinDestinatarios(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "secreto.txt")
	if err := os.WriteFile(input, []byte("contenido"), 0o600); err != nil {
		t.Fatal(err)
	}
	protected, err := domain.NewDocument(
		"secreto.txt.encrypted.p7m", []byte("cms"),
		domain.MIMETypeProtectedCMS,
	)
	if err != nil {
		t.Fatal(err)
	}
	useCase := &stubProteger{result: application.ProtectResult{
		Protected: domain.ProtectedPayload{
			Document: protected,
			Profile:  domain.ProtectionProfileCompat,
		},
	}}
	secret := []byte("0123456789abcdef0123456789abcdef")
	handler := &Manejador{Proteger: useCase}
	response := handler.handleProtect(context.Background(), peticionJSON(t, "protect", paramsProtection{
		InputPath:  input,
		Profile:    "compat",
		SaveToDisk: false,
		Options: map[string]string{
			"container": "cms-encrypted",
		},
		SecretB64: &secret,
	}).Params)
	if !response.OK {
		t.Fatalf("handleProtect() error = %s", response.Error)
	}
	if len(useCase.cmd.RecipientIDs) != 0 {
		t.Fatalf("RecipientIDs = %#v; want ninguno", useCase.cmd.RecipientIDs)
	}
	if useCase.cmd.Options["container"] != "cms-encrypted" {
		t.Fatalf("Options = %#v", useCase.cmd.Options)
	}
	if !bytes.Equal(useCase.symmetricKeyDuringExecute, secret) {
		t.Fatal("la clave binaria no llegó al caso de uso")
	}
	if !bytes.Equal(
		useCase.cmd.SymmetricKey,
		make([]byte, len(secret)),
	) {
		t.Fatal("el adaptador no borró la clave binaria tras Execute")
	}
}

func TestUnprotectEncryptedDataIPC_PropagaSecretoTransitorio(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "secreto.txt.encrypted.p7m")
	if err := os.WriteFile(input, []byte("cms"), 0o600); err != nil {
		t.Fatal(err)
	}
	unprotected, err := domain.NewDocument(
		"secreto.txt", []byte("contenido"), "text/plain",
	)
	if err != nil {
		t.Fatal(err)
	}
	useCase := &stubDesproteger{result: application.UnprotectResult{
		Unprotected: domain.UnprotectedPayload{
			Document: unprotected,
			Profile:  domain.ProtectionProfileCompat,
		},
	}}
	secret := []byte("0123456789abcdef0123456789abcdef")
	handler := &Manejador{Desproteger: useCase}
	response := handler.handleUnprotect(context.Background(), peticionJSON(t, "unprotect", paramsProtection{
		InputPath:  input,
		SaveToDisk: false,
		SecretB64:  &secret,
	}).Params)
	if !response.OK {
		t.Fatalf("handleUnprotect() error = %s", response.Error)
	}
	if !bytes.Equal(useCase.symmetricKeyDuringExecute, secret) {
		t.Fatal("la clave binaria no llegó al caso de uso")
	}
	if !bytes.Equal(
		useCase.cmd.SymmetricKey,
		make([]byte, len(secret)),
	) {
		t.Fatal("el adaptador no borró la clave binaria tras Execute")
	}
}

func TestProtectionSymmetricKeyIPC_RejectsAmbiguousAndWrongLength(t *testing.T) {
	t.Parallel()
	secret := []byte("0123456789abcdef0123456789abcdef")
	if _, err := protectionSymmetricKeyIPC(
		&secret,
		map[string]string{"secret_b64": "legacy"},
	); err == nil {
		t.Fatal("se esperaba rechazo de dos fuentes de clave")
	}
	if !bytes.Equal(secret, make([]byte, 32)) {
		t.Fatal("la clave ambigua no quedó borrada")
	}

	short := []byte("demasiado-corta")
	if _, err := protectionSymmetricKeyIPC(&short, nil); err == nil {
		t.Fatal("se esperaba rechazo de longitud no AES-256")
	}
	if !bytes.Equal(short, make([]byte, len(short))) {
		t.Fatal("la clave inválida no quedó borrada")
	}
}

func TestConstruirSalidaProtegidaRutaIPC_ConservaSufijoAuthEnvelopedData(t *testing.T) {
	input := filepath.Join(t.TempDir(), "secreto.txt")
	got := construirSalidaProtegidaRutaIPC(input, "secreto.txt.authenveloped.p7m")
	want := filepath.Join(filepath.Dir(input), "secreto.txt.authenveloped.p7m")
	if got != want {
		t.Fatalf("construirSalidaProtegidaRutaIPC() = %q; want %q", got, want)
	}
}

func TestConstruirSalidaProtegidaRutaIPC_ConservaSufijosCMSCompuestos(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "secreto.txt")
	for _, documentName := range []string{
		"secreto.txt.encrypted.p7m",
		"secreto.txt.signedenveloped.p7m",
		"secreto.txt.authenveloped.p7m",
	} {
		documentName := documentName
		t.Run(documentName, func(t *testing.T) {
			got := construirSalidaProtegidaRutaIPC(input, documentName)
			want := filepath.Join(dir, documentName)
			if got != want {
				t.Fatalf("construirSalidaProtegidaRutaIPC() = %q; want %q", got, want)
			}
		})
	}
}

func TestConstruirSalidaProtegidaRutaIPC_NoConfiaEnRutaCMSCompuesta(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	input := filepath.Join(dir, "secreto.txt")
	got := construirSalidaProtegidaRutaIPC(input, "../../fuera.encrypted.p7m")
	want := filepath.Join(dir, "fuera.encrypted.p7m")
	if got != want {
		t.Fatalf("construirSalidaProtegidaRutaIPC() = %q; want %q", got, want)
	}
}

func TestConstruirSalidaProtegidaRutaIPC_NoConfiaEnRutaDelDocumento(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "secreto.txt")
	got := construirSalidaProtegidaRutaIPC(input, "../../fuera.authenveloped.p7m")
	want := filepath.Join(dir, "fuera.authenveloped.p7m")
	if got != want {
		t.Fatalf("construirSalidaProtegidaRutaIPC() = %q; want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Certificados
// ---------------------------------------------------------------------------

func TestDespachar_Certificates_CatalogoNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if resp.OK {
		t.Error("certificates sin catalogo: esperaba OK=false")
	}
}

func TestDespachar_Certificates_OK(t *testing.T) {
	t.Parallel()
	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{
				{ID: "cert1", Subject: "CN=Juan Garcia", NotAfter: time.Now().Add(365 * 24 * time.Hour)},
			},
		},
	}
	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if !resp.OK {
		t.Errorf("certificates OK: esperaba OK=true, error=%s", resp.Error)
	}
}

func TestDespachar_Certificates_CompatibilidadQML(t *testing.T) {
	t.Parallel()
	ahora := time.Now()
	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{
				{
					ID:            "vigente",
					Subject:       "CN=Juan Garcia",
					Issuer:        "CN=FNMT",
					NotAfter:      ahora.Add(48 * time.Hour),
					Fingerprint:   "fp-vigente",
					HasSigningKey: true,
					NIF:           "12345678A",
				},
				{
					ID:            "caducado",
					Subject:       "CN=Caducado",
					Issuer:        "CN=FNMT",
					NotAfter:      ahora.Add(-48 * time.Hour),
					Fingerprint:   "fp-caducado",
					HasSigningKey: true,
				},
			},
		},
		Claves: &diagnosticKeyProviderStub{
			keys: map[string]ports.SigningKey{
				"vigente": stubSigningKey{id: "clave-vigente"},
			},
			errors: map[string]error{},
		},
	}
	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if !resp.OK {
		t.Fatalf("certificates OK: esperaba OK=true, error=%s", resp.Error)
	}
	items, ok := resp.Data.([]certJSON)
	if !ok {
		t.Fatalf("data no es []certJSON: %T", resp.Data)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].SubjectName != "CN=Juan Garcia" || items[0].IssuerName != "CN=FNMT" {
		t.Fatalf("aliases QML vacios: %+v", items[0])
	}
	if items[0].ValidTo == "" || items[0].Status != "Válido" || !items[0].CanSign {
		t.Fatalf("estado certificado vigente incorrecto: %+v", items[0])
	}
	if items[0].SerialNumber != "12345678A" {
		t.Fatalf("serialNumber = %q, want 12345678A", items[0].SerialNumber)
	}
	if items[1].Status != "Caducado" || items[1].CanSign {
		t.Fatalf("estado certificado caducado incorrecto: %+v", items[1])
	}
	if got := len(m.Claves.(*diagnosticKeyProviderStub).requested); got != 0 {
		t.Fatalf("listar certificados no debe abrir claves ni pedir PIN, llamadas=%d", got)
	}
}

func TestDespachar_Certificates_VigenciaSinClaveNoImplicaFirma(t *testing.T) {
	t.Parallel()
	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{
				{
					ID:       "solo-publico",
					Subject:  "CN=Certificado público",
					NotAfter: time.Now().Add(24 * time.Hour),
				},
			},
		},
	}

	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if !resp.OK {
		t.Fatalf("certificates OK: error=%s", resp.Error)
	}
	items := resp.Data.([]certJSON)
	if len(items) != 1 || items[0].CanSign {
		t.Fatalf("un certificado sin proveedor de clave no puede firmar: %+v", items)
	}
	if items[0].Status != "Sin clave privada" {
		t.Fatalf("estado sin clave privada inesperado: %+v", items[0])
	}
}

func TestDespachar_Certificates_UsaCapacidadNoInteractivaDelCatalogo(t *testing.T) {
	t.Parallel()
	keys := &diagnosticKeyProviderStub{
		keys:   map[string]ports.SigningKey{},
		errors: map[string]error{},
	}
	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{
				{
					ID:            "firmante",
					Subject:       "CN=Firmante",
					NotAfter:      time.Now().Add(24 * time.Hour),
					HasSigningKey: true,
				},
			},
		},
		Claves: keys,
	}

	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if !resp.OK {
		t.Fatalf("certificates OK: error=%s", resp.Error)
	}
	items := resp.Data.([]certJSON)
	if len(items) != 1 || !items[0].CanSign || items[0].Status != "Válido" {
		t.Fatalf("capacidad de firma real inesperada: %+v", items)
	}
	if len(keys.requested) != 0 {
		t.Fatalf("listar certificados no debe abrir la clave: %#v", keys.requested)
	}
}

func TestHandleImportCert_BorraP12TrasCadaResultado(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "exito"},
		{name: "error_importador", err: errors.New("fallo de prueba")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			importador := &stubImportador{err: tc.err}
			m := &Manejador{Importador: importador}
			p12 := []byte("contenido-P12-sensible")
			password := []byte("secreto")
			raw, err := json.Marshal(paramsImportCert{
				P12B64:      base64.StdEncoding.EncodeToString(p12),
				PasswordB64: &password,
			})
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			m.handleImportCert(context.Background(), raw)

			if string(importador.received) != string(p12) {
				t.Fatalf("el importador recibió %q, se esperaba %q", importador.received, p12)
			}
			for i, value := range importador.data {
				if value != 0 {
					t.Fatalf("byte sensible %d no borrado: %d", i, value)
				}
			}
			if importador.receivedPassword != "secreto" {
				t.Fatalf("contraseña recibida = %q", importador.receivedPassword)
			}
			for i := range importador.password {
				if importador.password[i] != 0 {
					t.Fatalf("byte de contraseña %d no borrado", i)
				}
			}
		})
	}
}

func TestInferirFormatoRuta(t *testing.T) {
	t.Parallel()
	casos := map[string]string{
		"/tmp/doc.pdf":   "pades",
		"/tmp/doc.xml":   "xades",
		"/tmp/doc.xsig":  "xades",
		"/tmp/doc.odt":   "odf",
		"/tmp/doc.docx":  "ooxml",
		"/tmp/doc.asics": "asic-xades",
		"/tmp/doc.bin":   "cades",
	}
	for ruta, want := range casos {
		if got := inferirFormatoRuta(ruta); got != want {
			t.Fatalf("inferirFormatoRuta(%q) = %q, want %q", ruta, got, want)
		}
	}
}

func TestConstruirOpcionesFirmaIPC(t *testing.T) {
	t.Parallel()
	got, err := construirOpcionesFirmaIPC(paramsFirma{
		AllowInvalidPDF: true,
		StrictCompat:    true,
		QRContent:       "https://verifica.ejemplo/",
		Reason:          "Aprobación interna",
		Location:        "Granada",
		ContactInfo:     "contacto@example.invalid",
		VisibleSeal: map[string]any{
			"x":          0.5,
			"y":          0.25,
			"w":          0.3,
			"h":          0.1,
			"page":       2,
			"pageWidth":  612.0,
			"pageHeight": 792.0,
			"rotation":   90,
		},
	}, "pades")
	if err != nil {
		t.Fatalf("construirOpcionesFirmaIPC: %v", err)
	}
	if got["allowInvalidPDF"] != "true" {
		t.Fatalf("allowInvalidPDF perdido: %#v", got)
	}
	if got["subfilter"] != "adbe.pkcs7.detached" {
		t.Fatalf("strictCompat no propagó subfilter: %#v", got)
	}
	if got["visibleSeal"] != "true" || got["page"] != "2" || got["rotation"] != "90" {
		t.Fatalf("visibleSeal incompleto: %#v", got)
	}
	if got["visibleSealRectX"] != "306.00" || got["visibleSealRectY"] != "198.00" ||
		got["visibleSealRectW"] != "183.60" || got["visibleSealRectH"] != "79.20" {
		t.Fatalf("rectángulo no convertido con las dimensiones reales de página: %#v", got)
	}
	if got["qrContent"] != "https://verifica.ejemplo/" {
		t.Fatalf("qrContent no propagado: %#v", got)
	}
	if got["reason"] != "Aprobación interna" || got["location"] != "Granada" || got["contactInfo"] != "contacto@example.invalid" {
		t.Fatalf("metadatos PAdES no propagados: %#v", got)
	}
}

func TestConstruirOpcionesFirmaIPC_RequiereDimensionesRealesParaSello(t *testing.T) {
	t.Parallel()

	_, err := construirOpcionesFirmaIPC(paramsFirma{
		VisibleSeal: map[string]any{
			"x": 0.5,
			"y": 0.25,
			"w": 0.3,
			"h": 0.1,
		},
	}, "pades")
	if err == nil {
		t.Fatal("se esperaba error sin dimensiones reales de la página")
	}
}

func TestConstruirOpcionesFirmaIPC_TodasLasPaginas(t *testing.T) {
	t.Parallel()

	got, err := construirOpcionesFirmaIPC(paramsFirma{
		VisibleSeal: map[string]any{
			"x":          0.5,
			"y":          0.25,
			"w":          0.3,
			"h":          0.1,
			"page":       "all",
			"pageWidth":  612.0,
			"pageHeight": 792.0,
		},
	}, "pades")
	if err != nil {
		t.Fatalf("construirOpcionesFirmaIPC: %v", err)
	}
	if got["page"] != "all" {
		t.Fatalf("page visibleSeal = %q, want all", got["page"])
	}
}

func TestConstruirOpcionesFirmaIPC_RangoPaginas(t *testing.T) {
	t.Parallel()

	got, err := construirOpcionesFirmaIPC(paramsFirma{
		VisibleSeal: map[string]any{
			"x":          0.5,
			"y":          0.25,
			"w":          0.3,
			"h":          0.1,
			"page":       "1,3-5",
			"pageWidth":  612.0,
			"pageHeight": 792.0,
		},
	}, "pades")
	if err != nil {
		t.Fatalf("construirOpcionesFirmaIPC: %v", err)
	}
	if got["page"] != "1,3-5" {
		t.Fatalf("page visibleSeal = %q, want 1,3-5", got["page"])
	}
}

func TestConstruirOpcionesFirmaIPC_PropagaExtraOptions(t *testing.T) {
	t.Parallel()

	got, err := construirOpcionesFirmaIPC(paramsFirma{
		ExtraOptions: map[string]string{
			"facturaePolicyVersion": "3.1",
			"signerClaimedRole":     "emisor",
			"":                      "ignorar",
			"vaciar":                "   ",
		},
	}, "facturae")
	if err != nil {
		t.Fatalf("construirOpcionesFirmaIPC: %v", err)
	}
	if got["facturaePolicyVersion"] != "3.1" || got["signerClaimedRole"] != "emisor" {
		t.Fatalf("extra options no propagadas: %#v", got)
	}
	if _, ok := got[""]; ok {
		t.Fatalf("clave vacia no deberia propagarse: %#v", got)
	}
	if _, ok := got["vaciar"]; ok {
		t.Fatalf("valor vacio no deberia propagarse: %#v", got)
	}
}

func TestConstruirOpcionesFirmaIPC_MaterializaImagenDeSello(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seal.png")
	if err := os.WriteFile(path, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := construirOpcionesFirmaIPC(paramsFirma{
		VisibleSeal: map[string]any{
			"imagePath": path, "pageWidth": 612.0, "pageHeight": 792.0,
		},
	}, "pades")
	if err != nil {
		t.Fatalf("construirOpcionesFirmaIPC: %v", err)
	}
	if got["visibleSealImageBase64"] != base64.StdEncoding.EncodeToString([]byte("png-data")) {
		t.Fatalf("imagen no materializada: %#v", got)
	}
	if _, ok := got["visibleSealImagePath"]; ok {
		t.Fatalf("la ruta llegó al signer: %#v", got)
	}
}

func TestConstruirOpcionesFirmaIPC_RechazaImagenSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.png")
	link := filepath.Join(dir, "seal.png")
	if err := os.WriteFile(target, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := construirOpcionesFirmaIPC(paramsFirma{
		VisibleSeal: map[string]any{
			"imagePath": link, "pageWidth": 612.0, "pageHeight": 792.0,
		},
	}, "pades"); err == nil {
		t.Fatal("construirOpcionesFirmaIPC accepted a symlink image")
	}
}

func TestDespachar_Certificates_ErrorCatalogo(t *testing.T) {
	t.Parallel()
	m := &Manejador{
		Catalogo: &stubCatalogo{err: errors.New("NSS error")},
	}
	resp := m.despachar(context.Background(), peticion{Action: "certificates"})
	if resp.OK {
		t.Error("catalogo con error: esperaba OK=false")
	}
}

// ---------------------------------------------------------------------------
// Check certificates — failCount
// ---------------------------------------------------------------------------

func TestDespachar_CheckCertificates_FailCount(t *testing.T) {
	t.Parallel()
	ahora := time.Now()
	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{
				{ID: "ok", Subject: "CN=Vigente", NotAfter: ahora.Add(30 * 24 * time.Hour), HasSigningKey: true},
				{ID: "caducado", Subject: "CN=Caducado", NotAfter: ahora.Add(-24 * time.Hour), HasSigningKey: true},
			},
		},
	}
	resp := m.despachar(context.Background(), peticion{Action: "check_certificates"})
	if !resp.OK {
		t.Fatalf("check_certificates: OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoCheckCerts)
	if !ok {
		t.Fatalf("data no es resultadoCheckCerts: %T", resp.Data)
	}
	if data.FailCount != 1 {
		t.Errorf("FailCount: got %d, queria 1", data.FailCount)
	}
	if data.OkCount != 1 {
		t.Errorf("OkCount: got %d, queria 1", data.OkCount)
	}
}

func TestDespachar_ValidateCertificateOnline_OK(t *testing.T) {
	t.Parallel()

	caDER, caCert, caKey := createIPCOnlineRevocationTestCA(t)
	leafDER := createIPCOnlineRevocationTestLeaf(t, caCert, caKey)
	key := &stubClosingSigningKey{
		stubSigningKey: stubSigningKey{id: "key-1", chain: [][]byte{leafDER, caDER}},
	}

	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{{ID: "cert-1", Subject: "CN=Leaf Revocation Test"}},
		},
		Claves: &stubClaves{
			key: key,
		},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "validate_certificate_online", map[string]any{
		"certificateId": "cert-1",
	}))
	if !resp.OK {
		t.Fatalf("validate_certificate_online: OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoEstadoCertificadoOnline)
	if !ok {
		t.Fatalf("data no es resultadoEstadoCertificadoOnline: %T", resp.Data)
	}
	if data.Status != "inconclusive" {
		t.Fatalf("status = %q, want inconclusive", data.Status)
	}
	if strings.TrimSpace(resp.Error) == "" {
		t.Fatal("mensaje de usuario vacío")
	}
	if key.closed != 1 {
		t.Fatalf("cierres de clave = %d, want 1", key.closed)
	}
}

func TestDespachar_ValidateCertificateOnline_NoAbreClaveNSS(t *testing.T) {
	caDER, ca, caKey := createIPCOnlineRevocationTestCA(t)
	_ = caDER
	leaf := createIPCOnlineRevocationTestLeaf(t, ca, caKey)
	m := &Manejador{
		Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{ID: "cert-1", DER: leaf}}},
		Claves:   &stubClaves{err: syscall.ELOOP},
	}
	resp := m.despachar(context.Background(), peticionJSON(t, "validate_certificate_online", map[string]any{
		"certificateId": "cert-1",
	}))
	if !resp.OK || strings.Contains(resp.Error, "symbolic links") {
		t.Fatalf("la validación pública no debe abrir NSS: %+v", resp)
	}
	data := resp.Data.(resultadoEstadoCertificadoOnline)
	if data.Status != "unavailable" || !strings.Contains(data.Reason, "AIA") {
		t.Fatalf("faltaba emisor y AIA: %+v", data)
	}
}

func TestDespachar_ValidateCertificateOnline_RechazaDERDistintoDeHuella(t *testing.T) {
	_, ca, caKey := createIPCOnlineRevocationTestCA(t)
	leaf := createIPCOnlineRevocationTestLeaf(t, ca, caKey)
	m := &Manejador{Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{
		ID: "cert-1", DER: leaf, Fingerprint: strings.Repeat("0", 64),
	}}}}
	resp := m.despachar(context.Background(), peticionJSON(t, "validate_certificate_online", map[string]any{
		"certificateId": "cert-1",
	}))
	if resp.OK || !strings.Contains(resp.Error, "no coincide") {
		t.Fatalf("se aceptó un DER ajeno a la identidad: %+v", resp)
	}
}

func TestDespachar_ValidateCertificateOnline_UsaCadenaPublicaImportada(t *testing.T) {
	caDER, ca, caKey := createIPCOnlineRevocationTestCA(t)
	leaf := createIPCOnlineRevocationTestLeaf(t, ca, caKey)
	m := &Manejador{
		Catalogo: &stubCatalogo{certs: []domain.CertificateRef{{ID: "cert-1", DER: leaf, ChainDER: [][]byte{caDER}}}},
		Claves:   &stubClaves{err: syscall.ELOOP},
	}
	resp := m.despachar(context.Background(), peticionJSON(t, "validate_certificate_online", map[string]any{
		"certificateId": "cert-1",
	}))
	if !resp.OK {
		t.Fatalf("la cadena pública no debe abrir la clave: %+v", resp)
	}
	data := resp.Data.(resultadoEstadoCertificadoOnline)
	if data.Status != "inconclusive" {
		t.Fatalf("estado = %q, want inconclusive", data.Status)
	}
}

func TestDespachar_ValidateCertificateOnline_CertificadoNoEncontrado(t *testing.T) {
	t.Parallel()

	m := &Manejador{
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{{ID: "cert-1", Subject: "CN=Leaf Revocation Test"}},
		},
		Claves: &stubClaves{},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "validate_certificate_online", map[string]any{
		"certificateId": "missing",
	}))
	if resp.OK {
		t.Fatal("validate_certificate_online esperaba OK=false")
	}
	if !strings.Contains(strings.ToLower(resp.Error), "certificado") {
		t.Fatalf("error inesperado: %s", resp.Error)
	}
}

func createIPCOnlineRevocationTestCA(t *testing.T) ([]byte, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey CA: %v", err)
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA Revocation Test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate CA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate CA: %v", err)
	}
	return der, cert, key
}

func createIPCOnlineRevocationTestLeaf(t *testing.T, issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey leaf: %v", err)
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Leaf Revocation Test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("CreateCertificate leaf: %v", err)
	}
	return der
}

// ---------------------------------------------------------------------------
// Firma
// ---------------------------------------------------------------------------

func documentoFacturaEPredeterminadoTest() ports.DocumentoConfiguracionUsuario {
	version := "3.1"
	policyID := "urn:oid:1.2.3.4"
	policyHash := "Ohixl6upD6av8N7pEvDABhEL6hM="
	qualifier := "https://www.facturae.gob.es/politica.html"
	role := "emisor"
	city := "Granada"
	province := "Granada"
	postalCode := "18001"
	country := "ES"
	return ports.DocumentoConfiguracionUsuario{
		FacturaE: ports.ConfiguracionUsuarioFacturaE{
			PolicyVersion:   &version,
			PolicyID:        &policyID,
			PolicyHash:      &policyHash,
			PolicyQualifier: &qualifier,
			SignerRole:      &role,
			City:            &city,
			Province:        &province,
			PostalCode:      &postalCode,
			Country:         &country,
		},
	}
}

func comprobarOpcionesFacturaEPredeterminadas(t *testing.T, got map[string]string) {
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

func documentoXAdESPredeterminadoTest() ports.DocumentoConfiguracionUsuario {
	policyID := "urn:oid:1.2.3.4.5"
	policyHash := base64.StdEncoding.EncodeToString(make([]byte, 32))
	policyHashAlgorithm := "SHA-256"
	policyQualifier := "https://sede.example/politica-xades.pdf"
	return ports.DocumentoConfiguracionUsuario{
		XAdES: ports.ConfiguracionUsuarioXAdES{
			PolicyID:            &policyID,
			PolicyHash:          &policyHash,
			PolicyHashAlgorithm: &policyHashAlgorithm,
			PolicyQualifier:     &policyQualifier,
		},
	}
}

func comprobarOpcionesXAdESPredeterminadas(t *testing.T, got map[string]string) {
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

func TestDespachar_Sign_FirmarNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{InputPath: "/tmp/x.pdf"}))
	if resp.OK {
		t.Error("sign sin caso de uso: esperaba OK=false")
	}
}

func TestDespachar_Sign_JSONInvalido(t *testing.T) {
	t.Parallel()
	m := &Manejador{Firmar: &stubFirmar{}}
	resp := m.despachar(context.Background(), peticion{
		Action: "sign",
		Params: json.RawMessage(`{invalid json`),
	})
	if resp.OK {
		t.Error("sign JSON invalido: esperaba OK=false")
	}
}

func TestDespachar_Sign_AplicaSubfilterPAdESPersistidoYRespetaOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(input, []byte("%PDF-1.4 prueba"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	subfilter := "adobe"
	settings := &stubTypedSettings{doc: ports.DocumentoConfiguracionUsuario{
		PAdES: ports.ConfiguracionUsuarioPAdES{SubFilter: &subfilter},
	}}
	firmar := &stubFirmar{result: application.SignResult{
		Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed")},
	}}
	m := &Manejador{Firmar: firmar, Settings: settings}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:          input,
		Format:             "pades",
		ReturnSignatureB64: true,
	}))
	if !resp.OK {
		t.Fatalf("sign con default PAdES falló: %s", resp.Error)
	}
	if firmar.cmd.Options["subfilter"] != "adobe" {
		t.Fatalf("subfilter persistido no llegó al motor: %#v", firmar.cmd.Options)
	}

	resp = m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:          input,
		Format:             "pades",
		ReturnSignatureB64: true,
		ExtraOptions:       map[string]string{"SubFilter": "etsi"},
	}))
	if !resp.OK {
		t.Fatalf("sign con override PAdES falló: %s", resp.Error)
	}
	if firmar.cmd.Options["SubFilter"] != "etsi" {
		t.Fatalf("el override explícito no prevaleció: %#v", firmar.cmd.Options)
	}
	if _, duplicate := firmar.cmd.Options["subfilter"]; duplicate {
		t.Fatalf("se filtró una clave duplicada al motor: %#v", firmar.cmd.Options)
	}
}

func TestDespachar_Sign_AplicaFacturaEPersistidaYRespetaOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "factura.xml")
	if err := os.WriteFile(input, []byte("<Facturae/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	settings := &stubTypedSettings{doc: documentoFacturaEPredeterminadoTest()}
	firmar := &stubFirmar{result: application.SignResult{
		Result: domain.SignatureResult{Format: domain.SignatureFormat("FacturaE"), Data: []byte("signed")},
	}}
	m := &Manejador{Firmar: firmar, Settings: settings}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:          input,
		Format:             "FacturaE",
		Action:             "sign",
		ReturnSignatureB64: true,
	}))
	if !resp.OK {
		t.Fatalf("sign con defaults FacturaE falló: %s", resp.Error)
	}
	comprobarOpcionesFacturaEPredeterminadas(t, firmar.cmd.Options)

	resp = m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:          input,
		Format:             "FacturaE",
		Action:             "sign",
		ReturnSignatureB64: true,
		ExtraOptions: map[string]string{
			"PolicyIdentifier":  "urn:oid:9.9",
			"SIGNERCLAIMEDROLE": "receptor",
		},
	}))
	if !resp.OK {
		t.Fatalf("sign con overrides FacturaE falló: %s", resp.Error)
	}
	if firmar.cmd.Options["PolicyIdentifier"] != "urn:oid:9.9" ||
		firmar.cmd.Options["SIGNERCLAIMEDROLE"] != "receptor" {
		t.Fatalf("los overrides explícitos no prevalecieron: %#v", firmar.cmd.Options)
	}
	for _, key := range []string{
		"facturaePolicyVersion",
		"policyIdentifier",
		"policyIdentifierHash",
		"policyQualifier",
		"signerClaimedRole",
	} {
		if _, duplicate := firmar.cmd.Options[key]; duplicate {
			t.Fatalf("se mezcló %q con la política/rol explícitos: %#v", key, firmar.cmd.Options)
		}
	}
}

func TestDespachar_Sign_AplicaPoliticaXAdESPersistidaYRespetaOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "documento.xml")
	if err := os.WriteFile(input, []byte("<documento/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	settings := &stubTypedSettings{doc: documentoXAdESPredeterminadoTest()}
	firmar := &stubFirmar{result: application.SignResult{
		Result: domain.SignatureResult{
			Format: domain.FormatXAdES,
			Data:   []byte("signed"),
		},
	}}
	m := &Manejador{Firmar: firmar, Settings: settings}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:          input,
		Format:             "XAdES",
		Action:             "sign",
		ReturnSignatureB64: true,
	}))
	if !resp.OK {
		t.Fatalf("sign con default XAdES falló: %s", resp.Error)
	}
	comprobarOpcionesXAdESPredeterminadas(t, firmar.cmd.Options)

	resp = m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:          input,
		Format:             "XAdES",
		Action:             "sign",
		ReturnSignatureB64: true,
		ExtraOptions: map[string]string{
			"XAdESPolicyIdentifier": "urn:oid:9.9",
		},
	}))
	if !resp.OK {
		t.Fatalf("sign con override XAdES falló: %s", resp.Error)
	}
	if len(firmar.cmd.Options) != 1 ||
		firmar.cmd.Options["XAdESPolicyIdentifier"] != "urn:oid:9.9" {
		t.Fatalf("la política explícita XAdES no prevaleció: %#v", firmar.cmd.Options)
	}
}

func TestDespachar_Sign_PreferenciasCorruptasNoBloqueanFirmaNiOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(input, []byte("%PDF-1.4 prueba"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	settings := &stubTypedSettings{err: errors.New("settings corruptos")}
	firmar := &stubFirmar{result: application.SignResult{
		Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed")},
	}}
	m := &Manejador{Firmar: firmar, Settings: settings}

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
			settings.usedLoadDoc = false
			resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
				InputPath:          input,
				Format:             tc.format,
				ReturnSignatureB64: true,
				ExtraOptions:       tc.options,
			}))
			if !resp.OK {
				t.Fatalf("sign falló: %s", resp.Error)
			}
			wantLoad := strings.EqualFold(tc.format, "PAdES") && tc.options == nil
			if settings.usedLoadDoc != wantLoad {
				t.Fatalf("usedLoadDoc = %t; want %t", settings.usedLoadDoc, wantLoad)
			}
			if tc.options == nil {
				if _, exists := firmar.cmd.Options["subfilter"]; exists {
					t.Fatalf("se inyectó un default desde settings corruptos: %#v", firmar.cmd.Options)
				}
			}
		})
	}
}

func TestDespachar_SignMultiCoSign_OK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := filepath.Join(dir, "doc.pdf")
	output := filepath.Join(dir, "doc_multifirmado.pdf")
	if err := os.WriteFile(input, []byte("%PDF-1.4 prueba"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}

	m := &Manejador{
		MultiCofirmar: &stubMultiCofirmar{
			result: application.SignResult{
				Result: domain.SignatureResult{
					Format: domain.FormatPAdES,
					Data:   []byte("signed-multi"),
				},
			},
		},
		ultimosCerts: []domain.CertificateRef{
			{ID: "cert-1", Subject: "CN=Uno"},
		},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign_multicosign", paramsFirma{
		InputPath:                input,
		OutputPath:               output,
		CertificateIndex:         0,
		AdditionalCertificateIDs: []string{"cert-2", "cert-3"},
		Format:                   "pades",
		Action:                   "sign",
	}))
	if !resp.OK {
		t.Fatalf("sign_multicosign OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoFirma)
	if !ok {
		t.Fatalf("data no es resultadoFirma: %T", resp.Data)
	}
	if data.OutputPath != output {
		t.Fatalf("outputPath = %q, want %q", data.OutputPath, output)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("os.ReadFile(output) error = %v", err)
	}
	if string(content) != "signed-multi" {
		t.Fatalf("contenido firmado = %q", string(content))
	}
	stub := m.MultiCofirmar.(*stubMultiCofirmar)
	if stub.cmd.PrimaryCertificateID != "cert-1" {
		t.Fatalf("PrimaryCertificateID = %q, want cert-1", stub.cmd.PrimaryCertificateID)
	}
	if len(stub.cmd.AdditionalCertificateIDs) != 2 {
		t.Fatalf("AdditionalCertificateIDs = %#v", stub.cmd.AdditionalCertificateIDs)
	}
}

func TestDespachar_SignBatch_ProcesarLoteNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths: []string{"/tmp/a.pdf"},
	}))
	if resp.OK {
		t.Error("sign_batch sin caso de uso: esperaba OK=false")
	}
}

func TestDespachar_SignMultiYBatch_ProyectanMismosDefaultsFacturaE(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "factura.xml")
	if err := os.WriteFile(input, []byte("<Facturae/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	settings := &stubTypedSettings{doc: documentoFacturaEPredeterminadoTest()}

	multi := &stubMultiCofirmar{result: application.SignResult{
		Result: domain.SignatureResult{Format: domain.SignatureFormat("FacturaE"), Data: []byte("signed-multi")},
	}}
	manejadorMulti := &Manejador{MultiCofirmar: multi, Settings: settings}
	resp := manejadorMulti.despachar(context.Background(), peticionJSON(t, "sign_multicosign", paramsFirma{
		InputPath:          input,
		Format:             "FacturaE",
		Action:             "sign",
		ReturnSignatureB64: true,
	}))
	if !resp.OK {
		t.Fatalf("sign_multicosign con defaults FacturaE falló: %s", resp.Error)
	}
	comprobarOpcionesFacturaEPredeterminadas(t, multi.cmd.Options)

	lote := &stubProcesarLote{result: application.BatchResult{
		Results: []application.SignResult{{
			Result: domain.SignatureResult{Format: domain.SignatureFormat("FacturaE"), Data: []byte("signed-batch")},
		}},
	}}
	manejadorLote := &Manejador{ProcesarLote: lote, Settings: settings}
	resp = manejadorLote.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths: []string{input},
		OutputDir:  t.TempDir(),
		Format:     "FacturaE",
		Action:     "sign",
	}))
	if !resp.OK {
		t.Fatalf("sign_batch con defaults FacturaE falló: %s", resp.Error)
	}
	if len(lote.cmd.Jobs) != 1 {
		t.Fatalf("jobs FacturaE = %d; want 1", len(lote.cmd.Jobs))
	}
	comprobarOpcionesFacturaEPredeterminadas(t, lote.cmd.Jobs[0].Options)
}

func TestDespachar_SignMultiYBatch_ProyectanMismosDefaultsXAdES(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "documento.xml")
	if err := os.WriteFile(input, []byte("<documento/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	settings := &stubTypedSettings{doc: documentoXAdESPredeterminadoTest()}

	multi := &stubMultiCofirmar{result: application.SignResult{
		Result: domain.SignatureResult{
			Format: domain.FormatXAdES,
			Data:   []byte("signed-multi"),
		},
	}}
	manejadorMulti := &Manejador{MultiCofirmar: multi, Settings: settings}
	resp := manejadorMulti.despachar(
		context.Background(),
		peticionJSON(t, "sign_multicosign", paramsFirma{
			InputPath:          input,
			Format:             "XAdES",
			Action:             "sign",
			ReturnSignatureB64: true,
		}),
	)
	if !resp.OK {
		t.Fatalf("sign_multicosign con default XAdES falló: %s", resp.Error)
	}
	comprobarOpcionesXAdESPredeterminadas(t, multi.cmd.Options)

	lote := &stubProcesarLote{result: application.BatchResult{
		Results: []application.SignResult{{
			Result: domain.SignatureResult{
				Format: domain.FormatXAdES,
				Data:   []byte("signed-batch"),
			},
		}},
	}}
	manejadorLote := &Manejador{ProcesarLote: lote, Settings: settings}
	resp = manejadorLote.despachar(
		context.Background(),
		peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: []string{input},
			OutputDir:  t.TempDir(),
			Format:     "XAdES",
			Action:     "sign",
		}),
	)
	if !resp.OK {
		t.Fatalf("sign_batch con default XAdES falló: %s", resp.Error)
	}
	if len(lote.cmd.Jobs) != 1 {
		t.Fatalf("jobs XAdES = %d; want 1", len(lote.cmd.Jobs))
	}
	comprobarOpcionesXAdESPredeterminadas(t, lote.cmd.Jobs[0].Options)
}

func TestDespachar_SignBatch_InputPathsOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputA := filepath.Join(dir, "a.pdf")
	inputB := filepath.Join(dir, "b.xml")
	if err := os.WriteFile(inputA, []byte("%PDF-1.4 prueba"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputA) error = %v", err)
	}
	if err := os.WriteFile(inputB, []byte("<root/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputB) error = %v", err)
	}

	uc := &stubProcesarLote{
		result: application.BatchResult{
			Results: []application.SignResult{
				{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed-a")}},
				{Result: domain.SignatureResult{Format: domain.FormatXAdES, Data: []byte("signed-b")}},
			},
		},
	}
	m := &Manejador{ProcesarLote: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths: []string{inputA, inputB},
		Action:     "sign",
	}))
	if !resp.OK {
		t.Fatalf("sign_batch OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoFirmaLote)
	if !ok {
		t.Fatalf("data no es resultadoFirmaLote: %T", resp.Data)
	}
	if data.OkCount != 2 || data.FailCount != 0 {
		t.Fatalf("conteo lote inesperado: %+v", data)
	}
	if len(data.Results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(data.Results))
	}
	if got := len(uc.cmd.Jobs); got != 2 {
		t.Fatalf("len(cmd.Jobs) = %d, want 2", got)
	}
	if uc.cmd.Jobs[0].Format != domain.FormatPAdES || uc.cmd.Jobs[1].Format != domain.FormatXAdES {
		t.Fatalf("formatos inferidos incorrectos: %+v", uc.cmd.Jobs)
	}
	if _, err := os.Stat(filepath.Join(dir, "a_firmado.pdf")); err != nil {
		t.Fatalf("salida a_firmado.pdf no creada: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b_firmado.xsig")); err != nil {
		t.Fatalf("salida b_firmado.xsig no creada: %v", err)
	}
}

func TestDespachar_SignBatch_AplicaOverrideVisiblePorDocumento(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputA := filepath.Join(dir, "a.pdf")
	inputB := filepath.Join(dir, "b.pdf")
	for path, content := range map[string]string{
		inputA: "%PDF-1.7 a",
		inputB: "%PDF-1.7 b",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	uc := &stubProcesarLote{result: application.BatchResult{
		Results: []application.SignResult{
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed-a")}},
			{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed-b")}},
		},
	}}
	m := &Manejador{ProcesarLote: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths: []string{inputA, inputB},
		Format:     "pades",
		Action:     "sign",
		VisibleSeal: map[string]any{
			"page": 1, "x": 0.1, "y": 0.2, "w": 0.3, "h": 0.1,
			"pageWidth": 600.0, "pageHeight": 800.0,
		},
		DocumentOverrides: []firmaLoteDocumentOverride{{
			InputPath: inputB,
			VisibleSeal: map[string]any{
				"page": "2,4", "x": 0.5, "y": 0.25, "w": 0.2, "h": 0.15,
				"pageWidth": 700.0, "pageHeight": 900.0,
			},
		}},
	}))
	if !resp.OK {
		t.Fatalf("sign_batch OK=false, error=%s", resp.Error)
	}
	if len(uc.cmd.Jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(uc.cmd.Jobs))
	}
	global := uc.cmd.Jobs[0].Options
	override := uc.cmd.Jobs[1].Options
	if global["page"] != "1" || global["visibleSealRectX"] != "60.00" || global["visibleSealRectY"] != "160.00" {
		t.Fatalf("plantilla global inesperada: %#v", global)
	}
	if override["page"] != "2,4" || override["visibleSealRectX"] != "350.00" ||
		override["visibleSealRectY"] != "225.00" || override["visibleSealRectW"] != "140.00" ||
		override["visibleSealRectH"] != "135.00" {
		t.Fatalf("override por documento inesperado: %#v", override)
	}
}

func TestDespachar_SignBatch_RechazaOverrideAjenoODuplicado(t *testing.T) {
	t.Parallel()

	input := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(input, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	seal := map[string]any{
		"page": 1, "pageWidth": 600.0, "pageHeight": 800.0,
	}
	for _, tc := range []struct {
		name      string
		overrides []firmaLoteDocumentOverride
		want      string
	}{
		{
			name: "ajeno",
			overrides: []firmaLoteDocumentOverride{{
				InputPath:   filepath.Join(filepath.Dir(input), "no-seleccionado.pdf"),
				VisibleSeal: seal,
			}},
			want: "no corresponde",
		},
		{
			name: "duplicado",
			overrides: []firmaLoteDocumentOverride{
				{InputPath: input, VisibleSeal: seal},
				{InputPath: filepath.Clean(input), VisibleSeal: seal},
			},
			want: "duplicado",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Manejador{ProcesarLote: &stubProcesarLote{}}
			resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
				InputPaths:        []string{input},
				Format:            "pades",
				Action:            "sign",
				VisibleSeal:       seal,
				DocumentOverrides: tc.overrides,
			}))
			if resp.OK || !strings.Contains(resp.Error, tc.want) {
				t.Fatalf("respuesta = %+v, want error con %q", resp, tc.want)
			}
		})
	}
}

func TestDespachar_SignBatch_DirectoryPathOK(t *testing.T) {
	t.Parallel()

	inputDir := t.TempDir()
	outputDir := t.TempDir()
	inputA := filepath.Join(inputDir, "a.pdf")
	inputB := filepath.Join(inputDir, "b.pdf")
	if err := os.WriteFile(inputA, []byte("%PDF-1.4 a"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputA) error = %v", err)
	}
	if err := os.WriteFile(inputB, []byte("%PDF-1.4 b"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputB) error = %v", err)
	}

	uc := &stubProcesarLote{
		result: application.BatchResult{
			Results: []application.SignResult{
				{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed-a")}},
				{Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed-b")}},
			},
		},
	}
	m := &Manejador{ProcesarLote: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		DirectoryPath: inputDir,
		OutputDir:     outputDir,
		Format:        "pades",
		Action:        "sign",
	}))
	if !resp.OK {
		t.Fatalf("sign_batch por carpeta OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoFirmaLote)
	if !ok {
		t.Fatalf("data no es resultadoFirmaLote: %T", resp.Data)
	}
	if data.OkCount != 2 || data.FailCount != 0 {
		t.Fatalf("conteo lote inesperado: %+v", data)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "a_firmado.pdf")); err != nil {
		t.Fatalf("salida lote a_firmado.pdf no creada: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "b_firmado.pdf")); err != nil {
		t.Fatalf("salida lote b_firmado.pdf no creada: %v", err)
	}
}

func TestDespachar_SignBatch_MultiCoSignOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputDir := t.TempDir()
	inputA := filepath.Join(dir, "a.pdf")
	inputB := filepath.Join(dir, "b.pdf")
	if err := os.WriteFile(inputA, []byte("%PDF-1.4 prueba a"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputA) error = %v", err)
	}
	if err := os.WriteFile(inputB, []byte("%PDF-1.4 prueba b"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputB) error = %v", err)
	}

	uc := &stubMultiCofirmar{
		result: application.SignResult{
			Result: domain.SignatureResult{Format: domain.FormatPAdES, Data: []byte("signed-multi")},
		},
	}
	m := &Manejador{MultiCofirmar: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign_batch", paramsFirmaLote{
		InputPaths:               []string{inputA, inputB},
		OutputDir:                outputDir,
		Format:                   "pades",
		Action:                   "sign",
		CertificateIndex:         0,
		AdditionalCertificateIDs: []string{"cert-2", "cert-3"},
		Reason:                   "Aprobación",
		QRContent:                "https://verifica",
	}))
	if !resp.OK {
		t.Fatalf("sign_batch multicosign OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoFirmaLote)
	if !ok {
		t.Fatalf("data no es resultadoFirmaLote: %T", resp.Data)
	}
	if data.OkCount != 2 || data.FailCount != 0 {
		t.Fatalf("conteo lote inesperado: %+v", data)
	}
	if len(uc.cmds) != 2 {
		t.Fatalf("len(cmds) = %d, want 2", len(uc.cmds))
	}
	for i, cmd := range uc.cmds {
		if cmd.InitialAction != domain.ActionSign {
			t.Fatalf("cmd[%d].InitialAction = %s, want sign", i, cmd.InitialAction)
		}
		if cmd.PrimaryCertificateID != "" {
			t.Fatalf("cmd[%d].PrimaryCertificateID = %q, want empty because cert index 0 unresolved sin catálogo", i, cmd.PrimaryCertificateID)
		}
		if len(cmd.AdditionalCertificateIDs) != 2 {
			t.Fatalf("cmd[%d].AdditionalCertificateIDs = %v, want 2 ids", i, cmd.AdditionalCertificateIDs)
		}
		if cmd.Options["reason"] != "Aprobación" {
			t.Fatalf("cmd[%d].Options[reason] = %q", i, cmd.Options["reason"])
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "a_firmado.pdf")); err != nil {
		t.Fatalf("salida a_firmado.pdf no creada: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "b_firmado.pdf")); err != nil {
		t.Fatalf("salida b_firmado.pdf no creada: %v", err)
	}
}

func TestDespachar_SignBatch_RechazaMasDocumentosQueElLimiteBackend(
	t *testing.T,
) {
	t.Parallel()

	inputPaths := make([]string, maxIPCBatchDocuments+1)
	for index := range inputPaths {
		inputPaths[index] = fmt.Sprintf("documento-%03d.pdf", index)
	}
	m := &Manejador{ProcesarLote: &stubProcesarLote{}}
	resp := m.despachar(
		context.Background(),
		peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: inputPaths,
			Format:     "pades",
			Action:     "sign",
		}),
	)
	if resp.OK ||
		!strings.Contains(resp.Error, fmt.Sprint(maxIPCBatchDocuments)) {
		t.Fatalf("respuesta = %+v, want límite backend", resp)
	}
}

func TestDespachar_SignBatch_RechazaTamanoAgregadoAntesDeLeer(
	t *testing.T,
) {
	t.Parallel()

	dir := t.TempDir()
	inputPaths := make([]string, 3)
	for index := range inputPaths {
		inputPaths[index] = filepath.Join(
			dir,
			fmt.Sprintf("grande-%d.pdf", index),
		)
		if err := os.WriteFile(inputPaths[index], nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(inputPaths[index], 90*1024*1024); err != nil {
			t.Fatal(err)
		}
	}

	m := &Manejador{ProcesarLote: &stubProcesarLote{}}
	resp := m.despachar(
		context.Background(),
		peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: inputPaths,
			Format:     "pades",
			Action:     "sign",
		}),
	)
	if resp.OK || !strings.Contains(resp.Error, "tamaño total máximo") {
		t.Fatalf("respuesta = %+v, want límite agregado", resp)
	}
}

func TestDespachar_SignBatch_RenameReservaSalidasUnicas(
	t *testing.T,
) {
	t.Parallel()

	inputDirA := t.TempDir()
	inputDirB := t.TempDir()
	outputDir := t.TempDir()
	inputA := filepath.Join(inputDirA, "documento.pdf")
	inputB := filepath.Join(inputDirB, "documento.pdf")
	if err := os.WriteFile(inputA, []byte("%PDF-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputB, []byte("%PDF-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &stubProcesarLote{result: application.BatchResult{
		Results: []application.SignResult{
			{
				Result: domain.SignatureResult{
					Format: domain.FormatPAdES,
					Data:   []byte("signed-a"),
				},
			},
			{
				Result: domain.SignatureResult{
					Format: domain.FormatPAdES,
					Data:   []byte("signed-b"),
				},
			},
		},
	}}
	m := &Manejador{ProcesarLote: uc}

	resp := m.despachar(
		context.Background(),
		peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: []string{inputA, inputB},
			OutputDir:  outputDir,
			Format:     "pades",
			Action:     "sign",
			Overwrite:  "rename",
		}),
	)
	if !resp.OK {
		t.Fatalf("sign_batch OK=false, error=%s", resp.Error)
	}
	for path, expected := range map[string]string{
		filepath.Join(outputDir, "documento_firmado.pdf"):   "signed-a",
		filepath.Join(outputDir, "documento_2_firmado.pdf"): "signed-b",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("leer %s: %v", filepath.Base(path), err)
		}
		if string(content) != expected {
			t.Fatalf(
				"contenido %s = %q, want %q",
				filepath.Base(path),
				content,
				expected,
			)
		}
	}
}

func TestDespachar_SignBatch_OverwriteRechazaColisionInterna(
	t *testing.T,
) {
	t.Parallel()

	inputDirA := t.TempDir()
	inputDirB := t.TempDir()
	outputDir := t.TempDir()
	inputA := filepath.Join(inputDirA, "documento.pdf")
	inputB := filepath.Join(inputDirB, "documento.pdf")
	if err := os.WriteFile(inputA, []byte("%PDF-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputB, []byte("%PDF-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manejador{ProcesarLote: &stubProcesarLote{}}

	resp := m.despachar(
		context.Background(),
		peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: []string{inputA, inputB},
			OutputDir:  outputDir,
			Format:     "pades",
			Action:     "sign",
			Overwrite:  "overwrite",
		}),
	)
	if resp.OK || !strings.Contains(resp.Error, "misma ruta de salida") {
		t.Fatalf("respuesta = %+v, want colisión segura", resp)
	}
}

func TestDespachar_SignBatch_NoReflejaRutaEnErrorODiagnostico(
	t *testing.T,
) {
	t.Parallel()

	inputPath := filepath.Join(
		t.TempDir(),
		"expediente-confidencial.pdf",
	)
	if err := os.WriteFile(inputPath, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Manejador{ProcesarLote: &stubProcesarLote{
		result: application.BatchResult{
			Errores: map[int]error{
				0: fmt.Errorf(
					"SAF_44: no se pudo guardar %s",
					inputPath,
				),
			},
		},
	}}

	resp := m.despachar(
		context.Background(),
		peticionJSON(t, "sign_batch", paramsFirmaLote{
			InputPaths: []string{inputPath},
			Format:     "pades",
			Action:     "sign",
		}),
	)
	if !resp.OK {
		t.Fatalf("sign_batch OK=false, error=%s", resp.Error)
	}
	data := resp.Data.(resultadoFirmaLote)
	if len(data.Results) != 1 {
		t.Fatalf("resultados=%d, want 1", len(data.Results))
	}
	visible := data.Results[0].Error
	if resp.Diagnostic != nil {
		visible += " " + resp.Diagnostic.UserMessage +
			" " + resp.Diagnostic.ExpertMessage
	}
	if strings.Contains(visible, inputPath) ||
		strings.Contains(visible, "expediente-confidencial.pdf") ||
		strings.Contains(visible, "Mensaje original") {
		t.Fatalf("la respuesta refleja la ruta: %#v", resp)
	}
	if !strings.Contains(data.Results[0].Error, "SAF_44") {
		t.Fatalf("error sin código estable: %q", data.Results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Verificacion
// ---------------------------------------------------------------------------

func TestDespachar_Verify_VerificarNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{InputPath: "/tmp/x.pdf"}))
	if resp.OK {
		t.Error("verify sin caso de uso: esperaba OK=false")
	}
	if resp.Diagnostic == nil || resp.Diagnostic.Category != string(application.GuidedDiagnosticAppLocal) {
		t.Fatalf("diagnostic=%#v, want app_local", resp.Diagnostic)
	}
}

func TestDespachar_Sign_ErrorCertificadoClasificado(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(inputPath, []byte("pdf"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}

	m := &Manejador{
		Firmar: &stubFirmar{err: errors.New("error accediendo al almacén PKCS#11")},
		ultimosCerts: []domain.CertificateRef{{
			ID:      "cert-1",
			Subject: "CN=Ana",
		}},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:        inputPath,
		CertificateIndex: 0,
		Format:           "pades",
		Action:           "sign",
		Overwrite:        "rename",
	}))
	if resp.OK {
		t.Fatal("sign con error de almacén debería devolver OK=false")
	}
	if resp.Diagnostic == nil || resp.Diagnostic.Category != string(application.GuidedDiagnosticCertificateStore) {
		t.Fatalf("diagnostic=%#v, want certificate_store", resp.Diagnostic)
	}
	if resp.Diagnostic.FailureCode != "" {
		t.Fatalf("failureCode inesperado para error sin código estable: %#v", resp.Diagnostic)
	}
}

func TestDespachar_Sign_ErrorConCodigoEstablePropagaFailureCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(inputPath, []byte("pdf"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}

	m := &Manejador{
		Firmar: &stubFirmar{err: errors.New("SAF_09: Error en la operacion de firma")},
		ultimosCerts: []domain.CertificateRef{{
			ID:      "cert-1",
			Subject: "CN=Ana",
		}},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "sign", paramsFirma{
		InputPath:        inputPath,
		CertificateIndex: 0,
		Format:           "pades",
		Action:           "sign",
		Overwrite:        "rename",
	}))
	if resp.OK {
		t.Fatal("sign con SAF_09 debería devolver OK=false")
	}
	if resp.Diagnostic == nil || resp.Diagnostic.FailureCode != "SAF_09" {
		t.Fatalf("diagnostic=%#v, want failureCode SAF_09", resp.Diagnostic)
	}
}

func TestDespachar_Verify_JSONInvalido(t *testing.T) {
	t.Parallel()
	m := &Manejador{Verificar: &stubVerificar{}}
	resp := m.despachar(context.Background(), peticion{
		Action: "verify",
		Params: json.RawMessage(`{invalid json`),
	})
	if resp.OK {
		t.Error("verify JSON invalido: esperaba OK=false")
	}
}

func TestDespachar_Verify_ResultRico(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "firma.csig")
	if err := os.WriteFile(inputPath, []byte("firma"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}

	m := &Manejador{
		Verificar: &stubVerificar{
			result: application.VerifyResult{
				Verification: domain.VerificationResult{
					Valid:    true,
					Reason:   "OK",
					Details:  []string{"firma íntegra"},
					Format:   "CAdES",
					Coverage: "full",
					Integrity: domain.VerificationAspect{
						Status:  domain.VerificationStatusValid,
						Reason:  "integridad comprobada",
						Details: []string{"firma íntegra"},
					},
					Certificate: domain.VerificationAspect{
						Status:  domain.VerificationStatusWarning,
						Reason:  "revocación no disponible",
						Details: []string{"OCSP no disponible"},
					},
					Trust: domain.VerificationAspect{
						Status:  domain.VerificationStatusValid,
						Reason:  "anclaje local encontrado",
						Details: []string{"cadena válida"},
					},
					SignerSummaries: []domain.VerificationSignerSummary{{
						ID:          "cert-1",
						Subject:     "CN=Ana",
						Issuer:      "CN=FNMT",
						Fingerprint: "fp1",
					}},
					Warnings: []string{"revocación no disponible"},
					Evidence: []domain.VerificationEvidence{{
						Type:    "signingCertificate",
						Summary: "CN=Ana",
					}},
				},
				Firmantes: []domain.CertificateRef{{
					Subject: "CN=Ana",
				}},
			},
		},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{InputPath: inputPath}))
	if !resp.OK {
		t.Fatalf("verify debería devolver OK=true: %#v", resp)
	}
	data, ok := resp.Data.(resultadoVerificacion)
	if !ok {
		t.Fatalf("data no es resultadoVerificacion: %T", resp.Data)
	}
	if data.Format != "CAdES" || data.Coverage != "full" {
		t.Fatalf("resultado rico incompleto: %#v", data)
	}
	if data.Integrity.Status != "valid" || data.Certificate.Status != "warning" || data.Trust.Status != "valid" {
		t.Fatalf("aspectos de verificación inesperados: %#v", data)
	}
	if len(data.SignerSummaries) != 1 || len(data.Warnings) != 1 || len(data.Evidence) != 1 {
		t.Fatalf("resultado rico sin detalles esperados: %#v", data)
	}
}

func TestDespachar_Verify_ConOriginal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "firma_detached.xsig")
	originalPath := filepath.Join(dir, "original.xml")
	if err := os.WriteFile(inputPath, []byte("<FirmaDetached/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(inputPath) error = %v", err)
	}
	if err := os.WriteFile(originalPath, []byte("<Original/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(originalPath) error = %v", err)
	}

	verificador := &stubVerificar{
		result: application.VerifyResult{
			Verification: domain.VerificationResult{Valid: true, Reason: "XAdES detached válida"},
		},
	}
	m := &Manejador{Verificar: verificador}

	resp := m.despachar(context.Background(), peticionJSON(t, "verify", paramsVerify{
		InputPath:    inputPath,
		OriginalPath: originalPath,
	}))
	if !resp.OK {
		t.Fatalf("verify con original debería devolver OK=true: %#v", resp)
	}
	if verificador.cmd.OriginalDocument == nil {
		t.Fatal("se esperaba OriginalDocument en VerifyCommand")
	}
	if got := verificador.cmd.OriginalDocument.Name; got != filepath.Base(originalPath) {
		t.Fatalf("OriginalDocument.Name=%q, want %q", got, filepath.Base(originalPath))
	}
}

// ---------------------------------------------------------------------------
// Hash
// ---------------------------------------------------------------------------

func TestDespachar_HashCreate_FicheroOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(input, []byte("hola"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}

	uc := &stubCrearHash{
		result: application.CreateHashResult{
			Algorithm: "SHA-256",
			Format:    application.HashFormatHex,
			Encoded:   "abc123h",
		},
	}
	m := &Manejador{CrearHash: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "hash_create", paramsHashCreate{
		InputPath: input,
		Format:    "hex",
	}))
	if !resp.OK {
		t.Fatalf("hash_create fichero OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoHash)
	if !ok {
		t.Fatalf("data no es resultadoHash: %T", resp.Data)
	}
	if data.OutputPath == "" || !strings.HasSuffix(data.OutputPath, ".hexhash") {
		t.Fatalf("outputPath inesperado: %+v", data)
	}
	content, err := os.ReadFile(data.OutputPath)
	if err != nil {
		t.Fatalf("os.ReadFile(outputPath) error = %v", err)
	}
	if string(content) != "abc123h" {
		t.Fatalf("contenido hash = %q, want abc123h", string(content))
	}
	if uc.cmd.Format != application.HashFormatHex {
		t.Fatalf("cmd.Format = %q, want hex", uc.cmd.Format)
	}
}

func TestDespachar_HashCreate_DirectorioOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	uc := &stubCrearHashDir{
		result: application.CreateDirectoryHashManifestResult{
			Algorithm: "SHA-256",
			Format:    domain.DirectoryHashFormatXML,
			Manifest: domain.DirectoryHashManifest{
				Algorithm: "SHA-256",
				Recursive: true,
				Entries: []domain.DirectoryHashEntry{
					{RelativePath: "a.txt", Digest: []byte{1, 2, 3}},
				},
			},
			Data: []byte("<entries/>"),
		},
	}
	m := &Manejador{CrearHashDir: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "hash_create", paramsHashCreate{
		InputPath: dir,
		Format:    "xml",
		Recursive: true,
	}))
	if !resp.OK {
		t.Fatalf("hash_create directorio OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoHashDirectorio)
	if !ok {
		t.Fatalf("data no es resultadoHashDirectorio: %T", resp.Data)
	}
	if data.Entries != 1 || !data.Recursive {
		t.Fatalf("resultado directorio inesperado: %+v", data)
	}
	manifestData, err := os.ReadFile(data.OutputPath)
	if err != nil {
		t.Fatalf("os.ReadFile(outputPath) error = %v", err)
	}
	if string(manifestData) != "<entries/>" {
		t.Fatalf("manifiesto guardado = %q", string(manifestData))
	}
	if got := uc.cmd.RootPath; got != dir {
		t.Fatalf("cmd.RootPath = %q, want %q", got, dir)
	}
}

func TestDespachar_HashCheck_FicheroOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := filepath.Join(dir, "doc.txt")
	hashPath := filepath.Join(dir, "doc.hexhash")
	if err := os.WriteFile(input, []byte("hola"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(input) error = %v", err)
	}
	if err := os.WriteFile(hashPath, []byte("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824h"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(hashPath) error = %v", err)
	}

	uc := &stubComprobarHash{
		result: application.CheckHashResult{
			Valid:           true,
			Algorithm:       "SHA-256",
			Format:          application.HashFormatHex,
			ExpectedEncoded: "esperadoh",
			ActualEncoded:   "actualh",
		},
	}
	m := &Manejador{ComprobarHash: uc}

	resp := m.despachar(context.Background(), peticionJSON(t, "hash_check", paramsHashCheck{
		InputPath: input,
		HashPath:  hashPath,
	}))
	if !resp.OK {
		t.Fatalf("hash_check fichero OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoComprobarHash)
	if !ok {
		t.Fatalf("data no es resultadoComprobarHash: %T", resp.Data)
	}
	if !data.Valid || data.ExpectedHash != "esperadoh" || data.ActualHash != "actualh" {
		t.Fatalf("resultado comprobar hash inesperado: %+v", data)
	}
	if uc.cmd.Algorithm != "SHA-256" || uc.cmd.Format != application.HashFormatHex {
		t.Fatalf("cmd inesperado: %+v", uc.cmd)
	}
}

func TestDespachar_HashCheck_DirectorioConInformeOK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hashPath := filepath.Join(t.TempDir(), "manifiesto.hashfiles")
	reportPath := filepath.Join(t.TempDir(), "resultado.hashreport")
	if err := os.WriteFile(hashPath, []byte("<entries/>"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(hashPath) error = %v", err)
	}

	uc := &stubComprobarHashDir{
		result: application.CheckDirectoryHashManifestResult{
			Valid: true,
			Report: domain.DirectoryHashCheckReport{
				Algorithm:    "SHA-256",
				Recursive:    true,
				MatchingHash: []string{"a.txt"},
			},
		},
	}
	m := &Manejador{
		ComprobarHashDir: uc,
		InformeHashDir:   &stubHashReportCodec{data: []byte("<report/>")},
	}

	resp := m.despachar(context.Background(), peticionJSON(t, "hash_check", paramsHashCheck{
		InputPath:        dir,
		HashPath:         hashPath,
		OutputPath:       reportPath,
		SaveReportToDisk: true,
	}))
	if !resp.OK {
		t.Fatalf("hash_check directorio OK=false, error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoComprobarHashDirectorio)
	if !ok {
		t.Fatalf("data no es resultadoComprobarHashDirectorio: %T", resp.Data)
	}
	if !data.Valid || data.ReportOutputPath != reportPath {
		t.Fatalf("resultado directorio inesperado: %+v", data)
	}
	if got := data.ReportB64; got != base64.StdEncoding.EncodeToString([]byte("<report/>")) {
		t.Fatalf("reportBase64 = %q", got)
	}
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("os.ReadFile(reportPath) error = %v", err)
	}
	if string(reportData) != "<report/>" {
		t.Fatalf("informe guardado = %q", string(reportData))
	}
	if uc.cmd.RootPath != dir || uc.cmd.ManifestHint != hashPath {
		t.Fatalf("cmd inesperado: %+v", uc.cmd)
	}
}

// ---------------------------------------------------------------------------
// PDF preview
// ---------------------------------------------------------------------------

func TestDespachar_PdfPreview_PreviewNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticionJSON(t, "pdf_preview", paramsPdfPreview{Path: "/tmp/doc.pdf", Page: 1}))
	if resp.OK {
		t.Error("pdf_preview sin caso de uso: esperaba OK=false")
	}
}

func TestDespachar_PdfPreview_JSONInvalido(t *testing.T) {
	t.Parallel()
	m := &Manejador{Preview: &stubPreview{}}
	resp := m.despachar(context.Background(), peticion{
		Action: "pdf_preview",
		Params: json.RawMessage(`{invalid json`),
	})
	if resp.OK {
		t.Error("pdf_preview JSON invalido: esperaba OK=false")
	}
}

// stubPreview implementa PdfPreviewUseCase.
type stubPreview struct{}

func (s *stubPreview) Ejecutar(_ context.Context, _ application.PdfPreviewCommand) (application.PdfPreviewResult, error) {
	return application.PdfPreviewResult{DataB64: "aGVsbG8=", Ancho: 595, Alto: 842, PaginaActual: 1, TotalPaginas: 8}, nil
}

// ---------------------------------------------------------------------------
// Diagnóstico TLS
// ---------------------------------------------------------------------------

func TestDespachar_ExportDiagnostic_NoExponeRutasNiNombresLocales(t *testing.T) {
	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	for nombre, contenido := range map[string]string{
		"cliente-secreto.crt":      "cert",
		"servidor-secreto.key.pem": "key",
		"notas-internas.txt":       "metadata",
	} {
		if err := os.WriteFile(filepath.Join(tlsDir, nombre), []byte(contenido), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", nombre, err)
		}
	}

	m := &Manejador{ConfigDir: dir}
	resp := m.despachar(context.Background(), peticion{Action: "export_diagnostic"})
	if !resp.OK {
		t.Fatalf("export_diagnostic OK=false, error=%s", resp.Error)
	}
	resultado, ok := resp.Data.(resultadoDiagnostico)
	if !ok {
		t.Fatalf("data inesperado: %#v", resp.Data)
	}
	if resultado.TLSStore.State != "available" ||
		resultado.TLSStore.ArtifactCount != 3 ||
		resultado.TLSStore.CertificateCount != 1 ||
		resultado.TLSStore.KeyCount != 1 {
		t.Fatalf("estado TLS inesperado: %#v", resultado.TLSStore)
	}

	serializado, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal(resp) error = %v", err)
	}
	for _, secreto := range []string{
		dir,
		tlsDir,
		"cliente-secreto.crt",
		"servidor-secreto.key.pem",
		"notas-internas.txt",
	} {
		if strings.Contains(string(serializado), secreto) {
			t.Fatalf("el diagnóstico IPC filtra %q: %s", secreto, serializado)
		}
	}

	tlsResp := m.despachar(context.Background(), peticion{Action: "tls_diagnostics"})
	if !tlsResp.OK {
		t.Fatalf("tls_diagnostics OK=false, error=%s", tlsResp.Error)
	}
	tlsSerializado, err := json.Marshal(tlsResp)
	if err != nil {
		t.Fatalf("json.Marshal(tlsResp) error = %v", err)
	}
	for _, secreto := range []string{dir, tlsDir, "cliente-secreto.crt"} {
		if strings.Contains(string(tlsSerializado), secreto) {
			t.Fatalf("el estado TLS IPC filtra %q: %s", secreto, tlsSerializado)
		}
	}
}

func TestDespachar_ExportDiagnostic_AlmacenNoCreadoCuentaCero(t *testing.T) {
	m := &Manejador{ConfigDir: t.TempDir()}

	resp := m.despachar(context.Background(), peticion{Action: "export_diagnostic"})

	if !resp.OK {
		t.Fatalf("export_diagnostic OK=false, error=%s", resp.Error)
	}
	resultado, ok := resp.Data.(resultadoDiagnostico)
	if !ok {
		t.Fatalf("data inesperado: %#v", resp.Data)
	}
	if resultado.TLSStore.State != "not_created" || resultado.TLSStore.ArtifactCount != 0 {
		t.Fatalf("almacén ausente inesperado: %#v", resultado.TLSStore)
	}
}

func TestDespachar_ExportDiagnostic_CanSignUsaMetadatoSinAbrirClave(t *testing.T) {
	t.Parallel()

	now := time.Now()
	usableKey := &stubClosingSigningKey{
		stubSigningKey: stubSigningKey{id: "usable-key"},
	}
	keys := &diagnosticKeyProviderStub{
		keys: map[string]ports.SigningKey{
			"usable": usableKey,
		},
		errors: map[string]error{
			"key-error": errors.New("clave no disponible"),
		},
	}
	m := &Manejador{
		ConfigDir: t.TempDir(),
		Catalogo: &stubCatalogo{
			certs: []domain.CertificateRef{
				{ID: "usable", NotAfter: now.Add(time.Hour), HasSigningKey: true},
				{ID: "key-error", NotAfter: now.Add(time.Hour)},
				{ID: "expired", NotAfter: now.Add(-time.Hour), HasSigningKey: true},
				{ID: "unknown-validity", HasSigningKey: true},
			},
		},
		Claves: keys,
	}

	resp := m.despachar(
		context.Background(),
		peticion{Action: "export_diagnostic"},
	)

	if !resp.OK {
		t.Fatalf("export_diagnostic OK=false, error=%s", resp.Error)
	}
	result, ok := resp.Data.(resultadoDiagnostico)
	if !ok {
		t.Fatalf("data inesperado: %#v", resp.Data)
	}
	if result.Certificates != 4 || result.CanSign != 1 {
		t.Fatalf("conteos inesperados: %#v", result)
	}
	if got := len(keys.requested); got != 0 {
		t.Fatalf("el diagnóstico no debe abrir claves ni pedir PIN, llamadas=%d", got)
	}
	if usableKey.closed != 0 {
		t.Fatalf("el diagnóstico abrió/cerró una clave: %d", usableKey.closed)
	}
}

func TestDespachar_ClearTlsTrust_RetiraConfianzaAntesDeArtefactosPropios(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tlsDir := filepath.Join(home, ".config", "grxfirma", "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	managed := tlsIPCManagedArtifactPaths(tlsDir)
	for _, path := range managed {
		if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", path, err)
		}
	}
	unrelated := filepath.Join(tlsDir, "raiz-ajena.crt.pem")
	if err := os.WriteFile(unrelated, []byte("unrelated"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(unrelated) error = %v", err)
	}

	removeCalled := false
	m := &Manejador{
		ConfigDir: filepath.Dir(tlsDir),
		tlsDeps: tlsIPCDependencies{
			removeManagedTrust: func(_ context.Context, rootFile string) error {
				removeCalled = true
				expected := filepath.Join(
					tlsDir,
					tlsIPCCertificatePrefix+"-root.crt.pem",
				)
				if rootFile != expected {
					t.Fatalf("rootFile = %q, want %q", rootFile, expected)
				}
				for _, path := range managed {
					if _, err := os.Lstat(path); err != nil {
						t.Fatalf(
							"la retirada se ejecutó después del borrado de %q: %v",
							path,
							err,
						)
					}
				}
				return nil
			},
		},
	}

	resp := m.despachar(
		context.Background(),
		peticion{Action: "clear_tls_trust"},
	)

	if !resp.OK || resp.Data != len(managed) {
		t.Fatalf("clear_tls_trust inesperado: %#v", resp)
	}
	if !removeCalled {
		t.Fatal("no se intentó retirar la confianza gestionada")
	}
	for _, path := range managed {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("el artefacto propio %q no fue eliminado: %v", path, err)
		}
	}
	if got, err := os.ReadFile(unrelated); err != nil ||
		string(got) != "unrelated" {
		t.Fatalf("se alteró el artefacto ajeno: %q, %v", got, err)
	}
}

func TestDespachar_ClearTlsTrust_FalloRetiradaPreservaArtefactos(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	managed := tlsIPCManagedArtifactPaths(tlsDir)
	for _, path := range managed {
		if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", path, err)
		}
	}

	m := &Manejador{
		ConfigDir: dir,
		tlsDeps: tlsIPCDependencies{
			removeManagedTrust: func(context.Context, string) error {
				return errors.New("almacén ocupado")
			},
		},
	}
	resp := m.despachar(
		context.Background(),
		peticion{Action: "clear_tls_trust"},
	)

	if resp.OK || resp.ErrorCode != "tls_trust_remove_failed" {
		t.Fatalf("clear_tls_trust debía fallar cerrado: %#v", resp)
	}
	for _, path := range managed {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "owned" {
			t.Fatalf("el fallo no preservó %q: %q, %v", path, got, err)
		}
	}
}

func TestDespachar_ClearTlsTrust_RechazaDirectorioTLSAliasSinRetirarConfianza(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	externalDir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.Symlink(externalDir, tlsDir); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}
	externalArtifact := filepath.Join(
		externalDir,
		tlsIPCCertificatePrefix+".key.pem",
	)
	if err := os.WriteFile(
		externalArtifact,
		[]byte("external"),
		0o600,
	); err != nil {
		t.Fatalf("os.WriteFile(externalArtifact) error = %v", err)
	}

	removeCalled := false
	m := &Manejador{
		ConfigDir: dir,
		tlsDeps: tlsIPCDependencies{
			removeManagedTrust: func(context.Context, string) error {
				removeCalled = true
				return nil
			},
		},
	}
	resp := m.despachar(
		context.Background(),
		peticion{Action: "clear_tls_trust"},
	)

	if resp.OK ||
		resp.ErrorCode != "tls_artifact_cleanup_failed" ||
		removeCalled {
		t.Fatalf("se aceptó un directorio TLS alias: %#v", resp)
	}
	if got, err := os.ReadFile(externalArtifact); err != nil ||
		string(got) != "external" {
		t.Fatalf("se alteró el destino externo: %q, %v", got, err)
	}
}

// ---------------------------------------------------------------------------
// Servicio
// ---------------------------------------------------------------------------

func TestDespachar_ServiceStatus_ServicioNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticion{Action: "service_status"})
	if resp.OK {
		t.Error("service_status sin servicio: esperaba OK=false")
	}
}

func TestDespachar_ServiceStatus_OK(t *testing.T) {
	t.Parallel()
	m := &Manejador{
		Servicio: &stubServicio{
			estado: ports.EstadoServicio{Instalado: true, Activo: true, Plataforma: "linux", Metodo: "systemd"},
		},
	}
	resp := m.despachar(context.Background(), peticion{Action: "service_status"})
	if !resp.OK {
		t.Errorf("service_status OK: esperaba OK=true, error=%s", resp.Error)
	}
}

func TestDespachar_InstallPublicRoots_OK(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	esperado := filepath.Join(
		dir,
		"tls",
		tlsIPCCertificatePrefix+"-root.crt.pem",
	)
	m := &Manejador{
		ConfigDir: dir,
		tlsDeps: tlsIPCDependencies{
			ensureBrowserCompatibleCertificate: func(
				certDir string,
				prefix string,
			) (string, string, string, string, error) {
				if certDir != filepath.Join(dir, "tls") ||
					prefix != tlsIPCCertificatePrefix {
					t.Fatalf(
						"generación TLS inesperada: dir=%q prefix=%q",
						certDir,
						prefix,
					)
				}
				return filepath.Join(certDir, prefix+".crt.pem"),
					filepath.Join(certDir, prefix+".key.pem"),
					filepath.Join(certDir, prefix+"-root.crt.pem"),
					"grxfirma-local-ca",
					nil
			},
			ensureManagedTrust: func(
				_ context.Context,
				certFile string,
			) error {
				if certFile != esperado {
					t.Fatalf("certFile = %q, want %q", certFile, esperado)
				}
				return nil
			},
		},
	}

	resp := m.despachar(context.Background(), peticion{Action: "install_public_roots"})
	if !resp.OK {
		t.Fatalf("install_public_roots OK=false, error=%s", resp.Error)
	}
	if got, ok := resp.Data.(string); !ok || got == "" {
		t.Fatalf("data inesperado: %#v", resp.Data)
	}
}

func TestDespachar_InstallPublicRoots_RechazaFuenteNoGestionada(t *testing.T) {
	t.Parallel()

	trustCalled := false
	dir := t.TempDir()
	m := &Manejador{
		ConfigDir: dir,
		tlsDeps: tlsIPCDependencies{
			ensureBrowserCompatibleCertificate: func(
				certDir string,
				prefix string,
			) (string, string, string, string, error) {
				return filepath.Join(certDir, prefix+".crt.pem"),
					filepath.Join(certDir, prefix+".key.pem"),
					filepath.Join(certDir, prefix+"-root.crt.pem"),
					"legacy-unmanaged",
					nil
			},
			ensureManagedTrust: func(context.Context, string) error {
				trustCalled = true
				return nil
			},
		},
	}

	resp := m.despachar(
		context.Background(),
		peticion{Action: "install_public_roots"},
	)

	if resp.OK ||
		resp.ErrorCode != "tls_certificate_prepare_failed" ||
		trustCalled {
		t.Fatalf("se aceptó una fuente TLS no gestionada: %#v", resp)
	}
}

func TestDespachar_InstallPublicRoots_FalloConfianzaUsaCodigoEstable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sensitiveError := "C:\\Users\\persona\\perfil-privado"
	m := &Manejador{
		ConfigDir: dir,
		tlsDeps: tlsIPCDependencies{
			ensureBrowserCompatibleCertificate: func(
				certDir string,
				prefix string,
			) (string, string, string, string, error) {
				return filepath.Join(certDir, prefix+".crt.pem"),
					filepath.Join(certDir, prefix+".key.pem"),
					filepath.Join(certDir, prefix+"-root.crt.pem"),
					"grxfirma-local-ca",
					nil
			},
			ensureManagedTrust: func(context.Context, string) error {
				return errors.New(sensitiveError)
			},
		},
	}

	resp := m.despachar(
		context.Background(),
		peticion{Action: "install_public_roots"},
	)
	wire, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal(resp) error = %v", err)
	}

	if resp.OK || resp.ErrorCode != "tls_trust_install_failed" {
		t.Fatalf("fallo de confianza inesperado: %#v", resp)
	}
	if strings.Contains(string(wire), sensitiveError) {
		t.Fatalf("el error TLS filtró una ruta local: %s", wire)
	}
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func TestDespachar_GetSettings_SettingsNil(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticion{Action: "get_settings"})
	// Con Settings nil devuelve mapa vacio (no es un error critico)
	if !resp.OK {
		t.Errorf("get_settings sin settings: esperaba OK=true (mapa vacio), error=%s", resp.Error)
	}
}

func TestDespachar_GetSettings_OK(t *testing.T) {
	t.Parallel()
	m := &Manejador{
		Settings: &stubSettings{datos: map[string]any{"tema": "oscuro"}},
	}
	resp := m.despachar(context.Background(), peticion{Action: "get_settings"})
	if !resp.OK {
		t.Errorf("get_settings OK: error=%s", resp.Error)
	}
}

func TestDespachar_GetSettings_UsaDocumentoTipadoSiDisponible(t *testing.T) {
	t.Parallel()
	idioma := "es"
	expertMode := true
	stub := &stubTypedSettings{
		doc: ports.DocumentoConfiguracionUsuario{
			General: ports.ConfiguracionUsuarioGeneral{
				Idioma:     &idioma,
				ExpertMode: &expertMode,
			},
			Extras: map[string]any{"tema": "oscuro"},
		},
	}
	m := &Manejador{Settings: stub}

	resp := m.despachar(context.Background(), peticion{Action: "get_settings"})
	if !resp.OK {
		t.Fatalf("get_settings tipado: error=%s", resp.Error)
	}
	if !stub.usedLoadDoc {
		t.Fatal("se esperaba uso de CargarDocumento")
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("data inesperada: %#v", resp.Data)
	}
	if data["idioma"] != "es" || data["expertMode"] != true || data["tema"] != "oscuro" {
		t.Fatalf("mapa legacy proyectado inesperado: %#v", data)
	}
}

func TestDespachar_SaveSettings_UsaDocumentoTipadoSiDisponible(t *testing.T) {
	t.Parallel()
	stub := &stubTypedSettings{}
	m := &Manejador{Settings: stub}

	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"idioma":                "en",
		"signAction":            "sign",
		"defaultHashFormatFile": "hex",
		"tema":                  "oscuro",
	}))
	if !resp.OK {
		t.Fatalf("save_settings tipado: error=%s", resp.Error)
	}
	if !stub.usedSaveDoc {
		t.Fatal("se esperaba uso de GuardarDocumento")
	}
	if stub.lastSavedDoc.General.Idioma == nil || *stub.lastSavedDoc.General.Idioma != "en" {
		t.Fatalf("idioma tipado inesperado: %#v", stub.lastSavedDoc.General.Idioma)
	}
	if stub.lastSavedDoc.Firma.Action == nil || *stub.lastSavedDoc.Firma.Action != "sign" {
		t.Fatalf("signAction tipado inesperado: %#v", stub.lastSavedDoc.Firma.Action)
	}
	if stub.lastSavedDoc.Hash.FormatFile == nil || *stub.lastSavedDoc.Hash.FormatFile != "hex" {
		t.Fatalf("defaultHashFormatFile tipado inesperado: %#v", stub.lastSavedDoc.Hash.FormatFile)
	}
	if stub.lastSavedDoc.Extras["tema"] != "oscuro" {
		t.Fatalf("extras inesperados: %#v", stub.lastSavedDoc.Extras)
	}
}

func TestDespachar_SaveSettings_DescartaCredencialSeguridadUIEnClaro(t *testing.T) {
	t.Parallel()
	stub := &stubTypedSettings{}
	m := &Manejador{Settings: stub}

	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"idioma":                 "es",
		"securityAccessPassword": "secreto-local",
	}))
	if !resp.OK {
		t.Fatalf("save_settings: error=%s", resp.Error)
	}
	if !stub.usedSaveDoc {
		t.Fatal("se esperaba uso de GuardarDocumento")
	}
	if _, presente := stub.lastSavedDoc.Extras["securityAccessPassword"]; presente {
		t.Fatalf("securityAccessPassword no debe entrar en Extras: %#v", stub.lastSavedDoc.Extras)
	}
	if _, presente := stub.lastSavedDoc.Mapa()["securityAccessPassword"]; presente {
		t.Fatalf("securityAccessPassword no debe proyectarse al mapa persistible: %#v", stub.lastSavedDoc.Mapa())
	}
}

func TestDespachar_SaveSettings_RechazaCredencialesProxyEnClaro(t *testing.T) {
	t.Parallel()
	stub := &stubTypedSettings{}
	m := &Manejador{Settings: stub}

	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"proxyUsername": "usuario",
		"proxyPassword": "secreta",
	}))
	if resp.OK {
		t.Fatal("save_settings debio rechazar credenciales de proxy en claro")
	}
	if !strings.Contains(resp.Error, "proxy") {
		t.Fatalf("error inesperado: %s", resp.Error)
	}
	if stub.usedSaveDoc {
		t.Fatal("no debio persistir documento tipado tras rechazo")
	}
}

func TestDespachar_SaveSettings_RechazaCaducidadWebInsegura(t *testing.T) {
	t.Parallel()
	stub := &stubTypedSettings{}
	m := &Manejador{Settings: stub}

	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"webCompatibilityDurationMinutes": 241,
	}))
	if resp.OK {
		t.Fatal("save_settings debio rechazar una caducidad web fuera de rango")
	}
	if !strings.Contains(resp.Error, "webCompatibilityDurationMinutes") {
		t.Fatalf("error inesperado: %s", resp.Error)
	}
	if stub.usedSaveDoc {
		t.Fatal("no debio persistir preferencias tras el rechazo")
	}
}

func TestDespachar_SaveSettings_PersisteCaducidadWebTipada(t *testing.T) {
	t.Parallel()
	stub := &stubTypedSettings{}
	m := &Manejador{Settings: stub}

	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"webCompatibilityDurationMinutes": 30,
	}))
	if !resp.OK {
		t.Fatalf("save_settings: %s", resp.Error)
	}
	if got := stub.lastSavedDoc.Desktop.WebCompatibilityDurationMinutes; got == nil || *got != 30 {
		t.Fatalf("duracion tipada inesperada: %#v", got)
	}
}

func TestDespachar_ProxySecretStoreStatus_OK(t *testing.T) {
	t.Parallel()
	m := &Manejador{
		ProxySecrets: &stubProxySecretStore{
			status: ports.ProxySecretStoreStatus{
				Available: true,
				Platform:  "linux",
				Backend:   "secret-service",
			},
		},
	}
	resp := m.despachar(context.Background(), peticion{Action: "proxy_secret_store_status"})
	if !resp.OK {
		t.Fatalf("proxy_secret_store_status error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoEstadoProxySecretStore)
	if !ok {
		t.Fatalf("data inesperada: %T", resp.Data)
	}
	if !data.Available || data.Platform != "linux" || data.Backend != "secret-service" {
		t.Fatalf("estado inesperado: %#v", data)
	}
}

func TestDespachar_ProxySecretStoreStatus_NoConfigurado(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despachar(context.Background(), peticion{Action: "proxy_secret_store_status"})
	if !resp.OK {
		t.Fatalf("proxy_secret_store_status no configurado error=%s", resp.Error)
	}
	data, ok := resp.Data.(resultadoEstadoProxySecretStore)
	if !ok {
		t.Fatalf("data inesperada: %T", resp.Data)
	}
	if data.Available || data.Backend != "none" || data.Reason != "proxy secret store no configurado" {
		t.Fatalf("estado inesperado: %#v", data)
	}
}

func TestDespachar_ProxySecretStore_RotaTrasPersistirReferenciaNueva(t *testing.T) {
	t.Parallel()
	events := []string{}
	oldID := "secret-old"
	oldRealm := "old-realm"
	settings := &transactionalSettingsStub{
		doc: ports.DocumentoConfiguracionUsuario{
			Proxy: ports.ConfiguracionUsuarioProxy{SecretID: &oldID, Realm: &oldRealm},
		},
		events: &events,
	}
	store := &stubProxySecretStore{
		descriptor: ports.ProxySecretDescriptor{ID: "secret-new"},
		events:     &events,
	}
	m := &Manejador{Settings: settings, ProxySecrets: store}
	raw := json.RawMessage(`{"realm":"corp-proxy","username":" alberto ","password":"c2VjcmV0YQ=="}`)

	resp := m.despachar(context.Background(), peticion{Action: "proxy_secret_store", Params: raw})
	if !resp.OK {
		t.Fatalf("proxy_secret_store error=%s", resp.Error)
	}
	if got := strings.Join(events, ","); got != "store,save,delete:secret-old" {
		t.Fatalf("orden transaccional inesperado: %s", got)
	}
	if settings.doc.Proxy.SecretID == nil || *settings.doc.Proxy.SecretID != "secret-new" ||
		settings.doc.Proxy.Realm == nil || *settings.doc.Proxy.Realm != "corp-proxy" {
		t.Fatalf("settings proxy inesperado: %#v", settings.doc.Proxy)
	}
	if store.storedRealm != "corp-proxy" || store.stored.Username != "alberto" ||
		string(store.stored.Password) != "secreta" {
		t.Fatalf("material entregado al store inesperado: realm=%q username=%q", store.storedRealm, store.stored.Username)
	}
	data, ok := resp.Data.(resultadoCredencialProxy)
	if !ok || !data.Configured || !data.Rotated || data.Realm != "corp-proxy" || data.Username != "alberto" {
		t.Fatalf("resultado inesperado: %#v", resp.Data)
	}
	for i, b := range raw {
		if b != 0 {
			t.Fatalf("params sensibles no zeroizados en byte %d", i)
		}
	}
}

func TestDespachar_ProxySecretStore_RollbackSiFallaSettings(t *testing.T) {
	t.Parallel()
	oldID := "secret-old"
	settings := &transactionalSettingsStub{
		doc:     ports.DocumentoConfiguracionUsuario{Proxy: ports.ConfiguracionUsuarioProxy{SecretID: &oldID}},
		saveErr: errors.New("disk full"),
	}
	store := &stubProxySecretStore{descriptor: ports.ProxySecretDescriptor{ID: "secret-new"}}
	m := &Manejador{Settings: settings, ProxySecrets: store}

	resp := m.despachar(context.Background(), peticionJSON(t, "proxy_secret_store", map[string]any{
		"realm": "corp-proxy", "username": "alberto", "password": []byte("secreta"),
	}))
	if resp.OK {
		t.Fatal("proxy_secret_store debió fallar al persistir settings")
	}
	if got := strings.Join(store.deletedIDs, ","); got != "secret-new" {
		t.Fatalf("rollback debía retirar solo el secreto nuevo, got=%q", got)
	}
	if settings.doc.Proxy.SecretID == nil || *settings.doc.Proxy.SecretID != oldID {
		t.Fatalf("la referencia anterior debía conservarse: %#v", settings.doc.Proxy)
	}
	if strings.Contains(resp.Error, "secreta") || strings.Contains(resp.Error, "secret-new") {
		t.Fatalf("el error expone material sensible: %q", resp.Error)
	}
}

func TestDespachar_ProxySecretStore_RestauraReferenciaSiFallaRotacion(t *testing.T) {
	t.Parallel()
	oldID := "secret-old"
	realm := "old-realm"
	settings := &transactionalSettingsStub{
		doc: ports.DocumentoConfiguracionUsuario{
			Proxy: ports.ConfiguracionUsuarioProxy{SecretID: &oldID, Realm: &realm},
		},
	}
	store := &stubProxySecretStore{
		descriptor: ports.ProxySecretDescriptor{ID: "secret-new"},
		deleteErr:  errors.New("keyring locked"),
	}
	m := &Manejador{Settings: settings, ProxySecrets: store}

	resp := m.despachar(context.Background(), peticionJSON(t, "proxy_secret_store", map[string]any{
		"realm": "corp-proxy", "username": "alberto", "password": []byte("secreta"),
	}))
	if resp.OK {
		t.Fatal("la rotación incompleta no debe anunciar éxito")
	}
	if settings.doc.Proxy.SecretID == nil || *settings.doc.Proxy.SecretID != oldID ||
		settings.doc.Proxy.Realm == nil || *settings.doc.Proxy.Realm != realm {
		t.Fatalf("la referencia anterior debía restaurarse: %#v", settings.doc.Proxy)
	}
	if got := strings.Join(store.deletedIDs, ","); got != "secret-old,secret-new" {
		t.Fatalf("debía intentar retirar secreto antiguo y rollback del nuevo, got=%q", got)
	}
}

func TestDespachar_ProxySecretStore_RechazaDatosSinInvocarBackend(t *testing.T) {
	t.Parallel()
	settings := &transactionalSettingsStub{}
	store := &stubProxySecretStore{descriptor: ports.ProxySecretDescriptor{ID: "secret-new"}}
	m := &Manejador{Settings: settings, ProxySecrets: store}

	resp := m.despachar(context.Background(), peticionJSON(t, "proxy_secret_store", map[string]any{
		"realm": "corp\nproxy", "username": "alberto", "password": []byte("secreta"),
	}))
	if resp.OK {
		t.Fatal("proxy_secret_store debió rechazar controles en realm")
	}
	if store.storedRealm != "" || settings.saveCalls != 0 {
		t.Fatal("una entrada inválida no debe llegar al store ni a settings")
	}
}

func TestDespachar_ProxySecretDelete_LimpiaSettingsAntesDelStore(t *testing.T) {
	t.Parallel()
	events := []string{}
	oldID := "secret-old"
	realm := "corp-proxy"
	settings := &transactionalSettingsStub{
		doc: ports.DocumentoConfiguracionUsuario{
			Proxy: ports.ConfiguracionUsuarioProxy{SecretID: &oldID, Realm: &realm},
		},
		events: &events,
	}
	store := &stubProxySecretStore{events: &events}
	m := &Manejador{Settings: settings, ProxySecrets: store}

	resp := m.despachar(context.Background(), peticion{Action: "proxy_secret_delete"})
	if !resp.OK {
		t.Fatalf("proxy_secret_delete error=%s", resp.Error)
	}
	if got := strings.Join(events, ","); got != "save,delete:secret-old" {
		t.Fatalf("orden transaccional inesperado: %s", got)
	}
	if settings.doc.Proxy.SecretID != nil || settings.doc.Proxy.Realm != nil {
		t.Fatalf("la referencia debía eliminarse: %#v", settings.doc.Proxy)
	}
}

func TestDespachar_ProxySecretDelete_RestauraReferenciaSiFallaStore(t *testing.T) {
	t.Parallel()
	oldID := "secret-old"
	realm := "corp-proxy"
	settings := &transactionalSettingsStub{
		doc: ports.DocumentoConfiguracionUsuario{
			Proxy: ports.ConfiguracionUsuarioProxy{SecretID: &oldID, Realm: &realm},
		},
	}
	store := &stubProxySecretStore{deleteErr: errors.New("keyring locked")}
	m := &Manejador{Settings: settings, ProxySecrets: store}

	resp := m.despachar(context.Background(), peticion{Action: "proxy_secret_delete"})
	if resp.OK {
		t.Fatal("proxy_secret_delete no debe anunciar éxito si el store no elimina")
	}
	if settings.doc.Proxy.SecretID == nil || *settings.doc.Proxy.SecretID != oldID ||
		settings.doc.Proxy.Realm == nil || *settings.doc.Proxy.Realm != realm {
		t.Fatalf("la referencia debía restaurarse para reintentar: %#v", settings.doc.Proxy)
	}
}

func TestDespachar_SaveSettings_ProtegeReferenciaProxyGestionada(t *testing.T) {
	t.Parallel()
	oldID := "secret-old"
	realm := "corp-proxy"
	settings := &transactionalSettingsStub{
		doc: ports.DocumentoConfiguracionUsuario{
			Proxy: ports.ConfiguracionUsuarioProxy{SecretID: &oldID, Realm: &realm},
		},
	}
	m := &Manejador{Settings: settings}

	resp := m.despachar(context.Background(), peticionJSON(t, "save_settings", map[string]any{
		"idioma":        "en",
		"proxySecretId": "attacker-selected",
		"proxyRealm":    "attacker-realm",
	}))
	if !resp.OK {
		t.Fatalf("save_settings error=%s", resp.Error)
	}
	if settings.doc.Proxy.SecretID == nil || *settings.doc.Proxy.SecretID != oldID ||
		settings.doc.Proxy.Realm == nil || *settings.doc.Proxy.Realm != realm {
		t.Fatalf("save_settings alteró referencia protegida: %#v", settings.doc.Proxy)
	}
}

// ---------------------------------------------------------------------------
// despacharConTimeout — verifica que no bloquea ni entra en panic
// ---------------------------------------------------------------------------

func TestDespacharConTimeout_Ping(t *testing.T) {
	t.Parallel()
	m := manejadorVacio()
	resp := m.despacharConTimeout(context.Background(), peticion{Action: "ping"})
	if !resp.OK {
		t.Errorf("despacharConTimeout ping: OK=false, error=%s", resp.Error)
	}
}

func generarDestinatarioAuthEnvelopedIPC(t *testing.T, id string) domain.ProtectionRecipient {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(98),
		Subject:               pkix.Name{CommonName: "Destinatario AuthEnvelopedData IPC"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &privateKey.PublicKey, privateKey,
	)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey() error = %v", err)
	}
	return domain.ProtectionRecipient{
		ID:                     id,
		Label:                  "Destinatario AuthEnvelopedData IPC",
		RSAOAEP256PublicKeyDER: publicKeyDER,
		CertificateDER:         certificateDER,
	}
}

func TestConstruirOpcionesFirmaIPC_PropagaSellosPorPagina(t *testing.T) {
	seal := map[string]any{
		"page": "all", "pageWidth": 612.0, "pageHeight": 792.0,
		"placements": []any{
			map[string]any{"page": 1, "rect": map[string]any{"x": 0.1, "y": 0.2, "w": 0.3, "h": 0.1}, "rotation": 0},
			map[string]any{"page": 2, "rect": map[string]any{"x": 0.4, "y": 0.1, "w": 0.2, "h": 0.2}, "rotation": 90},
		},
	}
	opciones, err := construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", nil, "pades")
	if err != nil {
		t.Fatal(err)
	}
	if got := opciones["visibleSealPlacements"]; !strings.Contains(got, `"page":1`) || !strings.Contains(got, `"rotation":90`) {
		t.Fatalf("lista IPC no propagada: %s", got)
	}
	seal["placements"] = []any{}
	if _, err := construirOpcionesFirmaIPCBase(false, false, seal, "", "", "", "", nil, "pades"); err == nil {
		t.Fatal("se aceptó una lista vacía")
	}
}
