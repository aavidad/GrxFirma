// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"crypto/sha1" // #nosec G505 -- only used for the size of legacy SHA-1 manifests; no hashing occurs here.
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

type CreateDirectoryHashManifestCommand struct {
	RootPath  string
	Algorithm string
	Format    domain.DirectoryHashManifestFormat
	Recursive bool
}

type CheckDirectoryHashManifestCommand struct {
	RootPath     string
	ManifestData []byte
	ManifestHint string
}

type CreateDirectoryHashManifestUseCase struct {
	TreeReader ports.DirectoryTreeReader
	Codec      ports.DirectoryHashManifestCodec
}

func NuevoCreateDirectoryHashManifestUseCase(reader ports.DirectoryTreeReader, codec ports.DirectoryHashManifestCodec) *CreateDirectoryHashManifestUseCase {
	return &CreateDirectoryHashManifestUseCase{TreeReader: reader, Codec: codec}
}

type CheckDirectoryHashManifestUseCase struct {
	TreeReader ports.DirectoryTreeReader
	Codec      ports.DirectoryHashManifestCodec
}

func NuevoCheckDirectoryHashManifestUseCase(reader ports.DirectoryTreeReader, codec ports.DirectoryHashManifestCodec) *CheckDirectoryHashManifestUseCase {
	return &CheckDirectoryHashManifestUseCase{TreeReader: reader, Codec: codec}
}

func (uc *CreateDirectoryHashManifestUseCase) Execute(ctx context.Context, cmd CreateDirectoryHashManifestCommand) (CreateDirectoryHashManifestResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateDirectoryHashManifestResult{}, err
	}
	if uc == nil || uc.TreeReader == nil || uc.Codec == nil {
		return CreateDirectoryHashManifestResult{}, errors.New("caso de uso de hash de directorio no configurado")
	}
	if strings.TrimSpace(cmd.RootPath) == "" {
		return CreateDirectoryHashManifestResult{}, errors.New("la ruta del directorio no puede estar vacia")
	}
	alg, err := ParseHashAlgorithm(cmd.Algorithm)
	if err != nil {
		return CreateDirectoryHashManifestResult{}, err
	}
	if err := cmd.Format.Validate(); err != nil {
		return CreateDirectoryHashManifestResult{}, err
	}

	files, err := uc.TreeReader.ListFiles(ctx, cmd.RootPath, cmd.Recursive)
	if err != nil {
		return CreateDirectoryHashManifestResult{}, err
	}
	entries := make([]domain.DirectoryHashEntry, 0, len(files))
	for _, file := range files {
		content, err := uc.TreeReader.ReadFile(ctx, file.Path)
		if err != nil {
			return CreateDirectoryHashManifestResult{}, err
		}
		entries = append(entries, domain.DirectoryHashEntry{
			RelativePath: file.RelativePath,
			Digest:       computeHash(alg, content),
		})
	}
	slices.SortFunc(entries, func(a, b domain.DirectoryHashEntry) int {
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	manifest := domain.DirectoryHashManifest{
		Algorithm: alg,
		Recursive: cmd.Recursive,
		Entries:   entries,
	}
	if err := manifest.Validate(); err != nil {
		return CreateDirectoryHashManifestResult{}, err
	}
	data, err := uc.Codec.EncodeManifest(ctx, manifest, cmd.Format)
	if err != nil {
		return CreateDirectoryHashManifestResult{}, err
	}
	return CreateDirectoryHashManifestResult{
		Algorithm: alg,
		Format:    cmd.Format,
		Manifest:  manifest,
		Data:      data,
	}, nil
}

func (uc *CheckDirectoryHashManifestUseCase) Execute(ctx context.Context, cmd CheckDirectoryHashManifestCommand) (CheckDirectoryHashManifestResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckDirectoryHashManifestResult{}, err
	}
	if uc == nil || uc.TreeReader == nil || uc.Codec == nil {
		return CheckDirectoryHashManifestResult{}, errors.New("caso de uso de comprobacion de hash de directorio no configurado")
	}
	if strings.TrimSpace(cmd.RootPath) == "" {
		return CheckDirectoryHashManifestResult{}, errors.New("la ruta del directorio no puede estar vacia")
	}
	if len(cmd.ManifestData) == 0 {
		return CheckDirectoryHashManifestResult{}, errors.New("el manifiesto no puede estar vacio")
	}

	manifest, err := uc.Codec.DecodeManifest(ctx, cmd.ManifestData, cmd.ManifestHint)
	if err != nil {
		return CheckDirectoryHashManifestResult{}, err
	}
	if err := manifest.Validate(); err != nil {
		return CheckDirectoryHashManifestResult{}, err
	}
	algorithm, err := ParseHashAlgorithm(manifest.Algorithm)
	if err != nil {
		return CheckDirectoryHashManifestResult{}, err
	}
	manifest.Algorithm = algorithm
	expectedDigestSize := directoryHashDigestSize(algorithm)
	for _, entry := range manifest.Entries {
		if len(entry.Digest) != expectedDigestSize {
			return CheckDirectoryHashManifestResult{}, fmt.Errorf(
				"la huella de %q no tiene la longitud de %s",
				entry.RelativePath,
				algorithm,
			)
		}
	}

	files, err := uc.TreeReader.ListFiles(ctx, cmd.RootPath, manifest.Recursive)
	if err != nil {
		return CheckDirectoryHashManifestResult{}, err
	}
	actualByPath := make(map[string][]byte, len(files))
	for _, file := range files {
		content, err := uc.TreeReader.ReadFile(ctx, file.Path)
		if err != nil {
			return CheckDirectoryHashManifestResult{}, err
		}
		actualByPath[file.RelativePath] = computeHash(manifest.Algorithm, content)
	}

	expectedByPath := make(map[string][]byte, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		expectedByPath[entry.RelativePath] = append([]byte(nil), entry.Digest...)
	}

	report := domain.DirectoryHashCheckReport{
		Algorithm: manifest.Algorithm,
		Recursive: manifest.Recursive,
	}
	for _, path := range sortedKeys(expectedByPath) {
		expected := expectedByPath[path]
		actual, ok := actualByPath[path]
		if !ok {
			report.HashWithoutFile = append(report.HashWithoutFile, path)
			continue
		}
		if digestsEqual(expected, actual) {
			report.MatchingHash = append(report.MatchingHash, path)
		} else {
			report.NotMatchingHash = append(report.NotMatchingHash, path)
		}
		delete(actualByPath, path)
	}
	report.FileWithoutHash = append(report.FileWithoutHash, sortedKeys(actualByPath)...)
	return CheckDirectoryHashManifestResult{
		Valid:  !report.HasErrors(),
		Report: report,
	}, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range maps.Keys(m) {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func digestsEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

func directoryHashDigestSize(algorithm string) int {
	switch algorithm {
	case "SHA-1":
		return sha1.Size
	case "SHA-384":
		return sha512.Size384
	case "SHA-512":
		return sha512.Size
	default:
		return sha256.Size
	}
}
