//go:build linux

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package securefile

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ReadTrustedSystemFileLimit admite enlaces solo en árboles administrados por
// root. Una ruta de usuario, incluso si apunta a /usr, sigue usando ReadFileLimit
// con O_NOFOLLOW. Se validan tanto el enlace como el destino y sus ancestros;
// luego el fichero final se abre sin seguir enlaces y se comprueba su descriptor.
func ReadTrustedSystemFileLimit(path string, maxBytes int64) ([]byte, error) {
	if !systemOwnedPath(path) {
		return nil, fmt.Errorf("ruta fuera de los archivos de confianza del sistema")
	}
	// La política es opcional; devolver ENOENT antes de validar permisos permite
	// tratar su ausencia igual que cualquier otro fichero no instalado.
	if _, err := os.Lstat(path); err != nil {
		return nil, err
	}
	return readOwnedSystemFileLimit(path, maxBytes, 0, "/")
}

func systemOwnedPath(path string) bool {
	for _, root := range []string{"/etc", "/usr", "/lib", "/lib64"} {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func readOwnedSystemFileLimit(path string, maxBytes int64, owner uint32, root string) ([]byte, error) {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		!filepath.IsAbs(root) || filepath.Clean(root) != root ||
		(path != root && root != "/" && !strings.HasPrefix(path, root+"/")) {
		return nil, os.ErrInvalid
	}
	if err := validateOwnedPath(path, owner, root); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if (root == "/" && !systemOwnedPath(resolved)) || (root != "/" && resolved != root && !strings.HasPrefix(resolved, root+"/")) {
		return nil, fmt.Errorf("el enlace sale de las rutas del sistema")
	}
	if err := validateOwnedPath(resolved, owner, root); err != nil {
		return nil, err
	}
	file, err := OpenRead(resolved) // O_NOFOLLOW sobre el destino final.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != owner || info.Mode().Perm()&0022 != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("el destino del sistema no es un fichero regular protegido")
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("el fichero del sistema supera %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("el fichero del sistema supera %d bytes", maxBytes)
	}
	return data, nil
}

func validateOwnedPath(path string, owner uint32, root string) error {
	current := root
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != owner {
			return fmt.Errorf("la ruta del sistema no pertenece a root")
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if current == path {
				if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
					return fmt.Errorf("el fichero del sistema no está protegido")
				}
			} else if !info.IsDir() || info.Mode().Perm()&0022 != 0 {
				return fmt.Errorf("un directorio del sistema es escribible o no es un directorio: %s", current)
			}
		}
		if current == path {
			return nil
		}
		rel, _ := filepath.Rel(current, path)
		current = filepath.Join(current, strings.Split(rel, string(filepath.Separator))[0])
	}
}
