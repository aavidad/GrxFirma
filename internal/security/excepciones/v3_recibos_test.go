// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
	"encoding/json"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/perfil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func v3Receipt(id string) Acceptance {
	return Acceptance{ID: id, AcceptedAt: time.Now().UTC().Add(-time.Minute), TextVersion: AcceptanceVersion, Origin: acceptanceOrigin, Channel: "cli"}
}
func v3Write(t *testing.T, dir, file string, receipt *Acceptance) {
	t.Helper()
	doc := map[string]any{UserField: map[string]bool{"tsa_http": true}}
	if receipt != nil {
		doc[AcceptanceField] = map[string]Acceptance{"tsa_http": *receipt}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, file), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestV3TrueWithoutReceiptIsDisabled(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	for _, file := range []string{"settings.json", "config.json"} {
		t.Run(file, func(t *testing.T) {
			dir := t.TempDir()
			v3Write(t, dir, file, nil)
			if Check("tsa_http", WithConfigDir(context.Background(), dir)) == nil {
				t.Fatalf("%s true sin recibo activa", file)
			}
		})
	}
}
func TestV3ReceiptMustMatchCurrentTextAndOrigin(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	for _, kind := range []string{"old_version", "zero_date", "wrong_origin", "wrong_id", "future_date", "missing_channel"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			r := v3Receipt("tsa_http")
			switch kind {
			case "old_version":
				r.TextVersion = "2026-11-14.0"
			case "zero_date":
				r.AcceptedAt = time.Time{}
			case "wrong_origin":
				r.Origin = "portal"
			case "wrong_id":
				r.ID = "rsa_corta"
			case "future_date":
				r.AcceptedAt = time.Now().Add(time.Hour)
			case "missing_channel":
				r.Channel = ""
			}
			v3Write(t, dir, "settings.json", &r)
			if Check("tsa_http", WithConfigDir(context.Background(), dir)) == nil {
				t.Fatal("recibo inválido activa", kind)
			}
		})
	}
}
func TestV3ReceiptNamesAreExact(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	r := v3Receipt("tsa_http")
	raw, _ := json.Marshal(map[string]any{UserField: map[string]bool{"tsa_http": true}, strings.ToUpper(AcceptanceField): map[string]Acceptance{"tsa_http": r}})
	os.WriteFile(filepath.Join(dir, "settings.json"), raw, 0600)
	if readAcceptance(dir, "tsa_http") != nil {
		t.Fatal("recibo falso por mayúsculas visible")
	}
	if Check("tsa_http", WithConfigDir(context.Background(), dir)) == nil {
		t.Fatal("alias de recibo activa")
	}
	if !ReservedSettings(strings.ToUpper(UserField)) || !ReservedSettings(strings.ToUpper(AcceptanceField)) {
		t.Fatal("guardado no reserva aliascase")
	}
}
func TestV3RelativeDirectoryFailsClosed(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	root := t.TempDir()
	t.Chdir(root)
	os.Mkdir("cfg", 0700)
	r := v3Receipt("tsa_http")
	v3Write(t, "cfg", "settings.json", &r)
	ctx := WithConfigDir(context.Background(), "cfg")
	if Check("tsa_http", ctx) == nil {
		t.Fatal("config relativa activa")
	}
	if _, err := ActivateLocal(ForLocalCLI(ctx), "", ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion}); err == nil {
		t.Fatal("activa sin directorio")
	}
}
func TestV3CurrentReceiptInEachFileWorks(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	for _, file := range []string{"config.json", "settings.json"} {
		t.Run(file, func(t *testing.T) {
			dir := t.TempDir()
			r := v3Receipt("tsa_http")
			v3Write(t, dir, file, &r)
			if err := Check("tsa_http", WithConfigDir(context.Background(), dir)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestV3AndroidHasNoImplicitHomeSettings(t *testing.T) {
	perfil.Nuevo(t)
	if dir, err := configDirForOS("android"); err == nil || dir != "" {
		t.Fatalf("Android usa HOME implícito: %q %v", dir, err)
	}
}

func TestV3AcceptingAnotherCaseDoesNotCanonicalizeForeignReceipt(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	machinepolicy.UnsetForTest(t, "permitir_rsa_corta")
	for _, variant := range []string{"uppercase", "extra"} {
		t.Run(variant, func(t *testing.T) {
			dir := t.TempDir()
			r := v3Receipt("tsa_http")
			raw, _ := json.Marshal(r)
			var fields map[string]json.RawMessage
			json.Unmarshal(raw, &fields)
			if variant == "uppercase" {
				upper := map[string]json.RawMessage{}
				for k, v := range fields {
					upper[strings.ToUpper(k)] = v
				}
				fields = upper
			} else {
				fields["portal"] = json.RawMessage(`true`)
			}
			bad, _ := json.Marshal(fields)
			doc, _ := json.Marshal(map[string]any{UserField: map[string]bool{"tsa_http": true}, AcceptanceField: map[string]json.RawMessage{"tsa_http": bad}})
			os.WriteFile(filepath.Join(dir, "settings.json"), doc, 0600)
			ctx := WithConfigDir(context.Background(), dir)
			if Check("tsa_http", ctx) == nil {
				t.Fatal("precondición: recibo mal formado activa")
			}
			if _, err := ActivateLocal(ForLocalCLI(ctx), dir, ActivationRequest{ID: "rsa_corta", Accepted: true, AcceptanceVersion: AcceptanceVersion}); err != nil {
				t.Fatal(err)
			}
			if Check("tsa_http", ctx) == nil {
				t.Fatal("activar otro caso legitima recibo falso")
			}
		})
	}
}

// P10: la capacidad reservada para un futuro callback local no se obtiene
// declarando su nombre en datos recibidos de un portal.
func TestV3ProtocolDialogNameDoesNotGrantLocalAuthority(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	ctx := context.WithValue(context.Background(), "source", "protocol_dialog")
	ctx = context.WithValue(ctx, "channel", "protocol_dialog")
	if IsLocal(ctx) {
		t.Fatal("datos externos otorgan capacidad local")
	}
	if _, err := ActivateLocal(ctx, t.TempDir(), ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion}); err != ErrNonLocal {
		t.Fatalf("activación externa: %v", err)
	}
}
