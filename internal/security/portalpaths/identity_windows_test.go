//go:build windows

// SPDX-License-Identifier: EUPL-1.2
package portalpaths

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWindowsUnionNoPermiteLeerDatosPrivados(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	roaming := filepath.Join(home, "AppData", "Roaming")
	local := filepath.Join(home, "AppData", "Local")
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", local)
	for _, pair := range [][2]string{{"Application Data", roaming}, {"Local Settings", local}} {
		private := filepath.Join(pair[1], "GrxFirma")
		if err := os.MkdirAll(private, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(private, "settings.json"), []byte("privado"), 0600); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(home, pair[0])
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", alias, pair[1]).CombinedOutput(); err != nil {
			t.Fatalf("crear unión: %v %s", err, out)
		}
		path := filepath.Join(alias, "GrxFirma", "settings.json")
		if _, err := Check(path); !errors.Is(err, ErrProtected) {
			t.Fatalf("unión admitida: %s %v", path, err)
		}
		if _, err := ReadFileLimit(path, 1024); !errors.Is(err, ErrProtected) {
			t.Fatalf("datos expuestos por unión: %v", err)
		}
	}
}
