// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package securefile

import (
	"fmt"
	"os"
	"syscall"
)

func checkOpenedDirOwner(dir *os.File) error {
	info, err := dir.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: owner unavailable", ErrNotPrivateDir)
	}
	if uint64(st.Uid) != uint64(os.Geteuid()) { // #nosec G115 -- geteuid nunca es negativo y uint32 cabe en uint64.
		return fmt.Errorf("%w: owned by uid %d", ErrNotPrivateDir, st.Uid)
	}
	return nil
}

func checkOpenedDirPrivate(dir *os.File) error {
	info, err := dir.Stat()
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: mode %o", ErrNotPrivateDir, info.Mode().Perm())
	}
	return nil
}
