// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package certselectionstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sessionID combina la sesión de inicio del token (su LUID cambia al cerrar
// sesión y volver a entrar) con la hora de arranque del sistema, porque los
// LUID se vuelven a repartir desde cero tras reiniciar.
func sessionID() (string, error) {
	logon, err := logonSessionLUID()
	if err != nil {
		return "", err
	}
	boot, err := bootTime()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("windows-logon:%08x%08x:boot:%d", logon.HighPart, logon.LowPart, boot), nil
}

// logonSessionLUID lee AuthenticationId de TOKEN_STATISTICS: después de
// TokenId (un LUID de 8 bytes) viene el LUID de la sesión de inicio.
func logonSessionLUID() (windows.LUID, error) {
	var buf [128]byte
	var n uint32
	if err := windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenStatistics, &buf[0], uint32(len(buf)), &n); err != nil {
		return windows.LUID{}, err
	}
	if n < 16 {
		return windows.LUID{}, errors.New("TOKEN_STATISTICS demasiado corto")
	}
	return windows.LUID{
		LowPart:  binary.LittleEndian.Uint32(buf[8:12]),
		HighPart: int32(binary.LittleEndian.Uint32(buf[12:16])), // #nosec G115 -- reinterpreta los 4 bytes del LONG HighPart.
	}, nil
}

// systemTimeOfDay es el principio de SYSTEM_TIMEOFDAY_INFORMATION.
type systemTimeOfDay struct {
	BootTime      int64
	CurrentTime   int64
	TimeZoneBias  int64
	TimeZoneID    uint32
	Reserved      uint32
	BootTimeBias  uint64
	SleepTimeBias uint64
}

// bootTime devuelve la hora de arranque en segundos. Se redondea al segundo
// porque Windows corrige BootTime en fracciones al ajustar el reloj.
func bootTime() (int64, error) {
	var info systemTimeOfDay
	var n uint32
	if err := windows.NtQuerySystemInformation(windows.SystemTimeOfDayInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &n); err != nil { // #nosec G103 -- búfer propio del tamaño de la estructura.
		return 0, err
	}
	if info.BootTime <= 0 {
		return 0, errors.New("hora de arranque no disponible")
	}
	return info.BootTime / 10_000_000, nil
}
