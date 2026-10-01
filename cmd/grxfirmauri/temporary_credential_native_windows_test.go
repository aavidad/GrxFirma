// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui && amd64

package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
	"unicode/utf16"

	"grxfirma/internal/domain"
	"grxfirma/presentation/desktop/certpicker"
)

func TestTemporaryPasswordUTF8ValidatesNativeBuffer(t *testing.T) {
	for _, value := range []string{"", "contraseña", "a🔑b"} {
		buffer := append(utf16.Encode([]rune(value)), 0)
		got, err := temporaryPasswordUTF8(buffer)
		if err != nil || string(got) != value {
			t.Fatalf("conversión UTF-16: %v", err)
		}
		clear(got)
		clear(buffer)
	}
	for _, buffer := range [][]uint16{{'a'}, {0xD800, 0}, {0xDC00, 0}} {
		got, err := temporaryPasswordUTF8(buffer)
		if err == nil || got != nil {
			t.Fatal("aceptó buffer inválido")
		}
	}
}

// Esta prueba requiere una sesión Windows reservada y un controlador UIA que
// pulse la acción indicada. No enumera almacenes ni usa identidades personales.
func TestNativeInteractiveCredentialSelector(t *testing.T) {
	if os.Getenv("GRXFIRMA_NATIVE_UI_PROBE") != "1" {
		t.Skip("sonda interactiva desactivada")
	}
	var expected error
	switch os.Getenv("GRXFIRMA_NATIVE_UI_ACTION") {
	case "load":
		expected = certpicker.ErrCargarCertificado
	case "refresh":
		expected = certpicker.ErrActualizarCertificados
	case "cancel":
		expected = certpicker.ErrSeleccionCancelada
	default:
		t.Fatal("acción QA no declarada")
	}
	for _, count := range []int{0, 1} {
		var certificates []domain.CertificateRef
		if count == 1 {
			certificates = []domain.CertificateRef{{ID: "synthetic-qa", Subject: "CN=GrxFirma Synthetic QA", Issuer: "CN=Synthetic QA", NotAfter: time.Now().Add(time.Hour)}}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_, err := (nativeWindowsCertSelector{}).Select(ctx, certificates)
		cancel()
		if !errors.Is(err, expected) {
			t.Fatalf("catálogo %d: %v", count, err)
		}
		t.Logf("catálogo=%d acción local confirmada", count)
	}
}

func TestNativeInteractiveTemporaryPasswordCancellation(t *testing.T) {
	if os.Getenv("GRXFIRMA_NATIVE_UI_PROBE") != "1" {
		t.Skip("sonda interactiva desactivada")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	password, err := solicitarPasswordTemporalNativo(ctx)
	defer clear(password)
	if !errors.Is(err, context.DeadlineExceeded) || len(password) != 0 {
		t.Fatalf("cancelación nativa de contraseña: %v", err)
	}
}
