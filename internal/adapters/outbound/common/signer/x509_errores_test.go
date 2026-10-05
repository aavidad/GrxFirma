// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func certificadoAutofirmadoPrueba(t *testing.T, desde, hasta time.Time) *x509.Certificate {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Prueba"},
		NotBefore: desde, NotAfter: hasta, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestCodigoErrorX509(t *testing.T) {
	ahora := time.Now()
	vigente := certificadoAutofirmadoPrueba(t, ahora.Add(-time.Hour), ahora.Add(time.Hour))
	_, errAutoridad := vigente.Verify(x509.VerifyOptions{Roots: x509.NewCertPool()})

	caducado := certificadoAutofirmadoPrueba(t, ahora.Add(-48*time.Hour), ahora.Add(-24*time.Hour))
	raices := x509.NewCertPool()
	raices.AddCert(caducado)
	_, errCaducado := caducado.Verify(x509.VerifyOptions{Roots: raices, CurrentTime: ahora})

	casos := []struct {
		err    error
		quiere string
	}{
		{errAutoridad, "x509_autoridad_desconocida"},
		{fmt.Errorf("envuelto: %w", errAutoridad), "x509_autoridad_desconocida"},
		{errCaducado, "x509_caducado"},
		{x509.CertificateInvalidError{Reason: x509.IncompatibleUsage}, "x509_uso_no_permitido"},
		{x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign}, "x509_emisor_no_autorizado"},
		{x509.CertificateInvalidError{Reason: x509.TooManyIntermediates}, "x509_cadena_demasiado_larga"},
		{x509.HostnameError{Certificate: vigente, Host: "ejemplo.es"}, "x509_nombre_no_coincide"},
		{x509.InsecureAlgorithmError(x509.SHA1WithRSA), "x509_algoritmo_inseguro"},
		{errors.New("otro"), "x509_otro"},
	}
	for _, caso := range casos {
		if got := CodigoErrorX509(caso.err); got != caso.quiere {
			t.Errorf("CodigoErrorX509(%v) = %q, quiere %q", caso.err, got, caso.quiere)
		}
	}
}

// La cadena de un certificado de pruebas no llega a la persona con el texto
// en inglés de crypto/x509, sino con un código traducible.
func TestConfianzaSinAncla_DetalleTraducible(t *testing.T) {
	ahora := time.Now()
	firmante := certificadoAutofirmadoPrueba(t, ahora.Add(-time.Hour), ahora.Add(time.Hour))
	otraRaiz := certificadoAutofirmadoPrueba(t, ahora.Add(-time.Hour), ahora.Add(time.Hour))
	aspecto := evaluateTrustFromCertificates([]*x509.Certificate{firmante}, nil,
		domain.CertificateChain{DERCertificates: [][]byte{otraRaiz.Raw}})
	unido := strings.Join(aspecto.Details, "\n")
	if strings.Contains(unido, "x509:") || strings.Contains(unido, "unknown authority") {
		t.Fatalf("los detalles no deben llevar el error en inglés: %v", aspecto.Details)
	}
	if !strings.Contains(unido, "error_cadena=x509_autoridad_desconocida") {
		t.Fatalf("falta el código traducible en los detalles: %v", aspecto.Details)
	}
}
