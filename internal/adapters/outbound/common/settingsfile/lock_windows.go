//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"time"
)

func lockFile(ctx context.Context, file *os.File) (func(), error) {
	h := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	for {
		err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
		if err == nil {
			return func() { _ = windows.UnlockFileEx(h, 0, 1, 0, &overlapped) }, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}
