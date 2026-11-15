// SPDX-License-Identifier: EUPL-1.2
package perfil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/appdirs"
)

// Todas las carpetas propias de GrxFirma quedan dentro del perfil aislado,
// también en Windows, donde las decide %APPDATA% y no HOME.
func TestAislarLlevaTodasLasCarpetasAlPerfil(t *testing.T) {
	home := Nuevo(t)
	real, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{appdirs.Config(real), appdirs.Data(real), appdirs.State(real), appdirs.Cache(real), DirConfig(home)} {
		rel, err := filepath.Rel(home, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			t.Fatalf("%q queda fuera del perfil aislado %q", dir, home)
		}
	}
}
