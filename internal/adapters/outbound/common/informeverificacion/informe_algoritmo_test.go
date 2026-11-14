// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package informeverificacion

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
)

// Lote 17a: resultado como el de una firma SHA-1 íntegra (ver
// signer/algoritmo_antiguo.go).
func resultadoSHA1(certificadoValido bool) domain.VerificationResult {
	const motivo = "El documento no ha cambiado desde que se firmó, pero la firma usa SHA-1, un algoritmo antiguo que hoy no se considera seguro. Por eso no se da por válida: sirve como prueba de una firma hecha cuando SHA-1 se aceptaba."
	res := domain.NewVerificationSuccess("XAdES", "firma XAdES valida", []string{"algoritmo_antiguo=sha1"})
	res.Integrity.Details = []string{"algoritmo_antiguo=sha1"}
	res.Valid = false
	res.Reason = motivo
	res.Warnings = []string{motivo}
	res.Evidence = []domain.VerificationEvidence{{Type: domain.EvidenciaAlgoritmoAntiguo, Summary: "algoritmo_antiguo=sha1"}}
	res.Certificate = domain.VerificationAspect{Status: domain.VerificationStatusValid}
	res.Trust = domain.VerificationAspect{Status: domain.VerificationStatusValid}
	if !certificadoValido {
		res.Certificate = domain.VerificationAspect{Status: domain.VerificationStatusInvalid, Reason: "certificado caducado"}
	}
	return res
}

func informe(t *testing.T, res domain.VerificationResult, idioma string) string {
	t.Helper()
	out, err := HTML(Datos{NombreDocumento: "f.xsig", Resultado: res, Fecha: time.Unix(0, 0), Idioma: idioma, Zona: time.UTC, VersionApp: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Con SHA-1 y el certificado acreditado, el veredicto dice que el problema
// es el algoritmo, no el certificado.
func TestHTML_SHA1DiceQueEsElAlgoritmo(t *testing.T) {
	html := informe(t, resultadoSHA1(true), "")
	if strings.Contains(html, "VALIDEZ DEL CERTIFICADO NO ACREDITADA") {
		t.Fatal("con el certificado acreditado no se puede decir que su validez no está acreditada")
	}
	for _, esperado := range []string{
		"FIRMA ÍNTEGRA · ALGORITMO ANTIGUO (SHA-1), NO SE DA POR VÁLIDA",
		"<strong>Algoritmo antiguo:</strong> SHA-1",
		"<li>Algoritmo antiguo: SHA-1</li>",
	} {
		if !strings.Contains(html, esperado) {
			t.Errorf("falta %q", esperado)
		}
	}
	if strings.Contains(html, "signature.algorithm.legacy") || strings.Contains(html, "algoritmo_antiguo=sha1") {
		t.Fatal("el informe enseña claves técnicas sin traducir")
	}
}

// Si además el certificado no está acreditado, se dicen las dos cosas.
func TestHTML_SHA1YCertificadoNoAcreditado(t *testing.T) {
	html := informe(t, resultadoSHA1(false), "")
	if !strings.Contains(html, "FIRMA ÍNTEGRA · ALGORITMO ANTIGUO (SHA-1) Y VALIDEZ DEL CERTIFICADO NO ACREDITADA") {
		t.Fatal("falta el veredicto con algoritmo y certificado")
	}
	// Sin SHA-1, el veredicto de siempre.
	res := resultadoSHA1(false)
	res.Evidence = nil
	if html := informe(t, res, ""); !strings.Contains(html, "FIRMA ÍNTEGRA · VALIDEZ DEL CERTIFICADO NO ACREDITADA") {
		t.Fatal("sin SHA-1 el veredicto no cambia")
	}
}

// Si el usuario aceptó SHA-1 en su equipo, la firma es válida y manda el
// veredicto de válida.
func TestHTML_SHA1AceptadoSigueSiendoValida(t *testing.T) {
	res := resultadoSHA1(true)
	res.Valid = true
	if html := informe(t, res, ""); !strings.Contains(html, "FIRMA VÁLIDA CON ADVERTENCIAS") {
		t.Fatal("una firma válida no cambia de veredicto")
	}
}

// Los rótulos, la puntuación y el pie salen del catálogo de cada idioma: los
// 11 idiomas tienen las claves y ninguno deja una sin traducir.
func TestHTML_TextosDelCatalogoEnTodosLosIdiomas(t *testing.T) {
	res := resultadoSHA1(true)
	res.SignerSummaries = []domain.VerificationSignerSummary{{Subject: "CN=Ana", Issuer: "CN=AC Uno,O=Entidad", Fingerprint: "ab"}}
	for _, idioma := range []string{"es", "en", "ca", "va", "gl", "eu", "fr", "de", "it", "pt", "zh"} {
		html := informe(t, res, idioma)
		loc := localizador.Para(idioma)
		for _, clave := range []string{"report.verdict.intact_legacy_algorithm", "report.label", "report.signer.issuer_with_organization", "report.app_version", "report.evidence.signature.algorithm.legacy", "verificacion.detalle.algoritmo_antiguo"} {
			if loc.T(clave) == clave {
				t.Fatalf("%s: falta %s", idioma, clave)
			}
		}
		for _, esperado := range []string{
			loc.T("report.verdict.intact_legacy_algorithm"),
			loc.T("report.label", loc.T("report.signer.issuer")) + " " + loc.T("report.signer.issuer_with_organization", "AC Uno", "Entidad"),
			loc.T("report.footer", loc.T("report.app_version", "GrxFirma", "1.0")),
		} {
			if !strings.Contains(html, template.HTMLEscapeString(esperado)) {
				t.Errorf("%s: falta %q", idioma, esperado)
			}
		}
	}
	// La puntuación es la del idioma.
	if html := informe(t, res, "zh"); !strings.Contains(html, "颁发者：") || strings.Contains(html, "颁发者:") {
		t.Fatal("en chino los dos puntos son de anchura completa")
	}
	if html := informe(t, res, "fr"); !strings.Contains(html, "Émetteur :") {
		t.Fatal("en francés va un espacio antes de los dos puntos")
	}
}
