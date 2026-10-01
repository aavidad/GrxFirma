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
