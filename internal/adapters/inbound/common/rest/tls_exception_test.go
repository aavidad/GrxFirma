// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2

// En Windows la clave de la CA local se protege con DPAPI y no hay almacén
// sustituible (UseManagedCAKeyStorage solo existe fuera de Windows).

//go:build !windows

package rest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/security/excepciones"
	"grxfirma/internal/security/machinepolicy"
)

type caStoreUnavailable struct{}

func (caStoreUnavailable) Store(context.Context, string, []byte) error {
	return errors.New("sin almacén")
}
func (caStoreUnavailable) Load(context.Context, string) ([]byte, error) {
	return nil, errors.New("sin almacén")
}
func (caStoreUnavailable) Delete(context.Context, string) error { return nil }

func TestCALocalClaveEnClaroBloqueadaNoRotaLaCA(t *testing.T) {
	dir := t.TempDir()
	const prefix = "ca-bloqueada"
	cert, key, root, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix)
	if err != nil {
		t.Fatal(err)
	}
	rootBefore, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, prefix+"-root.key.pem")
	keyDER, err := localtlstrust.LoadManagedCAKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyDER, 0600); err != nil {
		t.Fatal(err)
	}
	localtlstrust.UseManagedCAKeyStorage(caStoreUnavailable{})
	t.Cleanup(func() { localtlstrust.UseManagedCAKeyStorage(nil) })
	machinepolicy.UnsetForTest(t, "permitir_ca_clave_fichero")
	excepciones.SetForTest(t, "ca_clave_fichero", false)
	_, _, _, err = EnsureLocalhostCertificateWithLocalCA(dir, prefix)
	var block *excepciones.Block
	if !errors.As(err, &block) || block.ID != "ca_clave_fichero" {
		t.Fatalf("bloqueo: %v", err)
	}
	rootAfter, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(rootAfter) != string(rootBefore) {
		t.Fatal("se rotó la CA pese al bloqueo")
	}
	if _, err := os.Stat(cert); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(key); err != nil {
		t.Fatal(err)
	}
}
