// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui && amd64

package main

import (
	"context"
	"errors"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/escritorio"
	"grxfirma/presentation/desktop/trustdialog"
)

// Sin escritorio, ningún diálogo Win32 propio de grxfirmauri se abre ni da
// un botón por pulsado: TaskDialog, selector de ficheros, CredUI y el
// MessageBox del aviso gráfico vuelven al momento con el error tipado.
func TestSinEscritorioLosDialogosNativosDeWindowsNoEsperanNiAceptan(t *testing.T) {
	sinEscritorio(t)
	ctx := context.Background()

	err := enMenosDeUnSegundo(t, "showNativeTaskDialog", func() error {
		boton, radio, err := showNativeTaskDialog(ctx, nativeDialogSpec{
			Title:         "GrxFirma",
			Instruction:   "¿Firmar?",
			Buttons:       []nativeDialogChoice{{ID: nativeButtonAccept, Label: "Firmar"}, {ID: nativeButtonCancel, Label: "Cancelar"}},
			DefaultButton: nativeButtonAccept,
		})
		if boton != 0 || radio != 0 {
			t.Fatalf("sin escritorio se dio por pulsado: botón=%d radio=%d", boton, radio)
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("TaskDialog = %v", err)
	}

	aprobado, err := newNativeWindowsApproval("GrxFirma").Request(ctx, "Firmar documento")
	if aprobado || !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("aprobación nativa: aprobado=%v err=%v", aprobado, err)
	}
	decision, err := newNativeWindowsTrustUI().PedirDecision(ctx, "https://sede.example")
	if decision != trustdialog.Rechazar || !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("confianza nativa: decisión=%v err=%v", decision, err)
	}
	if _, err := newNativeWindowsCertSelector().Select(ctx, nil); !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("selector nativo = %v", err)
	}
	if _, _, err := (nativeWindowsCertSelector{}).ElegirPosicionSello(ctx); !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("posición del sello nativa = %v", err)
	}

	err = enMenosDeUnSegundo(t, "runNativeFileDialog", func() error {
		rutas, err := selectNativeWindowsLoadPaths(ctx, "", "pdf", false)
		if len(rutas) != 0 {
			t.Fatalf("sin escritorio se devolvieron rutas: %q", rutas)
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("selector de ficheros nativo = %v", err)
	}
	if _, err := selectNativeWindowsSaveTarget(ctx, "", "pdf"); !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("selector de guardado nativo = %v", err)
	}

	err = enMenosDeUnSegundo(t, "solicitarPasswordTemporalNativo", func() error {
		clave, err := solicitarPasswordTemporalNativo(ctx)
		if len(clave) != 0 {
			t.Fatal("sin escritorio se devolvió una contraseña")
		}
		return err
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("CredUI = %v", err)
	}

	err = enMenosDeUnSegundo(t, "platformGraphicsFailureDialog", func() error {
		return platformGraphicsFailureDialog("mensaje")
	})
	if !errors.Is(err, escritorio.ErrSinEscritorio) {
		t.Fatalf("MessageBox del aviso gráfico = %v", err)
	}
}
