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

func TestHTML_VeredictoYDatos(t *testing.T) {
	res := domain.NewVerificationSuccess("CAdES", "firma CAdES valida", []string{"referencias=ok"})
	res.SignerSummaries = []domain.VerificationSignerSummary{{Subject: "CN=Prueba", Issuer: "CN=AC Pruebas"}}
	out, err := HTML(Datos{NombreDocumento: "doc.csig", Contenido: []byte("abc"), Resultado: res, Fecha: time.Unix(0, 0), VersionApp: "2.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, esperado := range []string{
		"FIRMA VÁLIDA",
		"doc.csig",
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", // SHA-256 de "abc"
		"CN=Prueba", "CN=AC Pruebas", "Correcto", "2.0.1",
	} {
		if !strings.Contains(html, esperado) {
			t.Errorf("falta %q en el informe", esperado)
		}
	}
}

func TestHTML_FirmaInvalidaYEscapado(t *testing.T) {
	res := domain.NewVerificationFailure("XAdES", "digest de referencia no coincide", nil)
	res.SignerSummaries = []domain.VerificationSignerSummary{{Subject: `<script>alert(1)</script>`}}
	out, err := HTML(Datos{NombreDocumento: `<img src=x onerror=alert(1)>.xsig`, Resultado: res, Fecha: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if !strings.Contains(html, "FIRMA NO VÁLIDA") {
		t.Error("se esperaba el veredicto de firma no válida")
	}
	if strings.Contains(html, "<script>alert") || strings.Contains(html, "<img src=x") {
		t.Error("el contenido controlado por terceros debe escaparse")
	}
}

func TestHTML_IntegraPeroCertificadoNoAcreditado(t *testing.T) {
	res := domain.NewVerificationSuccess("PAdES", "firma PAdES válida", nil)
	res.Valid = false
	res.Trust = domain.VerificationAspect{Status: domain.VerificationStatusInvalid, Reason: "emisor no reconocido"}
	out, err := HTML(Datos{NombreDocumento: "f.pdf", Resultado: res, Fecha: time.Now()})
	if err != nil || !strings.Contains(string(out), "FIRMA ÍNTEGRA · VALIDEZ DEL CERTIFICADO NO ACREDITADA") {
		t.Fatalf("veredicto inesperado: %v", err)
	}
}

// Un U+202E (o cualquier carácter de formato o control) en un texto del
// documento o del certificado invertiría lo que se lee en el informe.
func TestHTML_QuitaControlYFormatoDeTodasLasCadenas(t *testing.T) {
	const rlo = "\u202e"
	res := domain.NewVerificationFailure("XAdES"+rlo, "motivo"+rlo+"fdp.exe", []string{"detalle" + rlo + "\u200b"})
	res.Warnings = []string{"aviso" + rlo + "\x07"}
	res.Errors = []string{"error" + rlo}
	res.Integrity.Reason = "integridad" + rlo
	res.Integrity.Details = []string{"det-integridad" + rlo + "\ufeff"}
	res.Certificate.Reason = "certificado" + "\u2066" + rlo
	res.Trust.Details = []string{"confianza" + rlo}
	res.SignerSummaries = []domain.VerificationSignerSummary{{Subject: "CN=Ana" + rlo + "gpj.exe", Issuer: "CN=AC" + rlo, Fingerprint: "ab" + rlo}}
	res.Evidence = []domain.VerificationEvidence{{Type: "tsa" + rlo, Summary: "sello" + rlo}}
	out, err := HTML(Datos{NombreDocumento: "doc" + rlo + ".xsig", Resultado: res, Fecha: time.Now(), VersionApp: "2.0" + rlo})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, prohibido := range []string{rlo, "\u200b", "\ufeff", "\u2066", "\x07"} {
		if strings.Contains(html, prohibido) {
			t.Fatalf("el informe conserva %U", []rune(prohibido)[0])
		}
	}
	for _, esperado := range []string{"CN=Anagpj.exe", "motivofdp.exe", "aviso", "error", "integridad", "det-integridad", "confianza", "sello", "doc.xsig"} {
		if !strings.Contains(html, esperado) {
			t.Errorf("falta %q", esperado)
		}
	}
	// No modifica el resultado del llamante.
	if !strings.Contains(res.SignerSummaries[0].Subject, rlo) {
		t.Fatal("se ha modificado el resultado original")
	}
}
