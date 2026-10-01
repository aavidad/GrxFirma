// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/pdffixture"
)

func main() {
	outDir := flag.String("out", "/tmp/grxfirma-pades-samples", "directorio de salida")
	tsaURL := flag.String("tsa-url", "", "URL RFC 3161 para generar también una muestra PAdES-B-T")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		fatalf("no se pudo crear el directorio de salida: %v", err)
	}

	priv, cert := generarCertificado()
	clave := desktopsigner.NuevaClaveLocal(priv, cert)
	motor := desktopsigner.NuevoMotorFirmaGo(nil)

	muestras := []struct {
		nombre  string
		options map[string]string
	}{
		{nombre: "pades-etsi.pdf", options: nil},
		{nombre: "pades-adobe.pdf", options: map[string]string{"subfilter": "adobe"}},
	}
	if *tsaURL != "" {
		muestras = append(muestras, struct {
			nombre  string
			options map[string]string
		}{
			nombre: "pades-adobe-t.pdf",
			options: map[string]string{
				"subfilter": "adobe",
				"level":     "T",
				"tsaURL":    *tsaURL,
			},
		})
	}

	for _, muestra := range muestras {
		doc, err := domain.NewDocument(muestra.nombre, pdffixture.Minimal(), "application/pdf")
		if err != nil {
			fatalf("no se pudo construir el documento base: %v", err)
		}
		result, err := motor.Sign(context.Background(), domain.SignatureJob{
			Document: doc,
			Format:   domain.FormatPAdES,
			Action:   domain.ActionSign,
			Options:  muestra.options,
		}, clave)
		if err != nil {
			fatalf("no se pudo firmar %s: %v", muestra.nombre, err)
		}
		ruta := filepath.Join(*outDir, muestra.nombre)
		if err := os.WriteFile(ruta, result.Data, 0o600); err != nil {
			fatalf("no se pudo escribir %s: %v", ruta, err)
		}
		fmt.Printf("%s\t%s\n", ruta, result.Algorithm)
	}
}

func generarCertificado() (*rsa.PrivateKey, *x509.Certificate) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fatalf("no se pudo generar clave RSA: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "GrxFirma PAdES Samples"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		fatalf("no se pudo crear certificado de muestra: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		fatalf("no se pudo parsear certificado de muestra: %v", err)
	}
	return priv, cert
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
