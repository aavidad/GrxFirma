// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
	"encoding/json"
	"errors"
	"grxfirma/internal/security/machinepolicy"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceRequiresLocalExplicitCurrentVersion(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	req := ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion}
	if _, err := ActivateLocal(context.Background(), dir, req); !errors.Is(err, ErrNonLocal) {
		t.Fatal("canal externo aceptado", err)
	}
	for _, flag := range []bool{false, true} {
		r := req
		r.Accepted = flag
		if flag {
			r.AcceptanceVersion = "anterior"
		}
		if _, err := ActivateLocal(ForLocalDesktop(context.Background()), dir, r); err == nil {
			t.Fatal("aceptación incompleta aceptada")
		}
	}
	if err := SetUserAt(dir, "tsa_http", true); err == nil {
		t.Fatal("setter sin aceptación habilita")
	}
	for channel, issuer := range map[string]func(context.Context) context.Context{"desktop": ForLocalDesktop, "android": ForLocalAndroid, "cli": ForLocalCLI} {
		t.Run(channel, func(t *testing.T) {
			d := t.TempDir()
			receipt, err := ActivateLocal(issuer(context.Background()), d, req)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.Channel != channel || receipt.TextVersion != AcceptanceVersion || receipt.AcceptedAt.IsZero() || receipt.Origin != acceptanceOrigin {
				t.Fatalf("recibo incompleto: %+v", receipt)
			}
			var found State
			for _, s := range ListAt(d) {
				if s.ID == req.ID {
					found = s
				}
			}
			if !found.Enabled || found.LastAcceptance == nil {
				t.Fatalf("preferencia no visible: %+v", found)
			}
			audit, err := os.ReadFile(filepath.Join(d, "unsafe-exceptions-audit.jsonl"))
			if err != nil || !strings.Contains(string(audit), AcceptanceVersion) {
				t.Fatal("sin auditoría", err)
			}
			if err = SetUserAt(d, req.ID, false); err != nil {
				t.Fatal(err)
			}
			if err = Check(req.ID, WithConfigDir(context.Background(), d)); err == nil {
				t.Fatal("no desactiva")
			}
		})
	}
}
func TestAcceptanceOrganizationBlocksAllCases(t *testing.T) {
	for _, e := range Catalog() {
		t.Run(e.ID, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado)
			machinepolicy.SetForTest(t, e.PolicyKey, false)
			dir := t.TempDir()
			req := ActivationRequest{ID: e.ID, Accepted: true, AcceptanceVersion: AcceptanceVersion}
			b := DescribeAt(dir, e.ID)
			if b.Activable || b.TitleKey == "" || b.RiskKey == "" || b.AcceptanceKey == "" || b.HelpKey == "" {
				t.Fatalf("metadatos: %+v", b)
			}
			if _, err := ActivateLocal(ForLocalDesktop(context.Background()), dir, req); err == nil {
				t.Fatal("la organización no prevalece")
			}
			machinepolicy.ErrorForTest(t, e.PolicyKey, errors.New("ilegible"))
			if DescribeAt(dir, e.ID).Activable {
				t.Fatal("política ilegible activable")
			}
			if _, err := ActivateLocal(ForLocalDesktop(context.Background()), dir, req); err == nil {
				t.Fatal("política ilegible permite")
			}
			if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
				t.Fatal("bloqueo escribió preferencias", err)
			}
		})
	}
}
func TestAcceptanceAuditFailureNeverEnables(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "unsafe-exceptions-audit.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := ActivateLocal(ForLocalCLI(context.Background()), dir, ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion})
	if err == nil {
		t.Fatal("error de auditoría ignorado")
	}
	if Check("tsa_http", WithConfigDir(context.Background(), dir)) == nil {
		t.Fatal("se activó sin auditoría")
	}
}
func TestAcceptancePreservesUnrelatedSettings(t *testing.T) {
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"idioma":"es","proxyEnabled":false}`), 0600)
	if _, err := ActivateLocal(ForLocalCLI(context.Background()), dir, ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc["idioma"] != "es" || doc["proxyEnabled"] != false {
		t.Fatal("preferencias perdidas")
	}
}
