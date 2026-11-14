// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"path/filepath"
	"testing"
)

// C-07: solo la extensión oficial cuenta como tal para la aprobación
// automática. Las extensiones portafirmas ya no son llamantes de GrxFirma
// (TestNativeCaller_RechazaManifiestoEIDsDePortafirmas); un ID admitido solo por el modo
// de desarrollo sigue autorizado, pero no es oficial.
func TestNativeCallerPolicy_SoloLaExtensionOficialEsOficial(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	// El manifiesto se busca en XDG_CONFIG_HOME si existe (los ejecutores de
	// CI lo definen): se fija al home de la prueba.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	executable := filepath.Join(dir, "bin", "grxfirma-nativehost")
	writeTestFile(t, executable, []byte("host"), 0o700)

	const packageID = "abcdefghijklmnopabcdefghijklmnop"
	writeTestFile(t, filepath.Join(dir, "extensions", "chromium", "grxfirma-extension-chromium.id"), []byte(packageID+"\n"), 0o600)

	const managedID = "ponmlkjihgfedcbaponmlkjihgfedcba"
	writeTestManifest(t, filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts", "io.github.aavidad.grxfirma.json"), nativeHostManifest{
		Name:           "io.github.aavidad.grxfirma",
		Path:           executable,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + managedID + "/"},
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

	const devID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	devPolicy := newNativeCallerPolicy(executable, home, true)
	devCaller := "chrome-extension://" + devID + "/"
	if _, err := nativeCallerFromArgs([]string{devCaller}, devPolicy); err != nil {
		t.Fatalf("en modo de desarrollo %s debe estar autorizado: %v", devCaller, err)
	}
	if devPolicy.isOfficialCaller(devCaller) {
		t.Errorf("%s (solo desarrollo) no debe contar como extensión oficial", devCaller)
	}
	if (nativeCallerPolicy{}).isOfficialCaller("chrome-extension://" + officialChromiumExtensionID + "/") {
		t.Error("una política vacía no reconoce llamantes oficiales")
	}
}
