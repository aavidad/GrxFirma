// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// sandboxDiagnosticError mantiene el mensaje público y errors.Is. Solo retiene
// motivos internos fijos, metadatos numéricos y rutas del sistema enumeradas.
// Nunca guarda errores externos completos, rutas privadas ni datos del protocolo.
type sandboxDiagnosticError struct {
	cause               error
	resource, condition string
	errno               unix.Errno
}

func (e *sandboxDiagnosticError) Error() string { return e.cause.Error() }
func (e *sandboxDiagnosticError) Unwrap() error { return e.cause }
func (e *sandboxDiagnosticError) Is(target error) bool {
	return e.errno != 0 && errors.Is(e.errno, target)
}

// SandboxDiagnostic permite a las pruebas imprimir la causa técnica sin cambiar
// Error(), que sigue siendo el texto fijo usado por la interfaz. No ejecuta nada.
func SandboxDiagnostic(err error) string {
	var detail *sandboxDiagnosticError
	if !errors.As(err, &detail) {
		return "sin detalle de recursos de aislamiento"
	}
	return detail.resource + ": " + detail.condition
}

func sandboxFailure(cause error, path, condition string, err error) error {
	var errno unix.Errno
	var previous *sandboxDiagnosticError
	if errors.As(err, &previous) {
		condition += "; " + previous.condition
		errno = previous.errno
	} else if errors.As(err, &errno) {
		condition += fmt.Sprintf("; errno=%d", errno)
	}
	return &sandboxDiagnosticError{cause: cause, resource: sandboxDiagnosticResource(path), condition: condition, errno: errno}
}

func sandboxDiagnosticResource(path string) string {
	switch path {
	case "contexto", "identidad", "recursos", "descriptores", "ejecutable y módulo",
		"ejecutable", "módulo", "fichero de lectura", "directorio de lectura", "directorio de escritura",
		"/usr/bin/bwrap", "/usr", "/lib", "/lib64", "/etc/passwd",
		"/etc/ld.so.cache", "/etc/localtime", "/etc/opensc.conf", "/etc/opensc/opensc.conf",
		"/run/pcscd/pcscd.comm":
		return path
	default:
		return "recurso privado"
	}
}

func sandboxMetadata(stat unix.Stat_t, err error) string {
	if err != nil {
		return "fstat falló"
	}
	return fmt.Sprintf("uid=%d modo=%#o enlaces=%d tamaño=%d", stat.Uid, stat.Mode, stat.Nlink, stat.Size)
}

func sandboxRegularCondition(stat unix.Stat_t, err error) string {
	switch {
	case err != nil:
		return "fstat falló"
	case stat.Mode&unix.S_IFMT != unix.S_IFREG:
		return "se requiere fichero regular; " + sandboxMetadata(stat, nil)
	case stat.Size < 0 || stat.Size > sandboxMaxFile:
		return "tamaño fuera del límite de 128 MiB; " + sandboxMetadata(stat, nil)
	case stat.Nlink != 1:
		return "se requiere un único enlace físico; " + sandboxMetadata(stat, nil)
	case stat.Mode&0022 != 0:
		return "escritura de grupo u otros no permitida; " + sandboxMetadata(stat, nil)
	default:
		return ""
	}
}

func sandboxFileCondition(stat unix.Stat_t, err error, rootOnly, executable bool) string {
	if condition := sandboxRegularCondition(stat, err); condition != "" {
		return condition
	}
	switch {
	case rootOnly && stat.Uid != 0:
		return "se requiere dueño root; " + sandboxMetadata(stat, nil)
	case stat.Size == 0:
		return "fichero vacío"
	case stat.Mode&06000 != 0:
		return "setuid o setgid no permitido"
	case executable && stat.Mode&0111 == 0:
		return "falta permiso de ejecución"
	default:
		return ""
	}
}
