// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package ipc

import (
	"fmt"
	"math"
	"net"
	"os"
	"syscall"
)

// verificarPeer comprueba via SO_PEERCRED que el proceso que se conecta al
// socket tiene el mismo UID efectivo y devuelve el PID acreditado por el
// kernel. Rechaza otros usuarios, incluido root si el servidor no corre como
// root.
func verificarPeer(conn net.Conn) (uint32, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, nil // los tests in-memory no representan un socket Unix
	}

	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("obteniendo conn raw: %w", err)
	}

	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		fdValue := uint64(fd)
		if fdValue > math.MaxInt {
			credErr = fmt.Errorf("descriptor de socket fuera de rango: %d", fdValue)
			return
		}
		cred, credErr = syscall.GetsockoptUcred(int(fdValue), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return 0, fmt.Errorf("getsockopt SO_PEERCRED: %w", err)
	}
	if credErr != nil {
		return 0, fmt.Errorf("leyendo credenciales del peer: %w", credErr)
	}

	uid := int64(os.Getuid())
	if uid < 0 || uid > math.MaxUint32 {
		return 0, fmt.Errorf("UID efectivo fuera de rango: %d", uid)
	}
	propio := uint32(uid)
	if cred.Uid != propio {
		return 0, fmt.Errorf("conexion rechazada: UID del peer (%d) != UID del servidor (%d)", cred.Uid, propio)
	}
	if cred.Pid <= 0 {
		return 0, fmt.Errorf("conexion rechazada: PID del peer no valido (%d)", cred.Pid)
	}
	return uint32(cred.Pid), nil
}

func soportaVinculacionPIDPeer() bool { return true }
