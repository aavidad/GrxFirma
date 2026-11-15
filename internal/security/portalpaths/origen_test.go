// SPDX-License-Identifier: EUPL-1.2
package portalpaths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"grxfirma/internal/appdirs"
	"grxfirma/internal/testsupport/perfil"
)

// carpetasDelSistema devuelve carpetas del perfil fuera de las de GrxFirma
// donde la persona guarda o abre ficheros con normalidad: el temporal, la
// caché de adjuntos de Outlook y, en Linux y macOS, ~/.cache y ~/.local.
func carpetasDelSistema(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(home, "AppData", "Local", "Temp"),
			filepath.Join(home, "AppData", "Local", "Microsoft", "Windows", "INetCache", "Content.Outlook", "ABCD1234"),
			filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows"),
		}
	}
	return []string{
		filepath.Join(home, ".cache", "thunderbird"),
		filepath.Join(home, ".local", "share", "Trash"),
		filepath.Join(home, ".config", "otra-app"),
	}
}

func carpetasDeGrxFirma(home string) []string {
	return []string{appdirs.Config(home), appdirs.Data(home), appdirs.State(home), appdirs.Cache(home)}
}

// La web nunca alcanza AppData (ni ~/.config, ~/.local o ~/.cache).
func TestCheckWebBloqueaAppData(t *testing.T) {
	home := perfil.Nuevo(t)
	for _, dir := range carpetasDelSistema(home) {
		_, err := Check(filepath.Join(dir, "doc.pdf"))
		if !errors.Is(err, ErrProtected) {
			t.Errorf("la web alcanzó %s: %v", dir, err)
		}
		if errors.Is(err, ErrCarpetaPropia) {
			t.Errorf("%s no es una carpeta de GrxFirma: %v", dir, err)
		}
	}
	for _, dir := range carpetasDeGrxFirma(home) {
		if _, err := Check(filepath.Join(dir, "doc.pdf")); !errors.Is(err, ErrCarpetaPropia) {
			t.Errorf("la web alcanzó %s o no se explicó: %v", dir, err)
		}
	}
}

// Lo que la persona elige en un diálogo del sistema se permite en el temporal
// o en la caché de Outlook, y se lee y se guarda allí.
func TestCheckElegidaPermiteTempYCacheDeAdjuntos(t *testing.T) {
	home := perfil.Nuevo(t)
	for _, dir := range carpetasDelSistema(home) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		destino := filepath.Join(dir, "adjunto.pdf")
		if _, err := CheckElegida(destino); err != nil {
			t.Fatalf("diálogo hacia %s rechazado: %v", dir, err)
		}
		guardado, err := WriteFileElegida(destino, []byte("firma"), false)
		if err != nil {
			t.Fatalf("guardar en %s: %v", dir, err)
		}
		if data, err := ReadFileLimitElegida(guardado, 16); err != nil || string(data) != "firma" {
			t.Fatalf("leer de %s: %q, %v", dir, data, err)
		}
		if _, err := WriteFile(destino, []byte("web"), true); !errors.Is(err, ErrProtected) {
			t.Fatalf("la web guardó en %s: %v", dir, err)
		}
		if _, err := ReadFileLimit(guardado, 16); !errors.Is(err, ErrProtected) {
			t.Fatalf("la web leyó de %s: %v", dir, err)
		}
	}
}

// Ni siquiera eligiéndolas en un diálogo se usan las carpetas de GrxFirma.
func TestCheckElegidaBloqueaCarpetasDeGrxFirma(t *testing.T) {
	home := perfil.Nuevo(t)
	for _, dir := range carpetasDeGrxFirma(home) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		destino := filepath.Join(dir, "settings.json")
		if _, err := CheckElegida(destino); !errors.Is(err, ErrCarpetaPropia) {
			t.Fatalf("diálogo hacia %s admitido: %v", dir, err)
		}
		if _, err := WriteFileElegida(destino, []byte("ataque"), true); !errors.Is(err, ErrCarpetaPropia) {
			t.Fatalf("se guardó en %s: %v", dir, err)
		}
		if _, err := os.Lstat(destino); !os.IsNotExist(err) {
			t.Fatalf("%s cambió: %v", destino, err)
		}
	}
}
