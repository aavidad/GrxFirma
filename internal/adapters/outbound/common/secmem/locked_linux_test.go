// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package secmem

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNewLockedSizeRealMemory(t *testing.T) {
	b, err := NewLockedSize(32)
	if errors.Is(err, ErrLockUnavailable) {
		t.Skip("el entorno no permite fijar memoria")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer b.Destroy()
	if !b.Locked() || !bytes.Equal(b.Bytes(), make([]byte, 32)) {
		t.Fatal("reserva estricta no fijada/inicializada a cero")
	}
	view := b.Bytes()
	copy(view, "QA-sin-identidades")
	b.Destroy()
	if !bytes.Equal(view, make([]byte, len(view))) {
		t.Fatal("vista retenida sin borrar")
	}
}

func TestLockedMemoryLimitOnlyInSubprocess(t *testing.T) {
	const marker = "GRXFIRMA_SECMEM_RLIMIT_TEST_CHILD"
	if os.Getenv(marker) == "1" {
		// CAP_IPC_LOCK permite ignorar RLIMIT_MEMLOCK; no alterar capacidades.
		var capabilities [2]unix.CapUserData
		header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
		if err := unix.Capget(&header, &capabilities[0]); err != nil {
			t.Fatal(err)
		}
		if capabilities[0].Effective&(1<<unix.CAP_IPC_LOCK) != 0 {
			t.Skip("CAP_IPC_LOCK anula el límite del test")
		}
		if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{}); err != nil {
			t.Fatal(err)
		}
		strict, err := NewLockedSize(16)
		if strict != nil || !errors.Is(err, ErrLockUnavailable) {
			t.Fatalf("strict degradado con RLIMIT cero: %v", err)
		}
		legacy := New([]byte("QA-sintetica"))
		defer legacy.Destroy()
		if legacy.Locked() || !bytes.Equal(legacy.Bytes(), []byte("QA-sintetica")) {
			t.Fatal("legacy best-effort cambió bajo RLIMIT cero")
		}
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestLockedMemoryLimitOnlyInSubprocess$", "-test.v")
	command.Env = append(os.Environ(), marker+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("subproceso RLIMIT: %v\n%s", err, output)
	}
}
