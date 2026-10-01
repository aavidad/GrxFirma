// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build cgo && !linux

package pkcs11store

// Este adaptador aislado se habilita exclusivamente para el auxiliar Linux.
func loadNativeModule(string) (module, error) { return nil, ErrModuloNoDisponible }
