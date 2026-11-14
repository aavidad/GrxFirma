// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
)

// Lote 17b (4): la confirmación [y/N] de «aceptar en este equipo» admite la
// respuesta afirmativa del idioma de la persona (o, j, b, s…) además de «y»,
// y la pregunta enseña esa letra. Lo de antes (s, si, sí, y, yes) se sigue
// aceptando en todos los idiomas.
func TestConfirmacionAceptaLaRespuestaDelIdioma(t *testing.T) {
	entrada := ficheroFirmaCLI(t)
	forzable := []domain.ObstaculoConfianza{{Tipo: domain.ConfianzaCADesconocida, Huella: huellaCLIPrueba}}
	casos := map[string]string{
		"es": "s", "ca": "s", "va": "s", "gl": "s", "it": "s", "pt": "s",
		"fr": "o", "de": "j", "eu": "b", "en": "y", "zh": "y",
	}
	for idioma, letra := range casos {
		loc := localizador.Para(idioma)
		respuestas := strings.Split(loc.T("cli.confirm.yes_answers"), ",")
		if respuestas[0] != letra {
			t.Fatalf("%s: la primera respuesta afirmativa es %q, want %q", idioma, respuestas[0], letra)
		}
		pregunta := loc.T("verify.forced_trust.cli.confirm")
		if !strings.Contains(pregunta, "["+letra+"/") {
			t.Fatalf("%s: la pregunta debe enseñar la letra %q: %q", idioma, letra, pregunta)
		}
		for _, respuesta := range append(respuestas, "y", "yes", "s", "sí", strings.ToUpper(letra)) {
			gestion := &gestionConfianzaMock{}
			a, _, _ := adaptadorConfianza(gestion, forzable, true, " "+respuesta+" \n")
			a.Localizador = loc
			if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada}); rc != 0 || len(gestion.aceptadas) != 1 {
				t.Fatalf("%s: la respuesta %q debe aceptar: rc=%d %v", idioma, respuesta, rc, gestion.aceptadas)
			}
		}
		for _, respuesta := range []string{"n", "", "no", "nein", "non", "ez", "x"} {
			gestion := &gestionConfianzaMock{}
			a, _, _ := adaptadorConfianza(gestion, forzable, true, respuesta+"\n")
			a.Localizador = loc
			if rc := a.Run(context.Background(), []string{"-aceptar-confianza", "-entrada", entrada}); rc != 0 || len(gestion.aceptadas) != 0 {
				t.Fatalf("%s: la respuesta %q no debe aceptar: rc=%d %v", idioma, respuesta, rc, gestion.aceptadas)
			}
		}
	}
}
