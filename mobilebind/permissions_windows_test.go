// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package mobilebind

import (
	"os"
	"testing"
)

func assertPrivateDirectoryMode(t *testing.T, info os.FileInfo) {
	t.Helper()
	if !info.IsDir() {
		t.Fatal("la ruta privada no es un directorio")
	}
	// Windows no expone una DACL como bits de modo POSIX: os.Chmod(0700)
	// conserva ModePerm como 0777. Las fachadas mobile se ejecutan en Android
	// e iOS; este helper mantiene portable su compilacion/prueba desde Windows.
}
