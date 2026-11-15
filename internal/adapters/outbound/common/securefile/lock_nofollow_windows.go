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

func openLockNoFollow(path string, _ os.FileMode) (*os.File, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("securefile: invalid lock path: %w", err)
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL|windows.WRITE_DAC,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open-lock-nofollow", Path: path, Err: err}
	}
	if err := protectOpenedRegularFileHandle(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("securefile: restrict lock file: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("securefile: could not represent opened lock file")
	}
	return file, nil
}
