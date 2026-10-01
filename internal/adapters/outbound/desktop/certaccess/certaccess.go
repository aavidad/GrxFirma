// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certaccess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type Gestor struct {
	ID       string
	Etiqueta string
	Comando  []string
}

type DestinoImportacion struct {
	ID       string
	Etiqueta string
	Ruta     string
}

func ListarGestores(directorioP12 string) []Gestor {
	var gestores []Gestor

	agregarPath := func(id, etiqueta string, comando ...string) {
		if len(comando) == 0 {
			return
		}
		if _, err := exec.LookPath(comando[0]); err != nil {
			return
		}
		gestores = append(gestores, Gestor{
			ID:       id,
			Etiqueta: etiqueta,
			Comando:  append([]string(nil), comando...),
		})
	}

	switch runtime.GOOS {
	case "windows":
		agregarPath("windows-certmgr", "Gestor de certificados de Windows", "cmd", "/c", "start", "", "certmgr.msc")
		agregarPath("chrome", "Chrome", "cmd", "/c", "start", "", "chrome://settings/certificates")
		agregarPath("chromium", "Chromium", "cmd", "/c", "start", "", "chrome://settings/certificates")
		agregarPath("edge", "Microsoft Edge", "cmd", "/c", "start", "", "edge://settings/certificates")
		agregarPath("brave", "Brave", "cmd", "/c", "start", "", "brave://settings/certificates")
		agregarPath("firefox", "Firefox", "cmd", "/c", "start", "", "about:preferences#privacy")
	case "darwin":
		agregarPath("keychain", "Acceso a Llaveros", "open", "-a", "Keychain Access")
		agregarPath("chrome", "Google Chrome", "open", "-a", "Google Chrome", "chrome://settings/certificates")
		agregarPath("edge", "Microsoft Edge", "open", "-a", "Microsoft Edge", "edge://settings/certificates")
		agregarPath("brave", "Brave", "open", "-a", "Brave Browser", "brave://settings/certificates")
		agregarPath("firefox", "Firefox", "open", "-a", "Firefox", "about:preferences#privacy")
	default:
		agregarPath("seahorse", "Gestor del sistema (Seahorse)", "seahorse")
		agregarPath("kleopatra", "Gestor del sistema (Kleopatra)", "kleopatra")
		agregarPath("gcr-viewer", "Visor de certificados GCR", "gcr-viewer")
		agregarPath("firefox", "Firefox", "firefox", "about:preferences#privacy")
		agregarPath("firefox-esr", "Firefox ESR", "firefox-esr", "about:preferences#privacy")
		agregarPath("chrome", "Chrome", "google-chrome", "chrome://settings/certificates")
		agregarPath("chrome-stable", "Chrome Stable", "google-chrome-stable", "chrome://settings/certificates")
		agregarPath("chromium", "Chromium", "chromium", "chrome://settings/certificates")
		agregarPath("chromium-browser", "Chromium Browser", "chromium-browser", "chrome://settings/certificates")
		agregarPath("edge", "Microsoft Edge", "microsoft-edge", "edge://settings/certificates")
		agregarPath("brave", "Brave", "brave-browser", "brave://settings/certificates")
	}

	if strings.TrimSpace(directorioP12) != "" {
		switch runtime.GOOS {
		case "windows":
			agregarPath("p12-dir", "Carpeta P12 de GrxFirma", "explorer.exe", directorioP12)
		case "darwin":
			agregarPath("p12-dir", "Carpeta P12 de GrxFirma", "open", directorioP12)
		default:
			agregarPath("p12-dir", "Carpeta P12 de GrxFirma", "xdg-open", directorioP12)
		}
	}

	slices.SortFunc(gestores, func(a, b Gestor) int {
		return strings.Compare(a.Etiqueta, b.Etiqueta)
	})
	return gestores
}

func ListarDestinosImportacion() []DestinoImportacion {
	var destinos []DestinoImportacion
	switch runtime.GOOS {
	case "windows":
		destinos = append(destinos, DestinoImportacion{
			ID:       "windows-my",
			Etiqueta: "Almacén personal de Windows",
		})
	case "darwin":
		destinos = append(destinos, DestinoImportacion{
			ID:       "mac-login",
			Etiqueta: "Llavero de inicio de sesión (macOS)",
		})
	default:
		if home, err := os.UserHomeDir(); err == nil {
			chromeDB := filepath.Join(home, ".pki", "nssdb")
			if nssDBDisponible(chromeDB) {
				destinos = append(destinos, DestinoImportacion{
					ID:       "nss:" + chromeDB,
					Etiqueta: "Chrome / Chromium (almacén NSS del usuario)",
					Ruta:     chromeDB,
				})
			}
		}
	}
	// Firefox mantiene perfiles NSS propios también en Windows y macOS. Se
	// ofrecen por separado del almacén del sistema para no instalar el P12 en
	// un destino distinto al elegido explícitamente por el usuario.
	for _, profile := range descubrirPerfilesFirefox() {
		destinos = append(destinos, DestinoImportacion{
			ID:       "nss:" + profile,
			Etiqueta: "Firefox (" + filepath.Base(profile) + ")",
			Ruta:     profile,
		})
	}

	slices.SortFunc(destinos, func(a, b DestinoImportacion) int {
		return strings.Compare(a.Etiqueta, b.Etiqueta)
	})
	return destinos
}

func ImportarP12(ctx context.Context, destinoID, rutaP12, password string) error {
	switch {
	case strings.HasPrefix(destinoID, "nss:"):
		return importarP12ANSS(ctx, strings.TrimPrefix(destinoID, "nss:"), rutaP12, password)
	case destinoID == "windows-my":
		return importarP12AWindows(ctx, rutaP12, password)
	case destinoID == "mac-login":
		return importarP12AMac(ctx, rutaP12, password)
	default:
		return fmt.Errorf("destino de importación no soportado: %s", destinoID)
	}
}

func nssDBDisponible(ruta string) bool {
	if strings.TrimSpace(ruta) == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(ruta, "cert9.db")); err == nil { // #nosec G703 -- read-only probe of an explicitly discovered user profile.
		return true
	}
	if _, err := os.Stat(filepath.Join(ruta, "cert8.db")); err == nil { // #nosec G703 -- read-only probe of an explicitly discovered user profile.
		return true
	}
	return false
}

// raicesPerfilesFirefox devuelve los directorios donde puede haber perfiles de
// Firefox. En Linux hay que mirar tambien Snap y Flatpak: cada empaquetado
// aisla su propio ~/.mozilla. nssstore ya contemplaba las tres, y certaccess
// solo la nativa, asi que con Firefox de Flatpak o Snap los certificados se
// listaban pero su perfil no aparecia como destino de importacion.
func raicesPerfilesFirefox() []string {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if strings.TrimSpace(appdata) == "" {
			return nil
		}
		return []string{filepath.Join(appdata, "Mozilla", "Firefox", "Profiles")}
	case "darwin":
		home := os.Getenv("HOME")
		if strings.TrimSpace(home) == "" {
			return nil
		}
		return []string{filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles")}
	default:
		home := os.Getenv("HOME")
		if strings.TrimSpace(home) == "" {
			return nil
		}
		return []string{
			filepath.Join(home, ".mozilla", "firefox"),
			filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
			filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
		}
	}
}

func descubrirPerfilesFirefox() []string {
	var profiles []string
	vistos := map[string]struct{}{}
	for _, base := range raicesPerfilesFirefox() {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			ruta := filepath.Join(base, entry.Name())
			if !nssDBDisponible(ruta) {
				continue
			}
			if _, repetido := vistos[ruta]; repetido {
				continue
			}
			vistos[ruta] = struct{}{}
			profiles = append(profiles, ruta)
		}
	}
	slices.Sort(profiles)
	return profiles
}

func importarP12ANSS(ctx context.Context, dbPath, rutaP12, password string) error {
	pk12utilPath, err := exec.LookPath("pk12util")
	if err != nil {
		return fmt.Errorf("pk12util no disponible: %w", err)
	}
	pwFile, err := os.CreateTemp("", "grxfirma-pw-*")
	if err != nil {
		return fmt.Errorf("crear fichero temporal de contraseña: %w", err)
	}
	pwPath := pwFile.Name()
	defer removeSensitiveFile(pwPath)
	if err := os.Chmod(pwPath, 0o600); err != nil {
		_ = pwFile.Close()
		return fmt.Errorf("proteger fichero temporal de contraseña: %w", err)
	}
	if _, err := pwFile.WriteString(password); err != nil {
		_ = pwFile.Close()
		return fmt.Errorf("escribir contraseña temporal: %w", err)
	}
	_ = pwFile.Close()

	intentar := func(base string) error {
		// #nosec G204 -- fixed pk12util executable resolved above; paths and
		// password-file reference are passed as distinct arguments.
		cmd := exec.CommandContext(ctx, pk12utilPath, "-i", rutaP12, "-d", base, "-w", pwPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("pk12util: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := intentar("sql:" + dbPath); err == nil {
		return nil
	}
	return intentar(dbPath)
}
