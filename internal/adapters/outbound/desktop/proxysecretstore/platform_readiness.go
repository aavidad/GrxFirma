// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package proxysecretstore

import (
	"fmt"
	"os/exec"
)

func backendReadinessForPlatform(goos string) (backend string, reason string) {
	return backendReadinessForPlatformWithProbe(goos, exec.LookPath)
}

func backendReadinessForPlatformWithProbe(goos string, lookPath func(string) (string, error)) (backend string, reason string) {
	switch goos {
	case "darwin":
		return "keychain", "proxysecretstore: macOS detectado; backend Keychain del sistema soportado (requiere build con cgo en macOS)"
	case "windows":
		return windowsBackendReadiness(lookPath)
	default:
		return "unsupported", fmt.Sprintf("proxysecretstore: sin backend seguro soportado para %s", goos)
	}
}

// windowsBackendReadiness: DPAPI se usa mediante la API nativa de Windows
// (CryptProtectData), sin depender de powershell.exe ni de otras
// herramientas externas.
func windowsBackendReadiness(_ func(string) (string, error)) (backend string, reason string) {
	return windowsBackendName, "proxysecretstore: Windows detectado; backend DPAPI de usuario nativo preparado para proxySecretId"
}
