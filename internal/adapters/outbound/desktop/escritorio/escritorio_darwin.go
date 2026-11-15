// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package escritorio

// hayEscritorioSistema supone escritorio en macOS, como hasta ahora. macOS
// está aparcado: no se detecta la sesión y esta rama no se ha probado.
func hayEscritorioSistema() bool { return true }
