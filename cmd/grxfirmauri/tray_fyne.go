// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"

	"fyne.io/fyne/v2"

	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/ports"
)

type trayResidentOption struct {
	configDir string
	enabled   bool
}

func newTrayResidentOption(configDir string) *trayResidentOption {
	return &trayResidentOption{
		configDir: configDir,
		enabled:   loadLegacyTrayResidentSetting(configDir),
	}
}

func (o *trayResidentOption) Enabled() bool {
	return o != nil && o.enabled
}

func (o *trayResidentOption) Set(enabled bool) {
	if o == nil {
		return
	}
	o.enabled = enabled
	_ = saveLegacyTrayResidentSetting(o.configDir, enabled)
}

func loadLegacyTrayResidentSetting(configDir string) bool {
	desktop, err := usersettings.CargarDesktopCompat(context.Background(), configDir)
	if err == nil && desktop.LegacyWebTrayResident != nil {
		return *desktop.LegacyWebTrayResident
	}
	return false
}

func saveLegacyTrayResidentSetting(configDir string, enabled bool) error {
	return usersettings.ActualizarDesktopCompat(context.Background(), configDir, func(desktop *ports.ConfiguracionUsuarioDesktop) {
		desktop.LegacyWebTrayResident = &enabled
	})
}

func installLegacyTrayMenu(
	a fyne.App,
	w fyne.Window,
	cancel context.CancelFunc,
) (bool, func()) {
	if !legacyTrayMenuSupported(a) || w == nil {
		return false, func() {}
	}
	driver := a.Driver().(interface{ SetSystemTrayMenu(*fyne.Menu) })

	showItem := fyne.NewMenuItem(tl("Abrir GrxFirma"), func() {
		fyne.Do(func() {
			w.Show()
			w.RequestFocus()
		})
	})
	statusItem := fyne.NewMenuItem(tl("Firmas desde portales: activo"), nil)
	statusItem.Disabled = true
	stopItem := fyne.NewMenuItem(tl("Salir"), func() {
		if cancel != nil {
			cancel()
		}
		fyne.Do(func() {
			w.SetCloseIntercept(nil)
			w.Close()
		})
	})
	menu := fyne.NewMenu(tl("GrxFirma"), showItem, statusItem, fyne.NewMenuItemSeparator(), stopItem)
	driver.SetSystemTrayMenu(menu)
	return true, func() {
		// Limpiar el tray explícitamente durante el cierre del proceso provoca
		// pánicos intermitentes en el driver GLFW de Fyne. Al terminar el proceso
		// el icono desaparece igualmente, así que evitamos tocar el tray aquí.
	}
}

func legacyTrayMenuSupported(a fyne.App) bool {
	if a == nil || a.Driver() == nil {
		return false
	}
	_, ok := a.Driver().(interface{ SetSystemTrayMenu(*fyne.Menu) })
	return ok
}
