// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs12importer_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/desktop/sessioncertstore"
	"software.sslmate.com/src/go-pkcs12"
)

func generarCadenaImportacion(t *testing.T) (*ecdsa.PrivateKey, []*x509.Certificate) {
	t.Helper()
	var certs []*x509.Certificate
	var parent *x509.Certificate
	var parentKey *ecdsa.PrivateKey
	for i, name := range []string{"Raíz sintética", "Intermedia sintética", "Firmante sintético"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(int64(i + 1)), Subject: pkix.Name{CommonName: name},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			BasicConstraintsValid: true, IsCA: i < 2, KeyUsage: x509.KeyUsageDigitalSignature,
		}
		if i < 2 {
			template.KeyUsage |= x509.KeyUsageCertSign
		}
		if parent == nil {
			parent = template
			parentKey = key
		}
		der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		certs = append(certs, cert)
		parent = cert
		parentKey = key
	}
	return parentKey, []*x509.Certificate{certs[2], certs[1], certs[0]}
}

func bundleCadenaImportacion(t *testing.T, key *ecdsa.PrivateKey, certs []*x509.Certificate) []byte {
	t.Helper()
	var bundle []byte
	for _, cert := range certs {
		bundle = append(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(keyDER)
	return append(bundle, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...)
}

func TestImportIdentityConservaCadenaEnP12PEMYAlmacenTemporal(t *testing.T) {
	key, certs := generarCadenaImportacion(t)
	p12, err := pkcs12.Modern.Encode(key, certs[0], []*x509.Certificate{certs[2], certs[1]}, "prueba-sintética")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(p12)
	pemBundle := bundleCadenaImportacion(t, key, []*x509.Certificate{certs[2], certs[0], certs[1], certs[2]})
	defer clear(pemBundle)
	for _, tc := range []struct {
		name     string
		data     []byte
		password string
	}{
		{"P12 desordenado", p12, "prueba-sintética"}, {"PEM con raíz primero y duplicados", pemBundle, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, err := pkcs12importer.New().ImportIdentity(context.Background(), tc.data, tc.password)
			if err != nil {
				t.Fatal(err)
			}
			if len(identity.Chain) != 2 || !bytes.Equal(identity.Certificate.Raw, certs[0].Raw) ||
				!bytes.Equal(identity.Chain[0].Raw, certs[1].Raw) || !bytes.Equal(identity.Chain[1].Raw, certs[2].Raw) {
				t.Fatal("cadena no conservada en orden hoja/emisor/raíz")
			}
			if len(identity.Reference.ChainDER) != 2 || !bytes.Equal(identity.Reference.ChainDER[0], certs[1].Raw) ||
				!bytes.Equal(identity.Reference.ChainDER[1], certs[2].Raw) {
				t.Fatal("la referencia pública no conserva los emisores de la credencial")
			}
			if identity.Reference.HasLocalDecryptionKey {
				t.Fatal("una clave ECDSA no sirve para Desproteger RSA-OAEP")
			}
			store := sessioncertstore.New()
			defer store.Clear()
			ref, err := store.Load(context.Background(), tc.data, tc.password)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := store.KeyFor(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			chain := loaded.CertificateChainDER()
			if len(chain) != 3 {
				t.Fatalf("cadena del proveedor tiene %d certificados, esperados3", len(chain))
			}
			for i, cert := range certs {
				if !bytes.Equal(chain[i], cert.Raw) {
					t.Fatalf("certificado%d incorrecto", i)
				}
			}
			// Adjuntar una raíz como evidencia no la convierte en confiable.
			intermediates := x509.NewCertPool()
			intermediates.AddCert(certs[1])
			if _, err := identity.Certificate.Verify(x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err == nil {
				t.Fatal("se confió en una raíz solo por estar adjunta")
			}
		})
	}
}

func TestImportIdentityNoAsociaClavePrivadaConOtroCertificado(t *testing.T) {
	key, _ := generarCadenaImportacion(t)
	_, other := generarCadenaImportacion(t)
	bundle := bundleCadenaImportacion(t, key, other)
	defer clear(bundle)
	if _, err := pkcs12importer.New().ImportIdentity(context.Background(), bundle, ""); err == nil {
		t.Fatal("aceptó certificado ajeno a la clave")
	}
}

func TestImportIdentityConservaHojaSinAnexarEmisorFalso(t *testing.T) {
	key, certs := generarCadenaImportacion(t)
	_, other := generarCadenaImportacion(t) // Mismos nombres de emisor, otras claves.
	bundle := bundleCadenaImportacion(t, key, []*x509.Certificate{certs[0], other[1], other[2]})
	defer clear(bundle)
	identity, err := pkcs12importer.New().ImportIdentity(context.Background(), bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(identity.Chain) != 0 {
		t.Fatal("se añadió un supuesto emisor sin verificar su firma")
	}
}

func TestImportIdentityLimitaCantidadDeCertificados(t *testing.T) {
	key, certs := generarCadenaImportacion(t)
	repeated := make([]*x509.Certificate, 65)
	for i := range repeated {
		repeated[i] = certs[0]
	}
	bundle := bundleCadenaImportacion(t, key, repeated)
	defer clear(bundle)
	if _, err := pkcs12importer.New().ImportIdentity(context.Background(), bundle, ""); err == nil {
		t.Fatal("aceptó exceso de certificados")
	}
}
