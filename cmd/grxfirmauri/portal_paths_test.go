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
	"grxfirma/internal/security/portalpaths"
)

func TestPortalNoPuedeLeerNiGuardarEnDatosDeLaAplicacion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	roots := []string{appdirs.Config(home), appdirs.Data(home), appdirs.State(home), appdirs.Cache(home)}
	if runtime.GOOS == "windows" {
		roots = append(roots, filepath.Join(home, "AppData", "Roaming", "OtraApp"))
	} else {
		roots = append(roots, filepath.Join(home, ".config", "otra-app"), filepath.Join(home, ".local", "share", "otra-app"), filepath.Join(home, ".cache", "otra-app"))
	}
	for _, root := range roots {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "settings.json")
		if err := os.WriteFile(target, []byte("secreto-config"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := buildSafeLegacyTargetPath(url.Values{"filePath": {target}}, "settings.json", ".json"); err == nil {
			t.Errorf("se aceptó como propuesta del portal: %q", target)
		}
		if _, ok := resolveUserChosenWritablePath(target); ok {
			t.Errorf("se aceptó tras el diálogo: %q", target)
		}
		if _, ok := resolveLegacyReadablePath(target); ok {
			t.Errorf("el portal puede leer datos de aplicación: %q", target)
		}
		if _, ok := resolveUserChosenReadablePath(target); ok {
			t.Errorf("la selección de lectura permite datos de aplicación: %q", target)
		}
	}
}

func TestPortalNoPuedeUsarAliasSimbolicoHaciaConfiguracion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	protected := appdirs.Config(home)
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protected, "settings.json"), []byte("secreto"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "alias-documentos")
	if err := os.Symlink(protected, alias); err != nil {
		t.Skipf("no se puede crear enlace simbólico: %v", err)
	}
	target := filepath.Join(alias, "settings.json")
	if _, err := buildSafeLegacyTargetPath(url.Values{"filePath": {target}}, "settings.json", ".json"); err == nil {
		t.Fatal("el portal propuso alias hacia config.json")
	}
	if _, ok := resolveUserChosenWritablePath(target); ok {
		t.Fatal("el diálogo aceptó alias hacia config.json")
	}
	if _, ok := resolveLegacyReadablePath(target); ok {
		t.Fatal("el portal leyó mediante alias")
	}
	if _, ok := resolveUserChosenReadablePath(target); ok {
		t.Fatal("la selección leyó mediante alias")
	}
}

func TestPortalSaveRevalidaDestinoDespuesDelDialogo(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	protected := appdirs.Config(home)
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(protected, "settings.json")
	h := &legacyWebSocketHandler{savePicker: func(context.Context, string, string) (string, error) { return target, nil }}
	result := h.handleSave(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionSave, LegacyParams: url.Values{
		"dat":      {base64.StdEncoding.EncodeToString([]byte(`{"excepciones_inseguras":{"aprobacion_automatica":true}}`))},
		"filename": {"firma.json"},
	}})
	if result == "SAVE_OK" {
		t.Fatalf("el portal escribió settings.json tras el diálogo: %q", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("settings.json fue creado o el estado no es comprobable: %v", err)
	}
}

func TestPortalSignAndSaveNoPublicaEnConfiguracion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	protected := appdirs.Config(home)
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(protected, "firma.pdf")
	h, request := newSignAndSaveFlowTest(t, func(context.Context, string, string) (string, error) { return target, nil })
	result, err := h.handleSignAndSave(context.Background(), request)
	if err != nil || result == "SAVE_OK" {
		t.Fatalf("signandsave pudo publicar en configuración: %q, %v", result, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("la firma apareció en configuración: %v", err)
	}
}

func TestPortalLoadNoEntregaConfiguracion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	protected := appdirs.Config(home)
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(protected, "settings.json")
	if err := os.WriteFile(target, []byte("secreto-config"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := (&legacyWebSocketHandler{}).handleLoad(context.Background(), afirmauri.Solicitud{Operacion: afirmauri.OperacionLoad, LegacyParams: url.Values{"filePath": {target}}})
	if !strings.HasPrefix(result, "SAF_25:") || strings.Contains(result, base64.StdEncoding.EncodeToString([]byte("secreto-config"))) {
		t.Fatalf("el portal recibió datos de configuración: %q", result)
	}
}

func TestPortalRevalidaSiUnDirectorioCambiaTrasSeleccion(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	protected := appdirs.Config(home)
	if err := os.MkdirAll(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(protected, "settings.json")
	if err := os.WriteFile(secret, []byte("config-privada"), 0o600); err != nil {
		t.Fatal(err)
	}
	safeDir := filepath.Join(home, "Descargas", "elegido")
	if err := os.MkdirAll(safeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(safeDir, "settings.json")
	if _, ok := resolveUserChosenWritablePath(selected); !ok {
		t.Fatal("la ruta segura elegida fue rechazada antes del cambio")
	}
	readPath := filepath.Join(safeDir, "documento.txt")
	if err := os.WriteFile(readPath, []byte("dato-seguro"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedRead, ok := resolveLegacyReadablePath(readPath)
	if !ok {
		t.Fatal("la lectura segura fue rechazada antes del cambio")
	}
	if err := os.Rename(safeDir, safeDir+"-anterior"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(protected, safeDir); err != nil {
		t.Skipf("no se puede crear enlace simbólico: %v", err)
	}
	if _, err := writeLegacySavedFile(selected, []byte("ataque"), true, ""); err == nil {
		t.Fatal("el guardado siguió un enlace introducido tras la selección")
	}
	if data, err := os.ReadFile(secret); err != nil || string(data) != "config-privada" {
		t.Fatalf("el fichero privado cambió: %q, %v", data, err)
	}
	if data, err := portalpaths.ReadFileLimit(selected, 1024); err == nil || len(data) != 0 {
		t.Fatalf("la lectura siguió el enlace introducido tras la selección: %q, %v", data, err)
	}
	if data, err := portalpaths.ReadFileLimit(resolvedRead, 1024); err == nil || len(data) != 0 {
		t.Fatalf("la lectura previa se desvió tras cambiar el directorio: %q, %v", data, err)
	}
}

func TestPortalNoUsaTemporalComoHomeSiFaltaHome(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("el sistema aún proporciona una carpeta personal fiable")
	}
	path := filepath.Join(t.TempDir(), "documento.txt")
	if _, ok := resolveUserChosenWritablePath(path); ok {
		t.Fatal("se aceptó una ruta sin carpeta personal fiable")
	}
	if _, err := portalpaths.Check(path); err == nil {
		t.Fatal("la barrera aceptó una ruta sin carpeta personal fiable")
	}
}

func TestPortalRechazaComponentesPadreAntesDeLimpiarRuta(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	machinepolicy.SetForTest(t, machinepolicy.PermitirRutasDirectas, true)
	path := home + string(filepath.Separator) + "Descargas" + string(filepath.Separator) + ".." + string(filepath.Separator) + "Documentos" + string(filepath.Separator) + "firma.pdf"
	if _, err := buildSafeLegacyTargetPath(url.Values{"filePath": {path}}, "firma.pdf", ".pdf"); err == nil {
		t.Fatal("se limpió un componente padre antes de comprobarlo")
	}
	if _, ok := resolveUserChosenWritablePath(path); ok {
		t.Fatal("el diálogo aceptó una ruta con componente padre")
	}
	if _, ok := resolveLegacyReadablePath(path); ok {
		t.Fatal("la lectura aceptó una ruta con componente padre")
	}
}
