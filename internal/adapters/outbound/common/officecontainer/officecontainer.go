// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package officecontainer

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strings"
)

type Kind string

const (
	KindUnknown Kind = ""
	KindODF     Kind = "ODF"
	KindOOXML   Kind = "OOXML"
)

const maxSignatureXMLSize = 16 << 20

var (
	errNotZIP             = errors.New("el documento no es un contenedor ZIP valido")
	errNoODFSignature     = errors.New("el documento ODF no contiene documentsignatures.xml")
	errNoOOXMLSignature   = errors.New("el documento OOXML no contiene firmas XML")
	errSignatureTooLarge  = errors.New("el envelope de firma supera el tamano maximo permitido")
	requiredOOXMLEntries  = []string{"[content_types].xml", "_rels/.rels", "docprops/app.xml", "docprops/core.xml"}
	requiredODFIndicators = []string{"mimetype", "meta-inf/manifest.xml"}
)

// Detect identifica un contenedor ODF u OOXML sin exponer el formato al dominio.
func Detect(data []byte) Kind {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return KindUnknown
	}
	names := make(map[string]struct{}, len(r.File))
	for _, f := range r.File {
		names[normalizeZipName(f.Name)] = struct{}{}
	}
	if containsAll(names, requiredOOXMLEntries) {
		return KindOOXML
	}
	if containsAll(names, requiredODFIndicators) {
		return KindODF
	}
	return KindUnknown
}

// ExtractODFSignatureXML recupera el envelope de firmas ODF.
func ExtractODFSignatureXML(data []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errNotZIP
	}
	for _, f := range r.File {
		if normalizeZipName(f.Name) == "meta-inf/documentsignatures.xml" {
			return readZipEntryLimited(f, maxSignatureXMLSize)
		}
	}
	return nil, errNoODFSignature
}

// ExtractOOXMLSignatureXMLs recupera los envelopes de firma XML de un documento
// OOXML. Las claves son las rutas completas normalizadas desde _xmlsignatures/;
// dos subdirectorios pueden contener firmas distintas con el mismo basename.
func ExtractOOXMLSignatureXMLs(data []byte) (map[string][]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errNotZIP
	}
	signatures := map[string][]byte{}
	for _, f := range r.File {
		name := normalizeZipName(f.Name)
		if !strings.HasPrefix(name, "_xmlsignatures/") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		if strings.HasSuffix(name, "origin.sigs.xml") {
			continue
		}
		if strings.HasSuffix(name, "/origin.sigs") {
			continue
		}
		// No reinterpretar segmentos ambiguos ni permitir que una entrada ZIP
		// sobrescriba otra tras la normalización histórica de case/separadores.
		if path.Clean(name) != name {
			return nil, errors.New("ruta de firma OOXML ambigua")
		}
		if _, duplicate := signatures[name]; duplicate {
			return nil, errors.New("ruta de firma OOXML duplicada o ambigua")
		}
		data, readErr := readZipEntryLimited(f, maxSignatureXMLSize)
		if readErr != nil {
			return nil, readErr
		}
		signatures[name] = data
	}
	if len(signatures) == 0 {
		return nil, errNoOOXMLSignature
	}
	return signatures, nil
}

func normalizeZipName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
}

func containsAll(names map[string]struct{}, required []string) bool {
	for _, name := range required {
		if _, ok := names[name]; !ok {
			return false
		}
	}
	return true
}

func readZipEntryLimited(f *zip.File, maxSize uint64) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("entrada ZIP nula")
	}
	if f.UncompressedSize64 > maxSize {
		return nil, errSignatureTooLarge
	}
	if maxSize >= math.MaxInt64 {
		return nil, fmt.Errorf("limite de lectura ZIP fuera de rango: %d", maxSize)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	limited := io.LimitReader(rc, int64(maxSize)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) > maxSize {
		return nil, errSignatureTooLarge
	}
	return data, nil
}

func hasAnySignatureEntry(data []byte, kind Kind) bool {
	switch kind {
	case KindODF:
		_, err := ExtractODFSignatureXML(data)
		return err == nil
	case KindOOXML:
		sigs, err := ExtractOOXMLSignatureXMLs(data)
		return err == nil && len(sigs) > 0
	default:
		return false
	}
}

// SignedDetect devuelve el tipo de contenedor solo cuando además contiene firmas embebidas.
func SignedDetect(data []byte) Kind {
	kind := Detect(data)
	if hasAnySignatureEntry(data, kind) {
		return kind
	}
	return KindUnknown
}

// SupportedKinds expone el listado canonico de contenedores documentales Office soportados.
func SupportedKinds() []Kind {
	return slices.Clone([]Kind{KindODF, KindOOXML})
}
