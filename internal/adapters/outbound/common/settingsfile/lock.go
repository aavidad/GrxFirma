// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2

// Package settingsfile coordinates updates of the user settings document.
// Every writer of exception settings and general preferences takes the same
// process-wide and cross-process lock before its read/modify/write sequence.
package settingsfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

const UserField = "excepciones_inseguras"
const AcceptanceField = "excepciones_inseguras_aceptaciones"

// Reserved recognizes the security fields, including ambiguous case variants
// accepted by Go's case-insensitive struct JSON decoder.
func Reserved(key string) bool {
	return strings.EqualFold(key, UserField) || strings.EqualFold(key, AcceptanceField)
}

var processLock = make(chan struct{}, 1)

func init() { processLock <- struct{}{} }

var ErrDirectory = errors.New("excepciones.directorio_no_disponible")

// WithLock serializes settings.json transactions across processes. The lock
// file is never removed, so waiters cannot end up locking different inodes.
func WithLock(ctx context.Context, dir string, fn func() error) error {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return context.Canceled
		}
		return ctx.Err()
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || filepath.Dir(dir) == dir {
		return ErrDirectory
	}
	select {
	case <-processLock:
		defer func() { processLock <- struct{}{} }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	directory, err := securefile.OpenDir(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := protectSettingsDirectory(directory); err != nil {
		return err
	}
	if info, err := directory.Stat(); err != nil {
		return err
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return ErrDirectory
	}
	file, err := securefile.OpenLockFile(filepath.Join(dir, ".settings.lock"), 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	unlock, err := lockFile(ctx, file)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}
