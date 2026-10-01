// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui && amd64

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
)

func TestNativeProtocolOperationPolicyAllowsSupportedOperations(t *testing.T) {
	nativeProtocolFallback.Store(true)
	t.Cleanup(disableNativeProtocolUIFallback)

	for _, operation := range []afirmauri.TipoOperacion{
		afirmauri.OperacionFirma,
		afirmauri.OperacionLote,
		afirmauri.OperacionSelectCert,
		afirmauri.OperacionSave,
		afirmauri.OperacionLoad,
		afirmauri.OperacionSignSave,
	} {
		if err := validateNativeProtocolOperation(operation); err != nil {
			t.Fatalf("operación simple %q rechazada: %v", operation, err)
		}
	}

	err := validateNativeProtocolOperation(afirmauri.TipoOperacion("delete"))
	if err == nil {
		t.Fatal("una operación desconocida debe fallar cerrada")
	}
	lower := strings.ToLower(err.Error())
	for _, expected := range []string{
		"fallo local",
		"delete",
		"no se ha ejecutado ninguna acción",
	} {
		if !strings.Contains(lower, expected) {
			t.Fatalf("error de operación desconocida sin %q: %v", expected, err)
		}
	}
}

func TestNativeSafeFailureDetailDoesNotExposeRequestData(t *testing.T) {
	raw := errors.New(
		`falló https://example.invalid/path?token=SECRETO ` +
			`en C:\Users\TestUser\Documents\nomina.pdf ` +
			`request_id=abc certificate_id=def`,
	)
	detail := nativeSafeFailureDetail("sign", raw)
	lower := strings.ToLower(detail)
	for _, forbidden := range []string{
		"https://",
		"example.invalid",
		`c:\users`,
		"nomina.pdf",
		"secreto",
		"abc",
		"def",
	} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("detalle contiene %q: %q", forbidden, detail)
		}
	}
	if !strings.Contains(lower, "posible problema") {
		t.Fatalf("detalle no orienta al usuario: %q", detail)
	}
}

func TestNativeSafeFailureDetailExplainsObsoleteTLSGenerally(t *testing.T) {
	detail := nativeSafeFailureDetail(
		"batch",
		errors.New(
			`Post "https://portal.example.invalid/pre": `+
				`tls: server selected unsupported protocol version 301`,
		),
	)
	lower := strings.ToLower(detail)
	for _, expected := range []string{
		"tls 1.0",
		"tls 1.1",
		"obsoleto",
		"tls 1.2",
		"portal",
		"tls_legacy_unsupported",
	} {
		if !strings.Contains(lower, expected) {
			t.Fatalf("detalle TLS sin %q: %q", expected, detail)
		}
	}
	for _, forbidden := range []string{
		"https://",
		"example.invalid",
		"/pre",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("detalle TLS contiene %q: %q", forbidden, detail)
		}
	}
}

func TestNativeDialogSanitizersBoundAndRejectInjection(t *testing.T) {
	clean := sanitizeNativeDialogText(
		"  texto\x00\toculto\u202E\n"+strings.Repeat("x", 300),
		24,
	)
	if strings.ContainsAny(clean, "\x00\t") ||
		strings.ContainsRune(clean, '\u202E') {
		t.Fatalf("texto conserva controles: %q", clean)
	}
	if len([]rune(clean)) > 25 {
		t.Fatalf("texto no acotado: %d runas", len([]rune(clean)))
	}

	extensions := normalizedNativeExtensions(
		"pdf, .XML, p7s;*.exe, ../../bat, pdf",
	)
	if got, want := strings.Join(extensions, ","), "pdf,xml"; got != want {
		t.Fatalf("extensiones = %q, want %q", got, want)
	}
}

func TestParseNativeFileDialogBufferRejectsUnsafeMultiselect(t *testing.T) {
	safe := nativeDialogUTF16Buffer(
		`C:\Users\TestUser\Documents`,
		"uno.pdf",
		"dos.xml",
	)
	paths, err := parseNativeFileDialogBuffer(safe)
	if err != nil {
		t.Fatalf("multiselección segura rechazada: %v", err)
	}
	if len(paths) != 2 ||
		paths[0] != filepath.Join(`C:\Users\TestUser\Documents`, "uno.pdf") {
		t.Fatalf("rutas inesperadas: %#v", paths)
	}

	unsafeBuffer := nativeDialogUTF16Buffer(
		`C:\Users\TestUser\Documents`,
		`..\fuera.pdf`,
	)
	if _, err := parseNativeFileDialogBuffer(unsafeBuffer); err == nil {
		t.Fatal("se aceptó un nombre relativo con traversal")
	}
}

func TestPublishNativeDirectProtocolResultPublishesBeforeClosingUI(t *testing.T) {
	result := make(chan int, 1)
	callbackCalled := false
	publishNativeDirectProtocolResult(
		result,
		0,
		func(code int) {
			callbackCalled = true
			select {
			case published := <-result:
				if published != code {
					t.Fatalf("resultado publicado=%d, want %d", published, code)
				}
			default:
				t.Fatal("el cierre de UI se solicitó antes de publicar el resultado")
			}
		},
	)
	if !callbackCalled {
		t.Fatal("no se solicitó el cierre tras publicar el resultado")
	}
}

func TestNativeWindowsABIStructSizes(t *testing.T) {
	if got := unsafe.Sizeof(nativeTaskDialogConfig{}); got != 160 {
		t.Fatalf("TASKDIALOGCONFIG size=%d, want 160", got)
	}
	if got := unsafe.Sizeof(nativeOpenFileName{}); got != 152 {
		t.Fatalf("OPENFILENAMEW size=%d, want 152", got)
	}
	if got := unsafe.Sizeof(nativeNotifyIconData{}); got != 976 {
		t.Fatalf("NOTIFYICONDATAW size=%d, want 976", got)
	}
	if got := unsafe.Sizeof(portalFlashInfo{}); got != 32 {
		t.Fatalf("FLASHWINFO size=%d, want 32", got)
	}
}

func TestShowNativeTaskDialogHonorsAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonCancel, Label: "Cerrar"},
		},
		DefaultButton: nativeButtonCancel,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestNativeTaskDialogInteractiveProbe(t *testing.T) {
	if os.Getenv("GRXFIRMA_NATIVE_UI_PROBE") != "1" {
		t.Skip("sonda interactiva Windows desactivada")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		8*time.Second,
	)
	defer cancel()
	created := make(chan struct{}, 1)
	type windowObservation struct {
		visible bool
		focused bool
		left    int32
		top     int32
		right   int32
		bottom  int32
	}
	observed := make(chan windowObservation, 1)
	button, _, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Sonda de compatibilidad",
		Instruction: "Interfaz nativa de Windows disponible",
		Content:     "Esta ventana confirma TaskDialogIndirect, Common Controls v6 y cancelación segura por contexto.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonCancel, Label: "Cerrar"},
		},
		DefaultButton: nativeButtonCancel,
		OnCreated: func(hwnd uintptr) {
			created <- struct{}{}
			go func() {
				time.Sleep(500 * time.Millisecond)
				procIsWindowVisible := user32Native.NewProc(
					"IsWindowVisible",
				)
				procGetWindowRect := user32Native.NewProc(
					"GetWindowRect",
				)
				var rect struct {
					left   int32
					top    int32
					right  int32
					bottom int32
				}
				visible, _, _ := procIsWindowVisible.Call(hwnd)
				foreground, _, _ := procGetForegroundWindowNative.Call()
				ok, _, _ := procGetWindowRect.Call(
					hwnd,
					uintptr(unsafe.Pointer(&rect)),
				)
				observed <- windowObservation{
					visible: visible != 0 && ok != 0,
					focused: foreground == hwnd,
					left:    rect.left,
					top:     rect.top,
					right:   rect.right,
					bottom:  rect.bottom,
				}
			}()
		},
	})
	select {
	case <-created:
	default:
		t.Fatalf(
			"TaskDialogIndirect no notificó la creación de la ventana (button=%d, err=%v)",
			button,
			err,
		)
	}
	if err == nil {
		if button != nativeButtonCancel && button != idCancel {
			t.Fatalf("botón=%d, want cerrar/cancelar", button)
		}
		return
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("TaskDialogIndirect: %v", err)
	}
	observation := <-observed
	if !observation.visible || !observation.focused ||
		observation.right <= observation.left ||
		observation.bottom <= observation.top {
		t.Fatalf("TaskDialog no visible o sin geometría: %+v", observation)
	}
	t.Logf(
		"TaskDialog visible rect=(%d,%d)-(%d,%d), pid=%d",
		observation.left,
		observation.top,
		observation.right,
		observation.bottom,
		os.Getpid(),
	)
}

func TestNativeInteractiveSecurityDialogs(t *testing.T) {
	if os.Getenv("GRXFIRMA_NATIVE_UI_PROBE") != "1" {
		t.Skip("sonda interactiva Windows desactivada")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		4*time.Second,
	)
	defer cancel()
	button, _, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Selección segura",
		Instruction: "Ningún certificado debe aparecer preseleccionado",
		Content:     "Sonda de seguridad de la selección explícita.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonAccept, Label: "Continuar"},
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton: nativeButtonCancel,
		RadioButtons: []nativeDialogChoice{
			{ID: nativeRadioBase, Label: "Certificado de prueba A"},
			{ID: nativeRadioBase + 1, Label: "Certificado de prueba B"},
		},
		AcceptNeedsRadio: nativeButtonAccept,
		OnCreated: func(hwnd uintptr) {
			go func() {
				time.Sleep(500 * time.Millisecond)
				postNativeWindowMessage(
					hwnd,
					tdmClickButton,
					idCancel,
				)
			}()
		},
	})
	if err != nil {
		t.Fatalf("diálogo de selección: %v", err)
	}
	if button != idCancel {
		t.Fatalf(
			"cancelación devolvió botón=%d, want IDCANCEL=%d",
			button,
			idCancel,
		)
	}
	fileCtx, fileCancel := context.WithTimeout(
		context.Background(),
		4*time.Second,
	)
	defer fileCancel()
	_, err = runNativeFileDialog(
		fileCtx,
		false,
		make([]uint16, 4096),
		"",
		"pdf,xml",
		ofnNoChangeDir|
			ofnEnableHook|
			ofnPathMustExist|
			ofnFileMustExist|
			ofnExplorer|
			ofnDontAddToRecent|
			ofnForceFileSystem,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("selector de fichero no respetó cancelación: %v", err)
	}
}

func nativeDialogUTF16Buffer(parts ...string) []uint16 {
	buffer := make([]uint16, 1024)
	offset := 0
	for _, part := range parts {
		encoded, err := windows.UTF16FromString(part)
		if err != nil {
			panic(err)
		}
		offset += copy(buffer[offset:], encoded)
	}
	return buffer
}
