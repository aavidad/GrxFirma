// SPDX-License-Identifier: EUPL-1.2
package main

import "testing"

func TestConfigDirGUIRechazaHomeVacioORelativo(t *testing.T) {
	for _, home := range []string{"", ".", "relative"} {
		t.Run(home, func(t *testing.T) {
			t.Setenv("HOME", home)
			if dir, err := configDirGUI(); err == nil {
				t.Fatalf("HOME=%q produjo configuración %q", home, dir)
			}
		})
	}
}
