// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui && linux

package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func focusPortalActionWindow(uint64) {}

// Fyne no expone la posición ni una variante de Show que preserve el foco.
// X11 permite ambas operaciones mediante xdotool. Si no está disponible,
// la ventana sigue siendo compacta y no solicita el foco explícitamente.
func capturePortalForeground() string {
	if os.Getenv("DISPLAY") == "" {
		return ""
	}
	return portalWindowID(portalXDoTool("getactivewindow"))
}

func placePortalWaitingWindow(previous string, generation uint64) {
	if os.Getenv("DISPLAY") == "" {
		return
	}
	go func() {
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) && portalWaitingGeneration.Load() == generation {
			window := portalOwnWindow()
			if window != "" {
				if portalWaitingGeneration.Load() != generation {
					return
				}
				positionPortalWindow(window, generation)
				if portalWaitingGeneration.Load() == generation {
					restorePortalForegroundIf(previous, func() bool {
						return portalWaitingGeneration.Load() == generation
					})
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

func portalOwnWindow() string {
	output := portalXDoTool("search", "--onlyvisible", "--pid", strconv.Itoa(os.Getpid()), "--name", "GrxFirma")
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if id := portalWindowID(line); id != "" {
			return id
		}
	}
	return ""
}

func positionPortalWindow(window string, generation uint64) {
	display := strings.Fields(portalXDoTool("getdisplaygeometry"))
	if len(display) != 2 {
		return
	}
	screenWidth, errW := strconv.Atoi(display[0])
	screenHeight, errH := strconv.Atoi(display[1])
	geometry := portalXDoTool("getwindowgeometry", "--shell", window)
	width, height := 0, 0
	for _, line := range strings.Split(geometry, "\n") {
		if strings.HasPrefix(line, "WIDTH=") {
			width, _ = strconv.Atoi(strings.TrimPrefix(line, "WIDTH="))
		}
		if strings.HasPrefix(line, "HEIGHT=") {
			height, _ = strconv.Atoi(strings.TrimPrefix(line, "HEIGHT="))
		}
	}
	if errW != nil || errH != nil || screenWidth <= 0 || screenHeight <= 0 || width <= 0 || height <= 0 {
		return
	}
	x, y := screenWidth-width-24, screenHeight-height-64
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	portalWaitingEffectMu.Lock()
	if portalWaitingGeneration.Load() == generation {
		portalXDoTool("windowmove", window, strconv.Itoa(x), strconv.Itoa(y))
	}
	portalWaitingEffectMu.Unlock()
}

func restorePortalForeground(previous string) {
	restorePortalForegroundIf(previous, nil)
}

func restorePortalForegroundIf(previous string, stillWaiting func() bool) {
	if previous == "" || os.Getenv("DISPLAY") == "" {
		return
	}
	active := capturePortalForeground()
	if active == previous {
		return
	}
	// No se interrumpe una aplicación distinta que el usuario haya abierto
	// mientras GrxFirma trabajaba.
	if active != "" && strings.TrimSpace(portalXDoTool("getwindowpid", active)) != strconv.Itoa(os.Getpid()) {
		return
	}
	portalWaitingEffectMu.Lock()
	if stillWaiting == nil || stillWaiting() {
		portalXDoTool("windowactivate", previous)
	}
	portalWaitingEffectMu.Unlock()
}

func portalWindowID(raw string) string {
	id := strings.TrimSpace(raw)
	if _, err := strconv.ParseUint(id, 10, 64); err != nil || id == "0" {
		return ""
	}
	return id
}

func portalXDoTool(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "xdotool", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
