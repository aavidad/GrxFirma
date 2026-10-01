// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package wintaskdialog muestra diálogos nativos de Windows (TaskDialog) con
// botones y lista de opciones, sin lanzar procesos auxiliares como
// PowerShell. Requiere que el ejecutable declare Common Controls v6 en su
// manifiesto; si no está disponible, Available devuelve false.
package wintaskdialog

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
}

// IDCancel es el identificador que devuelve Windows al cancelar.
const IDCancel int32 = 2
