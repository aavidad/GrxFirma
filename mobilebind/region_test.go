// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSetRegionValidaYCompletaLasOpcionesDelSello(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	if err := facade.SetRegion("en", "../../etc/passwd"); err == nil {
		t.Fatal("una zona con forma de ruta debe rechazarse")
	}
	if err := facade.SetRegion("en", "Nada/Inexistente"); err == nil {
		t.Fatal("una zona desconocida debe rechazarse")
	}
	if err := facade.SetRegion("ca-ES-valencia", "Europe/Madrid"); err != nil {
		t.Fatal(err)
	}
	got := facade.withRegionOptions(map[string]string{"visibleSeal": "true"})
	if got["sealLanguage"] != "va" || got["sealTimeZone"] != "Europe/Madrid" {
		t.Fatalf("opciones = %#v", got)
	}
	// Lo que envía la petición prevalece.
	got = facade.withRegionOptions(map[string]string{"sealLanguage": "es"})
	if got["sealLanguage"] != "es" {
		t.Fatalf("la opción explícita debe prevalecer: %#v", got)
	}
	if err := facade.SetRegion("", ""); err != nil {
		t.Fatal(err)
	}
	if got := facade.withRegionOptions(nil); len(got) != 0 {
		t.Fatalf("sin región no se añaden opciones: %#v", got)
	}
}

// En Android Go usa UTC como hora local: el informe debe salir con la hora
// y el idioma del móvil que le pasa la app.
func TestInformeMovilUsaIdiomaYZonaDeLaApp(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	facade.clock = func() time.Time { return time.Date(2026, 10, 5, 6, 25, 10, 0, time.UTC) }
	if err := facade.SetRegion("en", "Europe/Madrid"); err != nil {
		t.Fatal(err)
	}
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "clave"), "clave")
	original := []byte("contenido del informe")
	signed := signWithFacade(t, facade, id, "nota.txt", "text/plain", "cades", original)
	out, err := facade.VerifyJSON(mustJSON(t, verifyRequest{
		Name: "nota.txt.p7s", ContentBase64: base64.StdEncoding.EncodeToString(signed),
		MIMEType: "application/pkcs7-signature", OriginalBase64: base64.StdEncoding.EncodeToString(original),
		IncludeHTMLReport: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var response verifyResponse
	decodeResponse(t, out, &response)
	html, err := base64.StdEncoding.DecodeString(response.ReportHTMLBase64)
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if strings.Contains(text, "no se evalúa en el móvil") {
		t.Error("el motivo de confianza debe ir en el idioma de la app")
	}
	for _, esperado := range []string{`lang="en"`, "2026-10-05 08:25:10 (CEST)", "SIGNATURE INTACT", "Electronic signature validation report"} {
		if !strings.Contains(text, esperado) {
			t.Errorf("falta %q en el informe", esperado)
		}
	} // Las evidencias técnicas llegan también redactadas en el idioma de la app.
	if len(response.DetailsText) != len(response.Details) || len(response.Details) == 0 {
		t.Fatalf("details_text debe acompañar a cada evidencia: %v / %v", response.DetailsText, response.Details)
	}
	joined := strings.Join(response.DetailsText, "\n")
	if !strings.Contains(joined, "Detected format: CAdES") || strings.Contains(joined, "formato_detectado") {
		t.Errorf("evidencias sin traducir: %q", joined)
	}
}
