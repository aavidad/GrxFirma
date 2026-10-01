// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"grxfirma/internal/appdirs"
	"os"
	"strings"

	"fyne.io/fyne/v2"

	"grxfirma/internal/adapters/outbound/desktop/usersettings"
)

type legacyUISizePreset struct {
	key         string
	scale       string
	launchSize  fyne.Size
	serviceSize fyne.Size
	errorSize   fyne.Size
}

func aplicarEscalaProtocolUI(configDir string) {
	if strings.TrimSpace(os.Getenv("FYNE_SCALE")) != "" {
		return
	}
	preset := loadLegacyUISizePreset(configDir)
	_ = os.Setenv("FYNE_SCALE", preset.scale)
}

func loadLegacyUISizePreset(configDir string) legacyUISizePreset {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("GRXFIRMA_LEGACY_UI_SIZE")))
	if raw == "" {
		desktop, err := usersettings.CargarDesktopCompat(context.Background(), resolveLegacyConfigDir(configDir))
		if err == nil && desktop.LegacyWebUISize != nil {
			raw = strings.ToLower(strings.TrimSpace(*desktop.LegacyWebUISize))
		}
	}
	switch raw {
	case "normal", "pequeno", "pequeño", "s":
		return legacyUISizePreset{
			key:         "normal",
			scale:       "1.15",
			launchSize:  fyne.NewSize(1080, 560),
			serviceSize: fyne.NewSize(900, 420),
			errorSize:   fyne.NewSize(1080, 700),
		}
	case "extra", "extra-grande", "extra_grande", "xl":
		return legacyUISizePreset{
			key:         "extra",
			scale:       "1.5",
			launchSize:  fyne.NewSize(1500, 860),
			serviceSize: fyne.NewSize(1200, 620),
			errorSize:   fyne.NewSize(1500, 920),
		}
	default:
		return legacyUISizePreset{
			key:         "grande",
			scale:       "1.3",
			launchSize:  fyne.NewSize(1280, 760),
			serviceSize: fyne.NewSize(1040, 520),
			errorSize:   fyne.NewSize(1280, 820),
		}
	}
}

func resolveLegacyConfigDir(configDir string) string {
	if strings.TrimSpace(configDir) != "" {
		return strings.TrimSpace(configDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return appdirs.Config(home)
}
