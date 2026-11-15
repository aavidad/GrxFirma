// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package settingsfile

import "os"

// En Linux y macOS el modo 0700 del propietario ya decide el acceso; el caso
// de Windows (propietario Administradores tras escribir elevado) no se da.
func checkSettingsUser(*os.File, string) error { return nil }

func claimSettingsDirectory(*os.File, string) {}
