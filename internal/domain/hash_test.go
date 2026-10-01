// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"strings"
	"testing"
)

func TestDirectoryHashManifestValidateRechazaRutasAmbiguas(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"../secreto",
		"a/../b",
		"./a",
		"/etc/passwd",
		`C:\Windows\win.ini`,
		"a//b",
		"a\x00b",
	}
	for _, relativePath := range tests {
		relativePath := relativePath
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()
			manifest := DirectoryHashManifest{
				Algorithm: "SHA-256",
				Entries: []DirectoryHashEntry{{
					RelativePath: relativePath,
					Digest:       []byte{0x01},
				}},
			}
			if err := manifest.Validate(); err == nil {
				t.Fatalf("Validate() acepto %q", relativePath)
			}
		})
	}
}

func TestNormalizeDirectoryHashRelativePathAdmiteSeparadorLegacy(t *testing.T) {
	t.Parallel()

	got, err := NormalizeDirectoryHashRelativePath(`carpeta\sub\a.txt`)
	if err != nil {
		t.Fatalf("NormalizeDirectoryHashRelativePath() error = %v", err)
	}
	if got != "carpeta/sub/a.txt" {
		t.Fatalf("ruta = %q, want carpeta/sub/a.txt", got)
	}
}

func TestNormalizeDirectoryHashRelativePathRechazaLimitesYEspaciosAmbiguos(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		" a.txt",
		"a.txt ",
		strings.Repeat("a", MaxDirectoryHashRelativePathBytes+1),
	} {
		if _, err := NormalizeDirectoryHashRelativePath(raw); err == nil {
			t.Fatalf("NormalizeDirectoryHashRelativePath(%q) no rechazo la ruta", raw)
		}
	}
}

func TestDirectoryHashManifestValidateRechazaLimites(t *testing.T) {
	t.Parallel()

	tooMany := DirectoryHashManifest{
		Algorithm: "SHA-256",
		Entries:   make([]DirectoryHashEntry, MaxDirectoryHashEntries+1),
	}
	if err := tooMany.Validate(); err == nil {
		t.Fatal("Validate() acepto demasiadas entradas")
	}

	oversizedDigest := DirectoryHashManifest{
		Algorithm: "SHA-256",
		Entries: []DirectoryHashEntry{{
			RelativePath: "a.txt",
			Digest:       make([]byte, MaxDirectoryHashDigestBytes+1),
		}},
	}
	if err := oversizedDigest.Validate(); err == nil {
		t.Fatal("Validate() acepto una huella sobredimensionada")
	}
}
