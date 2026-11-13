// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// C-07: solo la extensión oficial cuenta como tal para la aprobación
// automática; las portafirmas siguen autorizadas pero no son oficiales.
func TestNativeCallerPolicy_SoloLaExtensionOficialEsOficial(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	executable := filepath.Join(dir, "bin", "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)

	const packageID = "abcdefghijklmnopabcdefghijklmnop"
	writeTestFile(t, filepath.Join(dir, "extensions", "chromium", "dipgra-extension-chromium.id"), []byte(packageID+"\n"), 0o600)

	const managedID = "ponmlkjihgfedcbaponmlkjihgfedcba"
	writeTestManifest(t, filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts", "com.dipgra.grxfirma.json"), nativeHostManifest{
		Name:           "com.dipgra.grxfirma",
		Path:           executable,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + managedID + "/"},
	})
	const portafirmasExtraID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	writeTestManifest(t, filepath.Join(home, ".config", "chromium", "NativeMessagingHosts", "com.dipgra.portafirmas.json"), nativeHostManifest{
		Name:              "com.dipgra.portafirmas",
		Path:              executable,
		Type:              "stdio",
		AllowedOrigins:    []string{"chrome-extension://" + portafirmasExtraID + "/"},
		AllowedExtensions: []string{"otra-portafirmas@dipgra.es"},
	})

	policy := newNativeCallerPolicy(executable, home, false)
	for _, caller := range []string{
		"chrome-extension://" + officialChromiumExtensionID + "/",
		"chrome-extension://" + packageID + "/",
		"chrome-extension://" + managedID + "/",
		"firefox-extension-id:" + officialFirefoxExtensionID,
	} {
		if !policy.isOfficialCaller(caller) {
			t.Errorf("%s debería contar como extensión oficial", caller)
		}
	}
	for _, caller := range []string{
		"chrome-extension://ipkpimgjhkjibkbhfdhggjldlaetbcoa/",
		"chrome-extension://knldjmfmopnpolahpmmgbagdohdnhkik/",
		"chrome-extension://" + portafirmasExtraID + "/",
		"firefox-extension-id:" + portafirmasFirefoxExtensionID,
		"firefox-extension-id:otra-portafirmas@dipgra.es",
	} {
		if policy.isOfficialCaller(caller) {
			t.Errorf("%s no debe contar como extensión oficial", caller)
		}
		// La lista de llamantes autorizados no cambia (decisión pendiente).
		// «ipkpim…» contiene una «t» y no es un ID de Chromium válido (solo
		// letras a-p): addChromiumID ya lo descartaba antes de este cambio.
		if strings.HasPrefix(caller, "chrome-extension://") && !strings.Contains(caller, "ipkpim") {
			if _, err := nativeCallerFromArgs([]string{caller}, policy); err != nil {
				t.Errorf("%s debe seguir autorizado: %v", caller, err)
			}
		}
	}
	if (nativeCallerPolicy{}).isOfficialCaller("chrome-extension://" + officialChromiumExtensionID + "/") {
		t.Error("una política vacía no reconoce llamantes oficiales")
	}
}
