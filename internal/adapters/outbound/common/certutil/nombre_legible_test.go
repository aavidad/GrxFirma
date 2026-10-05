// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certutil_test

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"grxfirma/internal/adapters/outbound/common/certutil"
)

// Un titular como los de la FNMT: Go escribe nombre, apellidos,
// organizationIdentifier y title como OID con el DER en hexadecimal.
func TestNombreLegible_CertificadoFNMT(t *testing.T) {
	nombre := pkix.Name{
		Country:      []string{"ES"},
		Organization: []string{"DIPUTACION DE GRANADA"},
		SerialNumber: "IDCES-12345678Z",
		CommonName:   "PEREZ LOPEZ ANA - 12345678Z",
		ExtraNames: []pkix.AttributeTypeAndValue{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "ANA"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 4}, Value: "PEREZ LOPEZ"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 97}, Value: "VATES-P1800000J"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 12}, Value: "Técnica, de sistemas"},
		},
	}
	bruto := nombre.String()
	if bruto == certutil.NombreLegible(nombre) {
		t.Fatalf("se esperaba que Go dejase OID en hexadecimal: %s", bruto)
	}
	// Mismo orden que pkix.Name.String(); solo cambian etiqueta y valor.
	quiere := "title=Técnica\\, de sistemas,organizationIdentifier=VATES-P1800000J,SN=PEREZ LOPEZ,GN=ANA," +
		"SERIALNUMBER=IDCES-12345678Z,CN=PEREZ LOPEZ ANA - 12345678Z,O=DIPUTACION DE GRANADA,C=ES"
	if got := certutil.NombreLegible(nombre); got != quiere {
		t.Fatalf("NombreLegible =\n%s\nquiere\n%s\n(bruto %s)", got, quiere, bruto)
	}
}

func TestDNLegible(t *testing.T) {
	casos := []struct{ entrada, quiere string }{
		{"CN=Prueba,O=Pruebas,C=ES", "CN=Prueba,O=Pruebas,C=ES"},
		// PrintableString «GARCIA» y UTF8String «José».
		{"2.5.4.4=#1306474152434941,2.5.4.42=#0c054a6f73c3a9,C=ES", "SN=GARCIA,GN=José,C=ES"},
		// Atributo multivalor y valor que hay que escapar.
		{"CN=A+2.5.4.4=#13032c2042", "CN=A+SN=\\, B"},
		// Un OID desconocido o un valor que no es texto se dejan igual.
		{"1.2.3.4=#130141,2.5.4.42=#020101", "1.2.3.4=A,GN=#020101"},
		{"2.5.4.42=#zz", "GN=#zz"},
		{"sin igual", "sin igual"},
		{"", ""},
	}
	for _, caso := range casos {
		if got := certutil.DNLegible(caso.entrada); got != caso.quiere {
			t.Errorf("DNLegible(%q) = %q, quiere %q", caso.entrada, got, caso.quiere)
		}
	}
}

// Con un certificado leído (pkix rellena Names, no ExtraNames) el resultado
// conserva el orden de Go y no deja hexadecimal en los atributos de texto.
func TestNombreLegible_CertificadoLeido(t *testing.T) {
	cert := certConCampos(t, pkix.Name{
		Country:    []string{"ES"},
		CommonName: "PEREZ LOPEZ ANA - 12345678Z",
		ExtraNames: []pkix.AttributeTypeAndValue{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "ANA"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 4}, Value: "PEREZ LOPEZ"},
		},
	}, nil, false)
	got := certutil.NombreLegible(cert.Subject)
	quiere := "CN=PEREZ LOPEZ ANA - 12345678Z,C=ES,SN=PEREZ LOPEZ,GN=ANA"
	if got != quiere {
		t.Fatalf("NombreLegible = %q, quiere %q (bruto %q)", got, quiere, cert.Subject.String())
	}
}
