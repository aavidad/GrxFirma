// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package escritorio

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	uoiFlags   = 1
	wsfVisible = 0x0001
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procProcessWindowStation = user32.NewProc("GetProcessWindowStation")
	procUserObjectInfo       = user32.NewProc("GetUserObjectInformationW")
)

// hayEscritorioSistema mira si la estación de ventanas del proceso es visible.
// Por SSH, en un servicio o en la sesión 0 no lo es. Si la consulta falla se
// supone que sí, para no ocultar un aviso a la persona.
func hayEscritorioSistema() bool {
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
