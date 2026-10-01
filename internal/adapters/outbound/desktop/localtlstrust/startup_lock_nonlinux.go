//go:build !linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"context"
	"errors"
)

// WithLocalTLSStartupLock conserva la interfaz de arranque en otras plataformas.
func WithLocalTLSStartupLock(ctx context.Context, _ string, fn func() error) error {
	return withLocalTLSLockWithoutFlock(ctx, fn)
}

// WithLocalTLSInventoryLock conserva la interfaz en otras plataformas.
func WithLocalTLSInventoryLock(ctx context.Context, _ string, fn func() error) error {
	return withLocalTLSLockWithoutFlock(ctx, fn)
}

func withLocalTLSLockWithoutFlock(ctx context.Context, fn func() error) error {
	if ctx == nil || fn == nil {
		return errors.New("localtlstrust: contexto o función de arranque TLS no configurados")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}
