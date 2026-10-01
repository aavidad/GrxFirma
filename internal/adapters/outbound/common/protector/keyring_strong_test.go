// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/protector"
)

func TestLocalStrongKeyring_AutogeneraYExponeIdentidad(t *testing.T) {
	t.Parallel()

	cfg := t.TempDir()
	keyring := protector.NuevoLocalStrongKeyring(cfg)

	recipients, err := keyring.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(recipients) != 1 {
		t.Fatalf("se esperaba 1 destinatario fuerte, obtenido=%d", len(recipients))
	}
	if got := recipients[0]; got.ID == "" || len(got.MLKEM768PublicKey) == 0 || len(got.X25519PublicKey) == 0 {
		t.Fatalf("destinatario fuerte incompleto: %#v", got)
	}

	keys, err := keyring.DecryptionKeys(context.Background())
	if err != nil {
		t.Fatalf("DecryptionKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("se esperaba 1 clave fuerte, obtenido=%d", len(keys))
	}
	if keys[0].RecipientID != recipients[0].ID {
		t.Fatalf("recipientID inesperado: %q != %q", keys[0].RecipientID, recipients[0].ID)
	}
	if len(keys[0].MLKEM768Seed) == 0 || len(keys[0].X25519PrivateKey) == 0 {
		t.Fatalf("clave fuerte incompleta: %#v", keys[0])
	}

	files, err := os.ReadDir(filepath.Join(cfg, "protection", "strong"))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("se esperaba 1 fichero de identidad fuerte, obtenido=%d", len(files))
	}
}

func TestLocalStrongKeyring_ImportRechazaIDNoDerivado(t *testing.T) {
	source := protector.NuevoLocalStrongKeyring(t.TempDir())
	recipients, err := source.List(context.Background())
	if err != nil {
		t.Fatalf("List source: %v", err)
	}
	_, exported, err := source.Export(context.Background(), recipients[0].ID)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	malicious := bytes.Replace(
		exported,
		[]byte(`"id": "`+recipients[0].ID+`"`),
		[]byte(`"id": "../../victim"`),
		1,
	)
	destination := protector.NuevoLocalStrongKeyring(t.TempDir())
	if _, err := destination.Import(context.Background(), malicious); err == nil {
		t.Fatal("Import accepted an identifier unrelated to the public keys")
	}
}

func TestLocalStrongKeyring_ImportRechazaDestinoSymlink(t *testing.T) {
	source := protector.NuevoLocalStrongKeyring(t.TempDir())
	recipients, err := source.List(context.Background())
	if err != nil {
		t.Fatalf("List source: %v", err)
	}
	_, exported, err := source.Export(context.Background(), recipients[0].ID)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	cfg := t.TempDir()
	recipientDir := filepath.Join(cfg, "protection", "recipients")
	if err := os.MkdirAll(recipientDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	victim := filepath.Join(cfg, "victim.json")
	if err := os.WriteFile(victim, []byte("untouched"), 0o600); err != nil {
		t.Fatalf("WriteFile victim: %v", err)
	}
	link := filepath.Join(recipientDir, recipients[0].ID+".json")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	destination := protector.NuevoLocalStrongKeyring(cfg)
	if _, err := destination.Import(context.Background(), exported); err == nil {
		t.Fatal("Import accepted a symlink destination")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("ReadFile victim: %v", err)
	}
	if string(got) != "untouched" {
		t.Fatalf("victim changed to %q", got)
	}
}
