// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package wintaskdialog muestra diálogos nativos de Windows (TaskDialog) con
// botones y lista de opciones, sin lanzar procesos auxiliares como
// PowerShell. Requiere que el ejecutable declare Common Controls v6 en su
// manifiesto; si no está disponible, Available devuelve false.
package wintaskdialog

import "grxfirma/internal/adapters/outbound/desktop/escritorio"

// Choice es un botón o una opción de radio del diálogo.
type Choice struct {
	ID    int32
	Label string
}

// Spec describe el diálogo.
type Spec struct {
	Title            string
	Instruction      string
	Content          string
	Buttons          []Choice
	DefaultButton    int32
	Radios           []Choice
	AcceptNeedsRadio int32
	// Verification es el texto de la casilla bajo los botones (por ejemplo,
	// «Recordar mi elección»). Vacío: sin casilla. Su estado lo devuelve
	// ShowVerification.
	Verification string
}

// ErrSinEscritorio indica que el proceso no tiene un escritorio visible
// (servicio, SSH, CI) y el diálogo no se ha mostrado. Quien lo recibe no puede
// suponer ninguna respuesta de la persona: un aviso sigue sin ventana y una
// confirmación se da por no concedida. Es el mismo error que el del paquete
// escritorio, que comparten todos los diálogos nativos.
var ErrSinEscritorio = escritorio.ErrSinEscritorio

// IDCancel es el identificador que devuelve Windows al cancelar.
const IDCancel int32 = 2
