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

// Los errores de cadena X.509 llegan como código y se leen traducidos en
// los once idiomas; el texto en inglés de crypto/x509 no aparece.
func TestTraducirDetalle_ErrorDeCadenaEnTodosLosIdiomas(t *testing.T) {
	codigos := []string{"x509_autoridad_desconocida", "x509_caducado", "x509_uso_no_permitido",
		"x509_emisor_no_autorizado", "x509_nombre_no_coincide", "x509_algoritmo_inseguro",
		"x509_cadena_demasiado_larga", "x509_otro"}
	for _, idioma := range []string{"es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh"} {
		loc := localizador.Para(idioma)
		for _, codigo := range codigos {
			linea := "error_cadena=" + codigo
			got := TraducirDetalle(loc, linea)
			if got == linea || strings.Contains(got, "x509") || strings.Contains(got, "error_cadena") {
				t.Errorf("%s: %q sin traducir: %q", idioma, linea, got)
			}
		}
	}
	if got := TraducirDetalle(localizador.Para("es"), "error_cadena=x509_autoridad_desconocida"); got !=
		"Cadena del certificado: emitido por una entidad que no está entre las de confianza" {
		t.Errorf("castellano = %q", got)
	}
}

// Las evidencias que no son el DN del certificado salen con su nombre
// traducido, no con el identificador interno.
func TestHTML_EvidenciasConTipoTraducido(t *testing.T) {
	res := domain.NewVerificationFailure("XAdES", "firma no válida", nil)
	res.Evidence = []domain.VerificationEvidence{
		{Type: "xml.canonicalization.compatibility", Summary: "c14n"},
		{Type: "certificate.subject", Summary: "CN=Ana,2.5.4.42=#1303414e41"},
	}
	out, err := HTML(Datos{NombreDocumento: "f.xml", Resultado: res, Fecha: time.Now(), Idioma: "es"})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, prohibido := range []string{"xml.canonicalization.compatibility", "certificate.subject", "2.5.4.42="} {
		if strings.Contains(html, prohibido) {
			t.Errorf("el informe no debe mostrar %q", prohibido)
		}
	}
	for _, esperado := range []string{"Compatibilidad de la canonicalización XML", "CN=Ana,GN=ANA"} {
		if !strings.Contains(html, esperado) {
			t.Errorf("falta %q en el informe", esperado)
		}
	}
}
