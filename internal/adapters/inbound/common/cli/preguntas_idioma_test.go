// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// Lote 17c: las preguntas interactivas de la CLI (borrar autoselecciones e
// importar dominios) salen del catálogo en el idioma pedido, con la letra
// afirmativa de ese idioma, y las respuestas se aceptan como antes.
var preguntasPorIdioma = map[string]string{
	"es": "[s/N]", "ca": "[s/N]", "va": "[s/N]", "gl": "[s/N]", "it": "[s/N]", "pt": "[s/N]",
	"fr": "[o/N]", "de": "[j/N]", "eu": "[b/E]", "en": "[y/N]", "zh": "[y/N]",
}

func TestPreguntasCLIDelCatalogoEnCadaIdioma(t *testing.T) {
	for idioma, marca := range preguntasPorIdioma {
		loc := localizador.Para(idioma)
		for _, clave := range []string{"cli.autoselection.clear.confirm", "cli.domains.import.confirm"} {
			texto := loc.T(clave)
			if texto == clave || !strings.Contains(texto, marca) {
				t.Fatalf("%s/%s: la pregunta debe estar en el catálogo y llevar %s: %q", idioma, clave, marca, texto)
			}
		}
		for _, clave := range []string{"cli.confirm.cancelled", "cli.autoselection.empty", "cli.autoselection.clear.summary", "cli.domains.import.done"} {
			if texto := loc.T(clave); texto == clave || texto == "" {
				t.Fatalf("%s: falta la clave %s", idioma, clave)
			}
		}
	}
}

func TestBorrarAutoseleccionPreguntaEnElIdiomaYAceptaLasRespuestas(t *testing.T) {
	for idioma, marca := range preguntasPorIdioma {
		loc := localizador.Para(idioma)
		afirmativas := append(strings.Split(loc.T("cli.confirm.yes_answers"), ","), "s", "sí", "y", "yes")
		for _, resp := range append(afirmativas, "n", "", "no") {
			tmp := t.TempDir()
			runtimeDir := filepath.Join(tmp, "runtime")
			if err := os.MkdirAll(filepath.Join(runtimeDir, "grxfirma"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
			persistente := filepath.Join(tmp, "preferred-certificates.json")
			if err := os.WriteFile(persistente, []byte(`{"https://uno":"cert-uno"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			a := New(nil, nil)
			a.ConfigDir = tmp
			a.Localizador = loc
			a.Stdin = strings.NewReader(resp + "\n")
			var stdout, stderr strings.Builder
			a.Stdout, a.Stderr = &stdout, &stderr
			if rc := a.Run(context.Background(), []string{"-modo-cli", "-operacion", "resetear-autoseleccion-cert"}); rc != 0 {
				t.Fatalf("%s/%q: rc=%d stderr=%s", idioma, resp, rc, stderr.String())
			}
			salida := stdout.String()
			if !strings.Contains(salida, loc.T("cli.autoselection.clear.confirm")) || !strings.Contains(salida, marca) {
				t.Fatalf("%s: la pregunta no sale en su idioma: %q", idioma, salida)
			}
			_, err := os.Stat(persistente)
			borrado := os.IsNotExist(err)
			quiere := false
			for _, af := range afirmativas {
				if strings.EqualFold(resp, af) {
					quiere = true
				}
			}
			if borrado != quiere {
				t.Fatalf("%s: la respuesta %q: borrado=%v, esperado=%v", idioma, resp, borrado, quiere)
			}
			if !quiere && !strings.Contains(salida, loc.T("cli.confirm.cancelled")) {
				t.Fatalf("%s: falta el aviso de cancelación: %q", idioma, salida)
			}
		}
	}
}

func TestImportacionInicialPreguntaEnElIdioma(t *testing.T) {
	for idioma := range preguntasPorIdioma {
		loc := localizador.Para(idioma)
		a := New(nil, nil)
		a.Localizador = loc
		a.Stdin = strings.NewReader(strings.Split(loc.T("cli.confirm.yes_answers"), ",")[0] + "\n")
		var stdout strings.Builder
		a.Stdout = &stdout
		if !a.preguntarImportacionInicial() {
			t.Fatalf("%s: la respuesta afirmativa del idioma debe aceptar", idioma)
		}
		if stdout.String() != loc.T("cli.domains.import.confirm") {
			t.Fatalf("%s: pregunta %q", idioma, stdout.String())
		}
	}
}
