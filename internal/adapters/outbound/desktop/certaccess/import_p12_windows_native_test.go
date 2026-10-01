// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certaccess

import (
	"os"
	"strings"
	"testing"
)

// La importación en Windows usa la API nativa: no debe volver a lanzar
// PowerShell (bloqueado en muchos puestos de la Administración).
func TestImportacionWindowsNoUsaPowerShell(t *testing.T) {
	nativo, err := os.ReadFile("import_p12_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(nativo), `"os/exec"`) {
		t.Fatal("la importación Windows vuelve a lanzar procesos externos")
	}
	comun, err := os.ReadFile("certaccess.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(comun), "windowsP12ImportScript") {
		t.Fatal("certaccess.go conserva el script PowerShell de importación")
	}
}
