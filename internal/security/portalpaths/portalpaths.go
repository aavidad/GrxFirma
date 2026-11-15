// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package portalpaths impide que una ruta recibida de una web alcance los
// ficheros privados de GrxFirma. Obtiene las raíces de la aplicación del
// sistema local; nunca las deriva de una ruta proporcionada por el portal.
package portalpaths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"grxfirma/internal/appdirs"
)

var ErrProtected = errors.New("portalpaths: ruta de datos de la aplicación no permitida")

// Check devuelve la ruta física cuando no pertenece a Config, Data, State ni
// Cache. Resuelve el ancestro existente también para destinos aún sin crear,
// de modo que un alias simbólico o una ruta con componentes padre no eludan
// la barrera. Sin HOME fiable falla cerrado.
func Check(raw string) (string, error) {
	if raw == "" || strings.IndexByte(raw, 0) >= 0 || !filepath.IsAbs(raw) || HasParentComponent(raw) {
		return "", ErrProtected
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return "", ErrProtected
	}
	path, err := physicalPath(raw)
	if err != nil {
		return "", ErrProtected
	}
	var identities []os.FileInfo
	for _, root := range protectedRoots(home) {
		physicalRoot, err := physicalPath(root)
		if err != nil {
			return "", ErrProtected
		}
		identity, statErr := os.Stat(root)
		if statErr == nil {
			identities = append(identities, identity)
		} else if !os.IsNotExist(statErr) {
			return "", ErrProtected
		}
		if within(root, raw) || within(physicalRoot, path) {
			return "", ErrProtected
		}
	}
	if protected, err := protectedAncestor(path, identities, os.Stat); err != nil || protected {
		return "", ErrProtected
	}
	return path, nil
}

func protectedRoots(home string) []string {
	roots := []string{appdirs.Config(home), appdirs.Data(home), appdirs.State(home), appdirs.Cache(home)}
	if runtime.GOOS == "windows" {
		return append(roots,
			filepath.Dir(appdirs.Config(home)),
			filepath.Dir(appdirs.Data(home)),
			filepath.Join(home, "AppData"),
		)
	}
	return append(roots,
		filepath.Join(home, ".config"),
		filepath.Join(home, ".local"),
		filepath.Join(home, ".cache"),
	)
}

// ReadFileLimit vuelve a comprobar el destino justo antes de abrirlo. La
// apertura de securefile no sigue un enlace en el componente final y lee por
// el descriptor ya abierto, aunque el nombre cambie después.
func ReadFileLimit(path string, max int64) ([]byte, error) {
	physical, err := Check(path)
	if err != nil {
		return nil, err
	}
	return readFilePinned(physical, max)
}

func physicalPath(raw string) (string, error) {
	path := filepath.Clean(raw)
	var suffix []string
	for {
		_, err := os.Lstat(path)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}

// HasParentComponent comprueba la entrada original antes de filepath.Clean.
func HasParentComponent(raw string) bool {
	for _, part := range strings.Split(filepath.ToSlash(raw), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func within(root, target string) bool {
	root, target = filepath.Clean(root), filepath.Clean(target)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		root, target = strings.ToLower(root), strings.ToLower(target)
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Compara identidades del sistema de ficheros, no nombres: cubre alias de
// mayúsculas y uniones de Windows aunque EvalSymlinks conserve su texto.
func protectedAncestor(path string, identities []os.FileInfo, stat func(string) (os.FileInfo, error)) (bool, error) {
	for {
		info, err := stat(path)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
		if err == nil && info.IsDir() {
			for _, root := range identities {
				if os.SameFile(root, info) {
					return true, nil
				}
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false, nil
		}
		path = parent
	}
}
