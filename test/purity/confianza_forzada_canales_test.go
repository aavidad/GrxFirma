// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// TestConfianzaForzadaSoloEnCanalesLocales (lote 14) impide que un canal que
// atiende a páginas web o al móvil cree, cambie, lea o aplique lo que el
// usuario aceptó en su equipo al verificar. Solo la CLI y el backend del
// escritorio (IPC con el frontend que lo lanzó) componen el almacén, la
// gestión y la verificación con confianza forzada.
package purity_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfianzaForzadaSoloEnCanalesLocales(t *testing.T) {
	raiz := raizModulo(t)

	// Canales web y móvil: no pueden ni nombrar la confianza forzada.
	canalesWeb := []string{
		filepath.Join("cmd", "nativehost"),
		filepath.Join("cmd", "grxfirmauri"),
		"mobilebind",
		filepath.Join("internal", "adapters", "inbound", "common", "rest"),
		filepath.Join("internal", "adapters", "inbound", "legacy"),
	}
	prohibidosWeb := []string{"ConfianzaForzada", "confianza_forzada", "forced_trust", "RegistroConfianzaForzada"}
	for _, dir := range canalesWeb {
		recorrerGoSinPruebas(t, filepath.Join(raiz, dir), func(ruta, texto string) {
			for _, patron := range prohibidosWeb {
				if strings.Contains(texto, patron) {
					t.Errorf("%s: un canal web o móvil no puede usar la confianza forzada (%q)", relParaTest(raiz, ruta), patron)
				}
			}
		})
	}
	// El servidor REST de cmd/grxfirma vive junto a la CLI: sus ficheros
	// tampoco pueden componerla.
	restCLI, _ := filepath.Glob(filepath.Join(raiz, "cmd", "grxfirma", "rest*.go"))
	for _, ruta := range restCLI {
		if strings.HasSuffix(ruta, "_test.go") {
			continue
		}
		data, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		for _, patron := range prohibidosWeb {
			if strings.Contains(string(data), patron) {
				t.Errorf("%s: el servidor REST no puede usar la confianza forzada (%q)", relParaTest(raiz, ruta), patron)
			}
		}
	}

	// Composición: solo la CLI y el backend del escritorio.
	composicion := map[string][]string{
		"ConConfianzaForzada(": {
			filepath.Join("cmd", "grxfirma", "bootstrap.go"),
			filepath.Join("cmd", "grxfirma-gui", "main.go"),
			filepath.Join("internal", "application", "verify.go"),
		},
		"NuevoAlmacenConfianzaForzada": {
			filepath.Join("cmd", "grxfirma", "bootstrap.go"),
			filepath.Join("cmd", "grxfirma-gui", "main.go"),
			filepath.Join("internal", "adapters", "outbound", "common", "config", "confianza_forzada.go"),
		},
		"NuevoGestionConfianzaForzadaUseCase(": {
			filepath.Join("cmd", "grxfirma", "bootstrap.go"),
			filepath.Join("cmd", "grxfirma-gui", "main.go"),
			filepath.Join("internal", "application", "confianza_forzada.go"),
		},
		"ConRegistroConfianzaForzada(": {
			filepath.Join("internal", "domain", "confianza_forzada.go"),
			filepath.Join("internal", "application", "verify.go"),
			filepath.Join("internal", "application", "confianza_forzada.go"),
			filepath.Join("internal", "adapters", "outbound", "common", "signer", "verifactu.go"),
			filepath.Join("internal", "adapters", "inbound", "common", "cli", "confianza_forzada.go"),
			filepath.Join("internal", "adapters", "inbound", "desktop", "ipc", "confianza_forzada.go"),
		},
	}
	for _, dir := range []string{"cmd", "internal", "mobilebind", "presentation", "scripts"} {
		recorrerGoSinPruebas(t, filepath.Join(raiz, dir), func(ruta, texto string) {
			rel := relParaTest(raiz, ruta)
			for patron, permitidos := range composicion {
				if !strings.Contains(texto, patron) {
					continue
				}
				permitido := false
				for _, p := range permitidos {
					if rel == p {
						permitido = true
						break
					}
				}
				if !permitido {
					t.Errorf("%s: %q solo se permite en la CLI y en el escritorio (%v)", rel, patron, permitidos)
				}
			}
		})
	}
}

func recorrerGoSinPruebas(t *testing.T, dir string, visitar func(ruta, texto string)) {
	t.Helper()
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visitar(path, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("recorriendo %s: %v", dir, err)
	}
}
