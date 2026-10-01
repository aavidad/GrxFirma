// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux

package main

import "time"

func readProcessCPUTime() time.Duration {
	return 0
}

func readProcessRSSBytes() uint64 {
	return 0
}
