// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"io"
	"reflect"
)

type signingKeyCloser interface {
	Close()
}

// CloseSigningKey libera, en modo best-effort, los recursos nativos que pueda
// retener una clave obtenida mediante SigningKeyProvider.KeyFor.
//
// Acepta claves nil, claves que no necesitan cierre y las dos firmas habituales
// de Close. Un adaptador defectuoso no puede propagar un panic durante la
// limpieza ni ocultar el resultado principal de la operación.
func CloseSigningKey(key SigningKey) {
	if key == nil || isNilSigningKey(key) {
		return
	}
	defer func() {
		_ = recover()
	}()

	switch closer := key.(type) {
	case io.Closer:
		_ = closer.Close()
	case signingKeyCloser:
		closer.Close()
	}
}

func isNilSigningKey(key SigningKey) bool {
	value := reflect.ValueOf(key)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
