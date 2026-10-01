// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui && !linux && !windows

package main

func capturePortalForeground() string         { return "" }
func placePortalWaitingWindow(string, uint64) {}
func restorePortalForeground(string)          {}
func focusPortalActionWindow(uint64)          {}
