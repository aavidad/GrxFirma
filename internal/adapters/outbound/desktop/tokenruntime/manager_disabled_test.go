// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux || !pkcs11_preview || production

package tokenruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/ports"
)

func TestManagerDisabledBuildDoesNotAccessConfiguration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-created")
	for _, path := range []string{dir, "\x00not-a-path", "relative"} {
		m := NewManager(path)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, call := range []func(context.Context) (ports.TokenSettingsSnapshot, error){m.Load, m.Diagnose} {
			s, err := call(ctx)
			if err != nil || s.Available || s.Editable || s.State != "unavailable" || s.Exists || s.Revision != "" {
				t.Fatal("disabled manager did not short-circuit before IO")
			}
		}
		if _, err := m.Save(context.Background(), ports.TokenSettingsUpdate{Confirmed: true, Enabled: true}); !errors.Is(err, ports.ErrTokenSettingsUnavailable) {
			t.Fatal("disabled save accepted")
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("disabled manager created configuration directory")
	}
}
