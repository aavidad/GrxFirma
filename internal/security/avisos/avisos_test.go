// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package avisos

import (
	"fmt"
	"testing"
)

func TestRegistrarGuardaSinRepetirYConTope(t *testing.T) {
	Reiniciar()
	t.Cleanup(Reiniciar)
	Registrar("Título", "motivo")
	Registrar("Título", "motivo")
	if got := len(Todos()); got != 1 {
		t.Fatalf("avisos = %d, want 1 (sin repetir)", got)
	}
	for i := 0; i < 3*maxAvisos; i++ {
		Registrar(fmt.Sprintf("Aviso %d", i), "")
	}
	if got := len(Todos()); got != maxAvisos {
		t.Fatalf("avisos = %d, want el tope %d", got, maxAvisos)
	}
}

// Lote 17b (3d): en el motor de escritorio nadie lee la lista global. Con
// SoloEnRegistro deja de guardar: no se llena con avisos de operaciones
// distintas y lo que hubiera se vacía.
func TestSoloEnRegistroNoGuardaNiMezclaOperaciones(t *testing.T) {
	Reiniciar()
	t.Cleanup(func() {
		soloRegistro.Store(false)
		Reiniciar()
	})
	Registrar("Aviso de una operación anterior", "")
	SoloEnRegistro()
	if got := Todos(); len(got) != 0 {
		t.Fatalf("SoloEnRegistro debe vaciar la lista: %+v", got)
	}
	for i := 0; i < 3*maxAvisos; i++ {
		Registrar(fmt.Sprintf("Aviso %d", i), "motivo")
	}
	if got := Todos(); len(got) != 0 {
		t.Fatalf("con SoloEnRegistro no se guarda nada: %d avisos", len(got))
	}
	if Texto() != "" || ConAvisos("texto") != "texto" {
		t.Fatal("sin avisos guardados no se añade nada al texto de la interfaz")
	}
}
