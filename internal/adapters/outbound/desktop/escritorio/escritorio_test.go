// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package escritorio

import (
	"errors"
	"testing"
)

func TestComprobarSinEscritorioDevuelveErrorTipado(t *testing.T) {
	t.Cleanup(SustituirDetector(func() bool { return false }))
	err := Comprobar("prueba")
	if !errors.Is(err, ErrSinEscritorio) {
		t.Fatalf("Comprobar sin escritorio = %v", err)
	}
	var tipado *Error
	if !errors.As(err, &tipado) || tipado.Dialogo != "prueba" || tipado.ClaveMensaje() != ClaveMensaje {
		t.Fatalf("error sin tipo o sin clave: %#v", err)
	}
	if Hay() {
		t.Fatal("Hay() con el detector sustituido dice que hay escritorio")
	}
}

func TestComprobarConEscritorioNoFalla(t *testing.T) {
	t.Cleanup(SustituirDetector(func() bool { return true }))
	if err := Comprobar("prueba"); err != nil {
		t.Fatalf("Comprobar con escritorio = %v", err)
	}
}

func TestSustituirDetectorRestaura(t *testing.T) {
	original := Hay()
	restaurar := SustituirDetector(func() bool { return !original })
	if Hay() == original {
		t.Fatal("el detector no se sustituyó")
	}
	restaurar()
	if Hay() != original {
		t.Fatal("el detector no se restauró")
	}
}
