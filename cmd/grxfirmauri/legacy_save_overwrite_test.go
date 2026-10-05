// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Sin la confirmación del diálogo del sistema, guardar desde un portal nunca
// reemplaza un fichero que ya existe.
func TestWriteLegacySavedFile_SoloReemplazaConConfirmacion(t *testing.T) {
	dir := t.TempDir()
	destino := filepath.Join(dir, "firmado.pdf")
	if err := os.WriteFile(destino, []byte("previo"), 0o600); err != nil {
		t.Fatal(err)
	}

	guardado, err := writeLegacySavedFile(destino, []byte("nuevo"), false)
	if err != nil {
		t.Fatalf("sin confirmación: %v", err)
	}
	if guardado == destino {
		t.Fatal("sin confirmación devolvió la ruta original")
	}
	if got, _ := os.ReadFile(destino); string(got) != "previo" {
		t.Fatalf("se reemplazó sin confirmación: %q", got)
	}
	if got, _ := os.ReadFile(guardado); string(got) != "nuevo" {
		t.Fatalf("contenido en la ruta devuelta = %q", got)
	}

	guardado, err = writeLegacySavedFile(destino, []byte("confirmado"), true)
	if err != nil || guardado != destino {
		t.Fatalf("con confirmación: ruta=%q err=%v", guardado, err)
	}
	if got, _ := os.ReadFile(destino); string(got) != "confirmado" {
		t.Fatalf("la confirmación no reemplazó: %q", got)
	}
}

func TestPickLegacySaveTarget_PickerInyectadoNoConfirmaPorDefecto(t *testing.T) {
	h := &legacyWebSocketHandler{savePicker: acceptLegacySaveDefaultForTest}
	if _, confirmed, err := h.pickLegacySaveTarget(t.Context(), filepath.Join(t.TempDir(), "x.pdf"), ".pdf"); err != nil || confirmed {
		t.Fatalf("confirmed=%v err=%v", confirmed, err)
	}
}
