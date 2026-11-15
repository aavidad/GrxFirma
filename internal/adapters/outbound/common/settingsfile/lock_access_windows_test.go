//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"context"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"path/filepath"
	"testing"
)

func TestWindowsLockHandlePermiteLecturaEscrituraYCerrojo(t *testing.T) {
	f, err := securefile.OpenLockFile(filepath.Join(t.TempDir(), ".settings.lock"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt([]byte("x"), 0); err != nil {
		t.Fatal("falta GENERIC_WRITE:", err)
	}
	var data [1]byte
	if _, err := f.ReadAt(data[:], 0); err != nil {
		t.Fatal("falta GENERIC_READ:", err)
	}
	unlock, err := lockFile(context.Background(), f)
	if err != nil {
		t.Fatal("LockFileEx:", err)
	}
	unlock()
}
