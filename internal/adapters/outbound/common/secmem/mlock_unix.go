// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux || darwin

package secmem

import "golang.org/x/sys/unix"

// lockMemory fija las páginas del buffer en RAM (mlock) para que el material
// sensible no acabe en swap. Best-effort: RLIMIT_MEMLOCK puede denegarlo
// (contenedores, usuarios sin privilegio) y en ese caso se sigue sin fijar.
func lockMemory(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return unix.Mlock(b) == nil
}

// unlockMemory libera el fijado. Llamar solo tras zeroizar el contenido.
func unlockMemory(b []byte) {
	if len(b) == 0 {
		return
	}
	_ = unix.Munlock(b)
}
