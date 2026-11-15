// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package informeverificacion

import (
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// Lote 14: el informe imprimible de una firma dada por válida porque el
// usuario lo aceptó en su equipo nunca dice «válida» a secas: el veredicto
// lleva advertencias y el aviso se lee en el idioma del informe.
func TestHTML_ConfianzaForzadaSiempreConAviso(t *testing.T) {
	res := domain.NewVerificationSuccess("CAdES", "firma CAdES valida", nil)
	res.Trust = domain.VerificationAspect{Status: domain.VerificationStatusValid, Reason: domain.ConfianzaCADesconocida.ClaveAplicada()}
	res = res.MarcarConfianzaForzada(domain.ObstaculoConfianza{Tipo: domain.ConfianzaCADesconocida, Huella: strings.Repeat("a", 64)})
	res = res.MarcarConfianzaForzada(domain.ObstaculoConfianza{Tipo: domain.ConfianzaAlgoritmoSHA1})
	for idioma, esperados := range map[string][]string{
		"es": {"FIRMA VÁLIDA CON ADVERTENCIAS", "Se da por válida porque lo aceptó en la configuración: autoridad de certificación que este equipo no conoce.", "Se da por válida porque lo aceptó en la configuración: firma con SHA-1."},
		"en": {"Treated as valid because you accepted it in the settings: signature using SHA-1."},
	} {
		out, err := HTML(Datos{NombreDocumento: "f.csig", Contenido: []byte("x"), Resultado: res, Fecha: time.Now(), Idioma: idioma})
		if err != nil {
			t.Fatal(err)
		}
		html := string(out)
		for _, esperado := range esperados {
			if !strings.Contains(html, esperado) {
				t.Errorf("%s: falta %q en el informe", idioma, esperado)
			}
		}
		if strings.Contains(html, "verify.forced_trust.") {
			t.Errorf("%s: el informe muestra una clave sin traducir", idioma)
		}
	}
}
