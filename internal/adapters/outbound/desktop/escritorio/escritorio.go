// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package escritorio dice si el proceso tiene un escritorio que alguien ve.
// Es el punto común que consultan los diálogos nativos antes de abrirse: en un
// servicio, por SSH o en una tarea programada no interactiva, una ventana se
// abriría donde nadie puede cerrarla y la operación esperaría para siempre.
//
// Sin escritorio, Comprobar devuelve un *Error que cumple
// errors.Is(err, ErrSinEscritorio). Quien lo recibe no puede suponer ninguna
// respuesta de la persona: una confirmación se da por no concedida y la
// operación sale denegada.
package escritorio

import (
	"errors"
	"log/slog"
	"sync"
)

// ClaveMensaje es la clave del catálogo con la explicación para la persona.
const ClaveMensaje = "escritorio.sin_escritorio"

// ErrSinEscritorio indica que no hay un escritorio visible y que el diálogo no
// se ha mostrado.
var ErrSinEscritorio = errors.New("escritorio: no hay un escritorio visible para mostrar el diálogo")

// Error es el error tipado que devuelve Comprobar. Dialogo identifica el
// diálogo que no se ha abierto, para el registro; no se muestra a la persona.
type Error struct {
	Dialogo string
}

func (e *Error) Error() string {
	if e == nil || e.Dialogo == "" {
		return ErrSinEscritorio.Error()
	}
	return ErrSinEscritorio.Error() + " (" + e.Dialogo + ")"
}

// Is hace que errors.Is(err, ErrSinEscritorio) sea cierto.
func (e *Error) Is(target error) bool { return target == ErrSinEscritorio }

// ClaveMensaje devuelve la clave del catálogo que explica el error.
func (e *Error) ClaveMensaje() string { return ClaveMensaje }

var (
	mu       sync.RWMutex
	detector = hayEscritorioSistema
)

// Hay dice si el proceso tiene un escritorio visible.
func Hay() bool {
	mu.RLock()
	detectar := detector
	mu.RUnlock()
	return detectar()
}

// Comprobar devuelve nil si hay escritorio. Si no lo hay, deja constancia en
// el registro y devuelve un *Error; dialogo es un identificador técnico.
func Comprobar(dialogo string) error {
	if Hay() {
		return nil
	}
	slog.Warn("dialogo_nativo_sin_escritorio", "dialogo", dialogo)
	return &Error{Dialogo: dialogo}
}

// SustituirDetector cambia la detección, para las pruebas, y devuelve la
// función que deja la anterior.
func SustituirDetector(hay func() bool) (restaurar func()) {
	mu.Lock()
	previo := detector
	detector = hay
	mu.Unlock()
	return func() {
		mu.Lock()
		detector = previo
		mu.Unlock()
	}
}
