// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func openAppendNoFollow(path string, _ os.FileMode) (*os.File, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("securefile: invalid append path: %w", err)
	}
	// Si el fichero no existe nace ya con propietario y DACL del usuario; si
	// existe, Windows ignora este descriptor y se corrige la DACL más abajo.
	attributes, err := privateFileSecurityAttributes()
	if err != nil {
		return nil, fmt.Errorf("securefile: build private descriptor: %w", err)
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.FILE_APPEND_DATA|windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		attributes,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open-append-nofollow", Path: path, Err: err}
	}
	if err := protectOpenedRegularFileHandle(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("securefile: restrict append file: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("securefile: could not represent opened append file")
	}
	return file, nil
}
