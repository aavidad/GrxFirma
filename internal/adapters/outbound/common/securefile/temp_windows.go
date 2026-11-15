// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package securefile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// createPrivateTemp crea el temporal de WriteFileAtomic con propietario y DACL
// del usuario desde el primer instante. os.CreateTemp lo dejaría con la DACL
// heredada de la carpeta y, en un proceso elevado, con propietario
// Administradores; MoveFileEx conserva ese descriptor en el destino.
func createPrivateTemp(dir, prefix string, _ os.FileMode) (*os.File, error) {
	attributes, err := privateFileSecurityAttributes()
	if err != nil {
		return nil, fmt.Errorf("securefile: build private descriptor: %w", err)
	}
	for range 16 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, fmt.Errorf("securefile: create temporary file: %w", err)
		}
		path := filepath.Join(dir, prefix+hex.EncodeToString(random[:]))
		pathUTF16, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil, fmt.Errorf("securefile: invalid temporary path: %w", err)
		}
		handle, err := windows.CreateFile(
			pathUTF16,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			attributes,
			windows.CREATE_NEW,
			windows.FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if errors.Is(err, windows.ERROR_FILE_EXISTS) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("securefile: create temporary file: %w", &os.PathError{Op: "create-temp", Path: path, Err: err})
		}
		file := os.NewFile(uintptr(handle), path)
		if file == nil {
			_ = windows.CloseHandle(handle)
			_ = os.Remove(path)
			return nil, fmt.Errorf("securefile: could not represent temporary file")
		}
		return file, nil
	}
	return nil, fmt.Errorf("securefile: create temporary file: %w", os.ErrExist)
}
