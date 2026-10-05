// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"strings"
	"testing"
	"time"

	pdfsign "github.com/digitorus/pdfsign/sign"
)

func textosSello(lineas []lineaSello) string {
	var partes []string
	for _, l := range lineas {
		partes = append(partes, l.texto)
	}
	return strings.Join(partes, "\n")
}

func TestSelloSigueElIdiomaYLaZonaPedidos(t *testing.T) {
	info := pdfsign.SignDataSignatureInfo{
		Name:     "PRUEBA SINTETICA",
		Reason:   "Conformidad",
		Location: "Certificado: AC PRUEBAS",
		Date:     time.Date(2026, 10, 5, 6, 25, 0, 0, time.UTC),
	}
	casos := []struct {
		opciones map[string]string
		quiere   []string
		evita    []string
	}{
		{
			opciones: map[string]string{OpcionIdiomaSello: "en", OpcionZonaSello: "Europe/Madrid"},
			quiere:   []string{"DIGITALLY SIGNED", "Date: 05/10/2026 08:25 (CEST)", "Reason: Conformidad", "Issued by AC PRUEBAS"},
			evita:    []string{"FIRMADO", "Fecha", "Emitido"},
		},
		{
			// Idioma fijo de la configuración (castellano para la Administración).
			opciones: map[string]string{OpcionIdiomaSello: "es", OpcionZonaSello: "Atlantic/Canary"},
			quiere:   []string{"FIRMADO DIGITALMENTE", "Fecha: 05/10/2026 07:25 (WEST)", "Motivo: Conformidad", "Emitido por AC PRUEBAS"},
		},
		{
			// Sin opciones: castellano (compatibilidad) y la hora no se queda sin zona.
			opciones: map[string]string{OpcionZonaSello: "UTC"},
			quiere:   []string{"FIRMADO DIGITALMENTE", "Fecha: 05/10/2026 06:25 (UTC)"},
		},
		{
			// Una zona con forma de ruta se ignora: se usa la del sistema.
			opciones: map[string]string{OpcionIdiomaSello: "de", OpcionZonaSello: "../../etc/passwd"},
			quiere:   []string{"DIGITAL SIGNIERT", "Datum: 05.10.2026"},
		},
	}
	for _, c := range casos {
		texto := textosSello(lineasSelloModerno(info, true, "", estiloTextoDesdeOpciones(c.opciones)))
		for _, q := range c.quiere {
			if !strings.Contains(texto, q) {
				t.Errorf("%v: falta %q en\n%s", c.opciones, q, texto)
			}
		}
		for _, e := range c.evita {
			if strings.Contains(texto, e) {
				t.Errorf("%v: no debe aparecer %q en\n%s", c.opciones, e, texto)
			}
		}
	}
}

func TestPatronSIGNDATEUsaLaZonaDelSello(t *testing.T) {
	ahora := time.Date(2026, 10, 5, 6, 25, 0, 0, time.UTC)
	got := expandirLayer2Text("$$SIGNDATE=dd/MM/yyyy HH:mm$$", nil, map[string]string{OpcionZonaSello: "Europe/Madrid"}, ahora)
	if got != "05/10/2026 08:25" {
		t.Fatalf("SIGNDATE = %q", got)
	}
}
