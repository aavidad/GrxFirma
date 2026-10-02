// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows || !fyne_gui || !amd64

package main

import (
	"context"
	"errors"
	"io"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/progressdialog"
	"grxfirma/presentation/desktop/trustdialog"
)

func enableNativeProtocolUIFallback(error) bool {
	return false
}

func disableNativeProtocolUIFallback() {}

func nativeProtocolUIEnabled() bool {
	return false
}

func nativeProtocolUIForced() bool {
	return false
}

func validateNativeProtocolOperation(afirmauri.TipoOperacion) error {
	return nil
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func newNativeWindowsApproval(string) ports.UserApproval {
	return nil
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func newNativeWindowsCertSelector() certpicker.CertSelector {
	return nil
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func newNativeWindowsTrustUI() trustdialog.TrustUIProvider {
	return nil
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func newNativeWindowsProgressProvider() progressdialog.ProgressProvider {
	return nil
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func newNativeWindowsDocumentPicker() ports.DocumentPicker {
	return nil
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func runNativeDirectProtocolUIShell(
	context.Context,
	io.Writer,
	string,
) int {
	return 1
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func waitNativeLegacyLaunchUI(
	context.Context,
	context.CancelFunc,
	string,
	string,
) int {
	return 1
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func updateNativeProtocolUI(string, string) {}

func reportNativeProtocolFailure(string, error) {}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func selectNativeWindowsLoadPaths(
	context.Context,
	string,
	string,
	bool,
) ([]string, error) {
	return nil, errors.New("selector nativo de Windows no disponible")
}

//lint:ignore U1000 Stub requerido por las combinaciones sin fallback nativo.
func selectNativeWindowsSaveTarget(
	context.Context,
	string,
	string,
) (string, error) {
	return "", errors.New("selector nativo de Windows no disponible")
}
