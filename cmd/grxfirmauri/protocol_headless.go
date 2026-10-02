// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
	"io"
)

func hasLegacyProtocolWindow() bool { return false }

func portalDeliveredUI(string) {}

func portalErrorUI() {}

func maybeRunProtocolUI(context.Context, io.Writer, string) (bool, int) {
	return false, 0
}

func waitLegacyLaunchUI(ctx context.Context, _ context.CancelFunc, _ string, _ string, _ string, _ string) int {
	<-ctx.Done()
	return 0
}
