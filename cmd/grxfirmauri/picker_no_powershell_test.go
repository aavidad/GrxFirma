// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Los selectores del protocolo reciben nombres y extensiones del portal. En
// Windows deben usar exclusivamente los diálogos Win32 nativos: construir un
// script de PowerShell con esos datos permitía inyectar órdenes mediante
// comillas tipográficas (U+2018..U+201B), que PowerShell trata como comillas.
func TestSelectoresProtocoloNoInvocanPowerShell(t *testing.T) {
	for _, nombre := range []string{"load_picker_fyne.go", "save_picker_fyne.go"} {
		contenido, err := os.ReadFile(filepath.Join(".", nombre))
		if err != nil {
			t.Fatalf("leyendo %s: %v", nombre, err)
		}
		if strings.Contains(strings.ToLower(string(contenido)), `"powershell"`) {
			t.Fatalf("%s vuelve a invocar PowerShell con datos del portal", nombre)
		}
	}
}
