// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const maxDirectoryTreeFileBytes = 100 * 1024 * 1024
const maxDirectoryTreeFiles = domain.MaxDirectoryHashEntries

type DirectoryTreeReader struct{}

func NuevoDirectoryTreeReader() *DirectoryTreeReader {
	return &DirectoryTreeReader{}
}

func (r *DirectoryTreeReader) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := securefile.ReadFileLimit(path, maxDirectoryTreeFileBytes)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el fichero %s: %w", path, err)
	}
	return data, nil
}

func (r *DirectoryTreeReader) ListFiles(ctx context.Context, rootPath string, recursive bool) ([]ports.DirectoryTreeEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo acceder al directorio %s: %w", rootPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("la ruta indicada no es un directorio: %s", rootPath)
	}
	var out []ports.DirectoryTreeEntry
	rootPath = filepath.Clean(rootPath)
	if recursive {
		err = filepath.WalkDir(rootPath, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			regular, err := regularDirectoryEntry(d)
			if err != nil {
				return err
			}
			if !regular {
				return nil
			}
			if shouldSkipLegacyHashFile(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(rootPath, path)
			if err != nil {
				return err
			}
			out = append(out, ports.DirectoryTreeEntry{
				Path:         path,
				RelativePath: filepath.ToSlash(rel),
			})
			if len(out) > maxDirectoryTreeFiles {
				return fmt.Errorf("el directorio supera el maximo de %d ficheros", maxDirectoryTreeFiles)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("no se pudo recorrer el directorio %s: %w", rootPath, err)
		}
	} else {
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			return nil, fmt.Errorf("no se pudo listar el directorio %s: %w", rootPath, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.IsDir() {
				continue
			}
			regular, err := regularDirectoryEntry(entry)
			if err != nil {
				return nil, fmt.Errorf("no se pudo inspeccionar %s: %w", entry.Name(), err)
			}
			if !regular {
				continue
			}
			if shouldSkipLegacyHashFile(entry.Name()) {
				continue
			}
			out = append(out, ports.DirectoryTreeEntry{
				Path:         filepath.Join(rootPath, entry.Name()),
				RelativePath: filepath.ToSlash(entry.Name()),
			})
			if len(out) > maxDirectoryTreeFiles {
				return nil, fmt.Errorf("el directorio supera el maximo de %d ficheros", maxDirectoryTreeFiles)
			}
		}
	}
	slices.SortFunc(out, func(a, b ports.DirectoryTreeEntry) int {
		return compareStrings(a.RelativePath, b.RelativePath)
	})
	return out, nil
}

func regularDirectoryEntry(entry os.DirEntry) (bool, error) {
	if entry.Type()&os.ModeSymlink != 0 {
		return false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func shouldSkipLegacyHashFile(name string) bool {
	if strings.Contains(name, "~$") {
		return true
	}
	switch {
	case strings.EqualFold(name, ".fseventsd"),
		strings.EqualFold(name, ".Spotlight-V100"),
		strings.EqualFold(name, ".Trashes"),
		strings.EqualFold(name, "._.Trashes"),
		strings.EqualFold(name, ".DS_Store"),
		strings.EqualFold(name, ".desktop"),
		strings.EqualFold(name, "thumbs.db"),
		strings.EqualFold(name, "$Recycle.Bin"):
		return true
	default:
		return false
	}
}
