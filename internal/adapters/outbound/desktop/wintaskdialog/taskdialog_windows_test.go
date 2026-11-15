// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wintaskdialog

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Sin escritorio visible (SSH, servicio, CI) ningún diálogo espera ni se da
// por respondido.
func TestSinEscritorioNingunDialogoEspera(t *testing.T) {
	previo := interactive
	t.Cleanup(func() { interactive = previo })
	interactive = func() bool { return false }
	spec := Spec{Title: "Confirmar", Instruction: "¿Firmar?", Buttons: []Choice{{ID: 1001, Label: "Firmar"}, {ID: IDCancel, Label: "Cancelar"}}, DefaultButton: 1001}
	inicio := time.Now()
	boton, _, marcada, err := ShowVerification(context.Background(), spec)
	if !errors.Is(err, ErrSinEscritorio) || boton != 0 || marcada {
		t.Fatalf("sin escritorio: botón=%d marcada=%v err=%v", boton, marcada, err)
	}
	if time.Since(inicio) > time.Second {
		t.Fatal("el diálogo esperó sin escritorio")
	}
}

// Por SSH la estación de ventanas no es visible.
func TestInteractiveDetectaLaSesionSinEscritorio(t *testing.T) {
	if Interactive() {
		t.Skip("la prueba corre con un escritorio visible")
	}
	if _, _, err := Show(context.Background(), Spec{Title: "x", Buttons: []Choice{{ID: IDCancel, Label: "Cerrar"}}}); !errors.Is(err, ErrSinEscritorio) {
		t.Fatalf("Show sin escritorio: %v", err)
	}
}
