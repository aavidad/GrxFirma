// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotPrivateDir indica que la carpeta existe pero no es privada del
// usuario: es un enlace, no es una carpeta o pertenece a otro usuario.
var ErrNotPrivateDir = errors.New("securefile: directory is not private to the current user")

// EnsurePrivateDir crea path con 0700 si falta y comprueba que es una carpeta
// real del usuario actual. En Unix exige que el propietario sea el usuario
// efectivo y quita los permisos de grupo y otros; en Windows rechaza los
// puntos de análisis (uniones y enlaces) y deja una DACL protegida del
// usuario, SYSTEM y Administradores.
func EnsurePrivateDir(path string) error {
	if err := validPrivateDirPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	dir, err := OpenDir(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotPrivateDir, err)
	}
	defer dir.Close()
	if err := checkOpenedDirOwner(dir); err != nil {
		return err
	}
	return protectOpenedDirectory(dir, 0o700)
}

// CheckPrivateDir comprueba sin crear nada que path, si existe, es una
// carpeta privada del usuario actual. Si no existe devuelve un error que
// cumple errors.Is(err, os.ErrNotExist).
func CheckPrivateDir(path string) error {
	if err := validPrivateDirPath(path); err != nil {
		return err
	}
	dir, err := OpenDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrNotPrivateDir, err)
	}
	defer dir.Close()
	if err := checkOpenedDirOwner(dir); err != nil {
		return err
	}
	return checkOpenedDirPrivate(dir)
}

func validPrivateDirPath(path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("securefile: invalid private directory: %q", path)
	}
	return nil
}
