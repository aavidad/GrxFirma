// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

func TestExplainAccessExplicaSoloElAccesoDenegado(t *testing.T) {
	denied := fmt.Errorf("leyendo preferencias actuales: %w", &os.PathError{Op: "open", Path: "settings.json", Err: fs.ErrPermission})
	err := ExplainAccess(denied)
	var explained *Error
	if !errors.As(err, &explained) || explained.Key != KeyNoPermission {
		t.Fatalf("ExplainAccess(%v) = %v; se esperaba la clave %s", denied, err, KeyNoPermission)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal("se perdió la causa técnica")
	}
	if got, want := err.Error(), localizador.Detectar().T(KeyNoPermission); got != want {
		t.Fatalf("texto %q; se esperaba el del catálogo %q", got, want)
	}
	if again := ExplainAccess(err); again != err {
		t.Fatalf("se volvió a envolver: %v", again)
	}
	other := errors.New("otro fallo")
	if ExplainAccess(other) != other || ExplainAccess(nil) != nil {
		t.Fatal("cambió un error que no es de permisos")
	}
}
