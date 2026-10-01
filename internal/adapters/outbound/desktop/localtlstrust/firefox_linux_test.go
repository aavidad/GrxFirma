// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package localtlstrust

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func setupFirefoxCertutil(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "certutil"), []byte(firefoxMockCertutil()), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestManagedFirefoxTrust_PerfilesYRetiradaSoloPropia(t *testing.T) {
	home := t.TempDir()
	setupFirefoxCertutil(t, home)
	roots := []string{
		filepath.Join(home, ".mozilla", "firefox"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
		filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
	}
	profiles := make([]string, 0, len(roots))
	for index, root := range roots {
		profiles = append(profiles, createFirefoxTestProfile(t, root, []string{"normal.default", "snap.default", "flatpak.default"}[index]))
	}
	// Un perfil Firefox recién creado puede figurar en profiles.ini antes
	// de que exista su cert9.db.
	if err := os.Remove(filepath.Join(profiles[2], "cert9.db")); err != nil {
		t.Fatal(err)
	}
	cert := newManagedLocalCATestCertificate(t, 901, true)
	certFile := createFirefoxTestCertFile(t, home, cert)
	alias := managedNSSNickname(fingerprintSHA256(cert))
	unrelated := filepath.Join(profiles[0], "unrelated.pem")
	if err := os.WriteFile(unrelated, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureManagedTrustedWithResult(context.Background(), certFile)
	if err != nil || !changed {
		t.Fatalf("primera instalación changed=%v err=%v", changed, err)
	}
	for _, profile := range profiles {
		if _, err := os.Stat(filepath.Join(profile, alias+".pem")); err != nil {
			t.Fatalf("CA ausente en %s: %v", profile, err)
		}
		requireFirefoxTestFileContent(t, filepath.Join(profile, "cert-trust"), "C,,")
		requireFirefoxTestFileContent(t, filepath.Join(profile, "cert-added-count"), "1")
	}
	if _, err := os.Stat(filepath.Join(profiles[2], "cert9.db")); err != nil {
		t.Fatalf("no se inicializó NSS de Flatpak: %v", err)
	}
	changed, err = EnsureManagedTrustedWithResult(context.Background(), certFile)
	if err != nil || changed {
		t.Fatalf("instalación repetida changed=%v err=%v", changed, err)
	}
	newProfile := addFirefoxTestProfile(t, roots[0], "new.default")
	changed, err = EnsureManagedTrustedWithResult(context.Background(), certFile)
	if err != nil || !changed {
		t.Fatalf("perfil nuevo changed=%v err=%v", changed, err)
	}
	profiles = append(profiles, newProfile)
	if err := RemoveManagedTrusted(context.Background(), certFile); err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		if _, err := os.Stat(filepath.Join(profile, alias+".pem")); !os.IsNotExist(err) {
			t.Fatalf("CA propia permanece en %s: %v", profile, err)
		}
	}
	requireFirefoxTestFileContent(t, unrelated, "preserve")
}

func TestManagedFirefoxTrust_PerfilOcupadoSeReintenta(t *testing.T) {
	home := t.TempDir()
	setupFirefoxCertutil(t, home)
	profile := createFirefoxTestProfile(t, filepath.Join(home, ".mozilla", "firefox"), "busy.default")
	busy := filepath.Join(profile, "busy")
	if err := os.WriteFile(busy, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cert := newManagedLocalCATestCertificate(t, 902, true)
	certFile := createFirefoxTestCertFile(t, home, cert)
	if err := EnsureManagedTrusted(context.Background(), certFile); err == nil {
		t.Fatal("se ocultó perfil ocupado")
	}
	if err := os.Remove(busy); err != nil {
		t.Fatal(err)
	}
	if err := EnsureManagedTrusted(context.Background(), certFile); err != nil {
		t.Fatalf("reintento: %v", err)
	}
	requireFirefoxTestFileContent(t, filepath.Join(profile, "cert-added-count"), "1")
}

func TestManagedFirefoxTrust_CACompartidaAmpliaSeConserva(t *testing.T) {
	home := t.TempDir()
	setupFirefoxCertutil(t, home)
	profile := createFirefoxTestProfile(t, filepath.Join(home, ".mozilla", "firefox"), "shared.default")
	cert := newManagedLocalCATestCertificate(t, 903, true)
	certFile := createFirefoxTestCertFile(t, home, cert)
	legacy := filepath.Join(profile, nicknameLocalhostRoot+".pem")
	if err := os.WriteFile(legacy, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "cert-trust"), []byte("CT,C,C\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureManagedTrusted(context.Background(), certFile); err == nil {
		t.Fatal("se aceptó una CA Firefox previa con confianza amplia")
	}
	requireFirefoxTestFileContent(t, filepath.Join(profile, "cert-trust"), "CT,C,C")
	if err := RemoveManagedTrusted(context.Background(), certFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("se borró una CA compartida: %v", err)
	}
}

func TestManagedChromeTrust_LegacyReduceConfianzaSinAtribuirla(t *testing.T) {
	home := t.TempDir()
	setupFirefoxCertutil(t, home)
	chrome := filepath.Join(home, ".pki", "nssdb")
	if err := os.MkdirAll(chrome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chrome, "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cert := newManagedLocalCATestCertificate(t, 904, true)
	certFile := createFirefoxTestCertFile(t, home, cert)
	legacy := filepath.Join(chrome, nicknameLocalhostRoot+".pem")
	if err := os.WriteFile(legacy, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chrome, "cert-trust"), []byte("CT,C,C\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureManagedTrustedWithResult(context.Background(), certFile)
	if err != nil || !changed {
		t.Fatalf("reducción de confianza: changed=%v err=%v", changed, err)
	}
	requireFirefoxTestFileContent(t, filepath.Join(chrome, "cert-trust"), "C,,")
	if _, err := os.Stat(filepath.Join(chrome, managedNSSNickname(fingerprintSHA256(cert))+".pem")); !os.IsNotExist(err) {
		t.Fatalf("se atribuyó una CA previa: %v", err)
	}
	if err := RemoveManagedTrusted(context.Background(), certFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("se borró una CA legacy: %v", err)
	}
}

func createFirefoxTestCertFile(t *testing.T, home string, cert *x509.Certificate) string {
	t.Helper()
	tlsDir := filepath.Join(home, "tls")
	if err := os.Mkdir(tlsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(tlsDir, "root.pem")
	if err := os.WriteFile(certFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile
}

func TestDiscoverFirefoxProfiles_NormalSnapFlatpak(t *testing.T) {
	home := t.TempDir()
	profilenames := []struct {
		root string
		name string
	}{
		{filepath.Join(home, ".mozilla", "firefox"), "normal.default-release"},
		{filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"), "snap.default"},
		{filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"), "flatpak.default"},
	}
	want := make([]string, 0, len(profilenames))
	for _, tc := range profilenames {
		want = append(want, createFirefoxTestProfile(t, tc.root, tc.name))
	}
	slices.Sort(want)

	got, err := discoverFirefoxProfiles(home)
	if err != nil {
		t.Fatalf("discoverFirefoxProfiles() error = %v", err)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("perfiles = %q; se esperaban %q", got, want)
	}
}

func TestDiscoverFirefoxProfiles_NoSigueEnlacesSimbolicos(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".mozilla", "firefox")
	legitimate := createFirefoxTestProfile(t, root, "real.default")
	outside := filepath.Join(t.TempDir(), "other-user-profile")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.default")); err != nil {
		t.Fatal(err)
	}
	linkedDB := filepath.Join(root, "linked-db.default")
	if err := os.MkdirAll(linkedDB, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "cert9.db"), filepath.Join(linkedDB, "cert9.db")); err != nil {
		t.Fatal(err)
	}
	escapedProfile := filepath.Join(filepath.Dir(root), "outside.default")
	if err := os.MkdirAll(escapedProfile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(escapedProfile, "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	snapRoot := filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox")
	if err := os.MkdirAll(snapRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "profiles.ini"), filepath.Join(snapRoot, "profiles.ini")); err != nil {
		t.Fatal(err)
	}
	ini := "[Profile0]\nIsRelative=1\nPath=real.default\n" +
		"[Profile1]\nIsRelative=1\nPath=linked.default\n" +
		"[Profile2]\nIsRelative=1\nPath=linked-db.default\n" +
		"[Profile3]\nIsRelative=1\nPath=../outside.default\n"
	if err := os.WriteFile(filepath.Join(root, "profiles.ini"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := discoverFirefoxProfiles(home)
	if err == nil {
		t.Fatal("discoverFirefoxProfiles() ocultó perfiles declarados inseguros")
	}
	if len(got) != 1 || got[0] != legitimate {
		t.Fatalf("perfiles con enlaces o salida del directorio aceptados: %q", got)
	}
}

func createFirefoxTestProfile(t *testing.T, root, name string) string {
	t.Helper()
	profile := filepath.Join(root, name)
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ini := "[Profile0]\nName=default\nIsRelative=1\nPath=" + name + "\nDefault=1\n"
	if err := os.WriteFile(filepath.Join(root, "profiles.ini"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	return profile
}

func addFirefoxTestProfile(t *testing.T, root, name string) string {
	t.Helper()
	profile := filepath.Join(root, name)
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(root, "profiles.ini")
	data, err := os.ReadFile(ini)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\n[Profile1]\nName=new\nIsRelative=1\nPath="+name+"\n")...)
	if err := os.WriteFile(ini, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return profile
}

func requireFirefoxTestFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer %s: %v", path, err)
	}
	if strings.TrimSpace(string(content)) != want {
		t.Fatalf("contenido de %s = %q; se esperaba %q", path, strings.TrimSpace(string(content)), want)
	}
}

func firefoxMockCertutil() string {
	return `#!/bin/sh
set -eu
mode=""
db=""
nick=""
input=""
trust=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -A|-D|-L|-N|-M) mode="$1"; shift ;;
    -d) db="$2"; shift 2 ;;
    -n) nick="$2"; shift 2 ;;
    -i) input="$2"; shift 2 ;;
    -t) trust="$2"; shift 2 ;;
    -a|--empty-password) shift ;;
    -f) shift 2 ;;
    *) shift ;;
  esac
done
case "$db" in
  sql:*) db="${db#sql:}" ;;
esac
[ -d "$db" ] || exit 1
certfile="$db/$nick.pem"
case "$mode" in
  -L)
    if [ -z "$nick" ]; then
      printf 'Certificate Nickname Trust Attributes\n'
      if [ -f "$db/GrxFirma Local Root CA.pem" ]; then
        flags=',,'
        if [ -f "$db/cert-trust" ]; then flags="$(cat "$db/cert-trust")"; fi
        printf 'GrxFirma Local Root CA %s\n' "$flags"
      fi
      exit 0
    fi
    [ -f "$certfile" ] || exit 255
    cat "$certfile"
    ;;
  -N)
    : > "$db/cert9.db"
    ;;
  -A)
    if [ -f "$db/busy" ]; then
      echo 'SEC_ERROR_BUSY: Firefox mantiene abierta la base NSS' >&2
      exit 1
    fi
    cp "$input" "$certfile"
    printf '%s\n' "$trust" > "$db/cert-trust"
    if [ -f "$db/cert-added-count" ]; then
      value="$(cat "$db/cert-added-count")"
      expr "$value" + 1 > "$db/cert-added-count"
    else
      printf '1\n' > "$db/cert-added-count"
    fi
    ;;
  -D)
    rm -f "$certfile"
    ;;
  -M)
    printf '%s\n' "$trust" > "$db/cert-trust"
    if [ -f "$db/cert-modified-count" ]; then
      value="$(cat "$db/cert-modified-count")"
      expr "$value" + 1 > "$db/cert-modified-count"
    else
      printf '1\n' > "$db/cert-modified-count"
    fi
    ;;
  *) exit 1 ;;
esac
`
}
