// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"crypto/sha1" // #nosec G505 -- explicit legacy checksum option; SHA-256 is the default and signature security does not rely on this utility.
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

type HashOutputFormat string

const (
	HashFormatHex    HashOutputFormat = "hex"
	HashFormatBase64 HashOutputFormat = "base64"
	HashFormatBinary HashOutputFormat = "bin"
)

type CreateHashCommand struct {
	Data      []byte
	Algorithm string
	Format    HashOutputFormat
}

type CheckHashCommand struct {
	Data         []byte
	ExpectedHash []byte
	Algorithm    string
	Format       HashOutputFormat
}

type CreateHashUseCase struct{}

func NuevoCreateHashUseCase() *CreateHashUseCase { return &CreateHashUseCase{} }

type CheckHashUseCase struct{}

func NuevoCheckHashUseCase() *CheckHashUseCase { return &CheckHashUseCase{} }

func (uc *CreateHashUseCase) Execute(ctx context.Context, cmd CreateHashCommand) (CreateHashResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateHashResult{}, err
	}
	alg, err := ParseHashAlgorithm(cmd.Algorithm)
	if err != nil {
		return CreateHashResult{}, err
	}
	format, err := ParseHashOutputFormat(string(cmd.Format))
	if err != nil {
		return CreateHashResult{}, err
	}
	digest := computeHash(alg, cmd.Data)
	return CreateHashResult{
		Algorithm: string(alg),
		Format:    format,
		Digest:    digest,
		Encoded:   encodeHash(format, digest),
	}, nil
}

func (uc *CheckHashUseCase) Execute(ctx context.Context, cmd CheckHashCommand) (CheckHashResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckHashResult{}, err
	}
	alg, err := ParseHashAlgorithm(cmd.Algorithm)
	if err != nil {
		return CheckHashResult{}, err
	}
	format, err := ParseHashOutputFormat(string(cmd.Format))
	if err != nil {
		return CheckHashResult{}, err
	}
	actual := computeHash(alg, cmd.Data)
	valid := subtle.ConstantTimeCompare(actual, cmd.ExpectedHash) == 1
	return CheckHashResult{
		Valid:           valid,
		Algorithm:       string(alg),
		Format:          format,
		ExpectedDigest:  append([]byte(nil), cmd.ExpectedHash...),
		ActualDigest:    actual,
		ExpectedEncoded: encodeHash(format, cmd.ExpectedHash),
		ActualEncoded:   encodeHash(format, actual),
	}, nil
}

func ParseHashAlgorithm(raw string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(raw, "_", "-"))) {
	case "", "SHA-256", "SHA256":
		return "SHA-256", nil
	case "SHA-1", "SHA1":
		return "SHA-1", nil
	case "SHA-384", "SHA384":
		return "SHA-384", nil
	case "SHA-512", "SHA512":
		return "SHA-512", nil
	default:
		return "", errors.New("algoritmo de huella no soportado")
	}
}

func ParseHashOutputFormat(raw string) (HashOutputFormat, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "hex", "hexadecimal":
		return HashFormatHex, nil
	case "b64", "base64":
		return HashFormatBase64, nil
	case "bin", "binary":
		return HashFormatBinary, nil
	default:
		return "", errors.New("formato de huella no soportado")
	}
}

func ParseStoredHash(raw []byte, hint string) ([]byte, string, HashOutputFormat, error) {
	format, err := inferHashFormat(raw, hint)
	if err != nil {
		return nil, "", "", err
	}
	digest, err := decodeHash(format, raw)
	if err != nil {
		return nil, "", "", err
	}
	alg, err := inferHashAlgorithmByLength(len(digest))
	if err != nil {
		return nil, "", "", err
	}
	return digest, alg, format, nil
}

func inferHashAlgorithmByLength(length int) (string, error) {
	switch length {
	case sha1.Size:
		return "SHA-1", nil
	case sha256.Size:
		return "SHA-256", nil
	case sha512.Size384:
		return "SHA-384", nil
	case sha512.Size:
		return "SHA-512", nil
	default:
		return "", errors.New("no se pudo inferir el algoritmo de huella")
	}
}

func inferHashFormat(raw []byte, hint string) (HashOutputFormat, error) {
	lowerHint := strings.ToLower(strings.TrimSpace(hint))
	switch {
	case strings.HasSuffix(lowerHint, ".hash"):
		return HashFormatBinary, nil
	case strings.HasSuffix(lowerHint, ".hashb64"):
		return HashFormatBase64, nil
	case strings.HasSuffix(lowerHint, ".hexhash"):
		return HashFormatHex, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasSuffix(strings.ToLower(trimmed), "h") {
		return HashFormatHex, nil
	}
	if _, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
		return HashFormatBase64, nil
	}
	return HashFormatBinary, nil
}

func decodeHash(format HashOutputFormat, raw []byte) ([]byte, error) {
	switch format {
	case HashFormatBinary:
		return append([]byte(nil), raw...), nil
	case HashFormatBase64:
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	case HashFormatHex:
		trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(string(raw), "h"), "H"))
		return hex.DecodeString(trimmed)
	default:
		return nil, errors.New("formato de huella no soportado")
	}
}

func computeHash(algorithm string, data []byte) []byte {
	switch algorithm {
	case "SHA-1":
		sum := sha1.Sum(data) // #nosec G401 -- explicitly selected legacy checksum compatibility, never the default.
		return sum[:]
	case "SHA-384":
		sum := sha512.Sum384(data)
		return sum[:]
	case "SHA-512":
		sum := sha512.Sum512(data)
		return sum[:]
	default:
		sum := sha256.Sum256(data)
		return sum[:]
	}
}

func encodeHash(format HashOutputFormat, digest []byte) string {
	switch format {
	case HashFormatBinary:
		return base64.StdEncoding.EncodeToString(digest)
	case HashFormatBase64:
		return base64.StdEncoding.EncodeToString(digest)
	default:
		return strings.ToLower(hex.EncodeToString(digest)) + "h"
	}
}
