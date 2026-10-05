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

// El informe sale entero en el idioma de la interfaz que lo pide, incluidos
// los motivos del motor, y la hora lleva su zona.
func TestHTML_IdiomaDeLaInterfazYHoraConZona(t *testing.T) {
	res := domain.NewVerificationSuccess("PAdES", "firma PAdES válida", []string{"subfiltro PDF detached verificado"})
	res.Trust = domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: "sin anclas de confianza disponibles"}
	res.Valid = false
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin tzdata en el sistema")
	}
	fecha := time.Date(2026, 10, 5, 6, 25, 10, 0, time.UTC)
	out, err := HTML(Datos{NombreDocumento: "f.pdf", Contenido: []byte("x"), Resultado: res, Fecha: fecha, VersionApp: "0.0.106", Idioma: "en", Zona: madrid})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, esperado := range []string{
		`<html lang="en">`, "Electronic signature validation report",
		"SIGNATURE INTACT", "Valid PAdES signature", "PDF detached subfilter verified",
		"No trusted root certificates are available", "Verified document", "1 byte<",
		"05/10/2026 08:25:10 (CEST)", "Generated by GrxFirma 0.0.106",
	} {
		if !strings.Contains(html, esperado) {
			t.Errorf("falta %q en el informe en inglés", esperado)
		}
	}
	for _, castellano := range []string{"Informe de validación", "Documento verificado", "firma PAdES válida", "Generado por", "FIRMA"} {
		if strings.Contains(html, castellano) {
			t.Errorf("el informe en inglés conserva %q", castellano)
		}
	}

	va, err := HTML(Datos{NombreDocumento: "f.pdf", Resultado: res, Fecha: fecha, Idioma: "ca-ES-valencia", Zona: time.UTC})
	if err != nil || !strings.Contains(string(va), `lang="ca-ES-valencia"`) || !strings.Contains(string(va), "(UTC)") {
		t.Fatalf("valenciano: %v", err)
	}
}

// Sin idioma se mantiene el castellano, con el motivo del motor ya redactado.
func TestHTML_CastellanoPorDefecto(t *testing.T) {
	res := domain.NewVerificationSuccess("PAdES", "firma PAdES válida", nil)
	out, err := HTML(Datos{NombreDocumento: "f.pdf", Resultado: res, Fecha: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if !strings.Contains(html, `<html lang="es">`) || !strings.Contains(html, "Firma PAdES válida") || !strings.Contains(html, "Informe de validación de firma electrónica") {
		t.Fatal("el informe por defecto debe ir en castellano")
	}
}

// En el móvil las huellas y evidencias largas no deben obligar a desplazar
// la página en horizontal; al imprimir se conserva la tabla.
func TestHTML_LegibleEnPantallasEstrechas(t *testing.T) {
	out, err := HTML(Datos{NombreDocumento: "f.pdf", Resultado: domain.NewVerificationSuccess("CAdES", "firma CAdES válida", nil), Fecha: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, regla := range []string{"overflow-wrap:anywhere", "table-layout:fixed", "@media (max-width:30rem)", "th,td{display:block;width:auto}", "@media print", "display:table-cell"} {
		if !strings.Contains(html, regla) {
			t.Errorf("falta la regla CSS %q", regla)
		}
	}
}

// El tamaño concuerda en número: «1 byte» y «2 bytes», según el catálogo.
func TestHTML_TamanoSingularYPlural(t *testing.T) {
	res := domain.NewVerificationSuccess("PAdES", "firma PAdES válida", nil)
	for _, caso := range []struct {
		idioma, contenido, esperado string
	}{
		{"es", "x", "1 byte<"}, {"es", "xy", "2 bytes<"},
		{"en", "x", "1 byte<"}, {"fr", "x", "1 octet<"}, {"fr", "xy", "2 octets<"},
	} {
		out, err := HTML(Datos{NombreDocumento: "f.pdf", Contenido: []byte(caso.contenido), Resultado: res, Idioma: caso.idioma, Zona: time.UTC})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), caso.esperado) {
			t.Errorf("%s/%d: falta %q", caso.idioma, len(caso.contenido), caso.esperado)
		}
	}
}
