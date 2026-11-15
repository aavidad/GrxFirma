// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
	"grxfirma/presentation/desktop/trustdialog"
)

// sinEscritorio sustituye el detector común durante la prueba.
func sinEscritorio(t *testing.T) {
	t.Helper()
	previo := comprobarEscritorioUI
	comprobarEscritorioUI = escritorio.Comprobar
	t.Cleanup(func() { comprobarEscritorioUI = previo })
	t.Cleanup(escritorio.SustituirDetector(func() bool { return false }))
}

// enMenosDeUnSegundo falla si la llamada espera: sin escritorio ningún
// diálogo puede quedarse abierto.
func enMenosDeUnSegundo(t *testing.T, nombre string, llamada func() error) error {
	t.Helper()
	hecho := make(chan error, 1)
	go func() { hecho <- llamada() }()
	select {
	case err := <-hecho:
		return err
	case <-time.After(time.Second):
		t.Fatalf("%s se quedó esperando sin escritorio", nombre)
		return nil
	}
}

func TestSinEscritorioLaInterfazDelProtocoloFallaCerradaSinFallbackNiAviso(t *testing.T) {
	sinEscritorio(t)
	previoCheck, previoAviso, previoFallback := checkPlatformGraphicsReadiness, showPlatformGraphicsFailure, activateNativeProtocolFallback
	t.Cleanup(func() {
		checkPlatformGraphicsReadiness, showPlatformGraphicsFailure, activateNativeProtocolFallback = previoCheck, previoAviso, previoFallback
	})
	var avisos, fallbacks int
	showPlatformGraphicsFailure = func(string) error { avisos++; return nil }
	activateNativeProtocolFallback = func(error) bool { fallbacks++; return true }

	for nombre, preparar := range map[string]func(*bytes.Buffer) error{
		"prepareProtocolInteractiveUI": func(b *bytes.Buffer) error { return prepareProtocolInteractiveUI(b) },
		"requireInteractiveGraphics":   func(b *bytes.Buffer) error { return requireInteractiveGraphics(b) },
	} {
		var stderr bytes.Buffer
		if err := preparar(&stderr); !errors.Is(err, escritorio.ErrSinEscritorio) {
			t.Fatalf("%s sin escritorio = %v", nombre, err)
		}
		if !strings.Contains(stderr.String(), tl(escritorio.ClaveMensaje)) {
			t.Fatalf("%s no explica el motivo con el catálogo: %q", nombre, stderr.String())
		}
	}
	if avisos != 0 || fallbacks != 0 {
		t.Fatalf("sin escritorio se abrió un aviso (%d) o la interfaz nativa (%d)", avisos, fallbacks)
	}
}

func TestSinEscritorioLaURINoSeEjecuta(t *testing.T) {
	sinEscritorio(t)
	llamado := false
	var stderr bytes.Buffer
	codigo := ejecutarURIProtocolo(context.Background(), &stderr, "afirma://sign?id=1", func(context.Context, io.Writer, string) int {
		llamado = true
		return 0
	})
	if codigo != graphicsUnavailableExitCode || llamado {
		t.Fatalf("sin escritorio: código=%d manejador llamado=%v", codigo, llamado)
	}
}

func TestSinEscritorioLosDialogosDelProtocoloNoEsperanNiAceptan(t *testing.T) {
	sinEscritorio(t)
	ctx := context.Background()

	err := enMenosDeUnSegundo(t, "selectLegacyLoadPaths", func() error {
		rutas, err := selectLegacyLoadPaths(ctx, "", "pdf", false)
		if len(rutas) != 0 {
			t.Fatalf("sin escritorio se devolvieron rutas: %q", rutas)
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("selector de carga = %v", err)
	}
	err = enMenosDeUnSegundo(t, "selectLegacySaveTargetPath", func() error {
		ruta, err := selectLegacySaveTargetPath(ctx, "", "pdf")
		if ruta != "" {
			t.Fatalf("sin escritorio se devolvió destino: %q", ruta)
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("selector de guardado = %v", err)
	}
	err = enMenosDeUnSegundo(t, "trustUIFyne", func() error {
		decision, err := trustUIFyne{}.PedirDecision(ctx, "https://sede.example")
		if decision != trustdialog.Rechazar {
			t.Fatalf("sin escritorio la decisión no es Rechazar: %v", decision)
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("confianza del origen = %v", err)
	}
	err = enMenosDeUnSegundo(t, "solicitarPasswordTemporalFyne", func() error {
		clave, err := solicitarPasswordTemporalFyne(ctx)
		if len(clave) != 0 {
			t.Fatal("sin escritorio se devolvió una contraseña")
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("contraseña del P12 = %v", err)
	}
	_ = enMenosDeUnSegundo(t, "mostrarAvisoCredencialTemporal", func() error {
		mostrarAvisoCredencialTemporal(ctx, "título", "detalle")
		return nil
	})
	if err := checkPlatformGraphicsReadiness(); !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("checkPlatformGraphicsReadiness sin escritorio = %v", err)
	}
}
