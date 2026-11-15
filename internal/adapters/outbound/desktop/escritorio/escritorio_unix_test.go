// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows && !darwin

package escritorio

import "testing"

func TestSinDisplayNiWaylandNoHayEscritorio(t *testing.T) {
	casos := []struct {
		display, wayland string
		hay              bool
	}{
		{"", "", false},
		{"  ", "", false},
		{":0", "", true},
		{"", "wayland-0", true},
	}
	for _, caso := range casos {
		t.Setenv("DISPLAY", caso.display)
		t.Setenv("WAYLAND_DISPLAY", caso.wayland)
		if got := hayEscritorioSistema(); got != caso.hay {
			t.Errorf("DISPLAY=%q WAYLAND_DISPLAY=%q: hay=%v, se esperaba %v", caso.display, caso.wayland, got, caso.hay)
		}
	}
}
