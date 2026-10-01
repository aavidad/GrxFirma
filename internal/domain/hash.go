// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

type DirectoryHashManifestFormat string

const (
	DirectoryHashFormatXML DirectoryHashManifestFormat = "xml"
	DirectoryHashFormatTXT DirectoryHashManifestFormat = "txt"
	DirectoryHashFormatCSV DirectoryHashManifestFormat = "csv"

	MaxDirectoryHashEntries           = 100_000
	MaxDirectoryHashRelativePathBytes = 4 * 1024
	MaxDirectoryHashDigestBytes       = 64
)

type DirectoryHashEntry struct {
	RelativePath string
	Digest       []byte
}

type DirectoryHashManifest struct {
	Algorithm string
	Recursive bool
	Entries   []DirectoryHashEntry
}

func (f DirectoryHashManifestFormat) Validate() error {
	switch f {
	case DirectoryHashFormatXML, DirectoryHashFormatTXT, DirectoryHashFormatCSV:
		return nil
	default:
		return errors.New("formato de manifiesto de hashes no soportado")
	}
}

func (m DirectoryHashManifest) Validate() error {
	if m.Algorithm == "" {
		return errors.New("el algoritmo del manifiesto no puede estar vacio")
	}
	if len(m.Entries) > MaxDirectoryHashEntries {
		return fmt.Errorf("el manifiesto supera el maximo de %d entradas", MaxDirectoryHashEntries)
	}
	seen := make(map[string]struct{}, len(m.Entries))
	for _, entry := range m.Entries {
		normalized, err := NormalizeDirectoryHashRelativePath(entry.RelativePath)
		if err != nil {
			return err
		}
		if normalized != entry.RelativePath {
			return fmt.Errorf("la ruta del manifiesto no es canonica: %q", entry.RelativePath)
		}
		if len(entry.Digest) == 0 {
			return errors.New("la huella de una entrada no puede estar vacia")
		}
		if len(entry.Digest) > MaxDirectoryHashDigestBytes {
			return fmt.Errorf("la huella de %q supera el maximo de %d bytes", normalized, MaxDirectoryHashDigestBytes)
		}
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("el manifiesto contiene una ruta duplicada: %q", normalized)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

// NormalizeDirectoryHashRelativePath convierte únicamente los separadores
// legacy de Windows. Rechaza rutas absolutas, traversal y representaciones
// ambiguas para que una entrada del manifiesto identifique un único fichero.
func NormalizeDirectoryHashRelativePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed != raw {
		return "", fmt.Errorf("la ruta del manifiesto no es canonica: %q", raw)
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if normalized == "" {
		return "", errors.New("la ruta relativa no puede estar vacia")
	}
	if len([]byte(normalized)) > MaxDirectoryHashRelativePathBytes {
		return "", fmt.Errorf("la ruta relativa supera el maximo de %d bytes", MaxDirectoryHashRelativePathBytes)
	}
	if strings.ContainsRune(normalized, '\x00') {
		return "", errors.New("la ruta relativa contiene un byte nulo")
	}
	if strings.HasPrefix(normalized, "/") || path.IsAbs(normalized) || hasWindowsVolumePrefix(normalized) {
		return "", fmt.Errorf("la ruta del manifiesto debe ser relativa: %q", raw)
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("la ruta del manifiesto no es canonica: %q", raw)
		}
	}
	if cleaned := path.Clean(normalized); cleaned != normalized || cleaned == "." {
		return "", fmt.Errorf("la ruta del manifiesto no es canonica: %q", raw)
	}
	return normalized, nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':'
}

type DirectoryHashCheckReport struct {
	Algorithm       string
	Recursive       bool
	MatchingHash    []string
	NotMatchingHash []string
	HashWithoutFile []string
	FileWithoutHash []string
}

func (r DirectoryHashCheckReport) HasErrors() bool {
	return len(r.NotMatchingHash) > 0 || len(r.HashWithoutFile) > 0 || len(r.FileWithoutHash) > 0
}
