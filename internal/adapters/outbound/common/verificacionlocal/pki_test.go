// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digitorus/timestamp"
)

// Material criptográfico sintético generado en tiempo de prueba. Nada de lo
// que aquí se crea se escribe fuera de t.TempDir().

var serieSintetica atomic.Int64

type entidad struct {
	cert  *x509.Certificate
	clave crypto.Signer
}

type pkiPrueba struct {
	raiz       entidad
	intermedia entidad
	firmante   entidad
	tsa        entidad
}

func nuevaSerie() *big.Int {
	return big.NewInt(time.Now().UnixNano() + serieSintetica.Add(1))
}

func claveEC(t *testing.T) crypto.Signer {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func emitir(t *testing.T, plantilla *x509.Certificate, emisor *entidad, clave crypto.Signer) entidad {
	t.Helper()
	padre, clavePadre := plantilla, clave
	if emisor != nil {
		padre, clavePadre = emisor.cert, emisor.clave
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, padre, clave.Public(), clavePadre)
	if err != nil {
		t.Fatalf("emitiendo certificado sintético: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return entidad{cert: cert, clave: clave}
}

func plantillaCA(nombre string) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber:          nuevaSerie(),
		Subject:               pkix.Name{CommonName: nombre, Organization: []string{"Pruebas sintéticas"}},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(48 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
}

// nuevaPKI crea raíz → intermedia → firmante y una TSA emitida por la raíz.
// El firmante usa RSA para ser compatible con todos los formatos.
func nuevaPKI(t *testing.T, nombre string) pkiPrueba {
	t.Helper()
	var p pkiPrueba
	p.raiz = emitir(t, plantillaCA(nombre+" Raiz"), nil, claveEC(t))
	p.intermedia = emitir(t, plantillaCA(nombre+" Intermedia"), &p.raiz, claveEC(t))
	claveRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p.firmante = emitir(t, &x509.Certificate{
		SerialNumber:          nuevaSerie(),
		Subject:               pkix.Name{CommonName: nombre + " Firmante Sintetico"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		CRLDistributionPoints: []string{"http://crl.invalid/intermedia.crl"},
	}, &p.intermedia, claveRSA)
	p.tsa = emitir(t, &x509.Certificate{
		SerialNumber: nuevaSerie(),
		Subject:      pkix.Name{CommonName: nombre + " TSA Sintetica"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
	}, &p.raiz, claveEC(t))
	return p
}

type opcionesCRL struct {
	revocados  []*big.Int
	thisUpdate time.Time
	nextUpdate time.Time
	extra      []pkix.Extension
}

func crearCRL(t *testing.T, emisor entidad, op opcionesCRL) []byte {
	t.Helper()
	if op.thisUpdate.IsZero() {
		op.thisUpdate = time.Now().Add(-time.Hour)
	}
	if op.nextUpdate.IsZero() {
		op.nextUpdate = time.Now().Add(6 * time.Hour)
	}
	entradas := make([]x509.RevocationListEntry, 0, len(op.revocados))
	for _, serie := range op.revocados {
		entradas = append(entradas, x509.RevocationListEntry{SerialNumber: serie, RevocationTime: time.Now().Add(-30 * time.Minute)})
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		RevokedCertificateEntries: entradas,
		Number:                    nuevaSerie(),
		ThisUpdate:                op.thisUpdate,
		NextUpdate:                op.nextUpdate,
		ExtraExtensions:           op.extra,
	}, emisor.cert, emisor.clave)
	if err != nil {
		t.Fatalf("creando CRL sintética: %v", err)
	}
	return der
}

// extensionIDP construye un IssuingDistributionPoint con un único URI.
func extensionIDP(t *testing.T, uri string) pkix.Extension {
	t.Helper()
	general, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 6, Bytes: []byte(uri)})
	if err != nil {
		t.Fatal(err)
	}
	fullName, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: general})
	if err != nil {
		t.Fatal(err)
	}
	dpn, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: fullName})
	if err != nil {
		t.Fatal(err)
	}
	valor, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: dpn})
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Critical: true, Value: valor}
}

func escribir(t *testing.T, dir, nombre string, datos []byte) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func pemCert(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// tsaSintetica implementa ports.TimestampAuthority emitiendo el token en
// memoria. alterar cambia el messageImprint para simular un sello ajeno.
type tsaSintetica struct {
	emisor  entidad
	alterar bool
}

func (s tsaSintetica) RequestTimestamp(_ context.Context, resumen []byte, algoritmo crypto.Hash) ([]byte, error) {
	huella := append([]byte(nil), resumen...)
	if s.alterar {
		huella[0] ^= 0xff
	}
	sello := timestamp.Timestamp{
		HashAlgorithm:     algoritmo,
		HashedMessage:     huella,
		Time:              time.Now().Add(-time.Minute).UTC(),
		SerialNumber:      nuevaSerie(),
		Policy:            asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55555, 1},
		AddTSACertificate: true,
	}
	respuesta, err := sello.CreateResponseWithOpts(s.emisor.cert, s.emisor.clave, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	analizado, err := timestamp.ParseResponse(respuesta)
	if err != nil {
		return nil, err
	}
	return analizado.RawToken, nil
}
