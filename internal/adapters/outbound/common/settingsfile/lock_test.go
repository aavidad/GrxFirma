// SPDX-License-Identifier: EUPL-1.2
package settingsfile

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOtherProcessCannotPassSettingsLock(t *testing.T) {
	dir := os.Getenv("GRXFIRMA_TEST_SETTINGS_LOCK_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	ready := filepath.Join(dir, "child-ready")
	entered := filepath.Join(dir, "child-entered")
	if os.Getenv("GRXFIRMA_TEST_SETTINGS_LOCK_CHILD") == "1" {
		if err := os.WriteFile(ready, []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		err := WithLock(context.Background(), dir, func() error { return os.WriteFile(entered, []byte("entered"), 0600) })
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	release := make(chan struct{})
	holderReady := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithLock(context.Background(), dir, func() error { close(holderReady); <-release; return nil })
	}()
	select {
	case <-holderReady:
	case err := <-done:
		t.Fatalf("no se pudo tomar el cerrojo: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("agotado al tomar el cerrojo")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".settings.lock")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sin cerrojo")
		}
		time.Sleep(time.Millisecond)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOtherProcessCannotPassSettingsLock$")
	cmd.Env = []string{"GRXFIRMA_TEST_SETTINGS_LOCK_CHILD=1", "GRXFIRMA_TEST_SETTINGS_LOCK_DIR=" + dir}
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		close(release)
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			close(release)
			t.Fatal("hijo no comenzó")
		}
		time.Sleep(time.Millisecond)
	}
	// The separate process has reached WithLock but cannot commit while held.
	time.Sleep(70 * time.Millisecond)
	if _, err := os.Stat(entered); !errors.Is(err, os.ErrNotExist) {
		close(release)
		t.Fatalf("otro proceso atravesó el cerrojo: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entered); err != nil {
		t.Fatalf("otro proceso no pudo continuar: %v", err)
	}
}
func TestLockRejectsRelativeDirectory(t *testing.T) {
	if err := WithLock(context.Background(), "settings", func() error { t.Fatal("callback invocado"); return nil }); !errors.Is(err, ErrDirectory) {
		t.Fatalf("directorio relativo: %v", err)
	}
}

func TestLockCannotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".settings.lock")); err != nil {
		t.Skipf("symlink no disponible: %v", err)
	}
	if err := WithLock(context.Background(), dir, func() error { t.Fatal("callback invocado"); return nil }); err == nil {
		t.Fatal("cerrojo simbólico admitido")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("destino cambiado: %v %q", err, data)
	}
}

func TestLockWaitRespectsContext(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	releaseHolder := sync.OnceFunc(func() { close(release) })
	defer releaseHolder()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithLock(context.Background(), dir, func() error { close(ready); <-release; return nil })
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("no se pudo tomar el cerrojo: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("agotado al tomar el cerrojo")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	err := WithLock(ctx, dir, func() error { t.Fatal("callback tras cancelación"); return nil })
	releaseHolder()
	if holderErr := <-done; holderErr != nil {
		t.Fatal(holderErr)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("espera sin cancelación: %v", err)
	}
}
