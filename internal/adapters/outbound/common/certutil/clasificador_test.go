// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certutil_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/domain"
)

func certConCampos(t *testing.T, subject pkix.Name, policies []asn1.ObjectIdentifier, isCA bool) *x509.Certificate {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generando clave: %v", err)
	}

	// Convertir asn1.ObjectIdentifier a x509.OID (requerido desde Go 1.23,
	// donde PolicyIdentifiers ya no se escribe ni se lee por defecto).
	var xPolicies []x509.OID
	for _, oid := range policies {
		ints := make([]uint64, len(oid))
		for i, v := range oid {
			ints[i] = uint64(v)
		}
		xoid, err := x509.OIDFromInts(ints)
		if err != nil {
			t.Fatalf("convirtiendo OID %v: %v", oid, err)
		}
		xPolicies = append(xPolicies, xoid)
	}

	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		IsCA:         isCA,
		Policies:     xPolicies,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatalf("creando certificado: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parseando certificado: %v", err)
	}
	return cert
}

func TestClasificarCertificado_Nil(t *testing.T) {
	t.Parallel()
	tipo, org, nif := certutil.ClasificarCertificado(nil)
	if tipo != domain.TipoCertDesconocido {
		t.Errorf("nil: tipo=%v, queria %v", tipo, domain.TipoCertDesconocido)
	}
	if org != "" || nif != "" {
		t.Errorf("nil: org=%q nif=%q, queria vacios", org, nif)
	}
}

func TestClasificarCertificado_CA(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{CommonName: "FNMT Root"}, nil, true)
	tipo, _, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertDesconocido {
		t.Errorf("CA: tipo=%v, queria desconocido", tipo)
	}
}

func TestClasificarCertificado_FNMTFisicaPorOID(t *testing.T) {
	t.Parallel()
	oid := asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 4, 1}
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Juan García López",
		SerialNumber: "IDCES-12345678A",
	}, []asn1.ObjectIdentifier{oid}, false)

	tipo, _, nif := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertFisica {
		t.Errorf("FNMT física OID: tipo=%v, queria fisica", tipo)
	}
	if nif != "IDCES-12345678A" {
		t.Errorf("NIF=%q, queria IDCES-12345678A", nif)
	}
}

func TestClasificarCertificado_RepresentacionPorOID(t *testing.T) {
	t.Parallel()
	oid := asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 4, 2}
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Empresa SL - Juan García",
		Organization: []string{"Empresa SL"},
		SerialNumber: "VATES-A12345678",
	}, []asn1.ObjectIdentifier{oid}, false)

	tipo, org, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertRepresentacion {
		t.Errorf("representacion OID: tipo=%v, queria representacion", tipo)
	}
	if org != "Empresa SL" {
		t.Errorf("org=%q, queria 'Empresa SL'", org)
	}
}

func TestClasificarCertificado_FisicaPorSerialIDCES(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Ana Martínez Ruiz",
		SerialNumber: "IDCES-87654321B",
	}, nil, false)

	tipo, _, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertFisica {
		t.Errorf("IDCES serial: tipo=%v, queria fisica", tipo)
	}
}

func TestClasificarCertificado_RepresentacionPorVATESConOrg(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Empresa Ejemplo SA - María López",
		Organization: []string{"Empresa Ejemplo SA"},
		SerialNumber: "VATES-A87654321",
	}, nil, false)

	tipo, org, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertRepresentacion {
		t.Errorf("VATES+org: tipo=%v, queria representacion", tipo)
	}
	if org != "Empresa Ejemplo SA" {
		t.Errorf("org=%q, queria 'Empresa Ejemplo SA'", org)
	}
}

func TestClasificarCertificado_SelloPorVATESSinOrg(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Sello Empresa SA",
		SerialNumber: "VATES-B12345678",
	}, nil, false)

	tipo, _, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertSello {
		t.Errorf("VATES sin org: tipo=%v, queria sello", tipo)
	}
}

// Una organización sin número de serie ni políticas de sello no es, por sí
// sola, un sello de entidad: se queda como certificado genérico.
func TestClasificarCertificado_OrgSinSerialNoEsSello(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Empresa Ejemplo SA",
		Organization: []string{"Empresa Ejemplo SA"},
	}, nil, false)

	tipo, org, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertDesconocido {
		t.Errorf("org sin serial: tipo=%v, queria desconocido", tipo)
	}
	if org != "Empresa Ejemplo SA" {
		t.Errorf("org=%q, queria 'Empresa Ejemplo SA'", org)
	}
}

func TestClasificarCertificado_EmpleadoPorOID(t *testing.T) {
	t.Parallel()
	oid := asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 4, 3}
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Pedro Funcionario",
		Organization: []string{"Ministerio de Hacienda"},
		SerialNumber: "IDCES-11223344C",
	}, []asn1.ObjectIdentifier{oid}, false)

	tipo, _, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertEmpleadoPublico {
		t.Errorf("empleado OID: tipo=%v, queria empleado_publico", tipo)
	}
}

func TestClasificarCertificado_Desconocido(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{CommonName: "Unknown Cert"}, nil, false)
	tipo, _, _ := certutil.ClasificarCertificado(cert)
	if tipo != domain.TipoCertDesconocido {
		t.Errorf("sin datos: tipo=%v, queria desconocido", tipo)
	}
}

func TestPuedeDigitalmenteSign(t *testing.T) {
	t.Parallel()
	tests := []struct {
		nombre   string
		keyUsage x509.KeyUsage
		quiere   bool
	}{
		{"sin restriccion", 0, true},
		{"digital signature", x509.KeyUsageDigitalSignature, true},
		{"content commitment", x509.KeyUsageContentCommitment, true},
		{"solo cifrado", x509.KeyUsageKeyEncipherment, false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			cert := certConCampos(t, pkix.Name{CommonName: "Test"}, nil, false)
			// Modificar KeyUsage directamente (truquillo de test)
			cert2 := *cert
			cert2.KeyUsage = tc.keyUsage
			if got := certutil.PuedeDigitalmenteSign(&cert2); got != tc.quiere {
				t.Errorf("KeyUsage=%v: got %v, queria %v", tc.keyUsage, got, tc.quiere)
			}
		})
	}
}

func TestPuedeCifrar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		nombre   string
		keyUsage x509.KeyUsage
		quiere   bool
	}{
		{"sin restriccion", 0, true},
		{"key encipherment", x509.KeyUsageKeyEncipherment, true},
		{"key agreement", x509.KeyUsageKeyAgreement, true},
		{"solo firma", x509.KeyUsageDigitalSignature, false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			cert := certConCampos(t, pkix.Name{CommonName: "Test"}, nil, false)
			cert2 := *cert
			cert2.KeyUsage = tc.keyUsage
			if got := certutil.PuedeCifrar(&cert2); got != tc.quiere {
				t.Errorf("KeyUsage=%v: got %v, queria %v", tc.keyUsage, got, tc.quiere)
			}
		})
	}
}

func certConExtensiones(t *testing.T, subject pkix.Name, extensiones []pkix.Extension) *x509.Certificate {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generando clave: %v", err)
	}
	plantilla := &x509.Certificate{
		SerialNumber:    big.NewInt(2),
		Subject:         subject,
		NotBefore:       time.Now(),
		NotAfter:        time.Now().Add(24 * time.Hour),
		KeyUsage:        x509.KeyUsageDigitalSignature,
		ExtraExtensions: extensiones,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatalf("creando certificado: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parseando certificado: %v", err)
	}
	return cert
}

// qcStatementsConTipos codifica QCStatements con QcCompliance y un QcType.
func qcStatementsConTipos(t *testing.T, tipos ...asn1.ObjectIdentifier) pkix.Extension {
	t.Helper()
	type declaracion struct {
		ID   asn1.ObjectIdentifier
		Info asn1.RawValue `asn1:"optional"`
	}
	info, err := asn1.Marshal(tipos)
	if err != nil {
		t.Fatal(err)
	}
	valor, err := asn1.Marshal([]declaracion{
		{ID: asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}},
		{ID: asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6}, Info: asn1.RawValue{FullBytes: info}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}, Value: valor}
}

func TestClasificarCertificado_SelloPorQcTypeESeal(t *testing.T) {
	t.Parallel()
	cert := certConExtensiones(t, pkix.Name{
		CommonName:   "Sello de Empresa Ejemplo",
		Organization: []string{"Empresa Ejemplo SA"},
	}, []pkix.Extension{qcStatementsConTipos(t, asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 2})})

	if tipo, _, _ := certutil.ClasificarCertificado(cert); tipo != domain.TipoCertSello {
		t.Errorf("QcType eseal: tipo=%v, queria sello", tipo)
	}
}

func TestClasificarCertificado_QcTypeESignNoEsSello(t *testing.T) {
	t.Parallel()
	cert := certConExtensiones(t, pkix.Name{
		CommonName:   "Empresa Ejemplo SA",
		Organization: []string{"Empresa Ejemplo SA"},
	}, []pkix.Extension{qcStatementsConTipos(t, asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 1})})

	if tipo, _, _ := certutil.ClasificarCertificado(cert); tipo != domain.TipoCertDesconocido {
		t.Errorf("QcType esign con org: tipo=%v, queria desconocido", tipo)
	}
}

func TestClasificarCertificado_SelloPorPoliticaQCPl(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{
		CommonName:   "Sello de Empresa Ejemplo",
		Organization: []string{"Empresa Ejemplo SA"},
	}, []asn1.ObjectIdentifier{{0, 4, 0, 194112, 1, 3}}, false)

	if tipo, _, _ := certutil.ClasificarCertificado(cert); tipo != domain.TipoCertSello {
		t.Errorf("QCP-l-qscd: tipo=%v, queria sello", tipo)
	}
}

func TestClasificarCertificado_FisicaPorNombreYApellidos(t *testing.T) {
	t.Parallel()
	cert := certConCampos(t, pkix.Name{
		CommonName:   "María López",
		Organization: []string{"Empresa Ejemplo SA"},
		ExtraNames: []pkix.AttributeTypeAndValue{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "María"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 4}, Value: "López"},
		},
	}, nil, false)

	if tipo, _, _ := certutil.ClasificarCertificado(cert); tipo != domain.TipoCertFisica {
		t.Errorf("nombre y apellidos: tipo=%v, queria fisica", tipo)
	}
}
