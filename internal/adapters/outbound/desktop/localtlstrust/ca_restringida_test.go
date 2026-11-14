// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// caLocalDePrueba crea una CA con la misma plantilla que el REST: marcador,
// restricciones de nombre de loopback y, si restringida, MarkManagedLocalCA.
// Con restringida=false reproduce las CAs anteriores a B-01 (sin uso
// extendido).
func caLocalDePrueba(t *testing.T, serial int64, restringida bool) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:   nicknameLocalhostRoot,
			Organization: []string{"Diputacion de Granada"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		PermittedDNSDomains:   []string{"localhost"},
		PermittedIPRanges:     managedLocalCAPermittedIPRanges(),
	}
	if restringida {
		MarkManagedLocalCA(tpl)
	} else {
		tpl.Subject.OrganizationalUnit = []string{managedLocalCAOwnershipMarker}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func hojaEmitida(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, uso x509.ExtKeyUsage) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{uso},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestCALocal_SoloValeParaServidorTLS(t *testing.T) {
	t.Parallel()

	ca, caKey := caLocalDePrueba(t, 501, true)
	if len(ca.ExtKeyUsage) != 1 || ca.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatalf("uso extendido de la CA = %v; want solo servidor TLS", ca.ExtKeyUsage)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	servidor := hojaEmitida(t, ca, caKey, x509.ExtKeyUsageServerAuth)
	if _, err := servidor.Verify(x509.VerifyOptions{Roots: pool, DNSName: "localhost"}); err != nil {
		t.Fatalf("el certificado de servidor local debe seguir valiendo: %v", err)
	}
	for _, uso := range []x509.ExtKeyUsage{
		x509.ExtKeyUsageCodeSigning,
		x509.ExtKeyUsageEmailProtection,
		x509.ExtKeyUsageClientAuth,
	} {
		hoja := hojaEmitida(t, ca, caKey, uso)
		if _, err := hoja.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{uso}}); err == nil {
			t.Fatalf("una hoja con uso %v emitida por la CA local fue aceptada", uso)
		}
	}
}

func TestCALocal_AnteriorSinUsoNoSeReconoceComoVigente(t *testing.T) {
	t.Parallel()

	anterior, _ := caLocalDePrueba(t, 502, false)
	if IsManagedLocalCA(anterior) {
		t.Fatal("una CA anterior sin uso restringido se dio por vigente")
	}
	if !IsManagedLocalCAOwned(anterior) {
		t.Fatal("la CA anterior debe seguir atribuyéndose a GrxFirma para poder retirarla")
	}
	actual, _ := caLocalDePrueba(t, 503, true)
	if !IsManagedLocalCA(actual) {
		t.Fatal("la CA restringida no se reconoce como vigente")
	}
}

func TestEnsureManagedTrusted_RechazaCAAnteriorSinUso(t *testing.T) {
	t.Parallel()

	anterior, _ := caLocalDePrueba(t, 504, false)
	path := filepath.Join(t.TempDir(), "websocket-localhost-root.crt.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: anterior.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	err := EnsureManagedTrusted(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "restringida") {
		t.Fatalf("EnsureManagedTrusted() = %v; want rechazo por CA sin restringir", err)
	}
}

func TestRotacion_RetiraLaCAAnteriorSinUsoInventariada(t *testing.T) {
	t.Parallel()

	certFile := filepath.Join(t.TempDir(), "websocket-localhost-root.crt.pem")
	anterior, _ := caLocalDePrueba(t, 505, false)
	store := newFakeManagedCertificateStore()
	store.certificates[fingerprintSHA256(anterior)] = anterior
	inventory := newManagedTrustInventory()
	inventory.CurrentSHA256 = fingerprintSHA256(anterior)
	inventory.OwnedSHA256 = []string{fingerprintSHA256(anterior)}
	if err := writeManagedTrustInventory(managedTrustInventoryPath(certFile), inventory); err != nil {
		t.Fatal(err)
	}

	nueva, _ := caLocalDePrueba(t, 506, true)
	if err := reconcileManagedTrust(context.Background(), certFile, nueva, store); err != nil {
		t.Fatalf("reconcileManagedTrust() error = %v", err)
	}
	if _, sigue := store.certificates[fingerprintSHA256(anterior)]; sigue {
		t.Fatal("la CA anterior sin restringir sigue confiada tras la rotación")
	}
	if _, instalada := store.certificates[fingerprintSHA256(nueva)]; !instalada {
		t.Fatal("la CA restringida no se instaló")
	}
}

func TestUsoServidorTLSParaPropiedadDeWindows(t *testing.T) {
	t.Parallel()

	der, err := serverAuthOnlyEnhancedKeyUsageDER()
	if err != nil {
		t.Fatal(err)
	}
	var usos []asn1.ObjectIdentifier
	if rest, err := asn1.Unmarshal(der, &usos); err != nil || len(rest) != 0 {
		t.Fatalf("codificación de usos inválida: %v", err)
	}
	if len(usos) != 1 || !usos[0].Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1}) {
		t.Fatalf("usos = %v; want solo id-kp-serverAuth", usos)
	}
}
