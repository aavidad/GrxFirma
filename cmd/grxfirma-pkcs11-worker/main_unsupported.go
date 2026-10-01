// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux || !cgo

package main

import "os"

// Native Windows keys use the system provider, not this Linux auxiliary.
func main() { os.Exit(78) }
