// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

func TestCatalogDecisionMatrix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, e := range Catalog() {
		t.Run(e.ID, func(t *testing.T) {
			machinepolicy.UnsetForTest(t, e.PolicyKey)
			machinepolicy.UnsetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado)
			SetForTest(t, e.ID, false)
			var block *Block
			if err := Check(e.ID); !errors.As(err, &block) || block.ID != e.ID || block.BlockKey != e.BlockKey {
				t.Fatalf("bloqueo: %#v", err)
			}
			SetForTest(t, e.ID, true)
			ctx, notices := domain.ConAvisosOperacion(context.Background())
			if err := Require(ctx, e.ID); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(notices.Claves(), e.WarningKey) {
				t.Fatal("falta aviso", notices.Claves())
			}
			machinepolicy.SetForTest(t, e.PolicyKey, false)
			if err := Require(ctx, e.ID); !errors.As(err, &block) {
				t.Fatal("usuario soslaya máquina", err)
			}
			machinepolicy.ErrorForTest(t, e.PolicyKey, errors.New("ilegible"))
			if err := Check(e.ID); !errors.As(err, &block) {
				t.Fatal("política ilegible abierta", err)
			}
			machinepolicy.SetForTest(t, e.PolicyKey, true)
			SetForTest(t, e.ID, false)
			if err := Check(e.ID); err != nil {
				t.Fatal("opt-in máquina existente", err)
			}
		})
	}
}
func TestNegativeCMSPolicyClosesOnError(t *testing.T) {
	for _, id := range []string{"cms_cbc", "cms_cifrado_heredado", "cms_aes_ecb"} {
		t.Run(id, func(t *testing.T) {
			SetForTest(t, id, true)
			e, _ := Find(id)
			machinepolicy.UnsetForTest(t, e.PolicyKey)
			machinepolicy.SetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, true)
			if Check(id) == nil {
				t.Fatal("prohibición soslayada")
			}
			machinepolicy.ErrorForTest(t, machinepolicy.ProhibirCifradoCMSHeredado, errors.New("ilegible"))
			if Check(id) == nil {
				t.Fatal("prohibición ilegible abierta")
			}
		})
	}
}
func TestUpgrade127DoesNotEnableExceptions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "grxfirma")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.json", "settings.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"version":"0.0.127","secureConnections":true,"confirmToSign":false,"tsaUrl":"http://example.invalid","rest_habilitado":true}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range Catalog() {
		machinepolicy.UnsetForTest(t, e.PolicyKey)
	}
	machinepolicy.UnsetForTest(t, machinepolicy.ProhibirCifradoCMSHeredado)
	for _, s := range List() {
		if s.Enabled {
			t.Errorf("actualización activa %s", s.ID)
		}
	}
}
func TestUserSettingsPersistenceAndPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	e, _ := Find("tsa_http")
	machinepolicy.UnsetForTest(t, e.PolicyKey)
	dir, _ := configDir()
	if _, err := ActivateLocal(ForLocalCLI(context.Background()), dir, ActivationRequest{ID: e.ID, Accepted: true, AcceptanceVersion: AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	if err := Check(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := SetUser(e.ID, false); err != nil {
		t.Fatal(err)
	}
	if Check(e.ID) == nil {
		t.Fatal("no se desactiva")
	}
	machinepolicy.SetForTest(t, e.PolicyKey, false)
	if SetUser(e.ID, true) == nil {
		t.Fatal("cambio de ajuste fijado")
	}
}
func TestContextDirectoryIsIsolated(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	machinepolicy.UnsetForTest(t, "permitir_tsa_http")
	if _, err := ActivateLocal(ForLocalCLI(context.Background()), first, ActivationRequest{ID: "tsa_http", Accepted: true, AcceptanceVersion: AcceptanceVersion}); err != nil {
		t.Fatal(err)
	}
	if err := Check("tsa_http", WithConfigDir(context.Background(), first)); err != nil {
		t.Fatal(err)
	}
	if Check("tsa_http", WithConfigDir(context.Background(), second)) == nil {
		t.Fatal("excepción compartida entre perfiles")
	}
}
