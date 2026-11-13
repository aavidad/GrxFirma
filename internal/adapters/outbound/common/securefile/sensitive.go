// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile

import (
	"errors"
	"fmt"
	"strings"
)

// RemoveSensitiveFile sobrescribe con ceros un fichero regular propio de la
// aplicación, lo vuelca a disco y lo borra, sin seguir un enlace simbólico o
// punto de reanálisis en el último componente. Sirve para retirar ficheros que
// contuvieron claves o contraseñas en claro.
//
// No garantiza el borrado físico en SSD, instantáneas, copias de seguridad o
// sistemas de ficheros de copia en escritura: reduce la exposición en los
// sistemas de ficheros habituales. El fichero se borra aunque la
// sobrescritura falle; en ese caso se devuelve el error de sobrescritura.
func RemoveSensitiveFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("securefile: empty path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("securefile: path contains NUL")
	}
	overwriteErr := overwriteWithZeros(path)
	if removeErr := RemoveFile(path); removeErr != nil {
		return errors.Join(overwriteErr, removeErr)
	}
	if overwriteErr != nil {
		return fmt.Errorf("securefile: removed without overwrite: %w", overwriteErr)
	}
	return nil
}

func overwriteWithZeros(path string) error {
	file, err := openWriteExistingNoFollow(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("securefile: stat file before overwrite: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("securefile: path is not a regular file: %s", path)
	}
	zeros := make([]byte, 32*1024)
	remaining := info.Size()
	var offset int64
	for remaining > 0 {
		chunk := int64(len(zeros))
		if remaining < chunk {
			chunk = remaining
		}
		if _, err := file.WriteAt(zeros[:chunk], offset); err != nil {
			return fmt.Errorf("securefile: overwrite file: %w", err)
		}
		offset += chunk
		remaining -= chunk
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("securefile: sync overwritten file: %w", err)
	}
	return nil
}
