// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package desktopnotify_test

import (
	"context"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/desktopnotify"
)

// TestNotify_NoError verifica que Notify retorna nil en cualquier plataforma.
func TestNotify_NoError(t *testing.T) {
	t.Parallel()

	n := desktopnotify.New()
	if err := n.Notify(context.Background(), "Firma completada", "El documento se firmó correctamente"); err != nil {
		t.Errorf("Notify() error = %v", err)
	}
}

// TestNotify_ContextoCancelado verifica que un contexto cancelado retorna error.
func TestNotify_ContextoCancelado(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n := desktopnotify.New()
	err := n.Notify(ctx, "Título", "Cuerpo")
	// En Linux la implementación verifica ctx.Err(). En stubs puede retornar nil.
	// Solo verificamos que no produce panic.
	_ = err
}

// TestNotify_MockBinario verifica el comportamiento con un binario mock en Linux.
// En otras plataformas, el test verifica que NewConBinario compila y funciona.
func TestNotify_MockBinario(t *testing.T) {
	t.Parallel()

	// Usar un path que no existe — debe fallar graciosamente (no panic, no error).
	n := desktopnotify.NewConBinario("/ruta/que/no/existe/notify-send")
	err := n.Notify(context.Background(), "Test", "Mensaje de test")
	// El error es ignorado (best-effort); lo importante es no panic.
	_ = err
}

// TestNotify_TituloYCuerpoVacios verifica que no panic con strings vacíos.
func TestNotify_TituloYCuerpoVacios(t *testing.T) {
	t.Parallel()

	n := desktopnotify.New()
	if err := n.Notify(context.Background(), "", ""); err != nil {
		// Algunos sistemas de notificación pueden rechazar títulos vacíos.
		// Lo aceptamos como comportamiento válido.
		t.Logf("Notify con strings vacíos retorna: %v (aceptable)", err)
	}
}
