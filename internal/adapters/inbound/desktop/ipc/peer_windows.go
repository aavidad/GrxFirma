// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/sys/windows"
)

type conexionConDescriptor interface {
	Fd() uintptr
}

// verificarPeer obtiene del handle servidor del named pipe el PID del cliente
// que Windows acredito. La DACL de sesion sigue siendo una primera barrera,
// pero el PID permite vincular el backend al frontend exacto.
func verificarPeer(conn net.Conn) (uint32, error) {
	conDescriptor, ok := conn.(conexionConDescriptor)
	if !ok {
		return 0, errors.New("la conexion named pipe no expone un descriptor verificable")
	}
	handle := windows.Handle(conDescriptor.Fd())
	if handle == 0 || handle == windows.InvalidHandle {
		return 0, errors.New("handle named pipe no valido")
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(handle, &pid); err != nil {
		return 0, fmt.Errorf("obteniendo PID del cliente named pipe: %w", err)
	}
	if pid == 0 {
		return 0, errors.New("Windows devolvio un PID de cliente vacio")
	}
	return pid, nil
}

func soportaVinculacionPIDPeer() bool { return true }
