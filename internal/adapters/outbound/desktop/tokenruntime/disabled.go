//go:build !linux || !pkcs11_preview || production

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tokenruntime

func EnabledInBuild() bool { return false }

func newRuntime(_ string, _ Prompt) *Runtime {
	return &Runtime{inactive: ErrNotApplicable, diagnostic: ErrNotApplicable}
}
