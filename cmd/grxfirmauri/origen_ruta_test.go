// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/appdirs"
	"grxfirma/internal/security/machinepolicy"
)

// carpetasHabitualesFueraDeDocumentos son carpetas del perfil donde la persona
// abre o guarda ficheros sin ser de GrxFirma: el temporal (lo que se abre
// desde un ZIP) y la caché de adjuntos de Outlook en Windows; ~/.cache y
// ~/.local en Linux y macOS.
func carpetasHabitualesFueraDeDocumentos(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(home, "AppData", "Local", "Temp", "Temp1_adjuntos.zip"),
			filepath.Join(home, "AppData", "Local", "Microsoft", "Windows", "INetCache", "Content.Outlook", "ABCD1234"),
		}
	}
	return []string{filepath.Join(home, ".cache", "thunderbird"), filepath.Join(home, ".local", "share", "otra-app")}
}

func capturarRutaRechazada(t *testing.T) *[]string {
	t.Helper()
	previo := mostrarRutaRechazada
	t.Cleanup(func() { mostrarRutaRechazada = previo })
	var vistos []string
	mostrarRutaRechazada = func(_, titulo, detalle string) { vistos = append(vistos, titulo+"\n"+detalle) }
	return &vistos
}

func guardarConDialogo(destino string, esDialogo bool) string {
	h := &legacyWebSocketHandler{
		savePicker:                  func(context.Context, string, string) (string, error) { return destino, nil },
		savePickerConfirmsOverwrite: esDialogo,
	}
	return h.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion:         afirmauri.OperacionSave,
		OrigenSolicitante: "https://sede.example",
		LegacyParams:      url.Values{"dat": {base64.StdEncoding.EncodeToString([]byte("firma"))}, "filename": {"firma.pdf"}},
	})
}

// Lo que la persona elige en el diálogo del sistema se guarda y se carga en
// %TEMP% o en la caché de Outlook.
func TestDialogoPermiteTempYCacheDeAdjuntos(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	vistos := capturarRutaRechazada(t)
	for _, dir := range carpetasHabitualesFueraDeDocumentos(home) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		destino := filepath.Join(dir, "firma.pdf")
		if resp := guardarConDialogo(destino, true); resp != "SAVE_OK" {
			t.Fatalf("guardar desde el diálogo en %s: %q", dir, resp)
		}
		if data, err := os.ReadFile(destino); err != nil || string(data) != "firma" {
			t.Fatalf("no se guardó en %s: %q, %v", destino, data, err)
		}
		h := &legacyWebSocketHandler{loadPicker: func(context.Context, string, string, bool) ([]string, error) { return []string{destino}, nil }}
		resp := h.handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad, LegacyParams: url.Values{}})
		if resp != "firma.pdf:"+base64.StdEncoding.EncodeToString([]byte("firma")) {
			t.Fatalf("cargar desde el diálogo en %s: %q", dir, resp)
		}
	}
	if len(*vistos) != 0 {
		t.Fatalf("no debía explicarse ningún rechazo: %q", *vistos)
	}
}

// La misma carpeta, si la propone la web (sin diálogo), se rechaza.
func TestWebNoAlcanzaTempNiCacheDeAdjuntos(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("en Linux y macOS ~/.cache y ~/.local ya se comprueban en TestPortalNoPuedeLeerNiGuardarEnDatosDeLaAplicacion")
	}
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	for _, dir := range carpetasHabitualesFueraDeDocumentos(home) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		destino := filepath.Join(dir, "firma.pdf")
		if err := os.WriteFile(destino, []byte("previo"), 0o600); err != nil {
			t.Fatal(err)
		}
		if resp := guardarConDialogo(filepath.Join(dir, "nuevo.pdf"), false); resp == "SAVE_OK" {
			t.Fatalf("sin diálogo se guardó en %s", dir)
		}
		resp := (&legacyWebSocketHandler{}).handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad, LegacyParams: url.Values{"filePath": {destino}}})
		if !strings.HasPrefix(resp, "SAF_25:") {
			t.Fatalf("la web leyó %s: %q", destino, resp)
		}
	}
}

// Ni desde el diálogo se usa la carpeta de configuración de GrxFirma, y la
// persona ve en GrxFirma por qué.
func TestDialogoHaciaConfiguracionSeBloqueaYSeExplica(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	vistos := capturarRutaRechazada(t)
	config := appdirs.Config(home)
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	secreto := filepath.Join(config, "settings.json")
	if err := os.WriteFile(secreto, []byte("privado"), 0o600); err != nil {
		t.Fatal(err)
	}
	if resp := guardarConDialogo(filepath.Join(config, "firma.pdf"), true); resp != "SAF_05: Ruta fuera del ámbito permitido" {
		t.Fatalf("guardar en la configuración: %q", resp)
	}
	if _, err := os.Stat(filepath.Join(config, "firma.pdf")); !os.IsNotExist(err) {
		t.Fatalf("se escribió en la configuración: %v", err)
	}
	h := &legacyWebSocketHandler{loadPicker: func(context.Context, string, string, bool) ([]string, error) { return []string{secreto}, nil }}
	if resp := h.handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad, LegacyParams: url.Values{}}); !strings.HasPrefix(resp, "SAF_25:") {
		t.Fatalf("cargar de la configuración: %q", resp)
	}
	esperado := tl("portalpaths.carpeta_propia.titulo") + "\n" + tl("portalpaths.carpeta_propia.detalle")
	if len(*vistos) != 2 || (*vistos)[0] != esperado || (*vistos)[1] != esperado {
		t.Fatalf("la persona no vio el motivo: %q", *vistos)
	}
	if esperado == "portalpaths.carpeta_propia.titulo\nportalpaths.carpeta_propia.detalle" {
		t.Fatal("faltan los textos en el catálogo")
	}
}
