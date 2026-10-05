// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestVerifyIncludesDesktopHTMLReportOnRequest(t *testing.T) {
	facade := newAndroidFacadeForTest(t)
	id := importPKCS12(t, facade, ephemeralRSAPKCS12(t, "clave"), "clave")
	original := []byte("contenido del informe")
	signed := signWithFacade(t, facade, id, "nota<b>.txt", "text/plain", "cades", original)

	plain := verifyWithFacade(t, facade, "nota<b>.txt.p7s", "application/pkcs7-signature", signed, original)
	if plain.ReportHTMLBase64 != "" {
		t.Fatal("sin pedirlo no se genera el informe")
	}

	out, err := facade.VerifyJSON(mustJSON(t, verifyRequest{
		Name: "nota<b>.txt.p7s", ContentBase64: base64.StdEncoding.EncodeToString(signed),
		MIMEType: "application/pkcs7-signature", OriginalBase64: base64.StdEncoding.EncodeToString(original),
		IncludeHTMLReport: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var response verifyResponse
	decodeResponse(t, out, &response)
	html, err := base64.StdEncoding.DecodeString(response.ReportHTMLBase64)
	if err != nil || len(html) == 0 {
		t.Fatalf("informe HTML: %v", err)
	}
	text := string(html)
	if !strings.Contains(text, "<!DOCTYPE html>") || !strings.Contains(text, "default-src 'none'") {
		t.Fatal("el informe debe ser el de escritorio, con CSP sin scripts")
	}
	if strings.Contains(text, "nota<b>") || !strings.Contains(text, "nota&lt;b&gt;") {
		t.Fatal("el nombre del documento debe ir escapado")
	}
	if strings.Contains(text, "FIRMA VÁLIDA") {
		t.Fatal("sin anclas del sistema el informe no puede declarar la firma válida")
	}
	if !strings.Contains(text, "FIRMA ÍNTEGRA") {
		t.Fatalf("veredicto inesperado: %s", text[:200])
	}
}
