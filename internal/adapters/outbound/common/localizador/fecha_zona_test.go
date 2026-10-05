// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localizador

import (
	"testing"
	"time"
)

func TestFechaHoraMuestraHoraLocalConZona(t *testing.T) {
	instante := time.Date(2026, 10, 5, 6, 25, 10, 0, time.UTC)
	madrid := Zona("Europe/Madrid")
	if madrid == nil {
		t.Fatal("Europe/Madrid debe cargarse")
	}
	casos := []struct {
		idioma   string
		zona     *time.Location
		segundos bool
		quiere   string
	}{
		{"es", madrid, false, "05/10/2026 08:25 (CEST)"},
		{"es", madrid, true, "05/10/2026 08:25:10 (CEST)"},
		{"de", madrid, false, "05.10.2026 08:25 (CEST)"},
		{"zh", Zona("Asia/Shanghai"), false, "2026-10-05 14:25 (CST)"},
		// Zona sin abreviatura propia en tzdata: se escribe el desfase.
		{"es", Zona("America/Sao_Paulo"), false, "05/10/2026 03:25 (UTC-03:00)"},
		// En inglés «05/10/2026» se leería como 10 de mayo: se usa ISO 8601.
		{"en", time.UTC, false, "2026-10-05 06:25 (UTC)"},
		{"en", madrid, true, "2026-10-05 08:25:10 (CEST)"},
		{"es", time.FixedZone("", 2*3600), false, "05/10/2026 08:25 (UTC+02:00)"},
	}
	for _, c := range casos {
		if got := Para(c.idioma).FechaHora(instante, c.zona, c.segundos); got != c.quiere {
			t.Errorf("%s: FechaHora = %q; quiere %q", c.idioma, got, c.quiere)
		}
	}
}

func TestZonaRechazaNombresNoIANA(t *testing.T) {
	for _, nombre := range []string{"", "../../etc/passwd", "/etc/localtime", "Europe/../../x", `C:\zona`, "Nada/Inexistente"} {
		if Zona(nombre) != nil {
			t.Errorf("Zona(%q) debe devolver nil", nombre)
		}
	}
	if Zona("UTC") == nil || Zona("America/Argentina/Buenos_Aires") == nil {
		t.Error("las zonas IANA válidas deben cargarse")
	}
}

func TestIdiomaYEtiquetaHTML(t *testing.T) {
	if Idioma("") != "" || Idioma("en-GB") != "en" || Idioma("ca-ES-valencia") != "va" {
		t.Error("Idioma no normaliza como se espera")
	}
	if Para("va").EtiquetaHTML() != "ca-ES-valencia" || Para("fr").EtiquetaHTML() != "fr" {
		t.Error("EtiquetaHTML no es BCP 47")
	}
}
