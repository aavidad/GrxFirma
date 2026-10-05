// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package informeverificacion

import (
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
)

func TestTraducirDetalleUsaElCatalogoDelIdioma(t *testing.T) {
	en := localizador.Para("en")
	casos := map[string]string{
		"formato_detectado=PAdES":                       "Detected format: PAdES",
		"firmas_pdf=1":                                  "Signatures in the PDF: 1",
		"cobertura_firma_pdf_1=documento_completo":      "Coverage of signature 1: whole document",
		"cobertura_firma_pdf_2=revision_hasta_10_de_20": "Coverage of signature 2: revision up to byte 10 of 20",
		"modo=detached":                                 "Mode: signature in a separate file",
		"clave_desconocida=valor":                       "clave_desconocida=valor",
		"subfiltro PDF detached verificado":             "PDF detached subfilter verified",
	}
	for linea, quiere := range casos {
		if got := TraducirDetalle(en, linea); got != quiere {
			t.Errorf("TraducirDetalle(%q) = %q; quiere %q", linea, got, quiere)
		}
	}
	if got := TraducirDetalle(localizador.Para("fr"), "firmas_pdf=1"); got != "Signatures dans le PDF : 1" {
		t.Errorf("francés: %q", got)
	}
}

// Las evidencias salen una sola vez, traducidas, en «Detalles técnicos».
func TestHTMLNoRepiteNiDejaClavesTecnicas(t *testing.T) {
	detalles := []string{"formato_detectado=PAdES", "firmas_pdf=1", "cobertura_firma_pdf_1=documento_completo"}
	out, err := HTML(Datos{
		NombreDocumento: "doc.pdf", Contenido: []byte("x"), Fecha: time.Unix(0, 0), Idioma: "en", Zona: time.UTC,
		Resultado: domain.VerificationResult{
			Valid: true, Format: "PAdES", Details: detalles,
			Integrity: domain.VerificationAspect{Status: domain.VerificationStatusValid, Details: detalles[1:]},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, clave := range []string{"formato_detectado", "firmas_pdf", "cobertura_firma_pdf", "documento_completo"} {
		if strings.Contains(html, clave) {
			t.Errorf("el informe en inglés conserva la clave %q", clave)
		}
	}
	if n := strings.Count(html, "Signatures in the PDF: 1"); n != 1 {
		t.Errorf("la evidencia aparece %d veces; debe aparecer una", n)
	}
}
