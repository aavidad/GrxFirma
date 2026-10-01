// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryTreeReader_IgnoraFicherosLegacyTemporalesYSistema(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files := map[string]string{
		"ok.txt":      "ok",
		".DS_Store":   "skip",
		"thumbs.db":   "skip",
		"~$temp.docx": "skip",
		".desktop":    "skip",
		"._.Trashes":  "skip",
		"visible.xml": "ok2",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	reader := NuevoDirectoryTreeReader()
	got, err := reader.ListFiles(context.Background(), root, false)
	if err != nil {
		t.Fatalf("ListFiles() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ListFiles()) = %d, want 2", len(got))
	}
	if got[0].RelativePath != "ok.txt" || got[1].RelativePath != "visible.xml" {
		t.Fatalf("relative paths inesperadas: %#v", got)
	}
}

func TestDirectoryTreeReader_IgnoraSymlinkDeFichero(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile outside: %v", err)
	}
	link := filepath.Join(root, "linked.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	reader := NuevoDirectoryTreeReader()
	for _, recursive := range []bool{false, true} {
		got, err := reader.ListFiles(context.Background(), root, recursive)
		if err != nil {
			t.Fatalf("ListFiles(recursive=%v): %v", recursive, err)
		}
		if len(got) != 0 {
			t.Fatalf("ListFiles(recursive=%v) included symlink: %#v", recursive, got)
		}
	}
	if _, err := reader.ReadFile(context.Background(), link); err == nil {
		t.Fatal("ReadFile accepted a final-component symlink")
	}
}

func TestDirectoryTreeReader_RespetaContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := NuevoDirectoryTreeReader()
	if _, err := reader.ReadFile(ctx, filepath.Join(t.TempDir(), "a.txt")); err == nil {
		t.Fatal("ReadFile() ignoro la cancelacion")
	}
	if _, err := reader.ListFiles(ctx, t.TempDir(), false); err == nil {
		t.Fatal("ListFiles() ignoro la cancelacion")
	}
}
