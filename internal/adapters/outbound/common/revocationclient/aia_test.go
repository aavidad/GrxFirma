// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func certificadoAIA(t *testing.T, cn string, padre *x509.Certificate, clavePadre *ecdsa.PrivateKey, aia string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  padre == nil,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	if aia != "" {
		plantilla.IssuingCertificateURL = []string{aia}
	}
	firmante, claveFirma := plantilla, clave
	if padre != nil {
		firmante, claveFirma = padre, clavePadre
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, firmante, &clave.PublicKey, claveFirma)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, clave
}

// El emisor descargado por AIA solo se acepta si su clave verifica la firma
// del certificado: un servidor que devuelva otra CA no completa la cadena.
func TestFetchIssuer_AceptaEmisorRealYRechazaImpostor(t *testing.T) {
	ca, claveCA := certificadoAIA(t, "CA real", nil, nil, "")
	impostor, _ := certificadoAIA(t, "CA real", nil, nil, "")
	servir := ca.Raw
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(servir)
	}))
	defer srv.Close()
	hoja, _ := certificadoAIA(t, "Firmante", ca, claveCA, srv.URL+"/ca.cer")

	cliente := newClient(srv.Client(), testEndpointPolicy())
	emisor, err := cliente.FetchIssuer(context.Background(), hoja)
	if err != nil || !emisor.Equal(ca) {
		t.Fatalf("FetchIssuer() = %v, %v; se esperaba la CA real", emisor, err)
	}

	servir = impostor.Raw
	if _, err := cliente.FetchIssuer(context.Background(), hoja); err == nil {
		t.Fatal("una CA con el mismo nombre pero otra clave no debe aceptarse como emisor")
	}
}
