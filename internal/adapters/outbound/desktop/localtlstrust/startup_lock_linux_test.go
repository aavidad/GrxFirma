// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWithLocalTLSStartupLockSerializesAndHonorsContext(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config", "grxfirma")
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- WithLocalTLSStartupLock(context.Background(), dir, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("el primer arranque no adquirió el bloqueo")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := WithLocalTLSStartupLock(ctx, dir, func() error {
		t.Error("se ejecutó con el bloqueo retenido")
		return nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("segundo arranque: obtenido %v, se esperaba DeadlineExceeded", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("primer arranque: %v", err)
	}
	called := false
	if err := WithLocalTLSStartupLock(context.Background(), dir, func() error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("bloqueo liberado: called=%v, err=%v", called, err)
	}
}

func TestWithLocalTLSStartupLockRejectsSymlinksAndUnsafeLock(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatal(err)
	}
	if err := WithLocalTLSStartupLock(context.Background(), linkedDir, func() error {
		t.Error("se siguió el enlace del directorio")
		return nil
	}); err == nil {
		t.Fatal("se aceptó un directorio simbólico")
	}
	lockPath := filepath.Join(realDir, localTLSStartupLockName)
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}
	if err := WithLocalTLSStartupLock(context.Background(), realDir, func() error {
		t.Error("se abrió un fichero de bloqueo simbólico")
		return nil
	}); err == nil {
		t.Fatal("se aceptó un bloqueo simbólico")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "unchanged" {
		t.Fatalf("se alteró el destino: %q, %v", data, err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WithLocalTLSStartupLock(context.Background(), realDir, func() error {
		t.Error("se usó un bloqueo legible por otros")
		return nil
	}); err == nil {
		t.Fatal("se aceptó un bloqueo con permisos excesivos")
	}
}

func TestWithLocalTLSInventoryLockIsSeparateAndSerializes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config", "tls")
	certFile := filepath.Join(dir, "localhost-root-ca.pem")
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- WithLocalTLSInventoryLock(context.Background(), certFile, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("no se adquirió el bloqueo de inventario")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := WithLocalTLSInventoryLock(ctx, certFile, func() error {
		t.Error("se modificó el inventario durante otro cambio")
		return nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("segundo cambio: obtenido %v, se esperaba DeadlineExceeded", err)
	}
	if err := WithLocalTLSStartupLock(context.Background(), filepath.Dir(dir), func() error { return nil }); err != nil {
		t.Fatalf("el bloqueo de inventario retuvo el de arranque: %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("primer cambio: %v", err)
	}
}

func TestWithLocalTLSInventoryLockRejectsSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(root, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatal(err)
	}
	if err := WithLocalTLSInventoryLock(context.Background(), filepath.Join(linkedDir, "ca.pem"), func() error {
		t.Error("se siguió el enlace al directorio de la CA")
		return nil
	}); err == nil {
		t.Fatal("se aceptó una ruta de CA con directorio simbólico")
	}
}
