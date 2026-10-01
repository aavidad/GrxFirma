// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"grxfirma/internal/domain"
)

type desktopMode struct {
	habilitado bool
	frontend   string
}

func extraerModoDesktop(args []string) (desktopMode, []string, error) {
	cfg := desktopMode{
		habilitado: len(args) == 0,
		frontend:   "fyne",
	}
	resto := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch strings.TrimSpace(strings.ToLower(arg)) {
		case "-desktop", "--desktop", "-gui", "--gui", "-modo-gui":
			cfg.habilitado = true
		case "-fyne", "--fyne":
			cfg.habilitado = true
			cfg.frontend = "fyne"
		case "-qt", "--qt":
			cfg.habilitado = true
			cfg.frontend = "qt"
		case "-frontend", "--frontend":
			if i+1 >= len(args) {
				return cfg, resto, fmt.Errorf("falta valor para %s", arg)
			}
			frontend, err := normalizarFrontendDesktop(args[i+1])
			if err != nil {
				return cfg, resto, err
			}
			cfg.habilitado = true
			cfg.frontend = frontend
			i++
		default:
			lower := strings.TrimSpace(strings.ToLower(arg))
			if strings.HasPrefix(lower, "-frontend=") || strings.HasPrefix(lower, "--frontend=") {
				raw, _, _ := strings.Cut(arg, "=")
				_ = raw
				valor := arg[strings.Index(arg, "=")+1:]
				frontend, err := normalizarFrontendDesktop(valor)
				if err != nil {
					return cfg, resto, err
				}
				cfg.habilitado = true
				cfg.frontend = frontend
				continue
			}
			resto = append(resto, arg)
		}
	}
	return cfg, resto, nil
}

func normalizarFrontendDesktop(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "fyne":
		return "fyne", nil
	case "qt":
		return "qt", nil
	default:
		return "", fmt.Errorf("frontend desktop no soportado: %s (use fyne|qt)", strings.TrimSpace(raw))
	}
}

func inferirFormatoDesktop(raw, ruta string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "cades":
		return string(domain.FormatCAdES)
	case "xades":
		return string(domain.FormatXAdES)
	case "pades":
		return string(domain.FormatPAdES)
	case "xmldsig":
		return "XMLdSig"
	case "facturae":
		return "FacturaE"
	case "asic-xades":
		return "ASiC-XAdES"
	case "odf":
		return "ODF"
	case "ooxml":
		return "OOXML"
	}
	switch strings.ToLower(filepath.Ext(ruta)) {
	case ".pdf":
		return string(domain.FormatPAdES)
	case ".asics":
		return "ASiC-XAdES"
	case ".odt", ".ods", ".odp", ".odg", ".odf":
		return "ODF"
	case ".docx", ".xlsx", ".pptx", ".ppsx":
		return "OOXML"
	case ".dsig", ".xmlsig":
		return "XMLdSig"
	case ".xml", ".xsig":
		return string(domain.FormatXAdES)
	default:
		return string(domain.FormatCAdES)
	}
}

//lint:ignore U1000 usado en builds con -tags fyne_gui (desktop_fyne.go)
func inferirTipoMIMEDesktop(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".asics":
		return "application/vnd.etsi.asic-s+zip"
	case ".odt":
		return "application/vnd.oasis.opendocument.text"
	case ".ods":
		return "application/vnd.oasis.opendocument.spreadsheet"
	case ".odp":
		return "application/vnd.oasis.opendocument.presentation"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".dsig", ".xmlsig":
		return "application/xmldsig+xml"
	case ".xml", ".xsig":
		return "application/xml"
	case ".json":
		return "application/json"
	case ".p7s", ".csig":
		return "application/pkcs7-signature"
	default:
		return "application/octet-stream"
	}
}

func construirSalidaDesktop(rutaEntrada string, formato domain.SignatureFormat) string {
	base := strings.TrimSuffix(rutaEntrada, filepath.Ext(rutaEntrada))
	switch formato {
	case domain.FormatPAdES:
		return base + "_firmado.pdf"
	case domain.FormatXAdES:
		return base + ".xsig"
	case domain.SignatureFormat("XMLdSig"):
		return base + ".dsig"
	case domain.SignatureFormat("FacturaE"):
		return base + "_firmada.xml"
	case domain.SignatureFormat("ASiC-XAdES"):
		return base + ".asics"
	default:
		return base + ".csig"
	}
}

func inferirOriginalRelacionado(rutaFirma string) string {
	rutaFirma = strings.TrimSpace(rutaFirma)
	if rutaFirma == "" {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(rutaFirma))
	switch ext {
	case ".pdf":
		return ""
	case ".csig", ".xsig":
	default:
		return ""
	}

	dir := filepath.Dir(rutaFirma)
	baseFirma := strings.TrimSuffix(filepath.Base(rutaFirma), ext)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		nombre := entry.Name()
		if nombre == filepath.Base(rutaFirma) {
			continue
		}
		if strings.TrimSuffix(nombre, filepath.Ext(nombre)) != baseFirma {
			continue
		}
		switch strings.ToLower(filepath.Ext(nombre)) {
		case ".csig", ".xsig":
			continue
		default:
			return filepath.Join(dir, nombre)
		}
	}

	return ""
}

func nombreDestinoP12Desktop(ref domain.CertificateRef) string {
	fp := strings.ToLower(strings.TrimSpace(ref.Fingerprint))
	if fp == "" {
		return "certificado-importado.p12"
	}
	return "certificado-" + fp + ".p12"
}

//lint:ignore U1000 usado en builds con -tags fyne_gui (desktop_fyne.go)
type gestorCertificadosDesktop struct {
	etiqueta string
	comando  []string
}

//lint:ignore U1000 usado en builds con -tags fyne_gui (desktop_fyne.go)
func gestoresCertificadosDesktop(directorioP12 string) []gestorCertificadosDesktop {
	var gestores []gestorCertificadosDesktop

	agregarSiExiste := func(etiqueta string, comando ...string) {
		if len(comando) == 0 {
			return
		}
		if _, err := exec.LookPath(comando[0]); err != nil {
			return
		}
		gestores = append(gestores, gestorCertificadosDesktop{
			etiqueta: etiqueta,
			comando:  append([]string(nil), comando...),
		})
	}

	if runtime.GOOS == "linux" {
		agregarSiExiste("Firefox", "firefox", "about:preferences#privacy")
		agregarSiExiste("Firefox ESR", "firefox-esr", "about:preferences#privacy")
		agregarSiExiste("Chrome", "google-chrome", "chrome://settings/certificates")
		agregarSiExiste("Chrome Stable", "google-chrome-stable", "chrome://settings/certificates")
		agregarSiExiste("Chromium", "chromium", "chrome://settings/certificates")
		agregarSiExiste("Chromium Browser", "chromium-browser", "chrome://settings/certificates")
		agregarSiExiste("Microsoft Edge", "microsoft-edge", "edge://settings/certificates")
	}

	if strings.TrimSpace(directorioP12) != "" {
		agregarSiExiste("Carpeta P12 de GrxFirma", "xdg-open", directorioP12)
	}

	if runtime.GOOS == "linux" && len(gestores) > 0 {
		slices.SortFunc(gestores, func(a, b gestorCertificadosDesktop) int {
			return strings.Compare(a.etiqueta, b.etiqueta)
		})
	}
	return gestores
}

func maybeRunQtDesktopApp(stderr io.Writer, rutaP12, password string) (bool, int) {
	rutaBackend, err := resolverBackendQtDesktop()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error arrancando frontend Qt: %v\n", err)
		return true, 1
	}

	cmd := prepararComandoBackendQt(rutaBackend, rutaP12, password)
	cmd.Stdout = os.Stdout
	cmd.Stderr = stderr
	cmd.Env = entornoQt(filtrarSecretosEntornoHijo(os.Environ()))
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "error ejecutando backend Qt (%s): %v\n", rutaBackend, err)
		return true, 1
	}
	return true, 0
}

func prepararComandoBackendQt(rutaBackend, rutaP12, password string) *exec.Cmd {
	args := []string{"--p12-password-stdin"}
	if strings.TrimSpace(rutaP12) != "" {
		args = append(args, "--p12", rutaP12)
	}
	// #nosec G204,G702 -- rutaBackend se resuelve desde candidatos fijos. La
	// contraseña viaja por un pipe anónimo creado por os/exec, nunca en argv
	// ni en el entorno heredable.
	cmd := exec.Command(rutaBackend, args...)
	cmd.Stdin = strings.NewReader(password + "\n")
	return cmd
}

func resolverBackendQtDesktop() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	candidatos := qtBackendCandidates(exeDir)

	for _, candidato := range candidatos {
		if strings.ContainsRune(candidato, filepath.Separator) {
			if st, err := os.Stat(candidato); err == nil && !st.IsDir() {
				return candidato, nil
			}
			continue
		}
		if ruta, err := exec.LookPath(candidato); err == nil {
			return ruta, nil
		}
	}

	return "", fmt.Errorf("backend Qt no encontrado (buscado grxfirma-gui)")
}

func qtBackendCandidates(baseDir string) []string {
	candidatos := executableCandidates(baseDir, "grxfirma-gui")
	candidatos = append(candidatos, "grxfirma-gui")
	return candidatos
}

func executableCandidates(baseDir, baseName string) []string {
	candidatos := []string{filepath.Join(baseDir, baseName)}
	if runtime.GOOS == "windows" {
		candidatos = append(candidatos, filepath.Join(baseDir, baseName+".exe"))
	}
	return candidatos
}

func filtrarSecretosEntornoHijo(base []string) []string {
	bloqueadas := map[string]struct{}{
		"GRXFIRMA_PKCS12_PASSWORD":       {},
		"GRXFIRMA_REST_TOKEN":            {},
		"GRXFIRMA_PROTECTION_SECRET_B64": {},
	}
	out := make([]string, 0, len(base))
	for _, item := range base {
		name, _, found := strings.Cut(item, "=")
		if found {
			if _, blocked := bloqueadas[strings.ToUpper(name)]; blocked {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func entornoQt(base []string) []string {
	if runtime.GOOS != "linux" {
		return base
	}
	runtimeDir := strings.TrimSpace(os.Getenv("GRXFIRMA_QT_RUNTIME_DIR"))
	if runtimeDir == "" {
		return base
	}

	libDir := filepath.Join(runtimeDir, "lib")
	pluginDir := filepath.Join(runtimeDir, "plugins")
	qmlDir := filepath.Join(runtimeDir, "qml")

	env := append([]string(nil), base...)
	encontroLD := false
	for i, item := range env {
		if strings.HasPrefix(item, "LD_LIBRARY_PATH=") {
			env[i] = "LD_LIBRARY_PATH=" + libDir + ":" + strings.TrimPrefix(item, "LD_LIBRARY_PATH=")
			encontroLD = true
			break
		}
	}
	if !encontroLD {
		env = append(env, "LD_LIBRARY_PATH="+libDir)
	}
	env = append(env, "QT_PLUGIN_PATH="+pluginDir, "QML2_IMPORT_PATH="+qmlDir)
	return env
}
