// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package wintaskdialog

import (
	"context"
	"errors"
)

// Available siempre es false fuera de Windows.
func Available() bool { return false }

// Interactive siempre es false fuera de Windows.
func Interactive() bool { return false }

// Show no está disponible fuera de Windows.
func Show(context.Context, Spec) (int32, int32, error) {
	return 0, 0, errors.New("TaskDialog solo está disponible en Windows")
}

// ShowVerification no está disponible fuera de Windows.
func ShowVerification(context.Context, Spec) (int32, int32, bool, error) {
	return 0, 0, false, errors.New("TaskDialog solo está disponible en Windows")
}

// CheckedRadio no está disponible fuera de Windows.
func CheckedRadio(uintptr, []int32, []string) int32 { return 0 }
