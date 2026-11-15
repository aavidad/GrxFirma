// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile

import (
	"errors"
	"io/fs"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

// Claves del catálogo para los fallos de ajustes que la persona puede
// resolver. KeyNoPermission depende del sistema (permissions_*.go).
const (
	// KeyOtherUser: la carpeta de ajustes es de otra cuenta y no se escribe.
	KeyOtherUser = "error.settings_otro_usuario"
)

// Error es un fallo de ajustes con explicación y remedio en el catálogo. Su
// texto sale del catálogo en el idioma del proceso; Err conserva la causa
// técnica para errors.Is y el registro.
type Error struct {
	Key string
	Err error
}

func (e *Error) Error() string { return localizador.Detectar().T(e.Key) }
func (e *Error) Unwrap() error { return e.Err }

// ExplainAccess cambia un «acceso denegado» al abrir o escribir los ajustes
// por un Error que dice qué pasa y cómo arreglarlo. El resto de errores, y
// uno ya explicado, salen tal cual.
func ExplainAccess(err error) error {
	if err == nil {
		return nil
	}
	var explained *Error
	if errors.As(err, &explained) {
		return err
	}
	if errors.Is(err, fs.ErrPermission) {
		return &Error{Key: KeyNoPermission, Err: err}
	}
	return err
}
