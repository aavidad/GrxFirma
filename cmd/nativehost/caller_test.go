// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/common/logging"
)

func testCallerPolicy(executable string) nativeCallerPolicy {
	return nativeCallerPolicy{
		chromiumIDs: map[string]struct{}{
			officialChromiumExtensionID: {},
		},
		firefoxIDs: map[string]struct{}{
			officialFirefoxExtensionID: {},
		},
		executable: executable,
	}
}

func TestNativeCallerFromArgs_ChromiumOficial(t *testing.T) {
	got, err := nativeCallerFromArgs(
		[]string{"chrome-extension://" + officialChromiumExtensionID + "/"},
		testCallerPolicy(""),
	)
	if err != nil {
		t.Fatalf("nativeCallerFromArgs() error = %v", err)
	}
	if got != "chrome-extension://"+officialChromiumExtensionID+"/" {
		t.Fatalf("caller inesperado: %q", got)
	}
}

func TestNativeCallerFromArgs_ChromiumDesarrolloRequiereOptIn(t *testing.T) {
	const origin = "chrome-extension://abcdefghijklmnopabcdefghijklmnop/"
	policy := testCallerPolicy("")
	if _, err := nativeCallerFromArgs([]string{origin}, policy); err == nil {
		t.Fatal("un ID Chromium de desarrollo no debe admitirse por defecto")
	}
	policy.allowDevelopment = true
	if got, err := nativeCallerFromArgs([]string{origin}, policy); err != nil || got != origin {
		t.Fatalf("caller de desarrollo = %q, %v", got, err)
	}
}

func TestNativeCallerFromArgs_FirefoxValidaManifiestoYBinario(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)
	manifest := filepath.Join(dir, "io.github.aavidad.grxfirma.json")
	writeTestManifest(t, manifest, nativeHostManifest{
		Name:              "io.github.aavidad.grxfirma",
		Path:              executable,
		Type:              "stdio",
		AllowedExtensions: []string{officialFirefoxExtensionID},
	})

	got, err := nativeCallerFromArgs(
		[]string{manifest, officialFirefoxExtensionID},
		testCallerPolicy(executable),
	)
	if err != nil {
		t.Fatalf("nativeCallerFromArgs() error = %v", err)
	}
	if got != "firefox-extension-id:"+officialFirefoxExtensionID {
		t.Fatalf("caller inesperado: %q", got)
	}
}

func TestNativeCallerFromArgs_RechazaAusenteMalformadoYOtroFirefox(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)
	manifest := filepath.Join(dir, "io.github.aavidad.grxfirma.json")
	writeTestManifest(t, manifest, nativeHostManifest{
		Name:              "io.github.aavidad.grxfirma",
		Path:              executable,
		Type:              "stdio",
		AllowedExtensions: []string{officialFirefoxExtensionID},
	})
	cases := [][]string{
		nil,
		{""},
		{"https://" + officialChromiumExtensionID + "/"},
		{"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/../"},
		{manifest, "otra-extension@example.invalid"},
		{"manifest-relativo.json", officialFirefoxExtensionID},
		{manifest, officialFirefoxExtensionID, "sobrante"},
	}
	for _, args := range cases {
		if got, err := nativeCallerFromArgs(args, testCallerPolicy(executable)); err == nil {
			t.Errorf("args=%q aceptados como %q", args, got)
		}
	}
}

func TestNewNativeCallerPolicy_CargaSoloIDYManifiestoLigadosAlBinario(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	executable := filepath.Join(dir, "bin", "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)

	const packageID = "abcdefghijklmnopabcdefghijklmnop"
	idPath := filepath.Join(dir, "extensions", "chromium", "grxfirma-extension-chromium.id")
	writeTestFile(t, idPath, []byte(packageID+"\n"), 0o600)

	const managedID = "ponmlkjihgfedcbaponmlkjihgfedcba"
	manifestPath := filepath.Join(
		home, ".config", "google-chrome", "NativeMessagingHosts",
		"io.github.aavidad.grxfirma.json",
	)
	writeTestManifest(t, manifestPath, nativeHostManifest{
		Name:           "io.github.aavidad.grxfirma",
		Path:           executable,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + managedID + "/"},
	})

	policy := newNativeCallerPolicy(executable, home, false)
	for _, id := range []string{officialChromiumExtensionID, packageID, managedID} {
		if _, ok := policy.chromiumIDs[id]; !ok {
			t.Errorf("falta ID Chromium autorizado %q", id)
		}
	}

	otherManifest := filepath.Join(
		home, ".config", "chromium", "NativeMessagingHosts",
		"io.github.aavidad.grxfirma.json",
	)
	writeTestManifest(t, otherManifest, nativeHostManifest{
		Name:           "io.github.aavidad.grxfirma",
		Path:           filepath.Join(dir, "otro-host"),
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/"},
	})
	policy = newNativeCallerPolicy(executable, home, false)
	if _, ok := policy.chromiumIDs["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]; ok {
		t.Fatal("un manifiesto de otro binario no debe ampliar la allowlist")
	}
}

func TestNativeCallerFromArgs_RechazaIdentidadesDelEspacioDeNombresAnterior(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)
	const legacyFirefoxID = "extension@dipgra.es"
	legacyManifest := filepath.Join(dir, "com.dipgra.grxfirma.json")
	writeTestManifest(t, legacyManifest, nativeHostManifest{
		Name:              "com.dipgra.grxfirma",
		Path:              executable,
		Type:              "stdio",
		AllowedExtensions: []string{legacyFirefoxID},
	})
	if _, ok := readNativeHostManifest(legacyManifest); ok {
		t.Fatal("el manifiesto con el nombre de host anterior no debe aceptarse")
	}
	if got, err := nativeCallerFromArgs(
		[]string{legacyManifest, legacyFirefoxID},
		newNativeCallerPolicy(executable, filepath.Join(dir, "home"), false),
	); err == nil {
		t.Fatalf("la extension Firefox anterior se acepto como %q", got)
	}
}

func TestReadNativeHostManifest_RechazaEnlaceYTamanoExcesivo(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	writeTestManifest(t, manifestPath, nativeHostManifest{
		Name: "io.github.aavidad.grxfirma", Path: filepath.Join(dir, "host"), Type: "stdio",
	})
	if _, ok := readNativeHostManifest(manifestPath); !ok {
		t.Fatal("un manifiesto regular y acotado debe ser aceptado")
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(manifestPath, link); err == nil {
		if _, ok := readNativeHostManifest(link); ok {
			t.Fatal("un enlace simbolico no debe ser aceptado")
		}
	}
	writeTestFile(t, manifestPath, bytes.Repeat([]byte("x"), maxNativeManifestBytes+1), 0o600)
	if _, ok := readNativeHostManifest(manifestPath); ok {
		t.Fatal("un manifiesto demasiado grande no debe ser aceptado")
	}
}

func TestNativeCallerDevelopmentAllowed_ProhibidoEnProduccion(t *testing.T) {
	t.Setenv("GRXFIRMA_NATIVEHOST_ALLOW_DEVELOPMENT_CALLER", "1")
	if !logging.DebugAllowed() && nativeCallerDevelopmentAllowed() {
		t.Fatal("production nunca debe aceptar callers de desarrollo")
	}
}

func writeTestManifest(t *testing.T, path string, manifest nativeHostManifest) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("codificar manifiesto: %v", err)
	}
	writeTestFile(t, path, raw, 0o600)
}

func writeTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("crear directorio: %v", err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("escribir %s: %v", path, err)
	}
}

// El ID del ejemplo público de Native Messaging de Chrome lo puede reproducir
// cualquiera con la clave publicada; nunca debe estar autorizado.
func TestBuiltinChromiumIDsExcludePublicExampleID(t *testing.T) {
	for _, id := range builtinChromiumExtensionIDs {
		if id == "knldjmfmopnpolahpmmgbagdohdnhkik" {
			t.Fatalf("el ID de ejemplo público de Chrome no puede estar autorizado")
		}
	}
}
