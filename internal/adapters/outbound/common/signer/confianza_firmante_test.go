// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// TestEvaluateTrust_RechazaCertificadoSoloServerAuth reproduce E1-05: un
// certificado de servidor web emitido bajo una raíz de confianza no es un
// firmante de confianza.
func TestEvaluateTrust_RechazaCertificadoSoloServerAuth(t *testing.T) {
	casos := []struct {
		nombre string
		eku    []x509.ExtKeyUsage
		apto   bool
	}{
		{nombre: "solo servidor TLS", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}},
		{nombre: "servidor y cliente TLS", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}},
		{nombre: "firma de código", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}},
		{nombre: "sellado de tiempo", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}},
		{nombre: "respondedor OCSP", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning}},
		{nombre: "sin EKU", apto: true},
		{nombre: "cliente y correo", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageEmailProtection}, apto: true},
		{nombre: "solo correo", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection}, apto: true},
		{nombre: "solo cliente TLS", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, apto: true},
		{nombre: "servidor y correo", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageEmailProtection}, apto: true},
		{nombre: "cualquier uso", eku: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, apto: true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			p := nuevaPKILote3(t, nil, caso.eku...)
			trust := evaluateTrustFromCertificates([]*x509.Certificate{p.hoja}, []*x509.Certificate{p.hoja, p.intermedia}, p.anclas)
			if caso.apto && trust.Status != domain.VerificationStatusValid {
				t.Fatalf("certificado apto rechazado: %s %s", trust.Status, trust.Reason)
			}
			if !caso.apto && trust.Status == domain.VerificationStatusValid {
				t.Fatalf("un certificado que no es de firma salió de confianza: %s", trust.Reason)
			}
		})
	}
}

func TestCertificadoAptoParaFirmar_EKUDeFirmaDeDocumentos(t *testing.T) {
	for _, oid := range []asn1.ObjectIdentifier{
		{1, 3, 6, 1, 4, 1, 311, 10, 3, 12}, // Microsoft documentSigning
		{1, 3, 6, 1, 5, 5, 7, 3, 36},       // id-kp-documentSigning (RFC 9336)
		{1, 2, 840, 113583, 1, 1, 5},       // Adobe authenticDocumentsTrust
	} {
		cert := &x509.Certificate{UnknownExtKeyUsage: []asn1.ObjectIdentifier{oid}}
		if !certificadoAptoParaFirmar(cert) {
			t.Fatalf("EKU de firma de documentos %s rechazado", oid)
		}
	}
	if certificadoAptoParaFirmar(&x509.Certificate{UnknownExtKeyUsage: []asn1.ObjectIdentifier{{1, 2, 3, 4}}}) {
		t.Fatal("un EKU desconocido sin ningún uso de firma se aceptó")
	}
}

// TestCertificadoAptoParaFirmar_CertificadosReales comprueba que los
// certificados de firma reales del repositorio (Uanataca: persona física,
// representante, empleado público y sellos) y los firmantes de la PKI
// sintética del dictamen v2 siguen aceptándose.
func TestCertificadoAptoParaFirmar_CertificadosReales(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..", "..")
	var rutas []string
	for _, patron := range []string{
		"tests/certificates/*/*.crt", "tests/certificates/*/*.cer",
		"testdata/dictamen-v2/pki/firmante-a.pem", "testdata/dictamen-v2/pki/firmante-b.pem",
	} {
		encontradas, err := filepath.Glob(filepath.Join(raiz, patron))
		if err != nil {
			t.Fatal(err)
		}
		rutas = append(rutas, encontradas...)
	}
	if len(rutas) < 20 {
		t.Fatalf("se esperaban los certificados reales del repositorio y hay %d", len(rutas))
	}
	for _, ruta := range rutas {
		t.Run(filepath.Base(ruta), func(t *testing.T) {
			cert := leerCertificadoPrueba(t, ruta)
			if !certificadoAptoParaFirmar(cert) {
				t.Fatalf("certificado de firma real rechazado: EKU=%v desconocidos=%v", cert.ExtKeyUsage, cert.UnknownExtKeyUsage)
			}
		})
	}
}

func leerCertificadoPrueba(t *testing.T, ruta string) *x509.Certificate {
	t.Helper()
	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	der := datos
	if bloque, _ := pem.Decode(datos); bloque != nil {
		der = bloque.Bytes
	} else if len(datos) > 0 && datos[0] != 0x30 {
		decodificado, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(datos)))
		if err != nil {
			t.Fatalf("%s: %v", ruta, err)
		}
		der = decodificado
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("%s: %v", ruta, err)
	}
	return cert
}

// TestEvaluateTrust_AnclaExplicitaNoSeDescribeComoRaizDelSistema: con anclas
// configuradas el motivo no habla del almacén del sistema.
func TestEvaluateTrust_AnclaExplicitaNoSeDescribeComoRaizDelSistema(t *testing.T) {
	p := nuevaPKILote3(t, nil)
	trust := evaluateTrustFromCertificates([]*x509.Certificate{p.hoja}, []*x509.Certificate{p.hoja, p.intermedia}, p.anclas)
	if trust.Status != domain.VerificationStatusValid || trust.Reason != motivoConfianzaAnclaConfigurada {
		t.Fatalf("confianza=%s motivo=%q", trust.Status, trust.Reason)
	}
}

// TestEvaluateTrust_RaizDelSistemaLoDice: si la cadena termina en una raíz del
// almacén del sistema, el motivo lo dice y no sugiere que el certificado sea
// cualificado.
func TestEvaluateTrust_RaizDelSistemaLoDice(t *testing.T) {
	if pool, err := x509.SystemCertPool(); err != nil || pool == nil {
		t.Skip("sin almacén raíz del sistema en este equipo")
	}
	var raiz *x509.Certificate
	for _, ruta := range []string{"/etc/ssl/certs/ca-certificates.crt", "/etc/pki/tls/certs/ca-bundle.crt", "/etc/ssl/cert.pem"} {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			continue
		}
		for len(datos) > 0 && raiz == nil {
			var bloque *pem.Block
			bloque, datos = pem.Decode(datos)
			if bloque == nil {
				break
			}
			// Solo una raíz que el propio sistema da por buena: en macOS el
			// fichero puede llevar raíces caducadas o retiradas del llavero.
			if cert, err := x509.ParseCertificate(bloque.Bytes); err == nil && esAutofirmado(cert) && raizAceptadaPorElSistema(cert) {
				raiz = cert
			}
		}
		if raiz != nil {
			break
		}
	}
	if raiz == nil {
		t.Skip("no se encontró un fichero de raíces del sistema legible")
	}
	trust := evaluateTrustFromCertificates([]*x509.Certificate{raiz}, nil, domain.CertificateChain{UseSystemRoots: true})
	if trust.Status != domain.VerificationStatusValid || trust.Reason != motivoConfianzaRaizSistema {
		t.Fatalf("confianza=%s motivo=%q", trust.Status, trust.Reason)
	}
}

func raizAceptadaPorElSistema(cert *x509.Certificate) bool {
	_, err := cert.Verify(x509.VerifyOptions{KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	return err == nil
}
