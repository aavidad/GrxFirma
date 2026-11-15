// SPDX-License-Identifier: EUPL-1.2
package mobilebind

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"grxfirma/internal/adapters/inbound/mobile/androidintent"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

func TestMobileExceptionsRequireVersionedLocalAcceptance(t *testing.T) {
	f := &Facade{configDir: t.TempDir(), platform: "android"}
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	if _, err := f.SetUnsafeExceptionJSON(`{"id":"tsa_http","enabled":true}`); !errors.Is(err, excepciones.ErrAcceptanceRequired) {
		t.Fatalf("el booleano heredado no puede activar: %v", err)
	}
	if _, err := f.ActivateUnsafeExceptionJSON(`{"id":"tsa_http","accepted":false}`); !errors.Is(err, excepciones.ErrAcceptanceRequired) {
		t.Fatalf("sin aceptación expresa no se activa: %v", err)
	}
	if _, err := f.ActivateUnsafeExceptionJSON(`{"id":"tsa_http","accepted":true,"acceptanceVersion":"anterior"}`); err == nil {
		t.Fatal("la versión anterior no activa la excepción")
	}
	request := mustJSON(t, map[string]any{
		"id": "tsa_http", "accepted": true, "acceptanceVersion": excepciones.AcceptanceVersion,
	})
	raw, err := f.ActivateUnsafeExceptionJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	var accepted excepciones.Acceptance
	if json.Unmarshal([]byte(raw), &accepted) != nil || accepted.ID != "tsa_http" {
		t.Fatalf("aceptación local = %s", raw)
	}
	states, err := f.UnsafeExceptionsJSON()
	if err != nil {
		t.Fatal(err)
	}
	var values []excepciones.State
	if json.Unmarshal([]byte(states), &values) != nil || len(values) != len(excepciones.Catalog()) {
		t.Fatal("catálogo móvil")
	}
	if err := excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), f.configDir)); err != nil {
		t.Fatal(err)
	}
	other := &Facade{configDir: t.TempDir(), platform: "android"}
	if err := excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), other.configDir)); err == nil {
		t.Fatal("el ajuste se compartió entre fachadas")
	}
	ios := &Facade{configDir: t.TempDir(), platform: "ios"}
	if _, err := ios.ActivateUnsafeExceptionJSON(request); err == nil {
		t.Fatal("la capacidad Android no se concede a iOS")
	}
	if _, err := f.SetUnsafeExceptionJSON(`{"id":"tsa_http","enabled":false}`); err != nil {
		t.Fatal(err)
	}
	if err := excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), f.configDir)); err == nil {
		t.Fatal("la desactivación local no se aplicó")
	}
}

func TestMobileExceptionsIgnoreOperationAcceptanceAndPolicyWins(t *testing.T) {
	f := &Facade{configDir: t.TempDir(), platform: "android", androidIntentAdapter: androidintent.New(nil)}
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	request := mustJSON(t, map[string]any{
		"id": "tsa_http", "accepted": true, "acceptanceVersion": excepciones.AcceptanceVersion,
	})
	if _, err := f.ActivateUnsafeExceptionJSON(`{"id":"tsa_http","accepted":true,"acceptanceVersion":"` + excepciones.AcceptanceVersion + `","origin":"android"}`); err == nil {
		t.Fatal("la solicitud no puede elegir el origen")
	}
	_, _ = f.SignJSON(`{"format":"cades","unsafe_exception":` + request + `}`)
	_, _ = f.TranslateAndroidIntentJSON(`{"action":"android.intent.action.SEND","descriptor":"a.pdf","payload_base64":"JVBERi0xLjQK","unsafe_exception":` + request + `}`)
	if err := excepciones.Check("tsa_http", excepciones.WithConfigDir(context.Background(), f.configDir)); err == nil {
		t.Fatal("SignJSON o AndroidIntent activaron un ajuste")
	}
	machinepolicy.SetForTest(t, "permitir_tsa_http", false)
	var block *excepciones.Block
	if _, err := f.ActivateUnsafeExceptionJSON(request); !errors.As(err, &block) || block.ID != "tsa_http" || block.Activable {
		t.Fatalf("la directiva debe cerrar la activación: %v", err)
	}
	meta, err := f.UnsafeExceptionBlockJSON("tsa_http")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		ID                    string `json:"id"`
		Activable             bool   `json:"activable"`
		RetryRequiresResubmit bool   `json:"retry_requires_resubmit"`
	}
	if json.Unmarshal([]byte(meta), &info) != nil || info.ID != "tsa_http" || info.Activable || !info.RetryRequiresResubmit {
		t.Fatalf("metadata de bloqueo = %s", meta)
	}
	machinepolicy.ErrorForTest(t, "permitir_tsa_http", errors.New("ilegible"))
	if _, err := f.ActivateUnsafeExceptionJSON(request); !errors.As(err, &block) || block.Activable {
		t.Fatalf("directiva ilegible debe cerrar la activación: %v", err)
	}
}
