// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package anidamientoasn1_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/anidamientoasn1"
)

// anidadoDefinido devuelve n SEQUENCE de longitud definida, una dentro de otra.
func anidadoDefinido(n int) []byte {
	interior := []byte{0x05, 0x00} // NULL
	for i := 0; i < n; i++ {
		l := len(interior)
		cab := []byte{0x30, 0x84, byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}
		interior = append(cab, interior...)
	}
	return interior
}

// anidadoIndefinido devuelve n SEQUENCE de longitud indefinida bien cerradas.
func anidadoIndefinido(n int) []byte {
	return append(append(bytes.Repeat([]byte{0x30, 0x80}, n), 0x05, 0x00), bytes.Repeat([]byte{0, 0}, n)...)
}

func TestComprobar_AnidamientoIndefinidoEnorme(t *testing.T) {
	// 8 millones de niveles en 16 MiB, el tamaño máximo de /Contents.
	datos := bytes.Repeat([]byte{0x30, 0x80}, 8<<20)
	inicio := time.Now()
	err := anidamientoasn1.Comprobar(datos, anidamientoasn1.MaxProfundidad)
	if !errors.Is(err, anidamientoasn1.ErrDemasiadoProfundo) {
		t.Fatalf("se esperaba ErrDemasiadoProfundo: %v", err)
	}
	if time.Since(inicio) > time.Second {
		t.Fatalf("la comprobación tardó %v", time.Since(inicio))
	}
}

func TestComprobar_Limites(t *testing.T) {
	max := anidamientoasn1.MaxProfundidad
	for _, caso := range []struct {
		nombre string
		datos  []byte
		err    error
	}{
		{"definido en el límite", anidadoDefinido(max), nil},
		{"definido sobre el límite", anidadoDefinido(max + 1), anidamientoasn1.ErrDemasiadoProfundo},
		{"indefinido en el límite", anidadoIndefinido(max), nil},
		{"indefinido sobre el límite", anidadoIndefinido(max + 1), anidamientoasn1.ErrDemasiadoProfundo},
		{"indefinido sin cerrar", bytes.Repeat([]byte{0x30, 0x80}, 3), anidamientoasn1.ErrMalFormado},
		{"longitud mayor que los datos", []byte{0x30, 0x05, 0x05, 0x00}, anidamientoasn1.ErrMalFormado},
		{"hijo que sale del padre", []byte{0x30, 0x02, 0x30, 0x03, 0x05, 0x00, 0x00}, anidamientoasn1.ErrMalFormado},
		{"indefinido en primitivo", []byte{0x04, 0x80, 0x00, 0x00}, anidamientoasn1.ErrMalFormado},
		{"relleno de ceros tras el elemento", append(anidadoDefinido(3), 0, 0, 0, 0), nil},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			if err := anidamientoasn1.Comprobar(caso.datos, max); !errors.Is(err, caso.err) {
				t.Fatalf("Comprobar = %v, se esperaba %v", err, caso.err)
			}
		})
	}
}

func TestComprobar_CertificadoReal(t *testing.T) {
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Prueba", Organization: []string{"Org"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, plantilla, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	if err := anidamientoasn1.Comprobar(der, anidamientoasn1.MaxProfundidad); err != nil {
		t.Fatalf("certificado real rechazado: %v", err)
	}
}
