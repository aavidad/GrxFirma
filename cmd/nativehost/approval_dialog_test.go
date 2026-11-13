// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"slices"
	"strings"
	"testing"
)

// C-02: zenity y qarma interpretan marcado Pango y secuencias \n en --text;
// kdialog decide solo si el texto es HTML. El texto de la página no puede
// vaciar el diálogo ni añadir líneas.
func TestDialogosAprobacionLinuxNeutralizanMarcado(t *testing.T) {
	mensaje := `a <b & \n`
	for _, d := range dialogosAprobacionLinux("GrxFirma", mensaje) {
		switch d.bin {
		case "zenity", "qarma":
			if !slices.Contains(d.args, "--no-markup") {
				t.Fatalf("%s sin --no-markup: %q", d.bin, d.args)
			}
			i := slices.Index(d.args, "--text")
			if i < 0 || d.args[i+1] != `a <b & \\n` {
				t.Fatalf("%s --text = %q", d.bin, d.args)
			}
		case "kdialog":
			if d.args[1] != `<qt>a &lt;b &amp; \n</qt>` {
				t.Fatalf("kdialog texto = %q", d.args[1])
			}
		default:
			t.Fatalf("diálogo inesperado %s", d.bin)
		}
	}
}

func TestSanitizarTextoSelectorEliminaBidi(t *testing.T) {
	if got := sanitizarTextoSelector("x‮y⁦z", 50); strings.ContainsAny(got, "‮⁦") {
		t.Fatalf("sanitizarTextoSelector conserva caracteres bidi: %q", got)
	}
}
