// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type treeReaderMock struct {
	files map[string][]byte
	list  []ports.DirectoryTreeEntry
}

func (m treeReaderMock) ListFiles(context.Context, string, bool) ([]ports.DirectoryTreeEntry, error) {
	return append([]ports.DirectoryTreeEntry(nil), m.list...), nil
}

func (m treeReaderMock) ReadFile(_ context.Context, path string) ([]byte, error) {
	return append([]byte(nil), m.files[path]...), nil
}

type manifestCodecMock struct {
	manifest domain.DirectoryHashManifest
	data     []byte
}

func (m manifestCodecMock) EncodeManifest(context.Context, domain.DirectoryHashManifest, domain.DirectoryHashManifestFormat) ([]byte, error) {
	return append([]byte(nil), m.data...), nil
}

func (m manifestCodecMock) DecodeManifest(context.Context, []byte, string) (domain.DirectoryHashManifest, error) {
	return m.manifest, nil
}

func TestCreateDirectoryHashManifestUseCase_XML(t *testing.T) {
	reader := treeReaderMock{
		list: []ports.DirectoryTreeEntry{
			{Path: "/tmp/b.txt", RelativePath: "b.txt"},
			{Path: "/tmp/a.txt", RelativePath: "a.txt"},
		},
		files: map[string][]byte{
			"/tmp/a.txt": []byte("uno"),
			"/tmp/b.txt": []byte("dos"),
		},
	}
	uc := NuevoCreateDirectoryHashManifestUseCase(reader, manifestCodecMock{data: []byte("<entries/>")})
	result, err := uc.Execute(context.Background(), CreateDirectoryHashManifestCommand{
		RootPath:  "/tmp",
		Algorithm: "SHA-256",
		Format:    domain.DirectoryHashFormatXML,
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Algorithm != "SHA-256" {
		t.Fatalf("Algorithm inesperado: %s", result.Algorithm)
	}
	if len(result.Manifest.Entries) != 2 || result.Manifest.Entries[0].RelativePath != "a.txt" {
		t.Fatalf("Entradas inesperadas: %#v", result.Manifest.Entries)
	}
}

func TestCheckDirectoryHashManifestUseCase_Report(t *testing.T) {
	reader := treeReaderMock{
		list: []ports.DirectoryTreeEntry{
			{Path: "/tmp/a.txt", RelativePath: "a.txt"},
			{Path: "/tmp/c.txt", RelativePath: "c.txt"},
		},
		files: map[string][]byte{
			"/tmp/a.txt": []byte("uno"),
			"/tmp/c.txt": []byte("tres"),
		},
	}
	manifest := domain.DirectoryHashManifest{
		Algorithm: "SHA-256",
		Recursive: false,
		Entries: []domain.DirectoryHashEntry{
			{RelativePath: "a.txt", Digest: computeHash("SHA-256", []byte("uno"))},
			{RelativePath: "b.txt", Digest: computeHash("SHA-256", []byte("dos"))},
		},
	}
	uc := NuevoCheckDirectoryHashManifestUseCase(reader, manifestCodecMock{manifest: manifest})
	result, err := uc.Execute(context.Background(), CheckDirectoryHashManifestCommand{
		RootPath:     "/tmp",
		ManifestData: []byte("x"),
		ManifestHint: "directorio.hashfiles",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Valid {
		t.Fatal("se esperaba resultado invalido por desajustes")
	}
	if len(result.Report.MatchingHash) != 1 || result.Report.MatchingHash[0] != "a.txt" {
		t.Fatalf("matching inesperado: %#v", result.Report.MatchingHash)
	}
	if len(result.Report.HashWithoutFile) != 1 || result.Report.HashWithoutFile[0] != "b.txt" {
		t.Fatalf("hash_without_file inesperado: %#v", result.Report.HashWithoutFile)
	}
	if len(result.Report.FileWithoutHash) != 1 || result.Report.FileWithoutHash[0] != "c.txt" {
		t.Fatalf("file_without_hash inesperado: %#v", result.Report.FileWithoutHash)
	}
}

func TestCheckDirectoryHashManifestUseCase_RechazaDuplicados(t *testing.T) {
	t.Parallel()

	manifest := domain.DirectoryHashManifest{
		Algorithm: "SHA-256",
		Entries: []domain.DirectoryHashEntry{
			{RelativePath: "a.txt", Digest: []byte{0x01}},
			{RelativePath: "a.txt", Digest: []byte{0x02}},
		},
	}
	uc := NuevoCheckDirectoryHashManifestUseCase(
		treeReaderMock{},
		manifestCodecMock{manifest: manifest},
	)
	_, err := uc.Execute(context.Background(), CheckDirectoryHashManifestCommand{
		RootPath:     "/tmp",
		ManifestData: []byte("x"),
	})
	if err == nil {
		t.Fatal("Execute() no rechazo rutas duplicadas")
	}
}

func TestCheckDirectoryHashManifestUseCase_RechazaAlgoritmoYLongitudIncoherentes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manifest domain.DirectoryHashManifest
	}{
		{
			name: "algoritmo desconocido",
			manifest: domain.DirectoryHashManifest{
				Algorithm: "MD5",
				Entries: []domain.DirectoryHashEntry{{
					RelativePath: "a.txt",
					Digest:       make([]byte, 16),
				}},
			},
		},
		{
			name: "longitud incorrecta",
			manifest: domain.DirectoryHashManifest{
				Algorithm: "SHA-256",
				Entries: []domain.DirectoryHashEntry{{
					RelativePath: "a.txt",
					Digest:       make([]byte, 20),
				}},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			uc := NuevoCheckDirectoryHashManifestUseCase(
				treeReaderMock{},
				manifestCodecMock{manifest: test.manifest},
			)
			if _, err := uc.Execute(context.Background(), CheckDirectoryHashManifestCommand{
				RootPath:     "/tmp",
				ManifestData: []byte("x"),
			}); err == nil {
				t.Fatal("Execute() acepto un manifiesto criptograficamente incoherente")
			}
		})
	}
}
