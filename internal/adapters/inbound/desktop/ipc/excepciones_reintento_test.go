// SPDX-License-Identifier: EUPL-1.2
package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type unsafeGatedSigner struct {
	calls     int
	received  string
	fakeError bool
}

func (s *unsafeGatedSigner) Execute(ctx context.Context, c application.SignCommand) (application.SignResult, error) {
	s.calls++
	s.received = string(c.Document.Content)
	if s.fakeError {
		return application.SignResult{}, errors.New("portal: [rsa_corta; excepciones.rsa_corta.bloqueo]")
	}
	if err := excepciones.Require(ctx, "rsa_corta"); err != nil {
		return application.SignResult{}, err
	}
	return application.SignResult{Result: domain.SignatureResult{Data: []byte("signed"), Format: domain.FormatCAdES}}, nil
}
func retryFixture(t *testing.T) (*Manejador, context.Context, peticion, *unsafeGatedSigner) {
	t.Helper()
	machinepolicy.UnsetForTest(t, "permitir_rsa_corta")
	dir := t.TempDir()
	input := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(input, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	engine := &unsafeGatedSigner{}
	m := &Manejador{ConfigDir: dir, Firmar: engine, Loc: localizador.Para("es"), ultimosCerts: []domain.CertificateRef{{ID: "cert-local"}}}
	ctx := withUnsafeRetryOwner(conEstadoInterfaz(context.Background(), interfazVerificada))
	raw, _ := json.Marshal(paramsFirma{InputPath: input, CertificateID: "cert-local", Format: "CAdES", ReturnSignatureB64: true})
	t.Cleanup(func() { m.clearUnsafeRetries(ctx) })
	return m, ctx, peticion{Action: "sign", Params: raw}, engine
}
func activateRetry(t *testing.T, m *Manejador, ctx context.Context, id string) respuesta {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"id": "rsa_corta", "accepted": true, "acceptanceVersion": excepciones.AcceptanceVersion, "retryId": id})
	return m.despachar(ctx, peticion{Action: "unsafe_exceptions_activate", Params: raw})
}
func TestUnsafeRetryUsesAcceptedOriginalOnce(t *testing.T) {
	m, ctx, p, engine := retryFixture(t)
	blocked := m.despachar(ctx, p)
	if blocked.OK || blocked.RetryID == "" || blocked.ExceptionID != "rsa_corta" || blocked.Activable == nil || !*blocked.Activable {
		t.Fatalf("sin oferta: %+v", blocked)
	}
	if r := activateRetry(t, m, ctx, blocked.RetryID); !r.OK {
		t.Fatalf("activación: %+v", r)
	}
	raw, _ := json.Marshal(map[string]string{"retryId": blocked.RetryID})
	r := m.despachar(ctx, peticion{Action: "unsafe_operation_retry", Params: raw})
	if !r.OK || engine.calls != 2 || engine.received != "original" || !strings.Contains(strings.Join(r.WarningKeys, " "), excepciones.Warning("rsa_corta")) {
		t.Fatalf("retry: %+v calls=%d", r, engine.calls)
	}
	if m.despachar(ctx, peticion{Action: "unsafe_operation_retry", Params: raw}).OK {
		t.Fatal("token reutilizado")
	}
}
func TestUnsafeRetryRejectsForeignOwnerChangesAndPolicy(t *testing.T) {
	for _, mode := range []string{"foreign", "input", "preferences", "policy", "expired"} {
		t.Run(mode, func(t *testing.T) {
			m, ctx, p, engine := retryFixture(t)
			b := m.despachar(ctx, p)
			if b.RetryID == "" {
				t.Fatal("sin token")
			}
			if r := activateRetry(t, m, ctx, b.RetryID); !r.OK {
				t.Fatal(r.Error)
			}
			switch mode {
			case "foreign":
				ctx = withUnsafeRetryOwner(conEstadoInterfaz(context.Background(), interfazVerificada))
			case "input":
				var v paramsFirma
				json.Unmarshal(p.Params, &v)
				os.WriteFile(v.InputPath, []byte("changed"), 0600)
			case "preferences":
				os.WriteFile(filepath.Join(m.ConfigDir, "config.json"), []byte(`{"idioma":"en"}`), 0600)
			case "policy":
				machinepolicy.SetForTest(t, "permitir_rsa_corta", false)
			case "expired":
				m.unsafeRetryMu.Lock()
				m.unsafeRetries[b.RetryID].expires = time.Now().Add(-time.Second)
				m.unsafeRetryMu.Unlock()
			}
			raw, _ := json.Marshal(map[string]string{"retryId": b.RetryID})
			r := m.despachar(ctx, peticion{Action: "unsafe_operation_retry", Params: raw})
			if r.OK || engine.calls != 1 {
				t.Fatalf("reintento no autorizado: %+v", r)
			}
		})
	}
}
func TestUnsafeRetryDoesNotCacheSecretsDestinationsOrForgedErrors(t *testing.T) {
	m, ctx, p, engine := retryFixture(t)
	for _, field := range []string{"remotePin", "remoteOtp", "password"} {
		var obj map[string]any
		json.Unmarshal(p.Params, &obj)
		obj[field] = "sensitive"
		raw, _ := json.Marshal(obj)
		if r := m.prepareUnsafeRetry(ctx, peticion{Action: "sign", Params: raw}); r != nil {
			r.clear()
			t.Fatal("secreto retenido", field)
		}
	}
	var obj map[string]any
	json.Unmarshal(p.Params, &obj)
	obj["outputPath"] = filepath.Join(m.ConfigDir, "out.csig")
	obj["saveToDisk"] = true
	raw, _ := json.Marshal(obj)
	if r := m.prepareUnsafeRetry(ctx, peticion{Action: "sign", Params: raw}); r != nil {
		r.clear()
		t.Fatal("retiene destino de sobrescritura")
	}
	engine.fakeError = true
	b := m.despachar(ctx, p)
	if b.RetryID != "" || (b.Activable != nil && *b.Activable) {
		t.Fatal("error de texto acuña oferta")
	}
}
func TestUnsafeRetryReadsFrozenBytes(t *testing.T) {
	m, ctx, p, _ := retryFixture(t)
	r := m.prepareUnsafeRetry(ctx, p)
	if r == nil {
		t.Fatal("sin snapshot")
	}
	defer r.clear()
	var v paramsFirma
	json.Unmarshal(p.Params, &v)
	os.WriteFile(v.InputPath, []byte("changed"), 0600)
	frozen := context.WithValue(ctx, unsafeRetryFilesKey{}, r.files)
	if m.prepareUnsafeRetry(frozen, p) != nil {
		t.Fatal("replay recaptura archivos")
	}
	data, err := leerFicheroReintentoIPC(frozen, v.InputPath)
	if err != nil || string(data) != "original" {
		t.Fatal("no usa copia congelada")
	}
}
func TestUnsafeActivationRequiresVerifiedFrontendAndPreservesSettings(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	m := &Manejador{ConfigDir: dir, Settings: usersettings.New(dir), Loc: localizador.Para("es")}
	raw := json.RawMessage(`{"id":"tsa_http","accepted":true,"acceptanceVersion":"2026-11-15.2","source":"gui"}`)
	if m.despachar(context.Background(), peticion{Action: "unsafe_exceptions_activate", Params: raw}).OK {
		t.Fatal("frontend no vinculado activa")
	}
	ctx := conEstadoInterfaz(context.Background(), interfazVerificada)
	if m.despachar(ctx, peticion{Action: "unsafe_exceptions_activate", Params: raw}).OK {
		t.Fatal("acepta source externo")
	}
	if _, err := excepciones.ActivateLocal(excepciones.ForLocalDesktop(ctx), dir, excepciones.ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	if r := m.despachar(ctx, peticion{Action: "save_settings", Params: json.RawMessage(`{"idioma":"en"}`)}); !r.OK {
		t.Fatal(r.Error)
	}
	if excepciones.Check("tsa_http", excepciones.WithConfigDir(ctx, dir)) != nil {
		t.Fatal("guardar preferencias borra excepción")
	}
	if r := m.despachar(ctx, peticion{Action: "save_settings", Params: json.RawMessage(`{"excepciones_inseguras":{"rsa_corta":true}}`)}); r.OK {
		t.Fatal("guardar genérico activa")
	}
	if err := excepciones.SetUserAt(dir, "tsa_http", false); err != nil {
		t.Fatal(err)
	}
	// Un override desactivado no desaparece al guardar otras preferencias.
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"excepciones_inseguras":{"tsa_http":true}}`), 0600)
	m.despachar(ctx, peticion{Action: "save_settings", Params: json.RawMessage(`{"idioma":"es"}`)})
	if excepciones.Check("tsa_http", excepciones.WithConfigDir(ctx, dir)) == nil {
		t.Fatal("se perdió desactivación frente a config.json")
	}
}

func TestUnsafeRetryRejectsNonCanonicalCertificateID(t *testing.T) {
	m, ctx, p, _ := retryFixture(t)
	for _, id := range []string{" ", " cert-local ", "csc-remote "} {
		var params paramsFirma
		json.Unmarshal(p.Params, &params)
		params.CertificateID = id
		raw, _ := json.Marshal(params)
		if r := m.prepareUnsafeRetry(ctx, peticion{Action: " SIGN ", Params: raw}); r != nil {
			r.clear()
			t.Fatalf("certificado ambiguo retenido: %q", id)
		}
	}
	p.Action = " SIGN "
	r := m.prepareUnsafeRetry(ctx, p)
	if r == nil || r.params.Action != "sign" {
		t.Fatal("acción no normalizada")
	}
	r.clear()
}

func TestUnsafeRetryUsesSigningTimeoutAndSensitiveFrames(t *testing.T) {
	if desktopIPCTimeoutDe("unsafe_operation_retry") != longIPCOperationTimeout {
		t.Fatal("reintento pierde el plazo de firma")
	}
	for _, action := range []string{"unsafe_exceptions_activate", "unsafe_operation_retry"} {
		if !isSensitiveIPCAction(action) {
			t.Fatal("trama no saneada", action)
		}
		if !requiereEstadoInterfaz(action) {
			t.Fatal("interfaz no revalidada", action)
		}
	}
}

func TestV3GenericSaveRejectsCaseFoldedReceipt(t *testing.T) {
	dir := t.TempDir()
	m := &Manejador{ConfigDir: dir, Settings: usersettings.New(dir), Loc: localizador.Para("es")}
	ctx := conEstadoInterfaz(context.Background(), interfazVerificada)
	for _, key := range []string{"EXCEPCIONES_INSEGURAS", "EXCEPCIONES_INSEGURAS_ACEPTACIONES"} {
		raw, _ := json.Marshal(map[string]any{key: map[string]any{"tsa_http": true}})
		if r := m.despachar(ctx, peticion{Action: "save_settings", Params: raw}); r.OK {
			t.Fatalf("guardado admite alias %s", key)
		}
	}
}
