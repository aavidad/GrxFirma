// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certaccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/ports"
)

const maxImportBytes = 16 * 1024 * 1024

// Adapter conecta el descubrimiento de gestores y almacenes con los casos de
// uso. El directorio local se ofrece solo como gestor; nunca se selecciona
// implícitamente como destino persistente.
type Adapter struct {
	localP12Dir string
}

func New(localP12Dir string) *Adapter {
	return &Adapter{localP12Dir: strings.TrimSpace(localP12Dir)}
}

func (a *Adapter) Options(ctx context.Context) (ports.CertificateAccessOptions, error) {
	if err := ctx.Err(); err != nil {
		return ports.CertificateAccessOptions{}, err
	}
	managers := ListarGestores(a.localP12Dir)
	targets := ListarDestinosImportacion()
	browser := detectedBrowser(os.Getenv("BROWSER"), managers)

	result := ports.CertificateAccessOptions{DetectedBrowser: browser}
	for _, manager := range managers {
		recommended := result.PreferredManager == "" && managerMatchesBrowser(manager.ID, browser)
		if recommended {
			result.PreferredManager = manager.ID
		}
		result.Managers = append(result.Managers, ports.CertificateAccessManager{
			ID: manager.ID, Label: manager.Etiqueta, Recommended: recommended,
		})
	}
	if result.PreferredManager == "" && len(result.Managers) > 0 {
		result.PreferredManager = result.Managers[0].ID
		result.Managers[0].Recommended = true
	}

	for _, target := range targets {
		targetBrowser := browserForTarget(target)
		recommended := result.PreferredTarget == "" && managerMatchesBrowser(targetBrowser, browser)
		if recommended {
			result.PreferredTarget = target.ID
		}
		result.ImportTargets = append(result.ImportTargets, ports.CertificateImportTarget{
			ID: target.ID, Label: target.Etiqueta, Browser: targetBrowser, Recommended: recommended,
		})
	}
	if result.PreferredTarget == "" && len(result.ImportTargets) > 0 {
		result.PreferredTarget = result.ImportTargets[0].ID
		result.ImportTargets[0].Recommended = true
	}
	return result, nil
}

func (a *Adapter) OpenManager(ctx context.Context, managerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	managerID = strings.TrimSpace(managerID)
	if managerID == "" {
		return errors.New("debe seleccionar un gestor de certificados")
	}
	for _, manager := range ListarGestores(a.localP12Dir) {
		if manager.ID != managerID {
			continue
		}
		if len(manager.Comando) == 0 {
			return errors.New("el gestor seleccionado no tiene un comando disponible")
		}
		// Los comandos proceden de la allowlist fija de ListarGestores.
		cmd := exec.Command(manager.Comando[0], manager.Comando[1:]...) // #nosec G204
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("no se pudo abrir %s: %w", manager.Etiqueta, err)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Release()
		}
		return nil
	}
	return errors.New("el gestor de certificados seleccionado no está disponible")
}

func (a *Adapter) Import(ctx context.Context, targetID string, data []byte, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return errors.New("debe seleccionar un almacén de destino")
	}
	if len(data) == 0 {
		return errors.New("el certificado que se va a importar está vacío")
	}
	if len(data) > maxImportBytes {
		return fmt.Errorf("el certificado supera el tamaño máximo de %d MiB", maxImportBytes/(1024*1024))
	}
	allowed := false
	for _, target := range ListarDestinosImportacion() {
		if target.ID == targetID {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("el almacén de destino seleccionado no está disponible")
	}

	working := append([]byte(nil), data...)
	defer secmem.Zeroize(working)
	file, err := os.CreateTemp("", "grxfirma-import-*.p12")
	if err != nil {
		return fmt.Errorf("no se pudo preparar la importación: %w", err)
	}
	path := file.Name()
	defer removeSensitiveFile(path)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("no se pudo proteger el temporal de importación: %w", err)
	}
	if _, err := file.Write(working); err != nil {
		_ = file.Close()
		return fmt.Errorf("no se pudo escribir el temporal de importación: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("no se pudo cerrar el temporal de importación: %w", err)
	}
	return ImportarP12(ctx, targetID, path, password)
}

func detectedBrowser(browserEnv string, managers []Gestor) string {
	if browser := classifyBrowser(browserEnv); browser != "" {
		return browser
	}
	for _, preferred := range []string{"firefox", "chrome", "chromium", "edge", "brave"} {
		for _, manager := range managers {
			if managerMatchesBrowser(manager.ID, preferred) {
				return preferred
			}
		}
	}
	return ""
}

func classifyBrowser(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(value, "firefox"):
		return "firefox"
	case strings.Contains(value, "chromium"):
		return "chromium"
	case strings.Contains(value, "chrome"):
		return "chrome"
	case strings.Contains(value, "edge"):
		return "edge"
	case strings.Contains(value, "brave"):
		return "brave"
	default:
		return ""
	}
}

func managerMatchesBrowser(value, browser string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	browser = strings.ToLower(strings.TrimSpace(browser))
	if value == "" || browser == "" {
		return false
	}
	if browser == "chrome" || browser == "chromium" {
		return strings.Contains(value, "chrome") || strings.Contains(value, "chromium")
	}
	return strings.Contains(value, browser)
}

func browserForTarget(target DestinoImportacion) string {
	label := strings.ToLower(target.Etiqueta)
	switch {
	case strings.Contains(label, "firefox"):
		return "firefox"
	case strings.Contains(label, "chrome"), strings.Contains(label, "chromium"):
		return "chromium"
	case target.ID == "windows-my", target.ID == "mac-login":
		return "system"
	default:
		return ""
	}
}

var _ ports.CertificateAccess = (*Adapter)(nil)
