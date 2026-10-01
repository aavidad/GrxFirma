// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux && !windows

package ipc

import "net"

// verificarPeer en otros Unix confía en los permisos del fichero de socket
// (0600). No anuncia binding exacto porque no hay una comprobacion de PID
// implementada en estas plataformas.
func verificarPeer(_ net.Conn) (uint32, error) { return 0, nil }

func soportaVinculacionPIDPeer() bool { return false }
