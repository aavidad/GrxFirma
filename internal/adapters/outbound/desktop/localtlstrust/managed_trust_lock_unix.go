// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package localtlstrust

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

func acquireManagedTrustLock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() {
				_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if (!errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN)) || time.Now().After(deadline) {
			_ = file.Close()
			return nil, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}
