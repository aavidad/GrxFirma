// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !windows

package ipc

// zonasSistemaProhibidas lista carpetas y ficheros del sistema que el IPC no
// lee ni escribe. Es una lista de robustez, no un control de acceso.
func zonasSistemaProhibidas() []string {
	return []string{
		"/proc", "/sys", "/dev", "/run/udev", "/boot",
		"/root", // home del superusuario
		"/etc/shadow", "/etc/shadow-", "/etc/gshadow", "/etc/gshadow-",
		"/etc/passwd", "/etc/passwd-",
		"/etc/ssh",         // claves de host SSH
		"/etc/ssl/private", // claves TLS del sistema
	}
}
