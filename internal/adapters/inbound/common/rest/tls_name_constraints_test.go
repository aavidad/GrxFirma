// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

func TestCALocalRestringidaALoopback(t *testing.T) {
	certFile, _, rootFile, err := EnsureLocalhostCertificateWithLocalCA(t.TempDir(), "prueba")
	if err != nil {
		t.Fatalf("EnsureLocalhostCertificateWithLocalCA: %v", err)
	}
	leer := func(ruta string) *x509.Certificate {
		data, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		block, _ := pem.Decode(data)
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		return cert
	}
	raiz, hoja := leer(rootFile), leer(certFile)
	if !raiz.PermittedDNSDomainsCritical || len(raiz.PermittedDNSDomains) != 1 || raiz.PermittedDNSDomains[0] != "localhost" {
		t.Fatalf("la CA local no restringe los nombres: %v", raiz.PermittedDNSDomains)
	}
	if !localCANameConstraintsValid(raiz) {
		t.Fatalf("rango IP permitido inesperado: %v", raiz.PermittedIPRanges)
	}
	pool := x509.NewCertPool()
	pool.AddCert(raiz)
	for _, nombre := range []string{"localhost", "127.0.0.1", "::1"} {
		if _, err := hoja.Verify(x509.VerifyOptions{Roots: pool, DNSName: nombre}); err != nil {
			t.Fatalf("el certificado local debe seguir siendo válido para %s: %v", nombre, err)
		}
	}
	if got := hoja.NotAfter.Sub(hoja.NotBefore); got > localServerCertLifetime {
		t.Fatalf("vigencia del servidor %s supera 30 días", got)
	}
	keyDER, err := localtlstrust.LoadManagedCAKey(filepath.Join(filepath.Dir(rootFile), "prueba-root.key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParsePKCS1PrivateKey(keyDER)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(999),
		Subject:      pkix.Name{CommonName: "otro.example"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"otro.example"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, raiz, &otherKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	other, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Verify(x509.VerifyOptions{Roots: pool, DNSName: "otro.example"}); err == nil {
		t.Fatal("una hoja para dominio ajeno pasó Name Constraints")
	}
}

func TestCALocalRotaLaAnteriorSinRestricciones(t *testing.T) {
	dir := t.TempDir()
	const prefix = "prueba"
	_, _, rootFile, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix)
	if err != nil {
		t.Fatal(err)
	}
	oldRoot, ok := cargarCertificadoLocal(rootFile)
	if !ok {
		t.Fatal("CA inicial inválida")
	}
	oldKeyDER, err := localtlstrust.LoadManagedCAKey(filepath.Join(dir, prefix+"-root.key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := x509.ParsePKCS1PrivateKey(oldKeyDER)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1001),
		Subject:               pkix.Name{CommonName: "GrxFirma Local Root CA", Organization: []string{"Diputacion de Granada"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	localtlstrust.MarkManagedLocalCA(tpl)
	legacyDER, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &oldKey.PublicKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePEMCertificate(rootFile, legacyDER); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix); err != nil {
		t.Fatal(err)
	}
	newRoot, ok := cargarCertificadoLocal(rootFile)
	if !ok || !localCANameConstraintsValid(newRoot) || string(newRoot.Raw) == string(legacyDER) {
		t.Fatal("la CA anterior sin restricciones no fue rotada")
	}
	if string(newRoot.Raw) == string(oldRoot.Raw) {
		t.Fatal("la CA inicial tampoco debió reutilizarse")
	}
}

// escribirCAAntigua deja en dir, con el prefijo dado, una CA de GrxFirma con
// el perfil de las versiones anteriores a B-01 (marcador de propiedad, sin uso
// extendido y sin restricciones de nombre), su clave y una hoja firmada por ella.
func escribirCAAntigua(t *testing.T, dir, prefix string) []string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(4242),
		Subject:               pkix.Name{CommonName: "GrxFirma Local Root CA", Organization: []string{"Diputacion de Granada"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	localtlstrust.MarkManagedLocalCA(tpl)
	tpl.ExtKeyUsage = nil
	rootDER, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	if !localtlstrust.IsManagedLocalCAOwned(root) || localtlstrust.IsManagedLocalCA(root) {
		t.Fatal("la CA de prueba no reproduce el perfil antiguo")
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(4243),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 20),
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rootFile := filepath.Join(dir, prefix+"-root.crt.pem")
	rootKeyFile := filepath.Join(dir, prefix+"-root.key.pem")
	leafFile := filepath.Join(dir, prefix+".crt.pem")
	leafKeyFile := filepath.Join(dir, prefix+".key.pem")
	if err := writePEMCertificate(rootFile, rootDER); err != nil {
		t.Fatal(err)
	}
	if err := localtlstrust.SaveManagedCAKey(rootKeyFile, x509.MarshalPKCS1PrivateKey(rootKey)); err != nil {
		t.Fatal(err)
	}
	if err := writePEMCertificate(leafFile, leafDER); err != nil {
		t.Fatal(err)
	}
	if err := writePEMKey(leafKeyFile, leafKey); err != nil {
		t.Fatal(err)
	}
	return []string{rootFile, rootKeyFile, leafFile, leafKeyFile}
}

func TestCALocalRESTAntiguaSinRestriccionesSeRetiraYSeRota(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, "grxfirma", "tls")
	antiguos := escribirCAAntigua(t, dir, legacyRESTPrefix)

	_, _, rootFile, _, err := EnsureBrowserCompatibleLocalhostCertificate(dir, ManagedLocalhostPrefix)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range antiguos {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("la CA REST antigua sin restricciones no se retiró: %s sigue (%v)", filepath.Base(path), err)
		}
	}
	nueva, ok := cargarCertificadoLocal(rootFile)
	if !ok || !localtlstrust.IsManagedLocalCA(nueva) || !nueva.PermittedDNSDomainsCritical {
		t.Fatal("la CA nueva no está restringida")
	}
}

func TestVaciarAlmacenTLSConCAAntiguaNoLaTomaPorAjena(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, "grxfirma", "tls")
	antiguos := escribirCAAntigua(t, dir, ManagedLocalhostPrefix)

	removed, err := ClearManagedLocalhostTLS(context.Background(), dir)
	if err != nil {
		t.Fatalf("vaciar el almacén con una CA propia antigua: %v", err)
	}
	if removed < len(antiguos) {
		t.Fatalf("ficheros retirados = %d; want al menos %d", removed, len(antiguos))
	}
	for _, path := range antiguos {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s sigue tras vaciar el almacén (%v)", filepath.Base(path), err)
		}
	}
}

func TestCALocalRenuevaHojaSinCambiarRaiz(t *testing.T) {
	dir := t.TempDir()
	const prefix = "prueba"
	certFile, _, rootFile, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix)
	if err != nil {
		t.Fatal(err)
	}
	oldRoot, ok := cargarCertificadoLocal(rootFile)
	if !ok {
		t.Fatal("CA inicial inválida")
	}
	if err := os.Remove(certFile); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix); err != nil {
		t.Fatal(err)
	}
	newRoot, ok := cargarCertificadoLocal(rootFile)
	if !ok || string(newRoot.Raw) != string(oldRoot.Raw) {
		t.Fatal("la renovación de la hoja rotó innecesariamente la CA")
	}
	if !certValidoLocalhostFirmadoPorCA(certFile, filepath.Join(dir, prefix+".key.pem"), rootFile) {
		t.Fatal("la hoja renovada no es válida")
	}
}

func TestCALocalConcurrenteMantieneParCoherente(t *testing.T) {
	dir := t.TempDir()
	const prefix = "prueba"
	var group sync.WaitGroup
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			certFile, keyFile, rootFile, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix)
			if err == nil && !certValidoLocalhostFirmadoPorCA(certFile, keyFile, rootFile) {
				err = fmt.Errorf("par certificado/CA inconsistente")
			}
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
