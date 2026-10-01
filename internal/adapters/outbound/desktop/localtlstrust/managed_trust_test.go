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
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeManagedCertificateStore struct {
	certificates map[string]*x509.Certificate
	removeErrors map[string]error
	addCalls     int
	removeCalls  []string
}

func TestRetireManagedCAKeyWithoutPromptConservaInventarioYMarcaPendiente(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	keyFile := filepath.Join(dir, "websocket-localhost-root.key.pem")
	inventoryFile := managedTrustInventoryPath(certFile)
	for _, file := range []string{certFile, keyFile, inventoryFile} {
		if err := os.WriteFile(file, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RetireManagedCAKeyWithoutPrompt(certFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clave CA aún existe: %v", err)
	}
	for _, file := range []string{certFile, inventoryFile, certFile + pendingManagedTrustRemovalSuffix} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("falta %s: %v", file, err)
		}
	}
}

func TestRetireManagedCAKeyWithoutPromptSinDatosTLS(t *testing.T) {
	certFile := filepath.Join(t.TempDir(), "tls", "websocket-localhost-root.crt.pem")
	if err := RetireManagedCAKeyWithoutPrompt(certFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(certFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("se creó directorio TLS vacío: %v", err)
	}
}

func TestRetiradaPendienteSoloBorraHuellaPropiaTrasExito(t *testing.T) {
	certFile := filepath.Join(t.TempDir(), "websocket-localhost-root.crt.pem")
	owned := newManagedLocalCATestCertificate(t, 118, true)
	unrelated := newManagedLocalCATestCertificate(t, 119, false)
	store := newFakeManagedCertificateStore()
	store.certificates[fingerprintSHA256(unrelated)] = unrelated
	if err := reconcileManagedTrust(context.Background(), certFile, owned, store); err != nil {
		t.Fatal(err)
	}
	if err := RetireManagedCAKeyWithoutPrompt(certFile); err != nil {
		t.Fatal(err)
	}
	marker := certFile + pendingManagedTrustRemovalSuffix
	store.removeErrors[fingerprintSHA256(owned)] = errors.New("rechazo")
	if err := finishPendingManagedTrustRemoval(context.Background(), certFile, store); err == nil {
		t.Fatal("se aceptó fallo de ROOT")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("se perdió el marcador: %v", err)
	}
	delete(store.removeErrors, fingerprintSHA256(owned))
	if err := finishPendingManagedTrustRemoval(context.Background(), certFile, store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marcador pendiente tras éxito: %v", err)
	}
	if _, exists := store.certificates[fingerprintSHA256(unrelated)]; !exists {
		t.Fatal("se borró CA ajena")
	}
}

func TestManagedTrustLifecycleSupported_WindowsYLinux(t *testing.T) {
	t.Parallel()

	got := ManagedTrustLifecycleSupported()
	want := runtime.GOOS == "windows" || runtime.GOOS == "linux"
	if got != want {
		t.Fatalf(
			"ManagedTrustLifecycleSupported() = %t, want %t en %s",
			got,
			want,
			runtime.GOOS,
		)
	}
}

func newFakeManagedCertificateStore() *fakeManagedCertificateStore {
	return &fakeManagedCertificateStore{
		certificates: make(map[string]*x509.Certificate),
		removeErrors: make(map[string]error),
	}
}

func (store *fakeManagedCertificateStore) Contains(
	_ context.Context,
	fingerprint string,
) (bool, error) {
	_, exists := store.certificates[fingerprint]
	return exists, nil
}

func (store *fakeManagedCertificateStore) Add(
	_ context.Context,
	cert *x509.Certificate,
) (bool, error) {
	fingerprint := fingerprintSHA256(cert)
	if _, exists := store.certificates[fingerprint]; exists {
		return false, nil
	}
	store.addCalls++
	store.certificates[fingerprint] = cert
	return true, nil
}

func (store *fakeManagedCertificateStore) Remove(
	_ context.Context,
	fingerprint string,
) (bool, error) {
	store.removeCalls = append(store.removeCalls, fingerprint)
	if err := store.removeErrors[fingerprint]; err != nil {
		return false, err
	}
	cert, exists := store.certificates[fingerprint]
	if !exists {
		return false, nil
	}
	if !IsManagedLocalCA(cert) {
		return false, errors.New("fake store rechazó borrar un certificado no gestionado")
	}
	delete(store.certificates, fingerprint)
	return true, nil
}

func TestManagedTrust_InstalaInventariaYDesinstalaSoloLaCAPropia(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	managed := newManagedLocalCATestCertificate(t, 1, true)
	unrelated := newManagedLocalCATestCertificate(t, 2, false)
	managedFingerprint := fingerprintSHA256(managed)
	unrelatedFingerprint := fingerprintSHA256(unrelated)
	store := newFakeManagedCertificateStore()
	store.certificates[unrelatedFingerprint] = unrelated

	if err := reconcileManagedTrust(context.Background(), certFile, managed, store); err != nil {
		t.Fatalf("reconcileManagedTrust() error = %v", err)
	}
	inventory, exists, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil || !exists {
		t.Fatalf("loadManagedTrustInventory() exists=%v error=%v", exists, err)
	}
	if inventory.CurrentSHA256 != managedFingerprint ||
		len(inventory.OwnedSHA256) != 1 ||
		inventory.OwnedSHA256[0] != managedFingerprint {
		t.Fatalf("inventario inesperado: %#v", inventory)
	}

	if err := removeManagedTrust(context.Background(), certFile, store); err != nil {
		t.Fatalf("removeManagedTrust() error = %v", err)
	}
	if _, exists := store.certificates[managedFingerprint]; exists {
		t.Fatal("la CA propia sigue instalada")
	}
	if _, exists := store.certificates[unrelatedFingerprint]; !exists {
		t.Fatal("se eliminó un certificado ajeno")
	}
	if _, err := os.Lstat(managedTrustInventoryPath(certFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("el inventario no se retiró: %v", err)
	}
}

func TestManagedTrust_PreexistenteSeRegistraComoCompartidaYNoSeBorra(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	cert := newManagedLocalCATestCertificate(t, 3, true)
	fingerprint := fingerprintSHA256(cert)
	store := newFakeManagedCertificateStore()
	store.certificates[fingerprint] = cert

	if err := reconcileManagedTrust(context.Background(), certFile, cert, store); err != nil {
		t.Fatalf("reconcileManagedTrust() error = %v", err)
	}
	inventory, _, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil {
		t.Fatalf("loadManagedTrustInventory() error = %v", err)
	}
	if len(inventory.OwnedSHA256) != 0 {
		t.Fatalf("una CA preexistente quedó atribuida al producto: %#v", inventory)
	}
	if err := removeManagedTrust(context.Background(), certFile, store); err != nil {
		t.Fatalf("removeManagedTrust() error = %v", err)
	}
	if _, exists := store.certificates[fingerprint]; !exists {
		t.Fatal("se borró una CA preexistente/compartida")
	}
	if len(store.removeCalls) != 0 {
		t.Fatalf("se intentaron eliminaciones no autorizadas: %v", store.removeCalls)
	}
}

func TestManagedTrust_RotacionRetiraLaHuellaPropiaAnterior(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	oldCert := newManagedLocalCATestCertificate(t, 4, true)
	newCert := newManagedLocalCATestCertificate(t, 5, true)
	oldFingerprint := fingerprintSHA256(oldCert)
	newFingerprint := fingerprintSHA256(newCert)
	store := newFakeManagedCertificateStore()

	if err := reconcileManagedTrust(context.Background(), certFile, oldCert, store); err != nil {
		t.Fatalf("primera reconciliación: %v", err)
	}
	if err := reconcileManagedTrust(context.Background(), certFile, newCert, store); err != nil {
		t.Fatalf("rotación: %v", err)
	}
	if _, exists := store.certificates[oldFingerprint]; exists {
		t.Fatal("la rotación acumuló la CA propia anterior")
	}
	if _, exists := store.certificates[newFingerprint]; !exists {
		t.Fatal("la CA nueva no quedó instalada")
	}
	inventory, _, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil {
		t.Fatalf("loadManagedTrustInventory() error = %v", err)
	}
	if len(inventory.OwnedSHA256) != 1 ||
		inventory.OwnedSHA256[0] != newFingerprint ||
		inventory.CurrentSHA256 != newFingerprint {
		t.Fatalf("inventario tras rotación inesperado: %#v", inventory)
	}
}

func TestManagedTrust_ReconciliacionesConcurrentesMantienenInventario(t *testing.T) {
	t.Parallel()
	certFile := filepath.Join(t.TempDir(), "root.crt.pem")
	first := newManagedLocalCATestCertificate(t, 111, true)
	second := newManagedLocalCATestCertificate(t, 112, true)
	store := newFakeManagedCertificateStore()
	var wait sync.WaitGroup
	var results [2]error
	for index, cert := range []*x509.Certificate{first, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = reconcileManagedTrust(context.Background(), certFile, cert, store)
		}()
	}
	wait.Wait()
	for _, err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	inventory, _, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.certificates) != 1 || len(inventory.OwnedSHA256) != 1 ||
		inventory.OwnedSHA256[0] != inventory.CurrentSHA256 {
		t.Fatalf("reconciliación concurrente perdió propiedad: %#v, almacén=%d", inventory, len(store.certificates))
	}
}

func TestManagedTrust_RotacionHaciaCompartidaNoSeAtribuyeLaNueva(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	oldCert := newManagedLocalCATestCertificate(t, 9, true)
	sharedCert := newManagedLocalCATestCertificate(t, 10, true)
	oldFingerprint := fingerprintSHA256(oldCert)
	sharedFingerprint := fingerprintSHA256(sharedCert)
	store := newFakeManagedCertificateStore()

	if err := reconcileManagedTrust(context.Background(), certFile, oldCert, store); err != nil {
		t.Fatalf("primera reconciliación: %v", err)
	}
	store.certificates[sharedFingerprint] = sharedCert
	if err := reconcileManagedTrust(context.Background(), certFile, sharedCert, store); err != nil {
		t.Fatalf("rotación hacia CA compartida: %v", err)
	}
	if _, exists := store.certificates[oldFingerprint]; exists {
		t.Fatal("la rotación acumuló la CA propia anterior")
	}
	if _, exists := store.certificates[sharedFingerprint]; !exists {
		t.Fatal("la CA compartida desapareció durante la rotación")
	}
	inventory, _, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil {
		t.Fatalf("loadManagedTrustInventory() error = %v", err)
	}
	if len(inventory.OwnedSHA256) != 0 || inventory.CurrentSHA256 != sharedFingerprint {
		t.Fatalf("la CA compartida quedó atribuida al producto: %#v", inventory)
	}

	removeCallsBeforeUninstall := len(store.removeCalls)
	if err := removeManagedTrust(context.Background(), certFile, store); err != nil {
		t.Fatalf("removeManagedTrust() error = %v", err)
	}
	if _, exists := store.certificates[sharedFingerprint]; !exists {
		t.Fatal("la desinstalación borró la CA compartida")
	}
	if len(store.removeCalls) != removeCallsBeforeUninstall {
		t.Fatalf("la desinstalación intentó borrar una CA compartida: %v", store.removeCalls)
	}
}

func TestManagedTrust_FalloAlRetirarConservaHuellaParaReintento(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	cert := newManagedLocalCATestCertificate(t, 6, true)
	fingerprint := fingerprintSHA256(cert)
	store := newFakeManagedCertificateStore()

	if err := reconcileManagedTrust(context.Background(), certFile, cert, store); err != nil {
		t.Fatalf("reconcileManagedTrust() error = %v", err)
	}
	store.removeErrors[fingerprint] = errors.New("store ocupado")
	if err := removeManagedTrust(context.Background(), certFile, store); err == nil {
		t.Fatal("removeManagedTrust() error = nil, want error")
	}
	inventory, exists, err := loadManagedTrustInventory(managedTrustInventoryPath(certFile))
	if err != nil || !exists {
		t.Fatalf("inventario de reintento exists=%v error=%v", exists, err)
	}
	if len(inventory.OwnedSHA256) != 1 || inventory.OwnedSHA256[0] != fingerprint {
		t.Fatalf("se perdió la huella pendiente: %#v", inventory)
	}
}

func TestManagedTrust_FalloDeProteccionConservaInventarioParaReintento(t *testing.T) {
	t.Parallel()

	inventoryPath := filepath.Join(t.TempDir(), "trust.grxfirma-trust.json")
	fingerprint := strings.Repeat("a", 64)
	inventory := newManagedTrustInventory()
	inventory.CurrentSHA256 = fingerprint
	inventory.OwnedSHA256 = []string{fingerprint}

	err := writeManagedTrustInventoryWithProtection(
		inventoryPath,
		inventory,
		func(string, os.FileMode) error {
			return errors.New("ACL ocupada")
		},
	)
	if err == nil {
		t.Fatal("writeManagedTrustInventoryWithProtection() error = nil, want error")
	}
	if info, statErr := os.Lstat(inventoryPath); statErr != nil || !info.Mode().IsRegular() {
		t.Fatalf("el fallo de ACL eliminó el inventario: info=%v error=%v", info, statErr)
	}

	recovered, exists, loadErr := loadManagedTrustInventory(inventoryPath)
	if loadErr != nil || !exists {
		t.Fatalf("el inventario no se pudo recuperar: exists=%v error=%v", exists, loadErr)
	}
	if len(recovered.OwnedSHA256) != 1 || recovered.OwnedSHA256[0] != fingerprint {
		t.Fatalf("se perdió la prueba de propiedad: %#v", recovered)
	}
}

func TestManagedTrust_InventarioManipuladoNoAutorizaBorrados(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certFile := filepath.Join(dir, "websocket-localhost-root.crt.pem")
	inventoryPath := managedTrustInventoryPath(certFile)
	if err := os.WriteFile(
		inventoryPath,
		[]byte(`{"version":1,"owner":"otro","scope":"CurrentUser","store":"ROOT","current_sha256":"`+
			strings.Repeat("0", 64)+`","owned_sha256":[]}`),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := newFakeManagedCertificateStore()
	if err := removeManagedTrust(context.Background(), certFile, store); err == nil {
		t.Fatal("removeManagedTrust() aceptó un inventario manipulado")
	}
	if len(store.removeCalls) != 0 {
		t.Fatalf("un inventario inválido autorizó borrados: %v", store.removeCalls)
	}
}

func TestManagedLocalCAMarker_DistingueCAsAnteriores(t *testing.T) {
	t.Parallel()

	if IsManagedLocalCA(newManagedLocalCATestCertificate(t, 7, false)) {
		t.Fatal("una CA sin marcador se atribuyó a GrxFirma")
	}
	if !IsManagedLocalCA(newManagedLocalCATestCertificate(t, 8, true)) {
		t.Fatal("la CA marcada no fue reconocida")
	}
}

func newManagedLocalCATestCertificate(
	t *testing.T,
	serial int64,
	marked bool,
) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:   nicknameLocalhostRoot,
			Organization: []string{"Diputacion de Granada"},
			Country:      []string{"ES"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	if marked {
		MarkManagedLocalCA(template)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return cert
}
