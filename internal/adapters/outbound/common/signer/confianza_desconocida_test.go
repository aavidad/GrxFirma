// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

// TestApplySignerVerificationMetadata_ConfianzaDesconocidaNoValida reproduce
// E1-11: si se pidió el almacén raíz y no se pudo leer, un certificado
// autofirmado del atacante no puede salir como firma válida.
func TestApplySignerVerificationMetadata_ConfianzaDesconocidaNoValida(t *testing.T) {
	original := cargarRaicesSistema
	cargarRaicesSistema = func() (*x509.CertPool, error) { return nil, errors.New("sin almacén raíz") }
	t.Cleanup(func() { cargarRaicesSistema = original })

	p := nuevaPKILote3(t, nil)
	res := applySignerVerificationMetadata(
		domain.NewVerificationSuccess(string(domain.FormatCAdES), "firma correcta", nil),
		[]*x509.Certificate{p.hoja}, []*x509.Certificate{p.hoja, p.intermedia, p.raiz},
		domain.CertificateChain{UseSystemRoots: true},
	)
	if res.Trust.Status == domain.VerificationStatusValid {
		t.Fatalf("confianza=%s sin almacén raíz", res.Trust.Status)
	}
	if res.Valid {
		t.Fatalf("sin almacén raíz la firma salió válida con confianza %s", res.Trust.Status)
	}
}

// TestApplySignerVerificationMetadata_RSA1024NoValida: una clave RSA de 1024
// bits ya no protege una firma; se avisa y la firma no se da por válida.
func TestApplySignerVerificationMetadata_RSA1024NoValida(t *testing.T) {
	clave, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("no se pudo generar RSA de 1024 bits: %v", err)
	}
	ahora := time.Now()
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(77), Subject: pkix.Name{CommonName: "RSA corta"},
		NotBefore: ahora.Add(-time.Hour), NotAfter: ahora.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	cert := emitirLote3(t, plantilla, nil, &clave.PublicKey, clave)
	res := applySignerVerificationMetadata(
		domain.NewVerificationSuccess(string(domain.FormatXAdES), "firma correcta", nil),
		[]*x509.Certificate{cert}, []*x509.Certificate{cert},
		domain.CertificateChain{DERCertificates: [][]byte{cert.Raw}},
	)
	if res.Valid {
		t.Fatal("una firma con RSA de 1024 bits salió válida")
	}
	if !containsDetail(res.Warnings, motivoClaveRSADebil) {
		t.Fatalf("falta el aviso de clave RSA corta: %v", res.Warnings)
	}
}
