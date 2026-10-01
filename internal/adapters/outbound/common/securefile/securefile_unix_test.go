// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"grxfirma/internal/adapters/outbound/common/securefile"
)

func TestProtectFileAplica0600SinSeguirEnlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		t.Fatalf("ProtectFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permisos = %04o", info.Mode().Perm())
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlink no disponible: %v", err)
	}
	if err := securefile.ProtectFile(link, 0o600); err == nil {
		t.Fatal("ProtectFile aceptó un enlace simbólico")
	}
}

func TestReadFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := securefile.ReadFile(path)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ReadFile accepted a FIFO")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadFile blocked while opening a FIFO")
	}
}

func TestOpenAppendRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		file, err := securefile.OpenAppend(path, 0o600)
		if file != nil {
			_ = file.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("OpenAppend accepted a FIFO")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OpenAppend blocked while opening a FIFO")
	}
}
