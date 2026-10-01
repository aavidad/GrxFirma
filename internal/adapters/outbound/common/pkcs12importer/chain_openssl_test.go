// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs12importer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"testing"

	"grxfirma/internal/testsupport/exttools"
	"software.sslmate.com/src/go-pkcs12"
)

func TestImportarDesdeOpenSSLConservaEmisor(t *testing.T) {
	exttools.Require(t, "openssl")
	caKey, template := generarMaterialOpenSSL(t, "CA sintética")
	template.IsCA = true
	template.BasicConstraintsValid = true
	template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	caDER, err := x509.CreateCertificate(rand.Reader, template, template, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	key, leafTemplate := generarMaterialOpenSSL(t, "Hoja sintética")
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pkcs12.Modern.Encode(key, leaf, []*x509.Certificate{ca}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(data)
	identity, err := importarDesdePKCS12ConOpenSSL(context.Background(), data, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identity.Certificate.Raw, leafDER) || len(identity.Chain) != 1 || !bytes.Equal(identity.Chain[0].Raw, caDER) {
		t.Fatal("fallback OpenSSL perdió el emisor o eligió otro certificado")
	}
}
