// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certpicker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
)

type peticionEditorSello struct {
	DocumentPath string `json:"documentPath"`
	ResultPath   string `json:"resultPath"`
	SignerName   string `json:"signerName"`
}

var resolverEjecutableEditorSello = executableEditorSello

func EjecutarEditorSello(ctx context.Context, documento domain.Document, nombreCertificado string) ([]byte, error) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return nil, ErrEditorSelloNoDisponible
	}
	if len(documento.Content) == 0 || len(documento.Content) > 100*1024*1024 {
		return nil, ErrEditorSelloNoDisponible
	}
	exe, err := resolverEjecutableEditorSello()
	if err != nil {
		return nil, ErrEditorSelloNoDisponible
	}
	dir, err := os.MkdirTemp("", "grxfirma-sello-")
	if err != nil {
		return nil, errors.New(tp("portal.seal.error.prepare"))
	}
	defer os.RemoveAll(dir)
	if err := securefile.ProtectDirectory(dir, 0700); err != nil {
		return nil, errors.New(tp("portal.seal.error.prepare"))
	}
	pdfPath := filepath.Join(dir, "document.pdf")
	if err := os.WriteFile(pdfPath, documento.Content, 0600); err != nil {
		return nil, errors.New(tp("portal.seal.error.prepare"))
	}
	if err := securefile.ProtectFile(pdfPath, 0600); err != nil {
		return nil, errors.New(tp("portal.seal.error.prepare"))
	}
	requestPath := filepath.Join(dir, "request.json")
	resultPath := filepath.Join(dir, "result.json")
	name := strings.TrimSpace(nombreCertificado)
	if len(name) > 256 {
		name = string([]rune(name)[:min(len([]rune(name)), 256)])
	}
	request, err := json.Marshal(peticionEditorSello{DocumentPath: pdfPath, ResultPath: resultPath, SignerName: name})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(requestPath, request, 0600); err != nil {
		return nil, errors.New(tp("portal.seal.error.prepare"))
	}
	if err := securefile.ProtectFile(requestPath, 0600); err != nil {
		return nil, errors.New(tp("portal.seal.error.prepare"))
	}
	limited, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	// #nosec G204 -- solo se ejecuta un frontend instalado y localizado por candidatos fijos.
	cmd := exec.CommandContext(limited, exe, "--portal-seal-request", requestPath)
	prepararGrupoEditorSello(cmd)
	cmd.Env = entornoEditorSello(os.Environ())
	if err := cmd.Run(); err != nil {
		if limited.Err() != nil {
			terminarGrupoEditorSello(cmd)
			return nil, limited.Err()
		}
		var salida *exec.ExitError
		if !errors.As(err, &salida) {
			return nil, ErrEditorSelloNoDisponible
		}
		// El editor llegó a ejecutarse y terminó con error. Si antes dejó una
		// decisión completa y válida, se respeta: volver al diálogo de
		// posiciones fijas descartaría el sitio que eligió la persona.
		raw, err := securefile.ReadFileLimit(resultPath, MaxResultadoEditorSello)
		if err != nil || !resultadoEditorSelloAceptable(raw, documento) {
			return nil, ErrEditorSelloNoDisponible
		}
		slog.Warn("portal_seal_editor_exit_error_result_used", "exit_code", salida.ExitCode())
		return raw, nil
	}
	raw, err := securefile.ReadFileLimit(resultPath, MaxResultadoEditorSello)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrEditorSelloNoDisponible
	}
	if err != nil {
		return nil, errors.New(tp("portal.seal.error.result"))
	}
	return raw, nil
}

// resultadoEditorSelloAceptable comprueba con las mismas reglas que el
// consumidor que el resultado de un editor terminado con error es un JSON
// completo y válido: colocar, firmar sin sello o cancelar. Un fichero
// truncado o con colocaciones imposibles no se aprovecha.
func resultadoEditorSelloAceptable(raw []byte, documento domain.Document) bool {
	paginas, err := paginasPDFEditor(documento)
	if err != nil {
		return false
	}
	_, _, err = validarResultadoEditorSello(raw, paginas)
	return err == nil || errors.Is(err, ErrSeleccionCancelada) || errors.Is(err, ErrSinSelloVisible)
}

func executableEditorSello() (string, error) {
	bin, err := os.Executable()
	if err != nil {
		return "", err
	}
	base := filepath.Dir(bin)
	var paths []string
	if runtime.GOOS == "windows" {
		paths = []string{filepath.Join(base, "grxfirma-winui.exe"), filepath.Join(base, "desktop-winui", "app", "grxfirma-winui.exe")}
		localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if filepath.IsAbs(localAppData) {
			paths = append(paths, filepath.Join(localAppData, "Programs", "GrxFirma", "DesktopWinUI", "grxfirma-winui.exe"))
		}
	} else {
		paths = []string{filepath.Join(base, "grxfirma-gui-qml"), "/usr/bin/grxfirma-gui-qml"}
	}
	for _, path := range paths {
		// #nosec G703 -- candidatos locales fijos bajo el ejecutable o la instalación del usuario; nunca provienen de la web.
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", ErrEditorSelloNoDisponible
}

func entornoEditorSello(base []string) []string {
	permitidas := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
		"LANG": true, "LANGUAGE": true, "DISPLAY": true, "WAYLAND_DISPLAY": true,
		"XAUTHORITY": true, "DBUS_SESSION_BUS_ADDRESS": true,
		"LD_LIBRARY_PATH": true, "TMPDIR": true, "TMP": true, "TEMP": true,
		"SYSTEMROOT": true, "WINDIR": true, "LOCALAPPDATA": true,
		"APPDATA": true, "USERPROFILE": true, "GRXFIRMA_QT_RUNTIME_DIR": true,
	}
	out := make([]string, 0, len(base))
	for _, item := range base {
		name, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") ||
			strings.Contains(upper, "TOKEN") || strings.Contains(upper, "CREDENTIAL") ||
			strings.Contains(upper, "PRIVATE_KEY") || strings.Contains(upper, "ACCESS_KEY") {
			continue
		}
		if permitidas[upper] || strings.HasPrefix(upper, "LC_") || strings.HasPrefix(upper, "XDG_") || strings.HasPrefix(upper, "QT_") || strings.HasPrefix(upper, "QML_") {
			out = append(out, item)
		}
	}
	if runtime.GOOS == "linux" {
		runtimeDir := strings.TrimSpace(os.Getenv("GRXFIRMA_QT_RUNTIME_DIR"))
		if filepath.IsAbs(runtimeDir) {
			for _, entry := range []struct{ key, dir string }{
				{"LD_LIBRARY_PATH", "lib"}, {"QT_PLUGIN_PATH", "plugins"}, {"QML2_IMPORT_PATH", "qml"},
			} {
				prefix := entry.key + "="
				value := filepath.Join(runtimeDir, entry.dir)
				found := false
				for i, item := range out {
					if strings.HasPrefix(item, prefix) {
						out[i] = prefix + value + ":" + strings.TrimPrefix(item, prefix)
						found = true
						break
					}
				}
				if !found {
					out = append(out, prefix+value)
				}
			}
		}
	}
	return out
}
