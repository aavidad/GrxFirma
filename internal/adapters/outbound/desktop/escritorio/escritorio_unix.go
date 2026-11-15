// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows && !darwin

package escritorio

import (
	"os"
	"strings"
)

// hayEscritorioSistema mira si la sesión gráfica está anunciada. Sin DISPLAY
// ni WAYLAND_DISPLAY (SSH, systemd, cron) ningún diálogo puede mostrarse.
func hayEscritorioSistema() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) != "" || strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != ""
}
