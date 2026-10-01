// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	minimumOpenGLMajor          = 2
	minimumOpenGLMinor          = 1
	graphicsUnavailableExitCode = 3

	graphicsFailureTitleID       = "GrxFirma — interfaz gráfica no disponible"
	graphicsUnavailableMessageID = "No se puede abrir la interfaz gráfica de GrxFirma en esta sesión."
	graphicsRemediationMessageID = "Comprueba que la sesión gráfica esté activa. Si el diagnóstico indica OpenGL, instala o activa el controlador gráfico del fabricante, o usa una sesión con aceleración 3D. La operación no se ha firmado."
	graphicsDiagnosticMessageID  = "Diagnóstico: %s"
)

var (
	openGLVersionPrefix = regexp.MustCompile(`^\s*(\d+)\.(\d+)`)

	checkPlatformGraphicsReadiness = platformGraphicsReadiness
	showPlatformGraphicsFailure    = platformGraphicsFailureDialog
	activateNativeProtocolFallback = enableNativeProtocolUIFallback
)

type graphicsCapability struct {
	Version  string
	Renderer string
}

type framebufferCapability struct {
	DrawToWindow   bool
	SupportsOpenGL bool
	RGBA           bool
	DoubleBuffered bool
	SoftwareOnly   bool
}

type graphicsReadinessError struct {
	reason     string
	capability graphicsCapability
}

func (e *graphicsReadinessError) Error() string {
	if e == nil {
		return ""
	}
	detail := strings.TrimSpace(e.reason)
	if version := sanitizeGraphicsDiagnostic(e.capability.Version); version != "" {
		detail += fmt.Sprintf(" (GL_VERSION %s", version)
		if renderer := sanitizeGraphicsDiagnostic(e.capability.Renderer); renderer != "" {
			detail += "; renderer " + renderer
		}
		detail += ")"
	}
	if detail == "" {
		return "interfaz gráfica no disponible"
	}
	return detail
}

func validateGraphicsCapability(capability graphicsCapability) error {
	major, minor, ok := parseOpenGLVersion(capability.Version)
	if !ok {
		return &graphicsReadinessError{
			reason:     "no se pudo determinar una versión OpenGL compatible",
			capability: capability,
		}
	}
	if major < minimumOpenGLMajor ||
		(major == minimumOpenGLMajor && minor < minimumOpenGLMinor) {
		return &graphicsReadinessError{
			reason: fmt.Sprintf(
				"el controlador gráfico ofrece OpenGL %d.%d; GrxFirma necesita OpenGL %d.%d o posterior",
				major,
				minor,
				minimumOpenGLMajor,
				minimumOpenGLMinor,
			),
			capability: capability,
		}
	}
	return nil
}

func validateFramebufferCapability(capability framebufferCapability) error {
	switch {
	case !capability.DrawToWindow:
		return errors.New("el formato gráfico no permite dibujar en una ventana")
	case !capability.SupportsOpenGL:
		return errors.New("el formato gráfico no admite OpenGL")
	case !capability.RGBA:
		return errors.New("el formato gráfico no admite color RGBA")
	case !capability.DoubleBuffered:
		return errors.New("el formato gráfico no admite doble búfer")
	case capability.SoftwareOnly:
		// GLFW descarta de forma explícita PFD_GENERIC_FORMAT cuando no
		// incluye PFD_GENERIC_ACCELERATED. Repetir ese criterio aquí evita
		// aceptar el rasterizador GDI que Fyne rechazará al crear la ventana.
		return errors.New("el formato OpenGL disponible es el rasterizador software GDI que GLFW no puede utilizar")
	default:
		return nil
	}
}

func parseOpenGLVersion(raw string) (major, minor int, ok bool) {
	match := openGLVersionPrefix.FindStringSubmatch(raw)
	if len(match) != 3 {
		return 0, 0, false
	}
	major, errMajor := strconv.Atoi(match[1])
	minor, errMinor := strconv.Atoi(match[2])
	if errMajor != nil || errMinor != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func sanitizeGraphicsDiagnostic(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var clean strings.Builder
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			clean.WriteRune(' ')
			continue
		}
		clean.WriteRune(r)
		if clean.Len() >= 160 {
			break
		}
	}
	return strings.Join(strings.Fields(clean.String()), " ")
}

func requireInteractiveGraphics(stderr io.Writer) error {
	err := checkPlatformGraphicsReadiness()
	if err == nil {
		return nil
	}
	return reportInteractiveGraphicsFailure(stderr, err)
}

func prepareProtocolInteractiveUI(stderr io.Writer) error {
	disableNativeProtocolUIFallback()
	if nativeProtocolUIForced() && activateNativeProtocolFallback(nil) {
		if stderr != nil {
			_, _ = fmt.Fprintln(
				stderr,
				"warning: usando la interfaz nativa segura de Windows por configuración explícita",
			)
		}
		return nil
	}

	err := checkPlatformGraphicsReadiness()
	if err == nil {
		return nil
	}
	if activateNativeProtocolFallback(err) {
		if stderr != nil {
			_, _ = fmt.Fprintf(
				stderr,
				"warning: OpenGL no está disponible; se usará la interfaz nativa segura de Windows: %s\n",
				sanitizeGraphicsDiagnostic(err.Error()),
			)
		}
		return nil
	}
	return reportInteractiveGraphicsFailure(stderr, err)
}

func reportInteractiveGraphicsFailure(stderr io.Writer, err error) error {
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "error: interfaz gráfica no disponible: %v\n", err)
	}
	message := graphicsFailureUserMessage(err)
	if dialogErr := showPlatformGraphicsFailure(message); dialogErr != nil && stderr != nil {
		_, _ = fmt.Fprintf(stderr, "warning: no se pudo mostrar el aviso gráfico nativo: %v\n", dialogErr)
	}
	return err
}

func graphicsFailureUserMessage(err error) string {
	base := tl(graphicsUnavailableMessageID)
	action := tl(graphicsRemediationMessageID)
	detail := ""
	if err != nil {
		detail = sanitizeGraphicsDiagnostic(err.Error())
	}
	if detail == "" {
		return base + "\n\n" + action
	}
	return base + "\n\n" + action + "\n\n" + tl(graphicsDiagnosticMessageID, detail)
}

//lint:ignore U1000 usado en builds Windows con -tags fyne_gui (graphics_readiness_windows.go)
func wrapGraphicsReadinessError(reason string, err error) error {
	reason = strings.TrimSpace(reason)
	if err == nil {
		return &graphicsReadinessError{reason: reason}
	}
	var readinessErr *graphicsReadinessError
	if errors.As(err, &readinessErr) {
		return err
	}
	return &graphicsReadinessError{reason: reason + ": " + sanitizeGraphicsDiagnostic(err.Error())}
}
