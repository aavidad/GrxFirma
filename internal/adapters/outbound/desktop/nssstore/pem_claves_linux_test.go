// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package nssstore

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

type parPrueba struct {
	clave *rsa.PrivateKey
	cert  *x509.Certificate
}

// parMismoSujeto crea un certificado autofirmado con el sujeto que NSS
// convierte en el mismo nickname, como tras renovar un certificado FNMT.
func parMismoSujeto(t *testing.T, serie int64) parPrueba {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(serie),
		Subject:      pkix.Name{Country: []string{"ES"}, Organization: []string{"FNMT"}, CommonName: "APELLIDO NOMBRE - 00000000T"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Duration(serie) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return parPrueba{clave: clave, cert: cert}
}

// salidaOpenSSL imita `openssl pkcs12 -nodes` sobre el P12 que exporta
// `pk12util -n` cuando dos certificados comparten nickname: trae las dos
// claves y los dos certificados, con atributos de bolsa entre medias.
func salidaOpenSSL(t *testing.T, pares ...parPrueba) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, par := range pares {
		pkcs8, err := x509.MarshalPKCS8PrivateKey(par.clave)
		if err != nil {
			t.Fatal(err)
		}
		out.WriteString("Bag Attributes\n    friendlyName: APELLIDO NOMBRE - 00000000T - FNMT\nKey Attributes: <No Attributes>\n")
		out.Write(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	}
	for _, par := range pares {
		out.WriteString("Bag Attributes\n    friendlyName: APELLIDO NOMBRE - 00000000T - FNMT\nsubject=/CN=APELLIDO NOMBRE\n")
		out.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: par.cert.Raw}))
	}
	return out.Bytes()
}

// F-09: el PEM se extrae sobre []byte en un único búfer y claveLocalDesdePEM
// deja a cero el bloque de entrada y las copias decodificadas de la clave.
func TestExtraerBloquesPEM_TrabajaEnBytesYSeBorra(t *testing.T) {
	t.Parallel()
	par := parMismoSujeto(t, 5)
	salida := salidaOpenSSL(t, par)
	datos := extraerBloquesPEM(salida)
	if !bytes.HasPrefix(datos, []byte("-----BEGIN PRIVATE KEY-----\n")) || bytes.Contains(datos, []byte("Bag Attributes")) {
		t.Fatalf("extraerBloquesPEM() no limpió los atributos:\n%s", datos)
	}
	if _, err := claveLocalDesdePEM(datos, domain.CertificateRef{Fingerprint: fingerprintSHA256(par.cert)}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(datos, []byte("PRIVATE KEY")) || bytes.Count(datos, []byte{0}) != len(datos) {
		t.Fatal("claveLocalDesdePEM() dejó el PEM con la clave privada sin borrar")
	}
}
