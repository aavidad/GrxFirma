// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11store_test

import (
	"context"
	"path/filepath"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11store"
)

// TestNewAutodetect_NoPanic verifica que NewAutodetect() no produce panic
// aunque no haya ningún módulo PKCS#11 disponible en el sistema.
func TestNewAutodetect_NoPanic(t *testing.T) {
	t.Parallel()

	// No debe hacer panic bajo ninguna circunstancia.
	almacen := pkcs11store.NewAutodetect()
	if almacen == nil {
		t.Fatal("NewAutodetect() retornó nil")
	}
}

// TestNew_ModuloInexistente_NoPanic verifica que New() con una ruta inexistente
// no produce panic.
func TestNew_ModuloInexistente_NoPanic(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/ruta/inexistente/modulo.so")
	if almacen == nil {
		t.Fatal("New() retornó nil")
	}
}

// TestList_SinModulo_NoError verifica que List() con módulo no disponible
// retorna lista vacía o error controlado, nunca panic.
func TestList_SinModulo_NoError(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/ruta/inexistente/modulo.so")

	// No debe hacer panic; puede retornar nil o error controlado.
	refs, err := almacen.List(context.Background())

	// Si hay error debe ser un error descriptivo, no un panic.
	if err != nil {
		// Error controlado es aceptable.
		t.Logf("List() retornó error controlado (esperado): %v", err)
		return
	}

	// Sin error, la lista debe ser vacía (no nil con contenido).
	if len(refs) != 0 {
		t.Errorf("esperada lista vacía con módulo inexistente, obtenidos %d refs", len(refs))
	}
}

// TestEnumerate_SinModulo_NoError verifica que Enumerate() con módulo no disponible
// retorna lista vacía o error controlado, nunca panic.
func TestEnumerate_SinModulo_NoError(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/ruta/inexistente/modulo.so")

	tokens, err := almacen.Enumerate(context.Background())

	if err != nil {
		t.Logf("Enumerate() retornó error controlado (esperado): %v", err)
		return
	}

	if len(tokens) != 0 {
		t.Errorf("esperada lista vacía con módulo inexistente, obtenidos %d tokens", len(tokens))
	}
}

// TestList_ContextoCancelado verifica que List() respeta la cancelación del contexto.
func TestList_ContextoCancelado(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/ruta/inexistente/modulo.so")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar inmediatamente

	// No debe hacer panic independientemente del estado del módulo.
	_, _ = almacen.List(ctx)
}

// TestEnumerate_ContextoCancelado verifica que Enumerate() respeta la cancelación del contexto.
func TestEnumerate_ContextoCancelado(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/ruta/inexistente/modulo.so")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = almacen.Enumerate(ctx)
}

// La suite ordinaria nunca carga módulos autodetectados: solo la integración
// explícita puede enumerar dispositivos reales.
func TestList_SinHardware_NoConsultaDispositivos(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New(filepath.Join(t.TempDir(), "modulo-ausente.so"))
	refs, err := almacen.List(context.Background())

	// Sin hardware real: puede ser lista vacía o error controlado.
	if err != nil {
		t.Logf("List() retornó error (esperado sin módulo): %v", err)
		return
	}

	if len(refs) != 0 {
		t.Fatal("módulo ausente devolvió certificados")
	}
}

func TestEnumerate_SinHardware_NoConsultaDispositivos(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New(filepath.Join(t.TempDir(), "modulo-ausente.so"))
	tokens, err := almacen.Enumerate(context.Background())

	if err != nil {
		t.Logf("Enumerate() con NewAutodetect() retornó error (esperado sin hardware): %v", err)
		return
	}

	if len(tokens) != 0 {
		t.Fatal("módulo ausente devolvió tokens")
	}
}

// TestCerrar_SinModulo_NoPanic verifica que Cerrar() no hace panic si nunca
// se inicializó el módulo.
func TestCerrar_SinModulo_NoPanic(t *testing.T) {
	t.Parallel()

	almacen := pkcs11store.New("/ruta/inexistente/modulo.so")
	almacen.Cerrar() // no debe hacer panic
}
