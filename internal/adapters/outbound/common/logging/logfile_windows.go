// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package logging

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func abrirLogAppendSeguro(ruta string) (*os.File, error) {
	rutaUTF16, err := windows.UTF16PtrFromString(ruta)
	if err != nil {
		return nil, fmt.Errorf("ruta del log no valida: %w", err)
	}
	// FILE_FLAG_OPEN_REPARSE_POINT prevents following a final-component
	// junction/symlink. FILE_APPEND_DATA preserves append semantics.
	handle, err := windows.CreateFile(
		rutaUTF16,
		windows.FILE_APPEND_DATA,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el fichero de log sin seguir enlaces: %w", err)
	}

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("no se pudo comprobar el fichero de log abierto: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("el fichero de log no puede ser un enlace o punto de reanalisis: %s", ruta)
	}

	fichero := os.NewFile(uintptr(handle), ruta)
	if fichero == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("no se pudo representar el fichero de log abierto")
	}
	if err := validarLogAbierto(fichero, ruta); err != nil {
		_ = fichero.Close()
		return nil, err
	}
	return fichero, nil
}
