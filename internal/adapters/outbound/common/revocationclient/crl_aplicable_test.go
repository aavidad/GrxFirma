// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"strings"
	"testing"
	"time"
)

// campoIDP describe un campo opcional de IssuingDistributionPoint.
type campoIDP struct {
	tag   int
	bytes []byte
}

// extensionIDPPrueba construye un IssuingDistributionPoint (RFC 5280, 5.2.5)
// con los URI indicados como fullName y los campos adicionales pedidos.
func extensionIDPPrueba(t *testing.T, uris []string, campos ...campoIDP) pkix.Extension {
	t.Helper()
	var contenido []byte
	if len(uris) > 0 {
		var generales []byte
		for _, uri := range uris {
			general, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 6, Bytes: []byte(uri)})
			if err != nil {
				t.Fatal(err)
			}
			generales = append(generales, general...)
		}
		fullName, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: generales})
		if err != nil {
			t.Fatal(err)
		}
		dpn, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: fullName})
		if err != nil {
			t.Fatal(err)
		}
		contenido = append(contenido, dpn...)
	}
	for _, campo := range campos {
		der, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: campo.tag, Bytes: campo.bytes})
		if err != nil {
			t.Fatal(err)
		}
		contenido = append(contenido, der...)
	}
	valor, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: contenido})
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Critical: true, Value: valor}
}

func extensionDelta(t *testing.T) pkix.Extension {
	t.Helper()
	valor, err := asn1.Marshal(big.NewInt(6))
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 27}, Critical: true, Value: valor}
}

func crlConExtensiones(t *testing.T, f validationFixture, revocado *big.Int, extra ...pkix.Extension) []byte {
	t.Helper()
	plantilla := &x509.RevocationList{
		Number:          big.NewInt(9),
		ThisUpdate:      f.now.Add(-time.Minute),
		NextUpdate:      f.now.Add(time.Hour),
		ExtraExtensions: extra,
	}
	if revocado != nil {
		plantilla.RevokedCertificateEntries = []x509.RevocationListEntry{{
			SerialNumber:   revocado,
			RevocationTime: f.now.Add(-2 * time.Minute),
		}}
	}
	der, err := x509.CreateRevocationList(rand.Reader, plantilla, f.ca, f.caKey)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// hojaConCRLDP emite una hoja de la CA del fixture con el CRLDP indicado.
func hojaConCRLDP(t *testing.T, f validationFixture, serie int64, esCA bool, crldp ...string) *x509.Certificate {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(serie),
		Subject:               pkix.Name{CommonName: "Titular particionado"},
		NotBefore:             f.now.Add(-time.Hour),
		NotAfter:              f.now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		AuthorityKeyId:        append([]byte(nil), f.ca.SubjectKeyId...),
		CRLDistributionPoints: crldp,
	}
	if esCA {
		plantilla.IsCA = true
		plantilla.BasicConstraintsValid = true
		plantilla.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, f.ca, &clave.PublicKey, f.caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

var verdadero = []byte{0xff}

// TestValidateCRLResponse_RechazaParticionAjena reproduce E1-04: una CRL
// auténtica y vigente de otra partición del mismo emisor no puede acreditar
// que el certificado no está revocado.
func TestValidateCRLResponse_RechazaParticionAjena(t *testing.T) {
	f := newValidationFixture(t)
	hoja := hojaConCRLDP(t, f, 301, false, "http://x/mia.crl")
	crl := crlConExtensiones(t, f, nil, extensionIDPPrueba(t, []string{"http://x/otra.crl"}))
	resultado, err := ValidateCRLResponse(crl, hoja, f.ca, f.now)
	if err == nil {
		t.Fatalf("una CRL de otra partición se aceptó con estado %d", resultado.Status)
	}
	if resultado.Status == CertificateStatusGood {
		t.Fatal("una CRL de otra partición no puede dar «no revocado»")
	}
}

func TestValidateCRLResponse_RechazaDelta(t *testing.T) {
	f := newValidationFixture(t)
	crl := crlConExtensiones(t, f, nil, extensionDelta(t))
	if _, err := ValidateCRLResponse(crl, f.leaf, f.ca, f.now); err == nil ||
		!strings.Contains(err.Error(), "crl_delta_no_admitida") {
		t.Fatalf("error=%v, se esperaba el rechazo de la CRL delta", err)
	}
}

func TestValidateCRLResponse_AplicabilidadIDP(t *testing.T) {
	f := newValidationFixture(t)
	hoja := hojaConCRLDP(t, f, 302, false, "http://x/mia.crl")
	intermedia := hojaConCRLDP(t, f, 303, true, "http://x/arl.crl")
	sinCRLDP := hojaConCRLDP(t, f, 304, false)

	casos := []struct {
		nombre  string
		cert    *x509.Certificate
		crl     []byte
		motivo  string
		aceptar bool
	}{
		{nombre: "partición propia", cert: hoja, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, []string{"ldap://x/mia", "http://x/mia.crl"})), aceptar: true},
		{nombre: "CRL completa sin IDP", cert: hoja, crl: crlConExtensiones(t, f, nil), aceptar: true},
		{nombre: "IDP solo de usuarios para hoja", cert: hoja, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, []string{"http://x/mia.crl"}, campoIDP{tag: 1, bytes: verdadero})), aceptar: true},
		{nombre: "solo CA para hoja", cert: hoja, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, nil, campoIDP{tag: 2, bytes: verdadero})), motivo: "crl_no_cubre_usuario"},
		{nombre: "solo usuarios para CA", cert: intermedia, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, nil, campoIDP{tag: 1, bytes: verdadero})), motivo: "crl_no_cubre_ca"},
		{nombre: "solo algunos motivos", cert: hoja, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, nil, campoIDP{tag: 3, bytes: []byte{0x06, 0x40}})), motivo: "crl_parcial_no_admitida"},
		{nombre: "indirecta", cert: hoja, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, nil, campoIDP{tag: 4, bytes: verdadero})), motivo: "crl_parcial_no_admitida"},
		{nombre: "solo certificados de atributo", cert: hoja, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, nil, campoIDP{tag: 5, bytes: verdadero})), motivo: "crl_parcial_no_admitida"},
		{nombre: "partición y certificado sin CRLDP", cert: sinCRLDP, crl: crlConExtensiones(t, f, nil,
			extensionIDPPrueba(t, []string{"http://x/mia.crl"})), motivo: "crl_de_otra_particion"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			resultado, err := ValidateCRLResponse(caso.crl, caso.cert, f.ca, f.now)
			if caso.aceptar {
				if err != nil || resultado.Status != CertificateStatusGood {
					t.Fatalf("estado=%d error=%v, se esperaba Good", resultado.Status, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), caso.motivo) {
				t.Fatalf("estado=%d error=%v, se esperaba %q", resultado.Status, err, caso.motivo)
			}
		})
	}
}

// Una CRL del mismo emisor que lista el número de serie revoca aunque no sea
// la partición del certificado: lo que no puede dar una CRL ajena es «no
// revocado».
func TestValidateCRLResponse_ParticionAjenaQueListaLaSerieRevoca(t *testing.T) {
	f := newValidationFixture(t)
	hoja := hojaConCRLDP(t, f, 305, false, "http://x/mia.crl")
	crl := crlConExtensiones(t, f, hoja.SerialNumber, extensionIDPPrueba(t, []string{"http://x/otra.crl"}))
	resultado, err := ValidateCRLResponse(crl, hoja, f.ca, f.now)
	if err != nil || resultado.Status != CertificateStatusRevoked {
		t.Fatalf("estado=%d error=%v, se esperaba «revocado»", resultado.Status, err)
	}
}
