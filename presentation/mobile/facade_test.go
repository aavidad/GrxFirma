// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobile

import "testing"

func TestViewModelOperacionTransitions(t *testing.T) {
	t.Parallel()

	vm := NuevaOperacionFirma("demo.pdf")
	if vm.Estado != EstadoReposo {
		t.Fatalf("Estado inicial = %s, want %s", vm.Estado, EstadoReposo)
	}
	vm = MarcarProcesando(vm, "Firmando")
	if vm.Estado != EstadoProcesando {
		t.Fatalf("Estado procesando = %s", vm.Estado)
	}
	vm = MarcarExito(vm, "Completado")
	if vm.Estado != EstadoExito {
		t.Fatalf("Estado exito = %s", vm.Estado)
	}
}
