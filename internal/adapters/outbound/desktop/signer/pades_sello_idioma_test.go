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
		Name:   "PRUEBA SINTETICA",
		Reason: "Conformidad",
		Date:   time.Date(2026, 10, 5, 6, 25, 0, 0, time.UTC),
	}
	casos := []struct {
		opciones map[string]string
		quiere   []string
		evita    []string
	}{
		{
			opciones: map[string]string{OpcionIdiomaSello: "en", OpcionZonaSello: "Europe/Madrid"},
			quiere:   []string{"DIGITALLY SIGNED", "Date: 2026-10-05 08:25 (CEST)", "Reason: Conformidad", "Issued by AC PRUEBAS"},
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
		c.opciones = fijarEmisorSello(c.opciones, "AC PRUEBAS")
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

// /Reason, /Location y el texto alternativo (/TU) siguen el idioma del sello;
// /Location nunca lleva el emisor del certificado y la fecha lleva su zona.
func TestMetadatosFirmaEnIdiomaDelSelloSinEmisorComoLugar(t *testing.T) {
	fecha := time.Date(2026, 10, 5, 8, 58, 0, 0, time.UTC)
	casos := []struct {
		opciones      map[string]string
		motivo, texto string
		evita         []string
	}{
		{
			opciones: map[string]string{OpcionIdiomaSello: "es", OpcionZonaSello: "Europe/Madrid"},
			motivo:   "Firma electrónica avanzada",
			texto:    "Firma digital | Firmante: PRUEBA SINTETICA | Motivo: Firma electrónica avanzada | Fecha: 05/10/2026 10:58 (CEST)",
		},
		{
			opciones: map[string]string{OpcionIdiomaSello: "en", OpcionZonaSello: "Europe/Madrid"},
			motivo:   "Advanced electronic signature",
			texto:    "Digital signature | Signer: PRUEBA SINTETICA | Reason: Advanced electronic signature | Date: 2026-10-05 10:58 (CEST)",
			evita:    []string{"Firma", "Motivo", "Ubicaci"},
		},
	}
	for _, c := range casos {
		// Una web no puede imponer el emisor: el firmador lo sustituye.
		c.opciones[opcionEmisorSelloInterna] = "FALSO"
		c.opciones = fijarEmisorSello(c.opciones, "AC PRUEBAS")
		info := pdfsign.SignDataSignatureInfo{Name: "PRUEBA SINTETICA", Date: fecha}
		aplicarMetadatosFirmaPdfsign(&info, c.opciones)
		if info.Reason != c.motivo || info.Location != "" || info.Description != c.texto {
			t.Errorf("%v: Reason=%q Location=%q /TU=%q", c.opciones, info.Reason, info.Location, info.Description)
		}
		for _, e := range c.evita {
			if strings.Contains(info.Description, e) {
				t.Errorf("%v: /TU contiene %q", c.opciones, e)
			}
		}
		lineas := textosSello(lineasSelloModerno(info, true, "", estiloTextoDesdeOpciones(c.opciones)))
		if strings.Contains(lineas, "FALSO") || !strings.Contains(lineas, "AC PRUEBAS") || strings.Contains(lineas, c.motivo) {
			t.Errorf("%v: sello\n%s", c.opciones, lineas)
		}
	}
	// Con un lugar dado por la persona, /Location y /TU lo recogen.
	opciones := map[string]string{OpcionIdiomaSello: "en", OpcionZonaSello: "UTC", "signatureLocation": "Granada"}
	info := pdfsign.SignDataSignatureInfo{Name: "X", Date: fecha}
	aplicarMetadatosFirmaPdfsign(&info, opciones)
	if info.Location != "Granada" || !strings.Contains(info.Description, "Place: Granada") {
		t.Fatalf("Location=%q /TU=%q", info.Location, info.Description)
	}
}
