// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/securefile"
	desktopdocumentpicker "grxfirma/internal/adapters/outbound/desktop/documentpicker"
	"grxfirma/internal/adapters/outbound/desktop/truststore"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/progressdialog"
	"grxfirma/presentation/desktop/trustdialog"
)

var protocoloUI struct {
	mu         sync.RWMutex
	app        fyne.App
	window     fyne.Window
	status     func(string)
	detail     func(string)
	completion func(bool)
	topHost    *fyne.Container
	restoreTop func()
}

const legacyFyneAppID = "io.github.aavidad.grxfirma.afirmauri"

func newLegacyFyneApp() fyne.App {
	a := app.NewWithID(legacyFyneAppID)
	applyLegacyAppIcon(a)
	return a
}

func applyLegacyAppIcon(a fyne.App) {
	if a == nil {
		return
	}
	if resource := resolveLegacyAppIconResource(); resource != nil {
		a.SetIcon(resource)
	}
}

func resolveLegacyAppIconResource() fyne.Resource {
	exePath, _ := os.Executable()
	binDir := filepath.Dir(exePath)
	candidates := []string{
		filepath.Join(binDir, "../share/icons/hicolor/256x256/apps/grxfirma.png"),
		filepath.Join(binDir, "../lib/grxfirma/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(binDir, "../lib64/grxfirma/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(binDir, "assets/grxfirma-icono-256.png"),
		filepath.Join(filepath.Dir(binDir), "cmd/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(filepath.Dir(filepath.Dir(binDir)), "cmd/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(".", "cmd/gui-qml/assets/grxfirma-icono-256.png"),
	}
	for _, candidate := range candidates {
		data, err := securefile.ReadFileLimit(candidate, 5*1024*1024)
		if err != nil || len(data) == 0 {
			continue
		}
		return fyne.NewStaticResource("grxfirma-icono-256.png", data)
	}
	return nil
}

func setProtocolUIContext(a fyne.App, w fyne.Window) {
	protocoloUI.mu.Lock()
	defer protocoloUI.mu.Unlock()
	protocoloUI.app = a
	protocoloUI.window = w
}

func setProtocolUIStateCallbacks(status func(string), detail func(string)) {
	protocoloUI.mu.Lock()
	defer protocoloUI.mu.Unlock()
	protocoloUI.status = status
	protocoloUI.detail = detail
}

func setProtocolUICompletionCallback(callback func(bool)) {
	protocoloUI.mu.Lock()
	defer protocoloUI.mu.Unlock()
	protocoloUI.completion = callback
}

func setProtocolCompletionUI(completed bool) {
	if nativeProtocolUIEnabled() {
		setNativeProtocolCompletionUI(completed)
		return
	}
	protocoloUI.mu.RLock()
	callback := protocoloUI.completion
	protocoloUI.mu.RUnlock()
	if callback != nil {
		callback(completed)
	}
}

func setProtocolUITopHost(host *fyne.Container, restore func()) {
	protocoloUI.mu.Lock()
	defer protocoloUI.mu.Unlock()
	protocoloUI.topHost = host
	protocoloUI.restoreTop = restore
}

func clearProtocolUIContext() {
	protocoloUI.mu.Lock()
	defer protocoloUI.mu.Unlock()
	protocoloUI.app = nil
	protocoloUI.window = nil
	protocoloUI.status = nil
	protocoloUI.detail = nil
	protocoloUI.completion = nil
	protocoloUI.topHost = nil
	protocoloUI.restoreTop = nil
}

func currentProtocolUIContext() (fyne.App, fyne.Window) {
	protocoloUI.mu.RLock()
	defer protocoloUI.mu.RUnlock()
	return protocoloUI.app, protocoloUI.window
}

func currentProtocolUITopHost() (*fyne.Container, func()) {
	protocoloUI.mu.RLock()
	defer protocoloUI.mu.RUnlock()
	return protocoloUI.topHost, protocoloUI.restoreTop
}

func updateProtocolUI(status, detail string) {
	if nativeProtocolUIEnabled() {
		updateNativeProtocolUI(status, detail)
		return
	}
	protocoloUI.mu.RLock()
	setStatus := protocoloUI.status
	setDetail := protocoloUI.detail
	protocoloUI.mu.RUnlock()
	if setStatus != nil && status != "" {
		setStatus(status)
	}
	if setDetail != nil && detail != "" {
		setDetail(detail)
	}
}

type selectorInteractivo struct{}

// ElegirPosicionSello deja al usuario situar la firma visible con el diálogo
// Fyne (visibleSignature=want).
func (s selectorInteractivo) ElegirPosicionSello(ctx context.Context) (string, string, error) {
	a, _ := currentProtocolUIContext()
	if a == nil {
		a = fyne.CurrentApp()
	}
	selector := certpicker.New()
	if a != nil {
		selector = selector.WithApp(a)
	}
	return selector.ElegirPosicionSello(ctx)
}

func (s selectorInteractivo) Select(ctx context.Context, certs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
	portalActionUI()
	certpicker.SetConfigDir(defaultProtocolConfigDir())
	a, w := currentProtocolUIContext()
	if a == nil {
		a = fyne.CurrentApp()
	}
	applyLegacyAppIcon(a)
	selector := certpicker.New().WithCredentialActions()
	if a != nil {
		selector = selector.WithApp(a)
	}
	if w != nil {
		selector = selector.WithWindow(w)
	}
	if host, restore := currentProtocolUITopHost(); host != nil {
		selector = selector.WithHost(host, restore)
	}
	return selector.Select(ctx, certs)
}

type trustUIFyne struct{}

func (t trustUIFyne) PedirDecision(ctx context.Context, origen string) (trustdialog.DecisionUnicaVez, error) {
	portalActionUI()
	trustdialog.SetConfigDir(defaultProtocolConfigDir())
	a, w := currentProtocolUIContext()
	if a == nil {
		a = fyne.CurrentApp()
	}
	if w != nil {
		return trustdialog.NewFyneTrustDialog(w).PedirDecision(ctx, origen)
	}
	if a == nil {
		a = newLegacyFyneApp()
	}
	w = a.NewWindow(tl("Autorización de firma"))
	w.Resize(fyne.NewSize(1180, 520))
	w.CenterOnScreen()

	resultado := make(chan trustdialog.DecisionUnicaVez, 1)
	enviar := func(decision trustdialog.DecisionUnicaVez) {
		select {
		case resultado <- decision:
		default:
		}
		w.Close()
	}
	msg := trustdialog.BuildPromptMessageForUI(origen)

	mensaje := container.NewVBox(
		widget.NewCard(
			msg.Headline,
			"",
			container.NewVBox(
				widget.NewLabel(msg.OriginLabel),
				func() fyne.CanvasObject {
					lbl := widget.NewLabel(msg.OriginValue)
					lbl.Wrapping = fyne.TextWrapWord
					return lbl
				}(),
			),
		),
		widget.NewCard(
			tl("Qué está intentando hacer"),
			"",
			func() fyne.CanvasObject {
				lbl := widget.NewLabel(msg.PrimaryMessage)
				lbl.Wrapping = fyne.TextWrapWord
				return lbl
			}(),
		),
		widget.NewCard(
			tl("Riesgo si continúas"),
			"",
			container.NewVBox(
				func() fyne.CanvasObject {
					lbl := widget.NewLabel(msg.RiskMessage)
					lbl.Wrapping = fyne.TextWrapWord
					return lbl
				}(),
				func() fyne.CanvasObject {
					lbl := widget.NewLabel(msg.ResidentRisk)
					lbl.Wrapping = fyne.TextWrapWord
					return lbl
				}(),
			),
		),
		widget.NewLabel(msg.Question),
	)
	botones := container.NewHBox(
		widget.NewButton(tl("Confiar siempre"), func() { enviar(trustdialog.ConfiarSiempre) }),
		widget.NewButton(tl("Confiar esta vez"), func() { enviar(trustdialog.ConfiarEstaVez) }),
		widget.NewButton(tl("Rechazar"), func() { enviar(trustdialog.Rechazar) }),
	)
	w.SetContent(container.NewBorder(nil, botones, nil, nil, container.NewPadded(mensaje)))
	w.SetCloseIntercept(func() { enviar(trustdialog.Rechazar) })

	go func() {
		select {
		case <-ctx.Done():
			enviar(trustdialog.Rechazar)
		default:
		}
	}()

	w.ShowAndRun()

	select {
	case <-ctx.Done():
		return trustdialog.Rechazar, ctx.Err()
	case decision := <-resultado:
		return decision, nil
	}
}

func newCertSelector() certpicker.CertSelector {
	if nativeProtocolUIEnabled() {
		return newNativeWindowsCertSelector()
	}
	return selectorInteractivo{}
}

func newProgressProvider() progressdialog.ProgressProvider {
	if nativeProtocolUIEnabled() {
		return newNativeWindowsProgressProvider()
	}
	_, w := currentProtocolUIContext()
	if w != nil {
		return progressdialog.NewFyneProgressDialog(w)
	}
	return &progressdialog.HeadlessProvider{}
}

func newTrustPolicy(configDir string) (ports.TrustPolicy, error) {
	trustdialog.SetConfigDir(configDir)
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, err
	}
	policy, err := config.LoadPolicy(config.DirPolicyDefecto)
	if err != nil {
		return nil, err
	}
	base, err := truststore.NewWithOptions(configDir, truststore.Options{
		SystemAllowlistFile: truststore.SystemAllowlistPath,
		Headless:            false,
		TOFUEnabled:         cfg.TofuHabilitado,
		ExtraAllowed:        policy.DominiosDeConfianza,
	})
	if err != nil {
		return nil, err
	}
	if nativeProtocolUIEnabled() {
		return trustdialog.New(base, newNativeWindowsTrustUI()), nil
	}
	_, w := currentProtocolUIContext()
	if w != nil {
		return trustdialog.New(base, trustdialog.NewFyneTrustDialog(w)), nil
	}
	return trustdialog.New(base, trustUIFyne{}), nil
}

func newLegacyDocumentPicker() ports.DocumentPicker {
	if nativeProtocolUIEnabled() {
		return newNativeWindowsDocumentPicker()
	}
	return desktopdocumentpicker.NewFyneWithParent(func() fyne.Window {
		_, parent := currentProtocolUIContext()
		return parent
	})
}
