// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
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
	manifest := filepath.Join(dir, "com.dipgra.grxfirma.json")
	writeTestManifest(t, manifest, nativeHostManifest{
		Name:              "com.dipgra.grxfirma",
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
	manifest := filepath.Join(dir, "com.dipgra.grxfirma.json")
	writeTestManifest(t, manifest, nativeHostManifest{
		Name:              "com.dipgra.grxfirma",
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
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	executable := filepath.Join(dir, "bin", "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)

	const packageID = "abcdefghijklmnopabcdefghijklmnop"
	idPath := filepath.Join(dir, "extensions", "chromium", "dipgra-extension-chromium.id")
	writeTestFile(t, idPath, []byte(packageID+"\n"), 0o600)

	const managedID = "ponmlkjihgfedcbaponmlkjihgfedcba"
	manifestPath := filepath.Join(
		home, ".config", "google-chrome", "NativeMessagingHosts",
		"com.dipgra.grxfirma.json",
	)
	writeTestManifest(t, manifestPath, nativeHostManifest{
		Name:           "com.dipgra.grxfirma",
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
		"com.dipgra.grxfirma.json",
	)
	writeTestManifest(t, otherManifest, nativeHostManifest{
		Name:           "com.dipgra.grxfirma",
		Path:           filepath.Join(dir, "otro-host"),
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/"},
	})
	policy = newNativeCallerPolicy(executable, home, false)
	if _, ok := policy.chromiumIDs["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]; ok {
		t.Fatal("un manifiesto de otro binario no debe ampliar la allowlist")
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
