// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/inbound/legacy/legacycrypto"
	desktopdocumentpicker "grxfirma/internal/adapters/outbound/desktop/documentpicker"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/presentation/desktop/certpicker"
)

// GetTempPath2 usa SystemTemp para SYSTEM, no TMP/TEMP. Mantener los cuatro
// selectores acotados al proceso de prueba evita considerar el temporal global
// (que también contiene los fixtures "fuera") como una raíz autorizada.
func setLegacyTestTempDir(t *testing.T, dir string) {
	t.Helper()
	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(absDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "SystemTemp"} {
		t.Setenv(name, absDir)
	}
	if got := os.TempDir(); !sameTestPath(got, absDir) {
		t.Fatalf("os.TempDir() = %q, want isolated fixture %q", got, absDir)
	}
}

func requireLegacyTestOutsideScope(t *testing.T, path string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{home, os.TempDir()} {
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatal(err)
		}
		if isPathWithinAllowedRoots(path, root, resolvedRoot) {
			t.Fatalf("negative fixture %q is inside an allowed root %q", path, root)
		}
	}
}

type signExecutorStub struct {
	result application.SignResult
	err    error
	cmd    application.SignCommand
}

func (s *signExecutorStub) Execute(_ context.Context, cmd application.SignCommand) (application.SignResult, error) {
	s.cmd = cmd
	return s.result, s.err
}

type batchExecutorStub struct {
	result application.BatchResult
	err    error
	cmd    *application.ProcessBatchCommand
}

func (b batchExecutorStub) Execute(_ context.Context, cmd application.ProcessBatchCommand) (application.BatchResult, error) {
	if b.cmd != nil {
		*b.cmd = cmd
	}
	return b.result, b.err
}

type remoteBatchExecutorStub struct {
	result string
	err    error
	called *int
}

func (b remoteBatchExecutorStub) ExecuteLegacyRemoteResult(context.Context, afirmauri.RemoteBatchCommand) (string, error) {
	if b.called != nil {
		(*b.called)++
	}
	return b.result, b.err
}

type userApprovalStub struct {
	approved bool
	err      error
	calls    int
}

func (a *userApprovalStub) Request(context.Context, string) (bool, error) {
	a.calls++
	return a.approved, a.err
}

type certificateCatalogStub struct {
	certs []domain.CertificateRef
	err   error
}

func (c certificateCatalogStub) List(context.Context) ([]domain.CertificateRef, error) {
	return c.certs, c.err
}

type signingKeyStub struct {
	id    string
	chain [][]byte
}

func (k signingKeyStub) KeyID() string { return k.id }
func (k signingKeyStub) CertificateChainDER() [][]byte {
	return k.chain
}

type keyProviderStub struct {
	key ports.SigningKey
	err error
}

func (k keyProviderStub) KeyFor(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
	return k.key, k.err
}

type keyProviderFunc func(context.Context, domain.CertificateRef) (ports.SigningKey, error)

func (f keyProviderFunc) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	return f(ctx, ref)
}

type documentPickerStub struct {
	doc domain.Document
	err error
}

func (d documentPickerStub) Pick(context.Context) (domain.Document, error) {
	return d.doc, d.err
}

type certSelectorStub struct {
	result certpicker.ResultadoSeleccion
	err    error
}

func (c certSelectorStub) Select(context.Context, []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
	return c.result, c.err
}

func TestResolveLegacySignPayload_ManifestXMLActualizaSolicitud(t *testing.T) {
	t.Parallel()

	props := base64.StdEncoding.EncodeToString([]byte("profile=baseline\nmode=implicit"))
	raw := []byte(`<sign>` +
		`<e k="dat" v="QUJDREVGRw=="></e>` +
		`<e k="format" v="PAdES"></e>` +
		`<e k="key" v="clave1234"></e>` +
		`<e k="id" v="req-xml"></e>` +
		`<e k="properties" v="` + props + `"></e>` +
		`</sign>`)

	solicitud := afirmauri.Solicitud{
		Formato: domain.FormatCAdES,
		Sesion: domain.ExchangeSession{
			RequestID:  "req-original",
			SessionKey: "clave-original",
		},
		Options: map[string]string{
			"existing": "yes",
		},
	}

	payload, actualizada, err := resolveLegacySignPayload(raw, solicitud)
	if err != nil {
		t.Fatalf("resolveLegacySignPayload() error = %v", err)
	}
	if got := string(payload); got != "ABCDEFG" {
		t.Fatalf("payload = %q, want %q", got, "ABCDEFG")
	}
	if actualizada.Formato != domain.FormatPAdES {
		t.Fatalf("formato = %q, want %q", actualizada.Formato, domain.FormatPAdES)
	}
	if actualizada.Sesion.SessionKey != "clave1234" {
		t.Fatalf("session key = %q, want %q", actualizada.Sesion.SessionKey, "clave1234")
	}
	if actualizada.Sesion.RequestID != "req-xml" {
		t.Fatalf("requestID = %q, want %q", actualizada.Sesion.RequestID, "req-xml")
	}
	if actualizada.Options["profile"] != "baseline" {
		t.Fatalf("profile = %q, want %q", actualizada.Options["profile"], "baseline")
	}
	if actualizada.Options["mode"] != "implicit" {
		t.Fatalf("mode = %q, want %q", actualizada.Options["mode"], "implicit")
	}
	if actualizada.Options["existing"] != "yes" {
		t.Fatalf("existing = %q, want %q", actualizada.Options["existing"], "yes")
	}
}

func TestProveedorClavesAgregado_NoOcultaErroresDeFuente(t *testing.T) {
	t.Parallel()

	ref := domain.CertificateRef{ID: "cert-1", Fingerprint: "fp-1"}
	proveedor := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{
		keyProviderFunc(func(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
			return nil, errors.New("nssstore: pk12util fallo")
		}),
		keyProviderFunc(func(context.Context, domain.CertificateRef) (ports.SigningKey, error) {
			return nil, errors.New("clave no encontrada en p12")
		}),
	}}

	_, err := proveedor.KeyFor(context.Background(), ref)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "clave de firma no disponible") {
		t.Fatalf("error sin contexto de clave de firma: %s", msg)
	}
	if !strings.Contains(msg, "nssstore: pk12util fallo") || !strings.Contains(msg, "clave no encontrada en p12") {
		t.Fatalf("se han perdido errores de fuente: %s", msg)
	}
}

func TestMaterializarCertificado_ErrorClaveSeTipificaComoFirma(t *testing.T) {
	t.Parallel()

	h := &legacyWebSocketHandler{
		keys: keyProviderStub{err: errors.New("clave no encontrada")},
	}
	_, _, err := h.materializarCertificado(context.Background(), domain.CertificateRef{ID: "cert-1", NotAfter: time.Now().Add(time.Hour)})
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if !strings.Contains(err.Error(), "clave de firma no disponible") {
		t.Fatalf("error no tipificado como clave de firma: %s", err)
	}
}

func TestResolveLegacySignPayload_SinManifestDevuelveContenidoOriginal(t *testing.T) {
	t.Parallel()

	solicitud := afirmauri.Solicitud{
		Formato: domain.FormatCAdES,
		Sesion: domain.ExchangeSession{
			RequestID:  "req-original",
			SessionKey: "clave-original",
		},
	}

	payload, actualizada, err := resolveLegacySignPayload([]byte("documento-plano"), solicitud)
	if err != nil {
		t.Fatalf("resolveLegacySignPayload() error = %v", err)
	}
	if got := string(payload); got != "documento-plano" {
		t.Fatalf("payload = %q, want %q", got, "documento-plano")
	}
	if actualizada.Formato != solicitud.Formato {
		t.Fatalf("formato = %q, want %q", actualizada.Formato, solicitud.Formato)
	}
	if actualizada.Sesion.SessionKey != solicitud.Sesion.SessionKey {
		t.Fatalf("session key = %q, want %q", actualizada.Sesion.SessionKey, solicitud.Sesion.SessionKey)
	}
}

func TestExpandLegacyProtocolSignOptions_ExpPolicyAGE18ParaXAdES(t *testing.T) {
	t.Parallel()

	params := url.Values{
		"properties": []string{base64.StdEncoding.EncodeToString([]byte("expPolicy=FirmaAGE18\nformat=XAdES Detached\nincludeOnlySigningCertificate=true\n"))},
	}
	opts := expandLegacyProtocolSignOptions(nil, params, "XAdES")
	if got := opts["policyIdentifier"]; got != "urn:oid:2.16.724.1.3.1.1.2.1.8" {
		t.Fatalf("policyIdentifier inesperado: %#v", got)
	}
	if got := opts["policyIdentifierHash"]; got != "V8lVVNGDCPen6VELRD1Ja8HARFk=" {
		t.Fatalf("policyIdentifierHash inesperado: %#v", got)
	}
	if got := opts["xadesNamespace"]; got != "http://uri.etsi.org/01903/v1.2.2#" {
		t.Fatalf("xadesNamespace inesperado: %#v", got)
	}
	if got := opts["signedPropertiesTypeUrl"]; got != "http://uri.etsi.org/01903/v1.2.2#SignedProperties" {
		t.Fatalf("signedPropertiesTypeUrl inesperado: %#v", got)
	}
}

func TestLegacyWebSocketHandler_SaveYLoadLocales(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)

	handler := &legacyWebSocketHandler{savePicker: acceptLegacySaveDefaultForTest}
	saveReq := afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("contenido-demo")),
			"filename": "demo.txt",
		}),
	}
	if got := handler.handleSave(context.Background(), saveReq); got != "SAVE_OK" {
		t.Fatalf("handleSave() = %q, want SAVE_OK", got)
	}

	savedPath := filepath.Join(home, "Descargas", "demo.txt")
	raw, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("ReadFile(savedPath): %v", err)
	}
	if got := string(raw); got != "contenido-demo" {
		t.Fatalf("contenido guardado = %q, want %q", got, "contenido-demo")
	}

	loadReq := afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
		LegacyParams: mapToValues(map[string]string{
			"filePath": savedPath,
		}),
	}
	got := handler.handleLoad(context.Background(), loadReq)
	want := "demo.txt:" + base64.StdEncoding.EncodeToString([]byte("contenido-demo"))
	if got != want {
		t.Fatalf("handleLoad() = %q, want %q", got, want)
	}

}

func TestLegacyWebSocketHandler_LoadRechazaRutaDirectaPorDefecto(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	path := filepath.Join(home, "secreto.txt")
	if err := os.WriteFile(path, []byte("secreto"), 0o600); err != nil {
		t.Fatalf("WriteFile(path): %v", err)
	}

	resp := (&legacyWebSocketHandler{}).handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
		LegacyParams: mapToValues(map[string]string{
			"filePath": path,
		}),
	})
	if resp != "SAF_25: Las rutas directas del protocolo están deshabilitadas" {
		t.Fatalf("handleLoad() = %q, want rechazo de ruta directa", resp)
	}
}

func TestLegacyWebSocketHandler_SaveRechazaRutaDirectaPorDefecto(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	target := filepath.Join(home, "victima.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}

	resp := (&legacyWebSocketHandler{}).handleSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("sobrescrito")),
			"filePath": target,
		}),
	})
	if resp != "SAF_05: No se pudo determinar ruta de guardado" {
		t.Fatalf("handleSave() = %q, want rechazo de ruta directa", resp)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target): %v", err)
	}
	if string(raw) != "original" {
		t.Fatalf("contenido de destino = %q, want original", raw)
	}
}

func TestLegacyWebSocketHandler_LoadInteractivoSinRuta(t *testing.T) {
	t.Parallel()

	documento, err := domain.NewDocument("origen.pdf", []byte("%PDF-cargado"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}

	handler := &legacyWebSocketHandler{
		documentos: documentPickerStub{doc: documento},
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return nil, errLegacyLoadPickerUnavailable
		},
	}

	got := handler.handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
	})
	want := "origen.pdf:" + base64.StdEncoding.EncodeToString([]byte("%PDF-cargado"))
	if got != want {
		t.Fatalf("handleLoad() = %q, want %q", got, want)
	}

}

func TestLegacyWebSocketHandler_LoadUsaBase64EstandarDeAutoScript(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	content := []byte{0xfb, 0xff}
	wantPayload := base64.StdEncoding.EncodeToString(content)
	if wantPayload == base64.URLEncoding.EncodeToString(content) {
		t.Fatal("el fixture no distingue Base64 estándar de Base64 URL-safe")
	}

	documento, err := domain.NewDocument("selector.bin", content, "application/octet-stream")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}
	fromDocumentPicker := (&legacyWebSocketHandler{
		documentos: documentPickerStub{doc: documento},
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return nil, errLegacyLoadPickerUnavailable
		},
	}).handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad})
	if want := "selector.bin:" + wantPayload; fromDocumentPicker != want {
		t.Fatalf("load mediante DocumentPicker = %q, want %q", fromDocumentPicker, want)
	}

	selectedPath := filepath.Join(home, "seleccion.bin")
	if err := os.WriteFile(selectedPath, content, 0o600); err != nil {
		t.Fatalf("WriteFile(selectedPath): %v", err)
	}
	fromLegacyPicker := (&legacyWebSocketHandler{
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return []string{selectedPath}, nil
		},
	}).handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad})
	if want := "seleccion.bin:" + wantPayload; fromLegacyPicker != want {
		t.Fatalf("load mediante selector legacy = %q, want %q", fromLegacyPicker, want)
	}
}

func TestLegacyWebSocketHandler_LoadInteractivoCanceladoDevuelveCancel(t *testing.T) {
	handler := &legacyWebSocketHandler{
		documentos: documentPickerStub{err: desktopdocumentpicker.ErrSeleccionCancelada},
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return nil, errLegacyLoadCanceled
		},
	}

	got := handler.handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
	})
	if got != "CANCEL" {
		t.Fatalf("handleLoad() = %q, want CANCEL", got)
	}
}

func TestLegacyWebSocketHandler_LoadContextoCanceladoNoActivaFallback(t *testing.T) {
	documento, err := domain.NewDocument("no-debe-cargarse.txt", []byte("fallback"), "text/plain")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}
	handler := &legacyWebSocketHandler{
		documentos: documentPickerStub{doc: documento},
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return nil, fmt.Errorf("selector cerrado: %w", context.Canceled)
		},
	}

	got := handler.handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad})
	if got != "CANCEL" {
		t.Fatalf("handleLoad() = %q, want CANCEL", got)
	}
}

func TestLegacyWebSocketHandler_LoadSeleccionVaciaEsCancelacion(t *testing.T) {
	handler := &legacyWebSocketHandler{
		documentos: documentPickerStub{err: errors.New("no debe invocarse")},
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return nil, nil
		},
	}

	got := handler.handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad})
	if got != "CANCEL" {
		t.Fatalf("handleLoad() = %q, want CANCEL", got)
	}
}

func TestLegacyWebSocketHandler_LoadRechazaNombreConSeparadorDeProtocolo(t *testing.T) {
	for _, name := range []string{"firma|final.pdf", "firma:final.pdf"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			setTestUserHome(t, home)
			selected := filepath.Join(home, name)
			handler := &legacyWebSocketHandler{
				loadPicker: func(context.Context, string, string, bool) ([]string, error) {
					return []string{selected}, nil
				},
			}

			got := handler.handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad})
			if got != "SAF_25: Nombre de fichero no representable en el protocolo" {
				t.Fatalf("handleLoad() = %q, want rechazo explícito del separador", got)
			}
		})
	}
}

func TestLegacyWebSocketHandler_LoadErrorSelectorDevuelveSAF25(t *testing.T) {
	handler := &legacyWebSocketHandler{
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return nil, errors.New("fallo selector")
		},
	}
	got := handler.handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
	})
	if got != "SAF_25: No se pudo cargar el fichero" {
		t.Fatalf("handleLoad() = %q, want SAF_25", got)
	}
}

func TestLegacyWebSocketHandler_LoadInteractivoMultiseleccion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	primero := filepath.Join(home, "Descargas", "uno.pdf")
	segundo := filepath.Join(home, "Descargas", "dos.xml")
	if err := os.MkdirAll(filepath.Dir(primero), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.WriteFile(primero, []byte("pdf"), 0o600); err != nil {
		t.Fatalf("WriteFile(primero): %v", err)
	}
	if err := os.WriteFile(segundo, []byte("xml"), 0o600); err != nil {
		t.Fatalf("WriteFile(segundo): %v", err)
	}

	handler := &legacyWebSocketHandler{
		loadPicker: func(_ context.Context, initialPath string, exts string, multi bool) ([]string, error) {
			if !multi {
				t.Fatalf("multi = %v, want true", multi)
			}
			if exts != "pdf,xml" {
				t.Fatalf("exts = %q, want %q", exts, "pdf,xml")
			}
			return []string{primero, segundo}, nil
		},
	}
	got := handler.handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
		LegacyParams: mapToValues(map[string]string{
			"multiLoad": "true",
			"exts":      "pdf,xml",
		}),
	})
	want := strings.Join([]string{
		"uno.pdf:" + base64.StdEncoding.EncodeToString([]byte("pdf")),
		"dos.xml:" + base64.StdEncoding.EncodeToString([]byte("xml")),
	}, "|")
	if got != want {
		t.Fatalf("handleLoad() = %q, want %q", got, want)
	}
}

func TestLegacyWebSocketHandler_IsolatedRootsKeepHomeAndTempButRejectSiblings(t *testing.T) {
	root := t.TempDir()
	home, temp := filepath.Join(root, "home"), filepath.Join(root, "temp")
	setTestUserHome(t, home)
	setLegacyTestTempDir(t, temp)
	for _, item := range []struct {
		dir     string
		allowed bool
	}{
		{home, true},
		{temp, true},
		{filepath.Join(root, "home-sibling"), false},
		{filepath.Join(root, "temp-sibling"), false},
	} {
		if err := os.MkdirAll(item.dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(item.dir, "synthetic.txt")
		if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
		if !item.allowed {
			requireLegacyTestOutsideScope(t, path)
		}
		if _, ok := resolveLegacyReadablePath(path); ok != item.allowed {
			t.Errorf("read %q allowed=%v, want %v", path, ok, item.allowed)
		}
		if _, ok := resolveLegacyWritablePath(path); ok != item.allowed {
			t.Errorf("write %q allowed=%v, want %v", path, ok, item.allowed)
		}
	}
}

func TestLegacyWebSocketHandler_LoadAdmiteHomeConAliasDelSistema(t *testing.T) {
	realHome := t.TempDir()
	aliasParent := t.TempDir()
	aliasHome := filepath.Join(aliasParent, "home-alias")
	if err := os.Symlink(realHome, aliasHome); err != nil {
		t.Skipf("el entorno no permite crear un alias del home: %v", err)
	}
	setTestUserHome(t, aliasHome)
	setLegacyTestTempDir(t, filepath.Join(aliasHome, "tmp"))

	selectedPath := filepath.Join(aliasHome, "seleccion.txt")
	if err := os.WriteFile(selectedPath, []byte("contenido"), 0o600); err != nil {
		t.Fatalf("WriteFile(selectedPath): %v", err)
	}
	handler := &legacyWebSocketHandler{
		loadPicker: func(context.Context, string, string, bool) ([]string, error) {
			return []string{selectedPath}, nil
		},
	}

	got := handler.handleLoad(
		context.Background(),
		afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad},
	)
	want := "seleccion.txt:" + base64.StdEncoding.EncodeToString([]byte("contenido"))
	if got != want {
		t.Fatalf("handleLoad() = %q, want %q", got, want)
	}

	outsidePath := filepath.Join(aliasParent, "fuera.txt")
	requireLegacyTestOutsideScope(t, outsidePath)
	if err := os.WriteFile(outsidePath, []byte("secreto"), 0o600); err != nil {
		t.Fatalf("WriteFile(outsidePath): %v", err)
	}
	selectedPath = filepath.Join(aliasHome, "escape.txt")
	if err := os.Symlink(outsidePath, selectedPath); err != nil {
		t.Fatalf("Symlink(escape): %v", err)
	}
	if got := handler.handleLoad(
		context.Background(),
		afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad},
	); got != "escape.txt:"+base64.StdEncoding.EncodeToString([]byte("secreto")) {
		t.Fatalf("handleLoad(escape) = %q: el fichero elegido por el usuario debe cargarse, como en AutoFirma Java", got)
	}
}

func TestLegacyWebSocketHandler_SaveDatosBinariosCrudos(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	handler := &legacyWebSocketHandler{savePicker: acceptLegacySaveDefaultForTest}

	rawPayload := string([]byte{0x30, 0x82, 0x01, 0x02, 0x00, 0x0a, 0xff, 0x41})
	saveReq := afirmauri.Solicitud{
		LegacyParams: url.Values{
			"dat":      []string{rawPayload},
			"filename": []string{"firma.bin"},
		},
	}

	if got := handler.handleSave(context.Background(), saveReq); got != "SAVE_OK" {
		t.Fatalf("handleSave(raw) = %q, want SAVE_OK", got)
	}
	savedPath := filepath.Join(home, "Descargas", "firma.bin")
	raw, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("ReadFile(savedPath): %v", err)
	}
	if string(raw) != rawPayload {
		t.Fatalf("contenido guardado inesperado")
	}
}

func TestLegacyWebSocketHandler_SavePropagaExtensionPorDefectoAlDialogo(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	var gotExts string
	handler := &legacyWebSocketHandler{
		savePicker: func(_ context.Context, defaultPath string, exts string) (string, error) {
			gotExts = exts
			return defaultPath, nil
		},
	}
	saveReq := afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("contenido-demo")),
			"filename": "demo.bin",
		}),
	}
	if got := handler.handleSave(context.Background(), saveReq); got != "SAVE_OK" {
		t.Fatalf("handleSave() = %q, want SAVE_OK", got)
	}
	if gotExts != "bin" {
		t.Fatalf("exts dialogo = %q, want %q", gotExts, "bin")
	}
}

func TestLegacyWebSocketHandler_SaveContextoCanceladoNoEsErrorSAF05(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	saved := false
	handler := &legacyWebSocketHandler{
		savePicker: func(context.Context, string, string) (string, error) {
			return "", fmt.Errorf("diálogo cerrado: %w", context.Canceled)
		},
		afterSave: func(string, int) {
			saved = true
		},
	}
	resp := handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("contenido")),
			"filename": "cancelado.txt",
		}),
	})
	if resp != "CANCEL" {
		t.Fatalf("handleSave() = %q, want CANCEL", resp)
	}
	if saved {
		t.Fatal("afterSave se ejecutó tras cancelar el diálogo")
	}
	if _, err := os.Stat(filepath.Join(home, "Descargas", "cancelado.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se creó un fichero tras cancelar: %v", err)
	}
}

func TestLegacyWebSocketHandler_SaveSeleccionVaciaEsCancelacion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	handler := &legacyWebSocketHandler{
		savePicker: func(context.Context, string, string) (string, error) {
			return "", nil
		},
	}
	resp := handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("contenido")),
			"filename": "cancelado.txt",
		}),
	})
	if resp != "CANCEL" {
		t.Fatalf("handleSave() = %q, want CANCEL", resp)
	}
	if _, err := os.Stat(filepath.Join(home, "Descargas", "cancelado.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se creó un fichero tras una selección vacía: %v", err)
	}
}

func TestLegacyWebSocketHandler_SaveAceptaDestinoElegidoPorElUsuarioFueraDelHome(t *testing.T) {
	outsideRoot := t.TempDir()
	home := t.TempDir()
	safeTemp := t.TempDir()
	setTestUserHome(t, home)
	setLegacyTestTempDir(t, safeTemp)
	outside := filepath.Join(outsideRoot, "fuera.txt")
	requireLegacyTestOutsideScope(t, outside)

	handler := &legacyWebSocketHandler{
		savePicker: func(context.Context, string, string) (string, error) {
			return outside, nil
		},
	}
	resp := handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("contenido")),
			"filename": "seguro.txt",
		}),
	})
	// Otra unidad o una carpeta de red elegida en el diálogo nativo es una
	// decisión del usuario, como en AutoFirma Java.
	if resp != "SAVE_OK" {
		t.Fatalf("handleSave() = %q, want SAVE_OK", resp)
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "contenido" {
		t.Fatalf("no se guardó en el destino elegido: %v", err)
	}
}

func TestLegacyWebSocketHandler_SaveRechazaRutasDeDispositivoORelativas(t *testing.T) {
	for _, ruta := range []string{`\\.\PhysicalDrive0`, `\\?\C:\x.txt`, "relativa.txt", ""} {
		if _, ok := resolveUserChosenWritablePath(ruta); ok {
			t.Fatalf("ruta %q aceptada", ruta)
		}
	}
	if _, ok := resolveUserChosenWritablePath(t.TempDir()); ok {
		t.Fatal("un directorio no es un destino de guardado válido")
	}
}

func TestLegacyWebSocketHandler_SaveRechazaDirectorioSimbolico(t *testing.T) {
	outsideRoot := t.TempDir()
	home := t.TempDir()
	safeTemp := t.TempDir()
	setTestUserHome(t, home)
	setLegacyTestTempDir(t, safeTemp)
	requireLegacyTestOutsideScope(t, outsideRoot)
	link := filepath.Join(home, "Descargas")
	if err := os.Symlink(outsideRoot, link); err != nil {
		t.Skipf("el entorno no permite crear symlinks: %v", err)
	}

	handler := &legacyWebSocketHandler{savePicker: acceptLegacySaveDefaultForTest}
	resp := handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSave,
		LegacyParams: mapToValues(map[string]string{
			"dat":      base64.StdEncoding.EncodeToString([]byte("contenido")),
			"filename": "escape.txt",
		}),
	})
	if resp != "SAF_05: No se pudo guardar el fichero" {
		t.Fatalf("handleSave() = %q, want SAF_05", resp)
	}
	if _, err := os.Stat(filepath.Join(outsideRoot, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se siguió el directorio simbólico fuera del ámbito: %v", err)
	}
}

func TestLegacyWebSocketHandler_LoadRechazaSymlinkFueraDelAmbito(t *testing.T) {
	outsideRoot := t.TempDir()
	home := t.TempDir()
	safeTemp := t.TempDir()
	setTestUserHome(t, home)
	setLegacyTestTempDir(t, safeTemp)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	outside := filepath.Join(outsideRoot, "secreto.txt")
	requireLegacyTestOutsideScope(t, outside)
	if err := os.WriteFile(outside, []byte("secreto"), 0o600); err != nil {
		t.Fatalf("WriteFile(outside): %v", err)
	}
	link := filepath.Join(home, "enlace.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("el entorno no permite crear symlinks: %v", err)
	}

	handler := &legacyWebSocketHandler{}
	resp := handler.handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
		LegacyParams: mapToValues(map[string]string{
			"filePath": link,
		}),
	})
	if resp != "SAF_25: Ruta fuera del ámbito permitido" {
		t.Fatalf("handleLoad() = %q, want SAF_25 por symlink", resp)
	}
}

func TestLegacyWebSocketHandler_LoadRechazaFicheroDemasiadoGrande(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	path := filepath.Join(home, "large.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := file.Truncate(legacyLoadMaxBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	handler := &legacyWebSocketHandler{}
	resp := handler.handleLoad(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLoad,
		LegacyParams: mapToValues(map[string]string{
			"filePath": path,
		}),
	})
	if resp != "SAF_25: No se pudo cargar el fichero" {
		t.Fatalf("handleLoad() = %q, want SAF_25", resp)
	}
}

func TestLegacyWebSocketHandler_HandleSignLocalSinServlets(t *testing.T) {
	t.Parallel()

	documento, err := domain.NewDocument("contrato.pdf", []byte("%PDF-demo"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}

	cert := domain.CertificateRef{
		ID:          "cert-legacy-1",
		Subject:     "Cert Legacy",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "AA:BB",
	}
	executor := &signExecutorStub{
		result: application.SignResult{
			Result: domain.SignatureResult{
				Format: domain.FormatPAdES,
				Data:   []byte("firma-pdf"),
			},
			CertificateUsed: cert,
		},
	}
	handler := &legacyWebSocketHandler{
		signUC:     executor,
		batchUC:    batchExecutorStub{},
		catalogo:   certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector:   certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		documentos: documentPickerStub{doc: documento},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
	}

	resp, err := handler.handleSign(context.Background(), afirmauri.Solicitud{
		Operacion:   afirmauri.OperacionFirma,
		Formato:     domain.FormatCAdES,
		AccionFirma: domain.ActionSign,
		LegacyParams: mapToValues(map[string]string{
			"idsession": "ses-local-1",
			"format":    "AUTO",
		}),
	})
	if err != nil {
		t.Fatalf("handleSign() error = %v", err)
	}
	if resp == "" || resp == "CANCEL" {
		t.Fatalf("respuesta legacy inesperada: %q", resp)
	}
	if executor.cmd.Format != domain.FormatPAdES {
		t.Fatalf("formato local inferido = %q, want %q", executor.cmd.Format, domain.FormatPAdES)
	}
	if executor.cmd.CertificateID != cert.ID {
		t.Fatalf("certificateID = %q, want %q", executor.cmd.CertificateID, cert.ID)
	}
	if executor.cmd.Document.Name != "contrato.pdf" {
		t.Fatalf("documento = %q, want %q", executor.cmd.Document.Name, "contrato.pdf")
	}
}

func TestLegacyWebSocketHandler_HandleSignLocalCancelacionDevuelveCancel(t *testing.T) {
	t.Parallel()

	handler := &legacyWebSocketHandler{
		signUC:     &signExecutorStub{},
		batchUC:    batchExecutorStub{},
		catalogo:   certificateCatalogStub{},
		selector:   certSelectorStub{},
		documentos: documentPickerStub{err: desktopdocumentpicker.ErrSeleccionCancelada},
	}

	resp, err := handler.handleSign(context.Background(), afirmauri.Solicitud{
		Operacion:   afirmauri.OperacionFirma,
		AccionFirma: domain.ActionSign,
		LegacyParams: mapToValues(map[string]string{
			"idsession": "ses-local-2",
			"format":    "AUTO",
		}),
	})
	if err != nil {
		t.Fatalf("handleSign() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("respuesta = %q, want CANCEL", resp)
	}
}

func TestLegacyWebSocketHandler_HandleLocalBatchCancelacionDevuelveCancel(t *testing.T) {
	t.Parallel()

	handler := &legacyWebSocketHandler{
		batchUC: batchExecutorStub{err: errors.New("el usuario ha cancelado el procesado del lote")},
	}

	cmd := application.ProcessBatchCommand{}
	solicitud := afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
	}
	meta := afirmauri.BatchMetadata{
		IDs: []string{"uno"},
	}

	resp, err := handler.handleLocalBatch(context.Background(), cmd, solicitud, meta)
	if err != nil {
		t.Fatalf("handleLocalBatch() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("handleLocalBatch() = %q, want CANCEL", resp)
	}
}

func TestLegacyWebSocketHandler_HandleLocalBatchErrorDevuelveSAF20(t *testing.T) {
	t.Parallel()

	handler := &legacyWebSocketHandler{
		batchUC: batchExecutorStub{err: errors.New("fallo interno del lote")},
	}

	resp, err := handler.handleLocalBatch(context.Background(), application.ProcessBatchCommand{}, afirmauri.Solicitud{
		Operacion: afirmauri.OperacionLote,
	}, afirmauri.BatchMetadata{IDs: []string{"uno"}})
	if err != nil {
		t.Fatalf("handleLocalBatch() error = %v", err)
	}
	if resp != "SAF_20: Error en el proceso local del lote de firma" {
		t.Fatalf("handleLocalBatch() = %q, want SAF_20", resp)
	}
}

func TestLegacyWebSocketHandler_HandleLocalBatchPropagaStopYFirmaObservableV19(t *testing.T) {
	t.Parallel()

	var captured application.ProcessBatchCommand
	handler := &legacyWebSocketHandler{
		batchUC: batchExecutorStub{
			cmd: &captured,
			result: application.BatchResult{
				Results: []application.SignResult{{
					Result: domain.SignatureResult{Data: []byte("firma-local-v19")},
				}},
				Errores: map[int]error{},
			},
		},
	}

	resp, err := handler.handleLocalBatch(
		context.Background(),
		application.ProcessBatchCommand{},
		afirmauri.Solicitud{Operacion: afirmauri.OperacionLote},
		afirmauri.BatchMetadata{
			IDs:         []string{"doc-v19"},
			StopOnError: true,
			IsJSON:      true,
		},
	)
	if err != nil {
		t.Fatalf("handleLocalBatch() error = %v", err)
	}
	if !captured.StopOnError {
		t.Fatal("handleLocalBatch() no propagó stoponerror al caso de uso")
	}
	raw, err := base64.StdEncoding.DecodeString(resp)
	if err != nil {
		t.Fatalf("respuesta batch no es Base64 estándar: %v", err)
	}
	var parsed struct {
		Signs []legacyBatchSingleResult `json:"signs"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("respuesta batch no es JSON V1: %v", err)
	}
	if len(parsed.Signs) != 1 {
		t.Fatalf("signs = %d, want 1", len(parsed.Signs))
	}
	wantSignature := base64.StdEncoding.EncodeToString([]byte("firma-local-v19"))
	if parsed.Signs[0].Result != "DONE_AND_SAVED" || parsed.Signs[0].Signature != wantSignature {
		t.Fatalf("resultado batch observable inesperado: %+v", parsed.Signs[0])
	}
}

func TestLegacyWebSocketHandler_HandleRemoteBatchCancelacionDevuelveCancel(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:          "cert-remote-batch-cancel",
		Subject:     "Cert Remote Batch",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "55:66",
	}
	handler := &legacyWebSocketHandler{
		approval: &userApprovalStub{approved: true},
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
		batchRemote: remoteBatchExecutorStub{err: errors.New("el usuario ha cancelado la operacion de lote remoto")},
	}

	resp, err := handler.handleRemoteBatch(context.Background(), afirmauri.RemoteBatchCommand{
		LegacyParams: mapToValues(map[string]string{
			"idsession": "ses-remote-batch-cancel",
		}),
	})
	if err != nil {
		t.Fatalf("handleRemoteBatch() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("handleRemoteBatch() = %q, want CANCEL", resp)
	}
}

func TestLegacyWebSocketHandler_HandleRemoteBatchErrorComunicacionDevuelveSAF26(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:          "cert-remote-batch-saf26",
		Subject:     "Cert Remote Batch",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "77:88",
	}
	handler := &legacyWebSocketHandler{
		approval: &userApprovalStub{approved: true},
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
		batchRemote: remoteBatchExecutorStub{err: errors.New("prefirma remota del lote fallido: EOF")},
	}

	resp, err := handler.handleRemoteBatch(context.Background(), afirmauri.RemoteBatchCommand{
		LegacyParams: mapToValues(map[string]string{
			"idsession": "ses-remote-batch-saf26",
		}),
	})
	if err != nil {
		t.Fatalf("handleRemoteBatch() error = %v", err)
	}
	if resp != "SAF_26: Error en la comunicación con el servicio de firma de lotes" {
		t.Fatalf("handleRemoteBatch() = %q, want SAF_26", resp)
	}
}

func TestLegacyWebSocketHandler_HandleRemoteBatchErrorFirmaDevuelveSAF27(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:          "cert-remote-batch-saf27",
		Subject:     "Cert Remote Batch",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "99:AA",
	}
	approval := &userApprovalStub{approved: true}
	handler := &legacyWebSocketHandler{
		approval: approval,
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
		batchRemote: remoteBatchExecutorStub{err: errors.New("resultado de lote invalido")},
	}

	resp, err := handler.handleRemoteBatch(context.Background(), afirmauri.RemoteBatchCommand{
		LegacyParams: mapToValues(map[string]string{
			"idsession": "ses-remote-batch-saf27",
		}),
	})
	if err != nil {
		t.Fatalf("handleRemoteBatch() error = %v", err)
	}
	if resp != "SAF_27: Error en la firma por lotes" {
		t.Fatalf("handleRemoteBatch() = %q, want SAF_27", resp)
	}
	if approval.calls != 1 {
		t.Fatalf("approval.calls = %d, want exactly 1", approval.calls)
	}
}

func TestLegacyRemoteBatchErrorResultExplicaRechazoSHA1(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("firma local PKCS#1 fallida: %w", cryptopolicy.ErrSHA1Disabled)
	got := legacyRemoteBatchErrorResult(err)
	for _, fragment := range []string{"SAF_27", "SHA-1", "inseguro", "ninguna firma", "portal", "SHA-256"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("legacyRemoteBatchErrorResult() no contiene %q: %q", fragment, got)
		}
	}
	if strings.Contains(got, cryptopolicy.EnvEnableLegacySHA1) {
		t.Fatalf("la respuesta al usuario no debe recomendar habilitar SHA-1: %q", got)
	}
}

func TestLegacyWebSocketHandler_HandleRemoteBatchRechazadoNoEjecutaFirma(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:       "cert-remote-batch-rejected",
		Subject:  "Cert Remote Batch",
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	approval := &userApprovalStub{approved: false}
	executorCalls := 0
	handler := &legacyWebSocketHandler{
		approval: approval,
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
		batchRemote: remoteBatchExecutorStub{
			result: "OK",
			called: &executorCalls,
		},
	}

	resp, err := handler.handleRemoteBatch(context.Background(), afirmauri.RemoteBatchCommand{})
	if err != nil {
		t.Fatalf("handleRemoteBatch() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("handleRemoteBatch() = %q, want CANCEL", resp)
	}
	if approval.calls != 1 {
		t.Fatalf("approval.calls = %d, want 1", approval.calls)
	}
	if executorCalls != 0 {
		t.Fatalf("executorCalls = %d, want 0", executorCalls)
	}
}

func TestLegacyWebSocketHandler_HandleRemoteBatchSinAprobadorFallaCerrado(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:       "cert-remote-batch-no-approval",
		Subject:  "Cert Remote Batch",
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	executorCalls := 0
	handler := &legacyWebSocketHandler{
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
		batchRemote: remoteBatchExecutorStub{
			result: "OK",
			called: &executorCalls,
		},
	}

	_, err := handler.handleRemoteBatch(context.Background(), afirmauri.RemoteBatchCommand{})
	if err == nil || !strings.Contains(err.Error(), "aprobador del lote remoto no configurado") {
		t.Fatalf("handleRemoteBatch() error = %v, want fail-closed", err)
	}
	if executorCalls != 0 {
		t.Fatalf("executorCalls = %d, want 0", executorCalls)
	}
}

// Como AutoFirma Java, una firma sin datos (VALIDe: format=AUTO y solo
// mode=implicit) pide el fichero al usuario y lo firma; nunca devuelve solo el
// certificado, que el portal tomaría por una firma.
func TestLegacyWebSocketHandler_HandleSignSinDocumentoPideFicheroYFirma(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:          "cert-legacy-sin-datos",
		Subject:     "Cert Identidad",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "CC:DD",
	}
	doc, err := domain.NewDocument("contrato.pdf", []byte("%PDF-1.7 contenido"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	executor := &signExecutorStub{}
	handler := &legacyWebSocketHandler{
		signUC:     executor,
		batchUC:    batchExecutorStub{},
		catalogo:   certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector:   certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		documentos: documentPickerStub{doc: doc},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
	}

	props := base64.StdEncoding.EncodeToString([]byte("mode=implicit\n"))
	resp, err := handler.handleSign(context.Background(), afirmauri.Solicitud{
		Operacion:   afirmauri.OperacionFirma,
		AccionFirma: domain.ActionSign,
		LegacyParams: mapToValues(map[string]string{
			"idsession":  "ses-sin-datos-1",
			"format":     "AUTO",
			"properties": props,
			"algorithm":  "SHA256withRSA",
			"sticky":     "false",
		}),
	})
	if err != nil {
		t.Fatalf("handleSign() error = %v", err)
	}
	if executor.cmd.CertificateID != cert.ID || executor.cmd.Document.Name != "contrato.pdf" {
		t.Fatalf("debe firmar el fichero elegido; cmd=%+v", executor.cmd)
	}
	if executor.cmd.Format != domain.FormatPAdES {
		t.Fatalf("formato AUTO sobre PDF = %q, want PAdES", executor.cmd.Format)
	}
	if !strings.Contains(resp, "|") {
		t.Fatalf("la respuesta debe llevar certificado y firma, no solo el certificado: %q", resp)
	}
}

// Con un único certificado el selector se muestra igualmente: es el
// consentimiento del usuario. Si lo cancela, la web no obtiene nada.
func TestLegacyWebSocketHandler_HandleSelectCertUnicoExigeSeleccionExplicita(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:          "cert-unico-selectcert",
		Subject:     "Cert Unico",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "AB:CD",
	}
	handler := &legacyWebSocketHandler{
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{err: certpicker.ErrSeleccionCancelada},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der-unico")},
		}},
	}

	resp, err := handler.handleSelectCert(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSelectCert,
	})
	if err == nil && resp == base64.URLEncoding.EncodeToString([]byte("cert-der-unico")) {
		t.Fatal("con un único certificado no debe entregarse sin que el usuario lo seleccione")
	}
}

func TestLegacyWebSocketHandler_HandleSelectCertUnicoConStickyGuardaPreferencia(t *testing.T) {
	t.Parallel()

	cert := domain.CertificateRef{
		ID:          "cert-unico-sticky",
		Subject:     "Cert Unico Sticky",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "EF:01",
	}
	handler := &legacyWebSocketHandler{
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der-sticky")},
		}},
	}

	_, err := handler.handleSelectCert(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSelectCert,
		LegacyParams: mapToValues(map[string]string{
			"sticky": "true",
		}),
	})
	if err != nil {
		t.Fatalf("handleSelectCert() error = %v", err)
	}
	if got := strings.TrimSpace(handler.stickyID); got != cert.ID {
		t.Fatalf("stickyID = %q, want %q", got, cert.ID)
	}
}

func TestLegacyWebSocketHandler_SignAndSaveAbreGuardadoSeguro(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	var gotExts string
	cert := domain.CertificateRef{
		ID:          "cert-signsave-1",
		Subject:     "Cert SignSave",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "EE:FF",
	}
	executor := &signExecutorStub{
		result: application.SignResult{
			Result: domain.SignatureResult{
				Format: domain.FormatPAdES,
				Data:   []byte("%PDF-firmado"),
			},
			CertificateUsed: cert,
		},
	}
	handler := &legacyWebSocketHandler{
		signUC:   executor,
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		savePicker: func(_ context.Context, defaultPath string, exts string) (string, error) {
			gotExts = exts
			return defaultPath, nil
		},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
	}

	documento, err := domain.NewDocument("entrada.pdf", []byte("%PDF-original"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}
	resp, err := handler.handleSignAndSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSignSave,
		Formato:   domain.FormatPAdES,
		SignCommand: &application.SignCommand{
			Document: documento,
			Format:   domain.FormatPAdES,
			Action:   domain.ActionSign,
		},
		LegacyParams: mapToValues(map[string]string{
			"filename": "salida.pdf",
		}),
	})
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "SAVE_OK" {
		t.Fatalf("handleSignAndSave() = %q, want SAVE_OK", resp)
	}
	raw, err := os.ReadFile(filepath.Join(home, "Descargas", "salida.pdf"))
	if err != nil {
		t.Fatalf("ReadFile(salida.pdf): %v", err)
	}
	if string(raw) != "%PDF-firmado" {
		t.Fatalf("contenido firmado = %q, want %q", string(raw), "%PDF-firmado")
	}
	if gotExts != "pdf" {
		t.Fatalf("exts dialogo = %q, want %q", gotExts, "pdf")
	}
}

func TestLegacyWebSocketHandler_SignAndSaveSinDatosDevuelveSAF44(t *testing.T) {
	t.Parallel()

	handler := &legacyWebSocketHandler{}
	resp, err := handler.handleSignAndSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSignSave,
	})
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "SAF_44: Operacion de firma sin datos" {
		t.Fatalf("handleSignAndSave() = %q, want SAF_44", resp)
	}
}

func TestLegacyWebSocketHandler_SignAndSaveCanceladoDevuelveCancel(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	cert := domain.CertificateRef{
		ID:          "cert-signsave-cancel",
		Subject:     "Cert SignSave",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "11:22",
	}
	executor := &signExecutorStub{
		result: application.SignResult{
			Result: domain.SignatureResult{
				Format: domain.FormatPAdES,
				Data:   []byte("%PDF-firmado"),
			},
			CertificateUsed: cert,
		},
	}
	handler := &legacyWebSocketHandler{
		signUC:   executor,
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		savePicker: func(context.Context, string, string) (string, error) {
			return "", errLegacySaveCanceled
		},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
	}

	documento, err := domain.NewDocument("entrada.pdf", []byte("%PDF-original"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}
	resp, err := handler.handleSignAndSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSignSave,
		Formato:   domain.FormatPAdES,
		SignCommand: &application.SignCommand{
			Document: documento,
			Format:   domain.FormatPAdES,
			Action:   domain.ActionSign,
		},
		LegacyParams: mapToValues(map[string]string{
			"filename": "salida.pdf",
		}),
	})
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("handleSignAndSave() = %q, want CANCEL", resp)
	}
}

func TestLegacyWebSocketHandler_SignAndSaveContextoCanceladoDevuelveCancel(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	handler, solicitud := newSignAndSaveFlowTest(t, func(context.Context, string, string) (string, error) {
		return "", fmt.Errorf("ventana cerrada: %w", context.Canceled)
	})

	resp, err := handler.handleSignAndSave(context.Background(), solicitud)
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("handleSignAndSave() = %q, want CANCEL", resp)
	}
	if _, err := os.Stat(filepath.Join(home, "Descargas", "salida.pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se guardó el resultado tras cancelar: %v", err)
	}
}

func TestLegacyWebSocketHandler_SignAndSaveSeleccionVaciaEsCancelacion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	handler, solicitud := newSignAndSaveFlowTest(t, func(context.Context, string, string) (string, error) {
		return "", nil
	})

	resp, err := handler.handleSignAndSave(context.Background(), solicitud)
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "CANCEL" {
		t.Fatalf("handleSignAndSave() = %q, want CANCEL", resp)
	}
	if _, err := os.Stat(filepath.Join(home, "Descargas", "salida.pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se guardó el resultado tras una selección vacía: %v", err)
	}
}

func TestLegacyWebSocketHandler_SignAndSaveAceptaDestinoElegidoFueraDelHome(t *testing.T) {
	outsideRoot := t.TempDir()
	home := t.TempDir()
	safeTemp := t.TempDir()
	setTestUserHome(t, home)
	setLegacyTestTempDir(t, safeTemp)
	outside := filepath.Join(outsideRoot, "firma.pdf")
	requireLegacyTestOutsideScope(t, outside)
	handler, solicitud := newSignAndSaveFlowTest(t, func(context.Context, string, string) (string, error) {
		return outside, nil
	})

	resp, err := handler.handleSignAndSave(context.Background(), solicitud)
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "SAVE_OK" {
		t.Fatalf("handleSignAndSave() = %q, want SAVE_OK en el destino elegido", resp)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("no se guardó la firma en el destino elegido: %v", err)
	}
}

func TestLegacyWebSocketHandler_SignAndSaveErrorDialogoDevuelveSAF05(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)

	cert := domain.CertificateRef{
		ID:          "cert-signsave-error",
		Subject:     "Cert SignSave",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "33:44",
	}
	executor := &signExecutorStub{
		result: application.SignResult{
			Result: domain.SignatureResult{
				Format: domain.FormatPAdES,
				Data:   []byte("%PDF-firmado"),
			},
			CertificateUsed: cert,
		},
	}
	handler := &legacyWebSocketHandler{
		signUC:   executor,
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		savePicker: func(context.Context, string, string) (string, error) {
			return "", errors.New("fallo selector")
		},
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
	}

	documento, err := domain.NewDocument("entrada.pdf", []byte("%PDF-original"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}
	resp, err := handler.handleSignAndSave(context.Background(), afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSignSave,
		Formato:   domain.FormatPAdES,
		SignCommand: &application.SignCommand{
			Document: documento,
			Format:   domain.FormatPAdES,
			Action:   domain.ActionSign,
		},
		LegacyParams: mapToValues(map[string]string{
			"filename": "salida.pdf",
		}),
	})
	if err != nil {
		t.Fatalf("handleSignAndSave() error = %v", err)
	}
	if resp != "SAF_05: No se pudo guardar el fichero" {
		t.Fatalf("handleSignAndSave() = %q, want SAF_05", resp)
	}
}

func TestBuildLegacyBatchResultListYSerializaJSON(t *testing.T) {
	t.Parallel()

	meta := afirmauri.BatchMetadata{
		IDs:         []string{"uno", "dos", "tres"},
		StopOnError: true,
		IsJSON:      true,
	}
	resultado := application.BatchResult{
		Errores: map[int]error{
			1: os.ErrPermission,
		},
	}

	items := buildLegacyBatchResultList(meta, resultado)
	if got := items[0].Result; got != "SKIPPED" {
		t.Fatalf("resultado[0] = %q", got)
	}
	if got := items[1].Result; got != "ERROR_PRE" {
		t.Fatalf("resultado[1] = %q", got)
	}
	if got := items[2].Result; got != "SKIPPED" {
		t.Fatalf("resultado[2] = %q", got)
	}

	raw, err := serializeLegacyBatchResponse(items, true)
	if err != nil {
		t.Fatalf("serializeLegacyBatchResponse() error = %v", err)
	}
	var parsed map[string][]map[string]string
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(parsed["signs"]) != 3 {
		t.Fatalf("signs serializados = %d", len(parsed["signs"]))
	}
}

func TestBuildLegacyBatchResultListIncluyeFirmasEnExitos(t *testing.T) {
	t.Parallel()

	meta := afirmauri.BatchMetadata{
		IDs:    []string{"uno", "dos", "tres"},
		IsJSON: true,
	}
	resultado := application.BatchResult{
		Results: []application.SignResult{
			{Result: domain.SignatureResult{Data: []byte("firma-uno")}},
			{Result: domain.SignatureResult{Data: []byte("firma-tres")}},
		},
		Errores: map[int]error{1: os.ErrPermission},
	}

	items := buildLegacyBatchResultList(meta, resultado)
	if got := items[0].Signature; got != base64.StdEncoding.EncodeToString([]byte("firma-uno")) {
		t.Fatalf("signature[0] = %q", got)
	}
	if items[1].Signature != "" || items[1].Result != "ERROR_PRE" {
		t.Fatalf("resultado fallido inesperado: %+v", items[1])
	}
	if got := items[2].Signature; got != base64.StdEncoding.EncodeToString([]byte("firma-tres")) {
		t.Fatalf("signature[2] = %q", got)
	}

	raw, err := serializeLegacyBatchResponse(items, true)
	if err != nil {
		t.Fatalf("serializeLegacyBatchResponse() error = %v", err)
	}
	var parsed struct {
		Signs []legacyBatchSingleResult `json:"signs"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(parsed.Signs) != 3 || parsed.Signs[0].Signature == "" || parsed.Signs[2].Signature == "" {
		t.Fatalf("firmas no serializadas: %+v", parsed.Signs)
	}
}

func TestBuildLegacyBatchResultListStopOnErrorEliminaFirmasPrevias(t *testing.T) {
	t.Parallel()

	items := buildLegacyBatchResultList(
		afirmauri.BatchMetadata{
			IDs:         []string{"uno", "dos", "tres"},
			StopOnError: true,
			IsJSON:      true,
		},
		application.BatchResult{
			Results: []application.SignResult{
				{Result: domain.SignatureResult{Data: []byte("firma-que-debe-descartarse")}},
			},
			Errores: map[int]error{1: os.ErrPermission},
		},
	)

	if items[0].Result != "SKIPPED" || items[1].Result != "ERROR_PRE" || items[2].Result != "SKIPPED" {
		t.Fatalf("estados stoponerror inesperados: %+v", items)
	}
	for i, item := range items {
		if item.Signature != "" {
			t.Fatalf("signature[%d] debe omitirse con stoponerror: %q", i, item.Signature)
		}
	}
}

func TestLegacyAutoScriptResponseEncodingsSinCifrado(t *testing.T) {
	t.Parallel()

	certDER := []byte{0xfb, 0xff}
	signature := []byte{0xff, 0xef}
	extraInfo := []byte{0xfa, 0xfb}

	certificateResponse, err := encodeLegacyCertificateResponse(certDER, "")
	if err != nil {
		t.Fatalf("encodeLegacyCertificateResponse() error = %v", err)
	}
	if want := base64.URLEncoding.EncodeToString(certDER); certificateResponse != want {
		t.Fatalf("selectcert = %q, want Base64 URL-safe %q", certificateResponse, want)
	}

	signResponse, err := encodeLegacySignResponse(certDER, signature, "", extraInfo)
	if err != nil {
		t.Fatalf("encodeLegacySignResponse() error = %v", err)
	}
	wantSign := strings.Join([]string{
		base64.URLEncoding.EncodeToString(certDER),
		base64.URLEncoding.EncodeToString(signature),
		base64.URLEncoding.EncodeToString(extraInfo),
	}, "|")
	if signResponse != wantSign {
		t.Fatalf("sign = %q, want cert|firma|extra %q", signResponse, wantSign)
	}

	batchResponse, err := encodeLegacyBatchResult(signature, certDER, true, "")
	if err != nil {
		t.Fatalf("encodeLegacyBatchResult() error = %v", err)
	}
	wantBatch := base64.StdEncoding.EncodeToString(signature) + "|" + base64.StdEncoding.EncodeToString(certDER)
	if batchResponse != wantBatch {
		t.Fatalf("batch = %q, want resultado|cert en Base64 estándar %q", batchResponse, wantBatch)
	}
}

func TestLegacyProtocolEncryptAndFormat_DESDesactivadoPorDefecto(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, false)

	if _, err := encryptAndFormatLegacyProtocol([]byte("firma"), []byte("clave123")); !errors.Is(err, legacycrypto.ErrDESDisabled) {
		t.Fatalf("encryptAndFormatLegacyProtocol() error = %v, want ErrDESDisabled", err)
	}
}

func TestLegacyProtocolEncryptAndFormat_DESRequiereOptInExplicito(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)

	got, err := encryptAndFormatLegacyProtocol([]byte("firma"), []byte("clave123"))
	if err != nil {
		t.Fatalf("encryptAndFormatLegacyProtocol() error = %v", err)
	}
	parts := strings.SplitN(got, ".", 2)
	if len(parts) != 2 || parts[0] != "3" {
		t.Fatalf("encryptAndFormatLegacyProtocol() = %q, want prefijo de padding 3", got)
	}
	if _, err := base64.URLEncoding.DecodeString(parts[1]); err != nil {
		t.Fatalf("payload cifrado no es base64 URL válido: %v", err)
	}
}

func newSignAndSaveFlowTest(t *testing.T, picker legacySavePicker) (*legacyWebSocketHandler, afirmauri.Solicitud) {
	t.Helper()
	cert := domain.CertificateRef{
		ID:          "cert-signsave-flow",
		Subject:     "Cert SignSave Flow",
		NotAfter:    time.Now().Add(24 * time.Hour),
		Fingerprint: "55:66",
	}
	documento, err := domain.NewDocument("entrada.pdf", []byte("%PDF-original"), "application/pdf")
	if err != nil {
		t.Fatalf("NewDocument(): %v", err)
	}
	handler := &legacyWebSocketHandler{
		signUC: &signExecutorStub{result: application.SignResult{
			Result: domain.SignatureResult{
				Format: domain.FormatPAdES,
				Data:   []byte("%PDF-firmado"),
			},
			CertificateUsed: cert,
		}},
		catalogo:   certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector:   certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		savePicker: picker,
		keys: keyProviderStub{key: signingKeyStub{
			id:    cert.ID,
			chain: [][]byte{[]byte("cert-der")},
		}},
	}
	solicitud := afirmauri.Solicitud{
		Operacion: afirmauri.OperacionSignSave,
		Formato:   domain.FormatPAdES,
		SignCommand: &application.SignCommand{
			Document: documento,
			Format:   domain.FormatPAdES,
			Action:   domain.ActionSign,
		},
		LegacyParams: mapToValues(map[string]string{
			"filename": "salida.pdf",
		}),
	}
	return handler, solicitud
}

func mapToValues(items map[string]string) map[string][]string {
	out := make(map[string][]string, len(items))
	for k, v := range items {
		out[k] = []string{v}
	}
	return out
}

func acceptLegacySaveDefaultForTest(_ context.Context, defaultPath, _ string) (string, error) {
	return defaultPath, nil
}

func TestSanitizeLegacyFilename_NeutralizaDatosDelPortal(t *testing.T) {
	casos := map[string]string{
		"a’;Start-Process calc;’.pdf": "a’;Start-Process calc;’.pdf",
		`..\..\Windows\evil.pdf`:      "evil.pdf",
		"fi\"ch<er>o|?*.pdf":          "fi_ch_er_o___.pdf",
		"linea\r\nnueva.pdf":          "linea__nueva.pdf",
		"con punto final. ":           "con punto final.bin",
	}
	for entrada, esperado := range casos {
		if obtenido := sanitizeLegacyFilename(entrada, ".bin"); obtenido != esperado {
			t.Errorf("sanitizeLegacyFilename(%q) = %q, esperado %q", entrada, obtenido, esperado)
		}
	}
}

func TestLegacyWebSocketHandler_SaveBloqueaExtensionesEjecutables(t *testing.T) {
	dir := t.TempDir()
	for _, nombre := range []string{"factura.exe", "FIRMA.LNK", "x.ps1.", "a.hta", "doc.pdf.bat"} {
		destino := filepath.Join(dir, nombre)
		handler := &legacyWebSocketHandler{
			savePicker: func(context.Context, string, string) (string, error) { return destino, nil },
		}
		resp := handler.handleSave(context.Background(), afirmauri.Solicitud{
			Operacion: afirmauri.OperacionSave,
			LegacyParams: mapToValues(map[string]string{
				"dat": base64.StdEncoding.EncodeToString([]byte("MZ")),
			}),
		})
		if resp != "SAF_05: Tipo de fichero no permitido por seguridad" {
			t.Fatalf("%s: handleSave() = %q", nombre, resp)
		}
		if _, err := os.Stat(destino); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: se escribió un ejecutable", nombre)
		}
	}
	if destinoEjecutable(filepath.Join(dir, "firma.csig")) || destinoEjecutable(filepath.Join(dir, "doc.pdf")) {
		t.Fatal("las extensiones de firma habituales deben permitirse")
	}
}

func TestAplicarFormatoXAdESPorDefectoJava(t *testing.T) {
	if got := aplicarFormatoXAdESPorDefectoJava(nil, domain.FormatXAdES)["format"]; got != "XAdES Enveloping" {
		t.Fatalf("XAdES sin formato = %q, want XAdES Enveloping como en Java", got)
	}
	explicito := map[string]string{"format": "XAdES Detached"}
	if got := aplicarFormatoXAdESPorDefectoJava(explicito, domain.FormatXAdES)["format"]; got != "XAdES Detached" {
		t.Fatalf("no debe pisar el formato pedido: %q", got)
	}
	if got := aplicarFormatoXAdESPorDefectoJava(nil, domain.FormatCAdES); got != nil {
		t.Fatalf("CAdES no debe cambiar: %v", got)
	}
}

type digestKeyStub struct{ signingKeyStub }

func (k digestKeyStub) SignDigest(digest []byte, _ crypto.Hash) ([]byte, error) {
	return append([]byte("pk1:"), digest...), nil
}

// CAdEStri con serverUrl por WebSocket: firma trifásica real contra el
// servidor de la web (como Java), con selector y confirmación del usuario.
func TestLegacyWebSocketHandler_FormatoTriUsaServidorTrifasico(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("op") {
		case "pre":
			td := `<xml><firmas format="CAdES"><firma Id="1"><param n="PRE">` + base64.StdEncoding.EncodeToString([]byte("pre")) + `</param></firma></firmas></xml>`
			_, _ = w.Write([]byte(base64.URLEncoding.EncodeToString([]byte(td))))
		case "post":
			_, _ = w.Write([]byte("OK NEWID=" + base64.URLEncoding.EncodeToString([]byte("cms-servidor"))))
		}
	}))
	defer srv.Close()

	cert := domain.CertificateRef{ID: "c1", Subject: "CN=Firmante", NotAfter: time.Now().Add(time.Hour), Fingerprint: "AA"}
	aprobacion := &userApprovalStub{approved: true}
	executor := &signExecutorStub{}
	h := &legacyWebSocketHandler{
		signUC:   executor,
		catalogo: certificateCatalogStub{certs: []domain.CertificateRef{cert}},
		selector: certSelectorStub{result: certpicker.ResultadoSeleccion{Certificado: cert}},
		keys:     keyProviderStub{key: digestKeyStub{signingKeyStub{id: "c1", chain: [][]byte{[]byte("cert-der")}}}},
		retrieve: triphase.New(srv.Client()),
		approval: aprobacion,
	}
	props := base64.StdEncoding.EncodeToString([]byte("serverUrl=" + srv.URL + "/SignatureService\n"))
	raw := "afirma://sign?op=sign&format=CAdEStri&algorithm=SHA256withRSA&dat=" +
		base64.StdEncoding.EncodeToString([]byte("documento")) + "&properties=" + url.QueryEscape(props)
	sol, err := afirmauri.New(nil).ParseSocket(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.handleSign(context.Background(), sol)
	if err != nil {
		t.Fatalf("handleSign: %v", err)
	}
	partes := strings.Split(resp, "|")
	if len(partes) < 2 || partes[1] != base64.URLEncoding.EncodeToString([]byte("cms-servidor")) {
		t.Fatalf("respuesta = %q", resp)
	}
	if aprobacion.calls == 0 || executor.cmd.CertificateID != "" {
		t.Fatalf("debe confirmar el usuario y no firmar en local: aprobado=%v cmd=%+v", aprobacion.calls > 0, executor.cmd)
	}
}
