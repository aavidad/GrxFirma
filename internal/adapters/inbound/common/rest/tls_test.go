// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"crypto/tls"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

func TestEnsureLocalhostCertificateWithLocalCAProtegeYReutilizaClaveRaiz(t *testing.T) {
	dir := t.TempDir()
	const prefix = "prueba"

	certFile, keyFile, rootCertFile, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix)
	if err != nil {
		t.Fatalf("EnsureLocalhostCertificateWithLocalCA() error = %v", err)
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("el par TLS generado no es valido: %v", err)
	}
	if !regularTLSFile(rootCertFile) {
		t.Fatal("no se genero el certificado publico de la CA")
	}
	rootCert, ok := cargarCertificadoLocal(rootCertFile)
	if !ok || !localtlstrust.IsManagedLocalCA(rootCert) {
		t.Fatal("la CA local no contiene el marcador de propiedad requerido")
	}
	rootKeyFile := filepath.Join(dir, prefix+"-root.key.pem")
	if !regularTLSFile(rootKeyFile) {
		t.Fatal("la clave privada de la CA no se guardó")
	}
	assertPrivateKeyPermissions(t, rootKeyFile)
	assertPrivateKeyPermissions(t, keyFile)

	if _, _, _, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix); err != nil {
		t.Fatalf("EnsureLocalhostCertificateWithLocalCA() al reutilizar error = %v", err)
	}
	if !regularTLSFile(rootKeyFile) {
		t.Fatal("la clave CA desapareció tras reutilizar")
	}
}

func TestEnsureBrowserCompatibleLocalhostCertificateNoReutilizaMaterialLegacyFueraDelInventario(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	legacyDir := filepath.Join(home, ".config", "AutofirmaDipgra", "certs")
	legacyCert, legacyKey, legacyRoot, err := EnsureLocalhostCertificateWithLocalCA(
		legacyDir,
		"legacy",
	)
	if err != nil {
		t.Fatalf("preparar material legacy: %v", err)
	}
	for source, target := range map[string]string{
		legacyCert: filepath.Join(legacyDir, "server.crt"),
		legacyKey:  filepath.Join(legacyDir, "server.key"),
		legacyRoot: filepath.Join(legacyDir, "rootCA.crt"),
	} {
		if err := os.Rename(source, target); err != nil {
			t.Fatalf("os.Rename(%s): %v", filepath.Base(source), err)
		}
	}

	managedDir := filepath.Join(t.TempDir(), "tls")
	certFile, keyFile, rootCertFile, source, err :=
		EnsureBrowserCompatibleLocalhostCertificate(managedDir, "websocket-localhost")
	if err != nil {
		t.Fatalf("EnsureBrowserCompatibleLocalhostCertificate() error = %v", err)
	}
	if source != "grxfirma-local-ca" {
		t.Fatalf("origen TLS = %q, want grxfirma-local-ca", source)
	}
	for _, path := range []string{certFile, keyFile, rootCertFile} {
		if filepath.Clean(filepath.Dir(path)) != filepath.Clean(managedDir) {
			t.Fatalf("artefacto fuera del directorio gestionado: %q", path)
		}
	}
	root, ok := cargarCertificadoLocal(rootCertFile)
	if !ok || !localtlstrust.IsManagedLocalCA(root) {
		t.Fatal("la CA elegida no contiene la identidad gestionada de V2")
	}
}

func TestEnsureBrowserCompatibleLocalhostCertificateMigraCARESTPropia(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	legacyCert, legacyKey, legacyRoot, err := EnsureLocalhostCertificateWithLocalCA(dir, legacyRESTPrefix)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "otro.crt.pem")
	if err := os.WriteFile(foreign, []byte("ajeno"), 0o600); err != nil {
		t.Fatal(err)
	}
	certFile, _, rootFile, _, err := EnsureBrowserCompatibleLocalhostCertificate(dir, ManagedLocalhostPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(rootFile) != "websocket-localhost-root.crt.pem" {
		t.Fatalf("CA = %s", rootFile)
	}
	if filepath.Base(certFile) != "websocket-localhost.crt.pem" {
		t.Fatalf("hoja = %s", certFile)
	}
	for _, path := range []string{legacyCert, legacyKey, legacyRoot, filepath.Join(dir, legacyRESTPrefix+"-root.key.pem")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("material antiguo conservado: %s: %v", path, err)
		}
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "ajeno" {
		t.Fatalf("fichero ajeno alterado: %v", err)
	}
}

func TestEnsureBrowserCompatibleLocalhostCertificateNoBorraCARESTAjena(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	foreignRoot := filepath.Join(dir, legacyRESTPrefix+"-root.crt.pem")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignRoot, []byte("ajena"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := EnsureBrowserCompatibleLocalhostCertificate(dir, ManagedLocalhostPrefix); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(foreignRoot); err != nil || string(data) != "ajena" {
		t.Fatalf("CA ajena alterada: %v", err)
	}
}

func TestEnsureBrowserCompatibleLocalhostCertificateRetiraHojaCLIAnterior(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	leafFile, keyFile, _, err := EnsureLocalhostCertificateWithLocalCA(dir, legacyRESTPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(leafFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyFile); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureLocalhostCertificate(dir, legacyRESTPrefix); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := EnsureBrowserCompatibleLocalhostCertificate(dir, ManagedLocalhostPrefix); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{leafFile, keyFile} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("hoja CLI antigua conservada: %s: %v", path, err)
		}
	}
}

func TestEnsureBrowserCompatibleLocalhostCertificateRetiraInventarioLegacyNSS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("inventario NSS específico de Linux")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "grxfirma", "tls")
	_, _, rootFile, err := EnsureLocalhostCertificateWithLocalCA(dir, legacyRESTPrefix)
	if err != nil {
		t.Fatal(err)
	}
	inventory := rootFile + ".grxfirma-nss-trust.json"
	if err := os.WriteFile(inventory, []byte(`{"version":1,"owner":"grxfirma-local-tls-ca","entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := EnsureBrowserCompatibleLocalhostCertificate(dir, ManagedLocalhostPrefix); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(inventory); !os.IsNotExist(err) {
		t.Fatalf("inventario antiguo conservado: %v", err)
	}
	if _, err := os.Lstat(rootFile); !os.IsNotExist(err) {
		t.Fatalf("CA antigua conservada: %v", err)
	}
}

func TestEnsureLocalhostCertificateReemplazaSymlinkDeClaveSinSeguirlo(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile, err := EnsureLocalhostCertificate(dir, "prueba")
	if err != nil {
		t.Fatalf("EnsureLocalhostCertificate() error = %v", err)
	}
	certAnterior, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("os.ReadFile(certFile) error = %v", err)
	}
	keyAnterior, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("os.ReadFile(keyFile) error = %v", err)
	}
	victim := filepath.Join(dir, "victima.pem")
	if err := os.WriteFile(victim, keyAnterior, 0o600); err != nil {
		t.Fatalf("os.WriteFile(victim) error = %v", err)
	}
	if err := os.Remove(keyFile); err != nil {
		t.Fatalf("os.Remove(keyFile) error = %v", err)
	}
	if err := os.Symlink(victim, keyFile); err != nil {
		t.Skipf("el sistema no permite crear symlinks: %v", err)
	}

	if _, _, err := EnsureLocalhostCertificate(dir, "prueba"); err != nil {
		t.Fatalf("EnsureLocalhostCertificate() con symlink error = %v", err)
	}
	info, err := os.Lstat(keyFile)
	if err != nil {
		t.Fatalf("os.Lstat(keyFile) error = %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("la clave regenerada no es un fichero regular: %v", info.Mode())
	}
	victimDespues, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("os.ReadFile(victim) error = %v", err)
	}
	if !bytes.Equal(victimDespues, keyAnterior) {
		t.Fatal("la escritura TLS siguio y modifico el destino del symlink")
	}
	certNuevo, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("os.ReadFile(certFile) posterior error = %v", err)
	}
	if bytes.Equal(certNuevo, certAnterior) {
		t.Fatal("se reutilizo un par TLS cuya clave era un symlink")
	}
	assertPrivateKeyPermissions(t, keyFile)
}

func TestEnsureLocalhostCertificateCorrigePermisosDeClaveInseguros(t *testing.T) {
	dir := t.TempDir()
	_, keyFile, err := EnsureLocalhostCertificate(dir, "prueba")
	if err != nil {
		t.Fatalf("EnsureLocalhostCertificate() error = %v", err)
	}
	if err := os.Chmod(keyFile, 0o644); err != nil {
		t.Fatalf("os.Chmod(keyFile) error = %v", err)
	}
	if _, _, err := EnsureLocalhostCertificate(dir, "prueba"); err != nil {
		t.Fatalf("EnsureLocalhostCertificate() con permisos inseguros error = %v", err)
	}
	assertPrivateKeyPermissions(t, keyFile)
}

func TestEnsureLocalhostCertificateRechazaDirectorioSymlink(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("os.Mkdir(realDir) error = %v", err)
	}
	linkDir := filepath.Join(base, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("el sistema no permite crear symlinks: %v", err)
	}
	if _, _, err := EnsureLocalhostCertificate(linkDir, "prueba"); err == nil {
		t.Fatal("EnsureLocalhostCertificate() acepto un directorio TLS symlink")
	}
}

func assertPrivateKeyPermissions(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat(%s) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permisos de clave = %04o, want 0600", got)
	}
}
