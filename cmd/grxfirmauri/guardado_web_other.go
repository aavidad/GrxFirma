// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package main

// marcarDescargaWeb no hace nada fuera de Windows: Linux y macOS no tienen un
// equivalente que consulten los programas que abren el fichero.
func marcarDescargaWeb(string, string) {}
