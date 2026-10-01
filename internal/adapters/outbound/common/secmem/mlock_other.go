// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !darwin && !windows

package secmem

// En plataformas sin mlock/VirtualLock el fijado de páginas no está
// disponible; el zeroing explícito sigue aplicando.
func lockMemory([]byte) bool { return false }

func unlockMemory([]byte) {}
