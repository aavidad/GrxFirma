// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin

package ipc

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// verificarPeer obtiene del núcleo de macOS el UID (LOCAL_PEERCRED) y el PID
// (LOCAL_PEERPID) del proceso conectado al socket. Rechaza otros usuarios y
// devuelve el PID para vincular el backend al frontend exacto, igual que en
// Linux con SO_PEERCRED.
func verificarPeer(conn net.Conn) (uint32, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, errors.New("la conexion IPC no es un socket Unix verificable")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("obteniendo conn raw: %w", err)
	}

	var (
		cred    *unix.Xucred
		pid     int
		credErr error
	)
	if err := raw.Control(func(fd uintptr) {
		fdValue := uint64(fd)
		if fdValue > math.MaxInt {
			credErr = fmt.Errorf("descriptor de socket fuera de rango: %d", fdValue)
			return
		}
		cred, credErr = unix.GetsockoptXucred(int(fdValue), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if credErr != nil {
			return
		}
		pid, credErr = unix.GetsockoptInt(int(fdValue), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	}); err != nil {
		return 0, fmt.Errorf("getsockopt LOCAL_PEERCRED: %w", err)
	}
	if credErr != nil {
		return 0, fmt.Errorf("leyendo credenciales del peer: %w", credErr)
	}
	if cred == nil {
		return 0, errors.New("el sistema no devolvio credenciales del peer")
	}

	uid := int64(os.Getuid())
	if uid < 0 || uid > math.MaxUint32 {
		return 0, fmt.Errorf("UID efectivo fuera de rango: %d", uid)
	}
	if cred.Uid != uint32(uid) {
		return 0, fmt.Errorf("conexion rechazada: UID del peer (%d) != UID del servidor (%d)", cred.Uid, uid)
	}
	if pid <= 0 || int64(pid) > math.MaxUint32 {
		return 0, fmt.Errorf("conexion rechazada: PID del peer no valido (%d)", pid)
	}
	return uint32(pid), nil
}

func soportaVinculacionPIDPeer() bool { return true }
