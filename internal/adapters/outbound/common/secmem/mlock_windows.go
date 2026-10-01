// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package secmem

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// lockMemory fija las páginas del buffer en RAM (VirtualLock) para que el
// material sensible no acabe en el fichero de paginación. Best-effort.
func lockMemory(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return windows.VirtualLock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))) == nil
}

// unlockMemory libera el fijado. Llamar solo tras zeroizar el contenido.
func unlockMemory(b []byte) {
	if len(b) == 0 {
		return
	}
	_ = windows.VirtualUnlock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
}
