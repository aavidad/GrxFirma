// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wintaskdialog

import "unsafe"

const (
	uoiFlags   = 1
	wsfVisible = 0x0001
)

var (
	procProcessWindowStation = user32.NewProc("GetProcessWindowStation")
	procUserObjectInfo       = user32.NewProc("GetUserObjectInformationW")
)

// Interactive dice si el proceso tiene un escritorio que alguien ve. En una
// sesión de servicio, por SSH o en un agente de CI, la estación de ventanas
// no es visible: un diálogo se abriría donde nadie puede cerrarlo y la
// operación esperaría para siempre. Si no se puede saber, se supone que sí,
// para no ocultar un aviso a la persona.
func Interactive() bool {
	station, _, _ := procProcessWindowStation.Call()
	if station == 0 {
		return true
	}
	var flags struct{ Inherit, Reserved, Flags uint32 }
	var needed uint32
	ok, _, _ := procUserObjectInfo.Call(station, uoiFlags, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 {
		return true
	}
	return flags.Flags&wsfVisible != 0
}
