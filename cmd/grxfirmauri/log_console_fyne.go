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
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type shellConsoleTheme struct {
	fyne.Theme
}

func (t shellConsoleTheme) Size(name fyne.ThemeSizeName) float32 {
	base := t.Theme.Size(name)
	if name == theme.SizeNameText {
		return base * 1.5
	}
	return base
}

func newShellLogConsole(ctx context.Context) fyne.CanvasObject {
	if !protocolDebugEnabled() {
		placeholder := widget.NewCard(
			tl("TRAZA DEL SISTEMA"),
			"",
			widget.NewLabel(tl("La traza detallada solo está disponible en modo debug.")),
		)
		return container.NewPadded(placeholder)
	}
	title := canvas.NewText(tl("TRAZA DEL SISTEMA (redactada)"), theme.Color(theme.ColorNameForeground))
	title.TextSize = 20
	title.TextStyle = fyne.TextStyle{Bold: true}

	logLabel := widget.NewLabel("")
	logLabel.Wrapping = fyne.TextWrapBreak

	path := resolveDebugLogPath()
	lastText := ""
	refresh := func() {
		text, err := readTailRedacted(path, 64*1024)
		if err != nil {
			text = tl("No se pudo leer el log en %s\n\n%s", path, err.Error())
		}
		if text == lastText {
			return
		}
		lastText = text
		fyne.Do(func() {
			logLabel.SetText(text)
		})
	}
	refresh()

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	pathLabel := canvas.NewText(path, theme.Color(theme.ColorNameDisabled))
	pathLabel.TextSize = 14
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.CornerRadius = 10
	scroll := container.NewVScroll(container.NewStack(
		bg,
		container.NewPadded(logLabel),
	))
	scroll.SetMinSize(fyne.NewSize(0, 320))

	content := container.NewBorder(
		container.NewVBox(title, pathLabel),
		nil,
		nil,
		nil,
		scroll,
	)
	return container.NewThemeOverride(content, shellConsoleTheme{Theme: theme.DefaultTheme()})
}

func resolveDebugLogPath() string {
	if path := strings.TrimSpace(os.Getenv("GRXFIRMA_LOG_FILE")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("GRXFIRMA_DEBUG_LOG_FILE")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/grxfirma-debug.log"
	}
	return filepath.Join(appdirs.State(home), "logs", "grxfirma-debug.log")
}

func readTailRedacted(path string, maxBytes int64) (string, error) {
	return readProtocolDebugLogTail(path, maxBytes)
}
