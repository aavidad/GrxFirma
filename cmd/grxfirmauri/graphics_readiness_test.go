// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseOpenGLVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		major      int
		minor      int
		compatible bool
	}{
		{name: "Microsoft Basic Display", raw: "1.1.0", major: 1, minor: 1, compatible: true},
		{name: "Mesa", raw: "4.5 (Compatibility Profile) Mesa 25.2", major: 4, minor: 5, compatible: true},
		{name: "espacios", raw: "  2.1 NVIDIA", major: 2, minor: 1, compatible: true},
		{name: "vacia", raw: "", compatible: false},
		{name: "texto", raw: "OpenGL 4.6", compatible: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			major, minor, ok := parseOpenGLVersion(test.raw)
			if ok != test.compatible || major != test.major || minor != test.minor {
				t.Fatalf(
					"parseOpenGLVersion(%q) = (%d, %d, %v), want (%d, %d, %v)",
					test.raw,
					major,
					minor,
					ok,
					test.major,
					test.minor,
					test.compatible,
				)
			}
		})
	}
}

func TestValidateGraphicsCapabilityRequiresOpenGL21(t *testing.T) {
	t.Parallel()

	if err := validateGraphicsCapability(graphicsCapability{Version: "2.1"}); err != nil {
		t.Fatalf("OpenGL 2.1 debe aceptarse: %v", err)
	}
	if err := validateGraphicsCapability(graphicsCapability{Version: "4.6"}); err != nil {
		t.Fatalf("OpenGL 4.6 debe aceptarse: %v", err)
	}
	err := validateGraphicsCapability(graphicsCapability{
		Version:  "1.1.0",
		Renderer: "GDI Generic",
	})
	if err == nil {
		t.Fatal("OpenGL 1.1 debe rechazarse")
	}
	if !strings.Contains(err.Error(), "OpenGL 1.1") ||
		!strings.Contains(err.Error(), "OpenGL 2.1") ||
		!strings.Contains(err.Error(), "GL_VERSION 1.1.0") ||
		!strings.Contains(err.Error(), "renderer GDI Generic") {
		t.Fatalf("diagnóstico inesperado: %v", err)
	}
}

func TestValidateFramebufferCapabilityMatchesGLFWWindowsCriteria(t *testing.T) {
	t.Parallel()

	usable := framebufferCapability{
		DrawToWindow:   true,
		SupportsOpenGL: true,
		RGBA:           true,
		DoubleBuffered: true,
	}
	if err := validateFramebufferCapability(usable); err != nil {
		t.Fatalf("framebuffer compatible rechazado: %v", err)
	}

	software := usable
	software.SoftwareOnly = true
	if err := validateFramebufferCapability(software); err == nil ||
		!strings.Contains(err.Error(), "GDI") {
		t.Fatalf("rasterizador GDI debería rechazarse con causa explícita: %v", err)
	}

	singleBuffered := usable
	singleBuffered.DoubleBuffered = false
	if err := validateFramebufferCapability(singleBuffered); err == nil ||
		!strings.Contains(err.Error(), "doble búfer") {
		t.Fatalf("framebuffer sin doble búfer debería rechazarse: %v", err)
	}
}

func TestRequireInteractiveGraphicsFailsClosedAndShowsNativeDiagnostic(t *testing.T) {
	originalCheck := checkPlatformGraphicsReadiness
	originalDialog := showPlatformGraphicsFailure
	t.Cleanup(func() {
		checkPlatformGraphicsReadiness = originalCheck
		showPlatformGraphicsFailure = originalDialog
	})

	probeErr := errors.New("OpenGL 1.1; GDI Generic")
	checkPlatformGraphicsReadiness = func() error {
		return probeErr
	}
	var dialogMessage string
	showPlatformGraphicsFailure = func(message string) error {
		dialogMessage = message
		return nil
	}

	var stderr bytes.Buffer
	err := requireInteractiveGraphics(&stderr)
	if !errors.Is(err, probeErr) {
		t.Fatalf("requireInteractiveGraphics() error = %v, want %v", err, probeErr)
	}
	if !strings.Contains(stderr.String(), "interfaz gráfica no disponible") ||
		!strings.Contains(stderr.String(), "OpenGL 1.1") {
		t.Fatalf("stderr sin diagnóstico técnico: %q", stderr.String())
	}
	for _, expected := range []string{
		tl(graphicsUnavailableMessageID),
		tl(graphicsRemediationMessageID),
		"OpenGL 1.1",
	} {
		if !strings.Contains(dialogMessage, expected) {
			t.Fatalf(
				"aviso nativo incompleto; falta %q en %q",
				expected,
				dialogMessage,
			)
		}
	}
	if strings.Contains(dialogMessage, "%s") {
		t.Fatalf("aviso nativo conserva un marcador sin interpolar: %q", dialogMessage)
	}
}

func TestPrepareProtocolInteractiveUIUsesNativeFallbackWhenOpenGLFails(t *testing.T) {
	originalCheck := checkPlatformGraphicsReadiness
	originalDialog := showPlatformGraphicsFailure
	originalActivate := activateNativeProtocolFallback
	t.Cleanup(func() {
		checkPlatformGraphicsReadiness = originalCheck
		showPlatformGraphicsFailure = originalDialog
		activateNativeProtocolFallback = originalActivate
	})

	probeErr := errors.New("OpenGL 1.1; GDI Generic")
	checkPlatformGraphicsReadiness = func() error {
		return probeErr
	}
	var activationError error
	activateNativeProtocolFallback = func(err error) bool {
		activationError = err
		return true
	}
	dialogCalls := 0
	showPlatformGraphicsFailure = func(string) error {
		dialogCalls++
		return nil
	}

	var stderr bytes.Buffer
	if err := prepareProtocolInteractiveUI(&stderr); err != nil {
		t.Fatalf("prepareProtocolInteractiveUI() error = %v", err)
	}
	if !errors.Is(activationError, probeErr) {
		t.Fatalf("fallback recibió %v, want %v", activationError, probeErr)
	}
	if dialogCalls != 0 {
		t.Fatalf("se mostró el fallo terminal %d veces pese al fallback", dialogCalls)
	}
	if !strings.Contains(stderr.String(), "interfaz nativa segura") ||
		!strings.Contains(stderr.String(), "OpenGL 1.1") {
		t.Fatalf("aviso de fallback incompleto: %q", stderr.String())
	}
}

func TestRunRejectsRequestBeforeProtocolHandlerWhenGraphicsAreUnavailable(t *testing.T) {
	originalCheck := checkPlatformGraphicsReadiness
	originalDialog := showPlatformGraphicsFailure
	originalActivate := activateNativeProtocolFallback
	t.Cleanup(func() {
		checkPlatformGraphicsReadiness = originalCheck
		showPlatformGraphicsFailure = originalDialog
		activateNativeProtocolFallback = originalActivate
	})

	checkPlatformGraphicsReadiness = func() error {
		return errors.New("OpenGL 1.1")
	}
	dialogCalls := 0
	showPlatformGraphicsFailure = func(string) error {
		dialogCalls++
		return nil
	}
	activateNativeProtocolFallback = func(error) bool {
		return false
	}
	handlerCalls := 0
	var stderr bytes.Buffer
	code := runConDependencias(
		context.Background(),
		[]string{"afirma://sign?op=sign"},
		&stderr,
		func(context.Context, io.Writer, string) int {
			handlerCalls++
			return 0
		},
	)

	if code != graphicsUnavailableExitCode {
		t.Fatalf("runConDependencias() code = %d, want %d", code, graphicsUnavailableExitCode)
	}
	if handlerCalls != 0 {
		t.Fatalf("el manejador se invocó %d veces sin interfaz de aprobación", handlerCalls)
	}
	if dialogCalls != 1 {
		t.Fatalf("el aviso nativo se invocó %d veces, want 1", dialogCalls)
	}
	if !strings.Contains(stderr.String(), "OpenGL 1.1") {
		t.Fatalf("stderr sin causa gráfica: %q", stderr.String())
	}
}

func TestSanitizeGraphicsDiagnosticRemovesControlsAndLimitsLength(t *testing.T) {
	t.Parallel()

	raw := "GDI\x00Generic\n" + strings.Repeat("x", 300)
	clean := sanitizeGraphicsDiagnostic(raw)
	if strings.ContainsAny(clean, "\x00\n\r") {
		t.Fatalf("diagnóstico conserva controles: %q", clean)
	}
	if len(clean) > 161 {
		t.Fatalf("diagnóstico no limitado: %d bytes", len(clean))
	}
}
