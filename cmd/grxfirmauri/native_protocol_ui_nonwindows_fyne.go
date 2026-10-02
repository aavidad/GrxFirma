// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui && !windows

package main

// El protocolo Fyne invoca estos puntos también en las plataformas sin
// interfaz nativa de Windows. nativeProtocolUIEnabled siempre es false allí.
func nativePortalErrorUI()               {}
func nativePortalDeliveredUI(string)     {}
func setNativeProtocolCompletionUI(bool) {}
