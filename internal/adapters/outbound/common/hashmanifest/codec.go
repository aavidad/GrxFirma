// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package hashmanifest

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
)

const maxDirectoryHashDocumentBytes = 16 * 1024 * 1024

type Codec struct{}

func NuevoCodec() *Codec { return &Codec{} }

func (c *Codec) EncodeManifest(ctx context.Context, manifest domain.DirectoryHashManifest, format domain.DirectoryHashManifestFormat) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if err := format.Validate(); err != nil {
		return nil, err
	}
	switch format {
	case domain.DirectoryHashFormatXML:
		return encodeXML(manifest)
	case domain.DirectoryHashFormatTXT:
		return encodeTXT(manifest), nil
	case domain.DirectoryHashFormatCSV:
		return encodeCSV(manifest), nil
	default:
		return nil, errors.New("formato de manifiesto no soportado")
	}
}

func (c *Codec) DecodeManifest(ctx context.Context, data []byte, hint string) (domain.DirectoryHashManifest, error) {
	if err := ctx.Err(); err != nil {
		return domain.DirectoryHashManifest{}, err
	}
	if len(data) == 0 {
		return domain.DirectoryHashManifest{}, errors.New("el manifiesto no puede estar vacio")
	}
	if len(data) > maxDirectoryHashDocumentBytes {
		return domain.DirectoryHashManifest{}, fmt.Errorf(
			"el manifiesto supera el maximo de %d bytes",
			maxDirectoryHashDocumentBytes,
		)
	}
	switch inferManifestFormat(hint, data) {
	case domain.DirectoryHashFormatXML:
		return decodeXML(data)
	case domain.DirectoryHashFormatTXT:
		return decodeTXT(data)
	default:
		return domain.DirectoryHashManifest{}, errors.New("solo se soporta comprobar manifiestos XML o TXT")
	}
}

func (c *Codec) EncodeReport(ctx context.Context, report domain.DirectoryHashCheckReport) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	totalEntries := len(report.MatchingHash) +
		len(report.NotMatchingHash) +
		len(report.HashWithoutFile) +
		len(report.FileWithoutHash)
	if totalEntries > domain.MaxDirectoryHashEntries {
		return nil, fmt.Errorf("el informe supera el maximo de %d entradas", domain.MaxDirectoryHashEntries)
	}
	doc := xmlHashReport{
		HashAlgorithm: report.Algorithm,
		Recursive:     report.Recursive,
	}
	appendEntries := func(dst *[]xmlReportEntry, paths []string) {
		for _, path := range paths {
			*dst = append(*dst, xmlReportEntry{Name: path})
		}
	}
	for _, paths := range [][]string{
		report.MatchingHash,
		report.NotMatchingHash,
		report.HashWithoutFile,
		report.FileWithoutHash,
	} {
		for _, reportPath := range paths {
			normalized, err := domain.NormalizeDirectoryHashRelativePath(reportPath)
			if err != nil || normalized != reportPath {
				return nil, fmt.Errorf("el informe contiene una ruta no valida: %q", reportPath)
			}
		}
	}
	appendEntries(&doc.MatchingHash, report.MatchingHash)
	appendEntries(&doc.NotMatchingHash, report.NotMatchingHash)
	appendEntries(&doc.HashWithoutFile, report.HashWithoutFile)
	appendEntries(&doc.FileWithoutHash, report.FileWithoutHash)
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("no se pudo codificar el informe XML de hashes: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

type xmlEntries struct {
	XMLName       xml.Name   `xml:"entries"`
	HashAlgorithm string     `xml:"hashAlgorithm,attr"`
	Recursive     bool       `xml:"recursive,attr"`
	Entries       []xmlEntry `xml:"entry"`
}

type xmlEntry struct {
	Name    string `xml:"name,attr"`
	Hash    string `xml:"hash,attr"`
	HexHash string `xml:"hexhash,attr"`
}

type xmlHashReport struct {
	XMLName         xml.Name         `xml:"entries"`
	HashAlgorithm   string           `xml:"hashAlgorithm,attr"`
	Recursive       bool             `xml:"recursive,attr"`
	MatchingHash    []xmlReportEntry `xml:"matching_hash>entry,omitempty"`
	NotMatchingHash []xmlReportEntry `xml:"not_matching_hash>entry,omitempty"`
	HashWithoutFile []xmlReportEntry `xml:"hash_without_file>entry,omitempty"`
	FileWithoutHash []xmlReportEntry `xml:"file_without_hash>entry,omitempty"`
}

type xmlReportEntry struct {
	Name string `xml:"name,attr"`
}

func encodeXML(manifest domain.DirectoryHashManifest) ([]byte, error) {
	doc := xmlEntries{
		HashAlgorithm: manifest.Algorithm,
		Recursive:     manifest.Recursive,
		Entries:       make([]xmlEntry, 0, len(manifest.Entries)),
	}
	for _, entry := range manifest.Entries {
		doc.Entries = append(doc.Entries, xmlEntry{
			Name:    entry.RelativePath,
			Hash:    base64.StdEncoding.EncodeToString(entry.Digest),
			HexHash: strings.ToLower(hex.EncodeToString(entry.Digest)) + "h",
		})
	}
	data, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("no se pudo codificar el manifiesto XML: %w", err)
	}
	return append([]byte(xml.Header), data...), nil
}

func decodeXML(data []byte) (domain.DirectoryHashManifest, error) {
	var doc xmlEntries
	if err := xml.Unmarshal(data, &doc); err != nil {
		return domain.DirectoryHashManifest{}, fmt.Errorf("el manifiesto XML no es valido: %w", err)
	}
	manifest := domain.DirectoryHashManifest{
		Algorithm: doc.HashAlgorithm,
		Recursive: doc.Recursive,
		Entries:   make([]domain.DirectoryHashEntry, 0, len(doc.Entries)),
	}
	if len(doc.Entries) > domain.MaxDirectoryHashEntries {
		return domain.DirectoryHashManifest{}, fmt.Errorf(
			"el manifiesto supera el maximo de %d entradas",
			domain.MaxDirectoryHashEntries,
		)
	}
	for _, entry := range doc.Entries {
		fromB64, err := decodeLegacyCompatibleBase64(entry.Hash)
		if err != nil {
			return domain.DirectoryHashManifest{}, err
		}
		fromHex, err := decodeHexHash(entry.HexHash)
		if err != nil {
			return domain.DirectoryHashManifest{}, err
		}
		if !equalBytes(fromB64, fromHex) {
			return domain.DirectoryHashManifest{}, errors.New("el manifiesto XML declara hashes distintos en base64 y hex")
		}
		relativePath, err := domain.NormalizeDirectoryHashRelativePath(entry.Name)
		if err != nil {
			return domain.DirectoryHashManifest{}, err
		}
		manifest.Entries = append(manifest.Entries, domain.DirectoryHashEntry{
			RelativePath: relativePath,
			Digest:       fromB64,
		})
		if len(manifest.Entries) > domain.MaxDirectoryHashEntries {
			return domain.DirectoryHashManifest{}, fmt.Errorf(
				"el manifiesto supera el maximo de %d entradas",
				domain.MaxDirectoryHashEntries,
			)
		}
	}
	return manifest, manifest.Validate()
}

func encodeTXT(manifest domain.DirectoryHashManifest) []byte {
	var b strings.Builder
	b.WriteString(";charset=UTF-8\r\n")
	b.WriteString(";hashAlgorithm=")
	b.WriteString(manifest.Algorithm)
	b.WriteString("\r\n;recursive=")
	if manifest.Recursive {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString("\r\n")
	for _, entry := range manifest.Entries {
		b.WriteString(entry.RelativePath)
		b.WriteString(";")
		b.WriteString(strings.ToLower(hex.EncodeToString(entry.Digest)))
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func decodeTXT(data []byte) (domain.DirectoryHashManifest, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	manifest := domain.DirectoryHashManifest{Algorithm: "SHA-256"}
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		if strings.HasPrefix(trimmedLine, ";") {
			switch {
			case strings.HasPrefix(trimmedLine, ";hashAlgorithm="):
				manifest.Algorithm = strings.TrimSpace(strings.TrimPrefix(trimmedLine, ";hashAlgorithm="))
			case strings.HasPrefix(trimmedLine, ";recursive="):
				manifest.Recursive = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trimmedLine, ";recursive=")), "true")
			}
			continue
		}
		pos := strings.Index(line, ";")
		if pos <= 0 || pos == len(line)-1 {
			return domain.DirectoryHashManifest{}, errors.New("se encontro una linea de hash no compatible")
		}
		rawDigest := line[pos+1:]
		if rawDigest != strings.TrimSpace(rawDigest) {
			return domain.DirectoryHashManifest{}, errors.New("se encontro una huella con espacios ambiguos en el manifiesto TXT")
		}
		digest, err := hex.DecodeString(rawDigest)
		if err != nil {
			return domain.DirectoryHashManifest{}, errors.New("se encontro una huella hexadecimal no valida en el manifiesto TXT")
		}
		relativePath, err := domain.NormalizeDirectoryHashRelativePath(line[:pos])
		if err != nil {
			return domain.DirectoryHashManifest{}, err
		}
		manifest.Entries = append(manifest.Entries, domain.DirectoryHashEntry{
			RelativePath: relativePath,
			Digest:       digest,
		})
	}
	return manifest, manifest.Validate()
}

func encodeCSV(manifest domain.DirectoryHashManifest) []byte {
	var b strings.Builder
	for _, entry := range manifest.Entries {
		b.WriteString(`"`)
		b.WriteString(strings.ReplaceAll(entry.RelativePath, `"`, `""`))
		b.WriteString(`","`)
		b.WriteString(strings.ToLower(hex.EncodeToString(entry.Digest)))
		b.WriteString(`h"` + "\r\n")
	}
	return []byte(b.String())
}

func inferManifestFormat(hint string, data []byte) domain.DirectoryHashManifestFormat {
	lowerHint := strings.ToLower(strings.TrimSpace(hint))
	switch {
	case strings.HasSuffix(lowerHint, ".hashfiles"), strings.HasSuffix(lowerHint, ".xml"):
		return domain.DirectoryHashFormatXML
	case strings.HasSuffix(lowerHint, ".txthashfiles"), strings.HasSuffix(lowerHint, ".txt"):
		return domain.DirectoryHashFormatTXT
	case strings.HasSuffix(lowerHint, ".csv"):
		return domain.DirectoryHashFormatCSV
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<entries") {
		return domain.DirectoryHashFormatXML
	}
	if strings.HasPrefix(trimmed, ";charset=") || strings.HasPrefix(trimmed, ";hashAlgorithm=") {
		return domain.DirectoryHashFormatTXT
	}
	return domain.DirectoryHashFormatCSV
}

func decodeHexHash(raw string) ([]byte, error) {
	if raw != strings.TrimSpace(raw) {
		return nil, errors.New("el manifiesto XML contiene un hash hexadecimal con espacios ambiguos")
	}
	trimmed := strings.TrimSuffix(strings.TrimSuffix(raw, "h"), "H")
	out, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, errors.New("el manifiesto XML contiene un hash hexadecimal invalido")
	}
	return out, nil
}

func decodeLegacyCompatibleBase64(raw string) ([]byte, error) {
	if raw != strings.TrimSpace(raw) {
		return nil, errors.New("el manifiesto XML contiene un hash base64 con espacios ambiguos")
	}
	if decoded, err := base64.StdEncoding.Strict().DecodeString(raw); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.Strict().DecodeString(raw); err == nil {
		return decoded, nil
	}
	return nil, errors.New("el manifiesto XML contiene un hash base64 invalido")
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
