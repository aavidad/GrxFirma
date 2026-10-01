// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package filesystem

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDirectoryTreeReader_IgnoraFIFO(t *testing.T) {
	root := t.TempDir()
	pipe := filepath.Join(root, "pipe")
	if err := unix.Mkfifo(pipe, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	reader := NuevoDirectoryTreeReader()
	got, err := reader.ListFiles(context.Background(), root, true)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListFiles included FIFO: %#v", got)
	}
	if _, err := reader.ReadFile(context.Background(), pipe); err == nil {
		t.Fatal("ReadFile accepted a FIFO")
	}
}
