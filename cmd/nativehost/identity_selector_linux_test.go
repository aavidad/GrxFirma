// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package main

import (
	"slices"
	"strings"
	"testing"
)

// zenity y qarma no admiten --no-markup en --list: el --text se pinta como
// marcado Pango y con las secuencias \n interpretadas (comprobado con zenity
// 4.2.1 en Xvfb: "<b" da «Failed to set text from markup»). El origen, la
// finalidad y la operación vienen de la página y no pueden ocultar ni
// recolocar el texto del selector. Las celdas de la lista son texto plano.
func TestSelectorIdentidadLinuxNeutralizaMarcado(t *testing.T) {
	prompt := "Origen: <span size=\"1\">x</span> & \\n"
	opciones := []string{"1 · <b>Nombre</b>"}
	for _, d := range dialogosSelectorIdentidadLinux("GrxFirma", prompt, "Usar", "Cancelar", opciones) {
		switch d.bin {
		case "zenity", "qarma":
			i := slices.Index(d.args, "--text")
			if i < 0 {
				t.Fatalf("%s sin --text: %q", d.bin, d.args)
			}
			texto := d.args[i+1]
			if strings.ContainsAny(texto, "<>") || !strings.Contains(texto, "&lt;span") ||
				!strings.Contains(texto, "&amp;") || !strings.Contains(texto, `\\n`) {
				t.Fatalf("%s --text sin neutralizar: %q", d.bin, texto)
			}
			if !slices.Contains(d.args, opciones[0]) {
				t.Fatalf("%s: la celda del certificado debe ir tal cual: %q", d.bin, d.args)
			}
		case "kdialog":
			i := slices.Index(d.args, "--menu")
			if i < 0 || d.args[i+1] != `<qt>Origen: &lt;span size=&#34;1&#34;&gt;x&lt;/span&gt; &amp; \n</qt>` {
				t.Fatalf("kdialog --menu sin neutralizar: %q", d.args)
			}
		default:
			t.Fatalf("diálogo inesperado %s", d.bin)
		}
	}
}
