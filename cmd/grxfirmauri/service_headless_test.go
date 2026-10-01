// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package main

import (
	"context"
	"strings"
	"testing"
)

func TestAprobacionHeadless_FallaCerrado(t *testing.T) {
	t.Parallel()

	approved, err := newSignApproval().Request(context.Background(), "firmar")
	if err == nil || !strings.Contains(err.Error(), "no hay interfaz gráfica") {
		t.Fatalf("Request() error = %v, want unavailable UI", err)
	}
	if approved {
		t.Fatal("Request() approved = true, want false")
	}
}
