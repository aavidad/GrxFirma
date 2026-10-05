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

// DN tal como lo escribe crypto/x509 para un certificado FNMT de pruebas:
// apellidos y nombre van en hexadecimal porque Go no conoce esos OID.
const dnFNMT = "SERIALNUMBER=IDCES-99999999R,CN=EIDAS CERTIFICADO PRUEBAS - 99999999R,C=ES," +
	"2.5.4.4=#1311454944415320434552544946494341444f,2.5.4.42=#130750525545424153"

func informeFNMT(t *testing.T, idioma string) (string, string) {
	t.Helper()
	res := domain.NewVerificationSuccess("PAdES", "firma PAdES válida", []string{"firmante=" + dnFNMT})
	res.Certificate.Status = domain.VerificationStatusValid
	res.Certificate.Details = []string{"subject=" + dnFNMT}
	res.Trust.Status = domain.VerificationStatusValid
	res.Trust.Details = []string{"signer[0].chain_length=3"}
	res.Evidence = []domain.VerificationEvidence{
		{Type: "certificate.subject", Summary: dnFNMT},
		{Type: "certificate.issuer", Summary: "CN=AC FNMT Usuarios,OU=Ceres,O=FNMT-RCM,C=ES"},
		{Type: "revocation", Summary: "cert[0] EIDAS CERTIFICADO PRUEBAS - 99999999R: bueno vía ocsp"},
	}
	res.SignerSummaries = []domain.VerificationSignerSummary{{
		Subject: dnFNMT, Issuer: "CN=AC FNMT Usuarios,OU=Ceres,O=FNMT-RCM,C=ES",
		SigningTime: "2026-10-05T11:00:44Z", SigningTimeSource: domain.SigningTimeSourceSignedAttribute,
	}}
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("sin base de zonas horarias")
	}
	out, err := HTML(Datos{NombreDocumento: "firmado.pdf", Resultado: res, Fecha: time.Now(), Idioma: idioma, Zona: madrid})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	inicio := strings.Index(html, "<h2>")
	fin := strings.LastIndex(html, "<h2>")
	return html[inicio:fin], html[fin:]
}

func TestHTML_FirmanteLegibleYFechaDeFirma(t *testing.T) {
	normal, tecnico := informeFNMT(t, "es")
	for _, esperado := range []string{
		"<th>EIDAS CERTIFICADO PRUEBAS - 99999999R</th>",
		"NIF / identificador: IDCES-99999999R",
		"Emisor: AC FNMT Usuarios (FNMT-RCM)",
		"Fecha de la firma: 05/10/2026 13:00:44 (CEST), hora del equipo de quien firmó",
		"Certificados en la cadena del firmante 1: 3",
		"Revocación",
	} {
		if !strings.Contains(normal, esperado) {
			t.Errorf("falta %q en la parte legible del informe:\n%s", esperado, normal)
		}
	}
	for _, prohibido := range []string{"SERIALNUMBER=", "2.5.4.4=", "#1311", "chain_length", "subject=", "certificate.subject", "certificate.issuer"} {
		if strings.Contains(normal, prohibido) {
			t.Errorf("la parte legible no debe mostrar %q", prohibido)
		}
	}
	if !strings.Contains(tecnico, dnFNMT) || !strings.Contains(tecnico, "Emisor (DN): CN=AC FNMT Usuarios") {
		t.Errorf("el DN completo debe seguir en los detalles técnicos:\n%s", tecnico)
	}
	if strings.Count(tecnico, dnFNMT) != 1 {
		t.Errorf("el DN del titular no debe repetirse en los detalles técnicos:\n%s", tecnico)
	}
}

func TestHTML_FechaSegunSelloDeTiempoEnIngles(t *testing.T) {
	res := domain.NewVerificationSuccess("CAdES", "firma CAdES válida", nil)
	res.SignerSummaries = []domain.VerificationSignerSummary{{
		Subject: "CN=Ana", Issuer: "CN=AC", SigningTime: "2026-10-05T11:00:44Z",
		SigningTimeSource: domain.SigningTimeSourceTimestamp,
	}}
	out, err := HTML(Datos{Resultado: res, Fecha: time.Now(), Idioma: "en", Zona: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Signing time: ") || !strings.Contains(string(out), "according to the timestamp") {
		t.Errorf("falta la fecha según el sello:\n%s", out)
	}
}

func TestHTML_SinFechaFiableNoSeMuestra(t *testing.T) {
	res := domain.NewVerificationSuccess("CAdES", "firma CAdES válida", nil)
	res.SignerSummaries = []domain.VerificationSignerSummary{{Subject: "CN=Ana", SigningTime: "no-es-fecha", SigningTimeSource: "timestamp"}}
	out, err := HTML(Datos{Resultado: res, Fecha: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "Fecha de la firma") {
		t.Error("una fecha que no se puede leer no debe aparecer")
	}
}

func TestLeerNombreDistinguido(t *testing.T) {
	n := leerNombreDistinguido(dnFNMT)
	if n.comun != "EIDAS CERTIFICADO PRUEBAS - 99999999R" || n.identificador != "IDCES-99999999R" ||
		n.apellidos != "EIDAS CERTIFICADO" || n.nombre != "PRUEBAS" {
		t.Fatalf("%+v", n)
	}
	sinCN := leerNombreDistinguido(`2.5.4.42=#1303414e41,SN=P\c3\a9rez\, L\c3\b3pez`)
	if got := sinCN.legible("x"); got != "ANA Pérez, López" {
		t.Fatalf("legible = %q", got)
	}
	if got := leerNombreDistinguido(`SN=P\c3\a9rez\, L\c3\b3pez`).apellidos; got != "Pérez, López" {
		t.Fatalf("apellidos = %q", got)
	}
}
