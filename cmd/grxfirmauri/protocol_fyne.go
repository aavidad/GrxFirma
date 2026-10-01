// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"bytes"
	"context"
	"grxfirma/internal/appdirs"
	"image/color"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func hasLegacyProtocolWindow() bool { return true }

func maybeRunProtocolUI(parent context.Context, stderr io.Writer, rawURI string) (bool, int) {
	if !shouldUseProtocolUI(rawURI) {
		return false, 0
	}
	if nativeProtocolUIEnabled() {
		return true, runNativeDirectProtocolUIShell(
			parent,
			stderr,
			rawURI,
		)
	}
	return true, runProtocolUIShell(parent, stderr, rawURI)
}

func runProtocolUIShell(parent context.Context, stderr io.Writer, rawURI string) int {
	aplicarEscalaProtocolUI("")
	traceWriter := newProtocolTraceWriter(stderr)

	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	setProtocolLocalizer(configDir)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	a := newLegacyFyneApp()
	w := a.NewWindow(tl("GrxFirma — Solicitud web"))
	startTray := configureLegacyLaunchWindow(a, w, ctx, cancel, "directo", rawURI, "", configDir, true, true)

	resultReady := make(chan struct{})
	code := 1
	go func() {
		code = handleProtocolRequestNoGrace(ctx, traceWriter, rawURI)
		if code == 0 {
			// La notificación y la ocultación se encolan en el bucle Fyne.
			// Esperar a que se procesen evita que el cierre del contexto las
			// descarte en una operación directa muy rápida.
			fyne.DoAndWait(func() {})
			cancel()
		}
		close(resultReady)
	}()

	showAndRunLegacyLaunch(a, w, startTray)
	<-resultReady
	return code
}

func shouldUseProtocolUI(rawURI string) bool {
	if strings.TrimSpace(rawURI) == "" {
		return false
	}
	if strings.TrimSpace(os.Getenv("GRXFIRMA_PROTOCOL_UI")) == "0" {
		return false
	}
	uriL := strings.ToLower(strings.TrimSpace(rawURI))
	if !strings.HasPrefix(uriL, "afirma://") {
		return false
	}
	if strings.Contains(uriL, "127.0.0.1") || strings.Contains(uriL, "localhost") || strings.Contains(uriL, "::1") {
		return false
	}
	return strings.Contains(uriL, "stservlet=") || strings.Contains(uriL, "storageservlet=") || strings.Contains(uriL, "rtservlet=") || strings.Contains(uriL, "retrieveservlet=")
}

func resumenURISeguro(rawURI string) string {
	uri := strings.TrimSpace(rawURI)
	if uri == "" {
		return ""
	}
	if len(uri) > 220 {
		uri = uri[:220] + "..."
	}
	return uri
}

type protocolTraceWriter struct {
	dst io.Writer
	mu  sync.Mutex
	buf bytes.Buffer
}

func newProtocolTraceWriter(dst io.Writer) *protocolTraceWriter {
	return &protocolTraceWriter{dst: dst}
}

func (w *protocolTraceWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buf.Write(redactProtocolTrace(p))
	if w.dst != nil {
		return w.dst.Write(p)
	}
	return len(p), nil
}

func (w *protocolTraceWriter) Text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	texto := strings.TrimSpace(w.buf.String())
	if texto == "" {
		return tl("No se ha capturado ninguna traza adicional.")
	}
	return texto
}

func redactProtocolTrace(p []byte) []byte {
	texto := string(p)
	reemplazos := []struct {
		old string
		new string
	}{
		{"Certificate", "Certificado"},
		{"fingerprint", "huella"},
		{"Subject", "Titular"},
		{"Issuer", "Emisor"},
	}
	for _, r := range reemplazos {
		texto = strings.ReplaceAll(texto, r.old, r.new)
	}
	return []byte(texto)
}

func waitLegacyLaunchUI(ctx context.Context, cancel context.CancelFunc, modo, addr, sessionID, configDir string) int {
	if nativeProtocolUIEnabled() {
		return waitNativeLegacyLaunchUI(ctx, cancel, modo, addr)
	}
	aplicarEscalaProtocolUI(configDir)
	setProtocolLocalizer(configDir)
	a := newLegacyFyneApp()
	w := a.NewWindow(tl("GrxFirma — Firma web"))
	startTray := configureLegacyLaunchWindow(a, w, ctx, cancel, modo, addr, sessionID, configDir, true, true)
	showAndRunLegacyLaunch(a, w, startTray)
	return 0
}

var scheduleLegacyUI = fyne.Do
var portalOperationGeneration atomic.Uint64
var portalWaitingGeneration atomic.Uint64
var portalPendingAction atomic.Bool
var portalWaitingEffectMu sync.Mutex

var portalWindowControls struct {
	mu        sync.RWMutex
	action    func()
	delivered func(string)
	failed    func()
}

// portalActionUI muestra la ventana completa únicamente cuando se necesita una
// decisión del usuario. Puede llamarse desde la goroutine que procesa el portal.
func portalActionUI() {
	portalPendingAction.Store(true)
	portalWaitingGeneration.Add(1)
	portalWaitingEffectMu.Lock()
	portalWaitingEffectMu.Unlock()
	portalWindowControls.mu.RLock()
	callback := portalWindowControls.action
	portalWindowControls.mu.RUnlock()
	if callback != nil {
		fyne.Do(callback)
	}
}

func portalErrorUI() {
	portalPendingAction.Store(true)
	portalWaitingGeneration.Add(1)
	portalWaitingEffectMu.Lock()
	portalWaitingEffectMu.Unlock()
	if nativeProtocolUIEnabled() {
		nativePortalErrorUI()
		return
	}
	portalWindowControls.mu.RLock()
	callback := portalWindowControls.failed
	portalWindowControls.mu.RUnlock()
	if callback != nil {
		fyne.Do(callback)
	}
}

func portalDeliveredUI(kind string) {
	portalWaitingGeneration.Add(1)
	if nativeProtocolUIEnabled() {
		nativePortalDeliveredUI(kind)
		return
	}
	portalWindowControls.mu.RLock()
	callback := portalWindowControls.delivered
	portalWindowControls.mu.RUnlock()
	if callback != nil {
		generation := portalOperationGeneration.Load()
		fyne.Do(func() {
			if portalOperationGeneration.Load() == generation {
				callback(kind)
			}
		})
	}
}

func configureLegacyLaunchWindow(
	a fyne.App,
	w fyne.Window,
	ctx context.Context,
	cancel context.CancelFunc,
	modo, addr, sessionID, configDir string,
	manageAppQuit bool,
	bindProtocolContext bool,
) func() {
	if bindProtocolContext {
		setProtocolUIContext(a, w)
	}
	preset := loadLegacyUISizePreset(configDir)
	residentOption := newTrayResidentOption(configDir)
	traySupported := manageAppQuit && legacyTrayMenuSupported(a)
	var trayStateMu sync.RWMutex
	trayReady := false
	var cleanupTrayMu sync.Mutex
	cleanupTrayFn := func() {}
	cleanupTray := func() {
		cleanupTrayMu.Lock()
		fn := cleanupTrayFn
		cleanupTrayMu.Unlock()
		fn()
	}
	canHideOnClose := func() bool {
		if !manageAppQuit {
			return true
		}
		trayStateMu.RLock()
		defer trayStateMu.RUnlock()
		return trayReady
	}
	previousWindow := capturePortalForeground()
	compactStatus := widget.NewLabel(estadoLegacyLaunchUI(modo))
	compactStatus.TextStyle = fyne.TextStyle{Bold: true}
	deliveredHidden := false
	actionShown := false
	var showCompact func()

	slog.Info("protocol_window_configure_start", "mode", modo, "session_id", maskSessionForLog(sessionID), "addr", sanitizeEndpointForLog(addr), "manage_app_quit", manageAppQuit, "bind_protocol_context", bindProtocolContext, "can_hide_on_close", canHideOnClose)
	w.Resize(fyne.NewSize(410, 150))

	titulo := canvas.NewText(tl("GRXFIRMA WEB"), theme.Color(theme.ColorNameForeground))
	titulo.TextSize = 34
	titulo.TextStyle = fyne.TextStyle{Bold: true}

	estado := canvas.NewText(estadoLegacyLaunchUI(modo), theme.Color(theme.ColorNameForeground))
	estado.TextSize = 22
	estado.TextStyle = fyne.TextStyle{Bold: true}

	protocolo := canvas.NewText(tl("Protocolo: %s", resumenModoLegacy(modo)), color.NRGBA{R: 0xBF, G: 0xDB, B: 0xFE, A: 0xFF})
	protocolo.TextSize = 18
	protocolo.TextStyle = fyne.TextStyle{Bold: true}

	detalleTexto := tl("Sesión %s activa en %s", resumenSesionLegacy(sessionID), strings.TrimSpace(addr))
	if strings.EqualFold(strings.TrimSpace(modo), "directo") {
		detalleTexto = resumenURISeguro(addr)
	}
	detalle := widget.NewLabel(detalleTexto)
	detalle.Wrapping = fyne.TextWrapWord

	setProtocolUIStateCallbacks(
		func(text string) {
			fyne.Do(func() {
				estado.Text = text
				estado.Refresh()
				compactStatus.SetText(text)
			})
		},
		func(text string) {
			fyne.Do(func() {
				detalle.SetText(text)
			})
		},
	)

	ayuda := widget.NewRichTextFromMarkdown(strings.Join(legacyLaunchHelp(modo), "\n\n"))
	ayuda.Wrapping = fyne.TextWrapWord
	initialResident := residentOption.Enabled()
	var startTray func()
	var ocultarBtn *widget.Button
	residenteCheck := widget.NewCheck(tl("Mantener esta sesión web en segundo plano al cerrar"), nil)
	residenteCheck.SetChecked(initialResident)
	residenteCheck.OnChanged = func(checked bool) {
		residentOption.Set(checked)
		if checked && startTray != nil {
			startTray()
		}
		if ocultarBtn != nil {
			if checked && canHideOnClose() {
				ocultarBtn.Show()
			} else {
				ocultarBtn.Hide()
			}
		}
	}

	cerrarBtn := widget.NewButton(tl("Cancelar solicitud web"), func() {
		slog.Info("protocol_window_cancel_clicked", "mode", modo, "session_id", maskSessionForLog(sessionID))
		if cancel != nil {
			cancel()
		}
		w.Close()
	})
	setProtocolUICompletionCallback(func(completed bool) {
		if !completed {
			portalOperationGeneration.Add(1)
			portalPendingAction.Store(false)
		}
		fyne.Do(func() {
			if completed {
				cerrarBtn.SetText(tl("Cerrar"))
			} else {
				cerrarBtn.SetText(tl("Cancelar solicitud web"))
				if (deliveredHidden || actionShown) && !portalPendingAction.Load() {
					deliveredHidden = false
					actionShown = false
					showCompact()
				}
			}
		})
	})
	ocultarBtn = widget.NewButton(tl("Ocultar a bandeja"), func() {
		slog.Info("protocol_window_hide_clicked", "mode", modo, "session_id", maskSessionForLog(sessionID))
		if canHideOnClose() {
			w.Hide()
		}
	})
	ocultarBtn.Importance = widget.LowImportance
	ocultarBtn.Hide()
	if initialResident && canHideOnClose() {
		ocultarBtn.Show()
	}

	topContent := container.NewPadded(container.NewVBox(titulo, estado, protocolo, detalle, ayuda))
	topHost := container.NewStack(topContent)
	setProtocolUITopHost(topHost, func() {
		slog.Info("protocol_window_restore_top_content", "mode", modo, "session_id", maskSessionForLog(sessionID))
		fyne.Do(func() {
			topHost.Objects = []fyne.CanvasObject{topContent}
			topHost.Refresh()
			w.SetTitle(tl("GrxFirma — Firma web"))
		})
	})
	mainContent := fyne.CanvasObject(container.NewPadded(topHost))
	if protocolDebugEnabled() {
		logConsole := newShellLogConsole(ctx)
		split := container.NewVSplit(topHost, logConsole)
		split.Offset = 0.56
		mainContent = container.NewPadded(split)
	}
	fullContent := container.NewBorder(
		nil,
		container.NewVBox(residenteCheck, container.NewHBox(cerrarBtn, ocultarBtn)),
		nil,
		nil,
		mainContent,
	)
	compactContent := container.NewPadded(container.NewBorder(
		nil,
		widget.NewButton(tl("Cancelar solicitud web"), func() {
			if cancel != nil {
				cancel()
			}
		}),
		nil,
		nil,
		container.NewVBox(compactStatus, widget.NewLabel(tl("Consulte el portal en el navegador."))),
	))
	showCompact = func() {
		if actionShown || portalPendingAction.Load() {
			return
		}
		generation := portalWaitingGeneration.Add(1)
		w.SetContent(compactContent)
		w.Resize(fyne.NewSize(410, 150))
		w.Show()
		placePortalWaitingWindow(previousWindow, generation)
	}
	showAction := func() {
		generation := portalWaitingGeneration.Add(1)
		deliveredHidden = false
		actionShown = true
		w.SetContent(fullContent)
		w.Resize(preset.launchSize)
		w.CenterOnScreen()
		w.Show()
		w.RequestFocus()
		focusPortalActionWindow(generation)
	}
	portalWindowControls.mu.Lock()
	portalWindowControls.action = showAction
	portalWindowControls.failed = showAction
	portalWindowControls.delivered = func(kind string) {
		portalWaitingGeneration.Add(1)
		portalPendingAction.Store(false)
		actionShown = false
		deliveredHidden = true
		w.Hide()
		restorePortalForeground(previousWindow)
		message := tl("Firma entregada al portal: consulte el resultado en la web")
		if kind == "certificate" {
			message = tl("Certificado entregado al portal: consulte el resultado en la web")
		}
		a.SendNotification(fyne.NewNotification(tl("GrxFirma"), message))
	}
	portalWindowControls.mu.Unlock()
	w.SetContent(compactContent)
	w.SetCloseIntercept(func() {
		canHide := canHideOnClose()
		slog.Info("protocol_window_close_intercept", "mode", modo, "session_id", maskSessionForLog(sessionID), "resident_enabled", residentOption.Enabled(), "can_hide_on_close", canHide)
		if residentOption.Enabled() && canHide {
			slog.Info("protocol_window_hidden_to_tray", "mode", modo, "session_id", maskSessionForLog(sessionID))
			w.Hide()
			return
		}
		if cancel != nil {
			cancel()
		}
		w.SetCloseIntercept(nil)
		w.Close()
	})
	w.SetOnClosed(func() {
		portalWindowControls.mu.Lock()
		portalWindowControls.action = nil
		portalWindowControls.failed = nil
		portalWindowControls.delivered = nil
		portalWindowControls.mu.Unlock()
		slog.Info("protocol_window_closed", "mode", modo, "session_id", maskSessionForLog(sessionID))
		cleanupTray()
		if bindProtocolContext {
			if _, current := currentProtocolUIContext(); current == w {
				clearProtocolUIContext()
			}
		}
	})
	placePortalWaitingWindow(previousWindow, portalWaitingGeneration.Add(1))

	go func() {
		<-ctx.Done()
		slog.Info("protocol_window_context_done", "mode", modo, "session_id", maskSessionForLog(sessionID))
		fyne.Do(func() {
			w.SetCloseIntercept(nil)
			w.Close()
			if manageAppQuit {
				a.Quit()
			}
		})
	}()

	var startTrayOnce sync.Once
	startTray = func() {
		startTrayOnce.Do(func() {
			if !traySupported {
				return
			}
			installed, cleanup := installLegacyTrayMenu(a, w, cancel)
			trayStateMu.Lock()
			trayReady = installed
			trayStateMu.Unlock()
			cleanupTrayMu.Lock()
			cleanupTrayFn = cleanup
			cleanupTrayMu.Unlock()
			if installed && ocultarBtn != nil {
				ocultarBtn.Show()
			}
		})
	}
	return func() {
		if !initialResident {
			return
		}
		// La preferencia residente puede venir activada de una sesión previa.
		// Esperamos a que GLFW haya materializado la primera ventana antes de
		// crear SystrayMonitor; hacerlo durante el arranque deja Windows sin
		// ninguna ventana Fyne visible.
		go func() {
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				scheduleLegacyUI(startTray)
			}
		}()
	}
}

// showAndRunLegacyLaunch materializa primero el viewport principal. En Windows,
// inicializar el systray antes de mostrar la primera ventana puede dejar al
// driver GLFW únicamente con sus ventanas auxiliares ocultas.
func showAndRunLegacyLaunch(a fyne.App, w fyne.Window, startTray func()) {
	if a == nil || w == nil {
		return
	}
	w.SetMaster()
	w.Show()
	// El portal conserva el foco durante la espera. Los diálogos que requieren
	// una decisión llaman explícitamente a portalActionUI.
	if startTray != nil {
		// Show() queda encolado hasta que arranca el loop GLFW. Encolar
		// también el systray conserva el orden: primero se materializa la
		// ventana maestra y después se crea SystrayMonitor.
		scheduleLegacyUI(startTray)
	}
	a.Run()
}

func estadoLegacyLaunchUI(modo string) string {
	switch strings.ToLower(strings.TrimSpace(modo)) {
	case "service":
		return tl("Canal legacy de servicio preparado")
	case "directo":
		return tl("Solicitud web de firma en curso")
	default:
		return tl("Canal legacy WebSocket preparado")
	}
}

func legacyLaunchHelp(modo string) []string {
	items := []string{
		tl("GrxFirma está esperando la solicitud del portal."),
		tl("Cuando llegue la petición, se mostrará el selector de documento o de certificado como en V1."),
		tl("Si activas el modo residente, al cerrar la ventana se ocultará a la bandeja y la sesión seguirá viva."),
	}
	if notice := getStartupTLSTrustNotice(); notice != "" {
		items = append(items, notice)
	}
	if strings.EqualFold(strings.TrimSpace(modo), "websocket") {
		items = append(items, tl("Tras entregar la firma o el certificado, GrxFirma se ocultará y podrá llegar otra operación del portal."))
	} else {
		items = append(items, tl("La ventana se cerrará automáticamente al terminar la sesión del portal."))
	}
	switch strings.ToLower(strings.TrimSpace(modo)) {
	case "websocket", "service":
		items = append(items, tl(legacyBrowserLocalAccessHelpID))
	}
	return items
}

func resumenSesionLegacy(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tl("sin id")
	}
	if len(raw) <= 10 {
		return raw
	}
	return raw[:4] + "..." + raw[len(raw)-3:]
}

func resumenModoLegacy(modo string) string {
	switch strings.ToLower(strings.TrimSpace(modo)) {
	case "service":
		return tl("afirma:// service")
	case "directo":
		return tl("afirma:// directo")
	default:
		return tl("afirma:// websocket")
	}
}
