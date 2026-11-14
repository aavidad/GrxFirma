// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package certaccess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/appdirs"
)

// removeSensitiveFile sobrescribe el temporal antes de eliminarlo, sin seguir
// enlaces. No promete borrado físico en SSD, snapshots o sistemas copy-on-
// write, pero reduce la exposición en sistemas de ficheros convencionales.
func removeSensitiveFile(path string) {
	if path == "" {
		return
	}
	_ = securefile.RemoveSensitiveFile(path)
}

// directorioTemporalPrivado crea un directorio 0700 en la caché del usuario
// para los ficheros con secretos de una importación, en lugar del TMPDIR
// compartido (F-03). Quien lo pide lo borra con securefile.RemoveSensitiveTree.
func directorioTemporalPrivado() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolver carpeta del usuario: %w", err)
	}
	return securefile.MkdirPrivateTemp(filepath.Join(appdirs.Cache(home), "temporal"), "grxfirma-import-*")
}

// contrasenaPk12util prepara la contraseña para la opción -w de pk12util. En
// Linux y macOS se entrega por una tubería heredada como descriptor 3
// (/dev/fd/3): no pasa por argv ni queda en disco. En Windows no hay
// /dev/fd y se escribe en el directorio privado dir, que se borra al acabar.
func contrasenaPk12util(dir, password string) (string, []*os.File, error) {
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "contrasena")
		if err := os.WriteFile(path, []byte(password), 0o600); err != nil {
			return "", nil, fmt.Errorf("escribir contraseña temporal: %w", err)
		}
		return path, nil, nil
	}
	if len(password) > 4096 {
		return "", nil, errors.New("contraseña demasiado larga")
	}
	lectura, escritura, err := os.Pipe()
	if err != nil {
		return "", nil, fmt.Errorf("crear tubería de contraseña: %w", err)
	}
	_, writeErr := escritura.WriteString(password)
	closeErr := escritura.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = lectura.Close()
		return "", nil, fmt.Errorf("escribir contraseña en tubería: %w", err)
	}
	return "/dev/fd/3", []*os.File{lectura}, nil
}
