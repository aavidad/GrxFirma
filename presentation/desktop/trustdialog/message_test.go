// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog

import (
	"strings"
	"testing"
)

func TestBuildPromptMessageForUI_IncluyeOrigenYRiesgoResidente(t *testing.T) {
	t.Parallel()

	msg := BuildPromptMessageForUI("https://portal.ejemplo.gob.es")
	if msg.OriginValue != "https://portal.ejemplo.gob.es" {
		t.Fatalf("origin inesperado: %q", msg.OriginValue)
	}
	residentRisk := strings.ToLower(msg.ResidentRisk)
	if !strings.Contains(residentRisk, "residente") && !strings.Contains(residentRisk, "resident") {
		t.Fatalf("residentRisk no menciona el modo residente: %q", msg.ResidentRisk)
	}
	primary := strings.ToLower(msg.PrimaryMessage)
	if !strings.Contains(primary, "firma electrónica") && !strings.Contains(primary, "electronic signing") {
		t.Fatalf("primaryMessage inesperado: %q", msg.PrimaryMessage)
	}
}

func TestBuildPromptReport_ContieneBloquesClave(t *testing.T) {
	t.Parallel()

	origin := "https://sede.ejemplo.es"
	msg := buildPromptMessage(origin)
	report := buildPromptReport(origin)
	for _, fragment := range []string{msg.Headline, msg.OriginLabel, origin, tt("Riesgo si continúas")} {
		if !strings.Contains(report, fragment) {
			t.Fatalf("el informe no contiene %q: %q", fragment, report)
		}
	}
}
