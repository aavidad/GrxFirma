// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package asiccontainer

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"path"
	"strings"
)

const (
	MIMETypeASiCS            = "application/vnd.etsi.asic-s+zip"
	EntryMIMEType            = "mimetype"
	EntryXMLSignature        = "META-INF/signatures.xml"
	EntryBinarySign          = "META-INF/signature.p7s"
	defaultDataName          = "dataobject.bin"
	maxEntryRead      uint64 = 32 << 20
)

var (
	ErrNotASiC         = errors.New("el documento no es un contenedor ASiC-S valido")
	ErrNoASiCSignature = errors.New("el contenedor ASiC-S no contiene META-INF/signatures.xml")
	ErrNoASiCData      = errors.New("el contenedor ASiC-S no contiene objeto de datos")
	ErrEntryTooLarge   = errors.New("la entrada ASiC supera el tamano maximo permitido")
	ErrAmbiguousEntry  = errors.New("el contenedor ASiC tiene entradas repetidas, ambiguas o con rutas no admitidas")
	ErrManyDataObjects = errors.New("el contenedor ASiC-S debe tener un único objeto de datos")
)

// abrirEntradas lee el directorio del ZIP y rechaza lo que haría que el
// verificador y el programa que abre el contenedor leyeran entradas distintas:
// nombres repetidos (exactos o que solo difieren en mayúsculas o en "\"),
// rutas absolutas y rutas con "." o "..".
func abrirEntradas(data []byte) ([]*zip.File, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrNotASiC
	}
	vistos := make(map[string]struct{}, len(r.File))
	for _, f := range r.File {
		limpio := strings.TrimSuffix(f.Name, "/")
		if limpio == "" || strings.ContainsAny(f.Name, "\\\x00") || strings.HasPrefix(f.Name, "/") ||
			(len(f.Name) > 1 && f.Name[1] == ':') || path.Clean(limpio) != limpio ||
			limpio == ".." || strings.HasPrefix(limpio, "../") {
			return nil, ErrAmbiguousEntry
		}
		clave := strings.ToLower(limpio)
		if _, repetido := vistos[clave]; repetido {
			return nil, ErrAmbiguousEntry
		}
		vistos[clave] = struct{}{}
	}
	return r.File, nil
}

func Detect(data []byte) bool {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	hasMIME := false
	for _, f := range r.File {
		switch normalizeName(f.Name) {
		case strings.ToLower(EntryMIMEType):
			body, readErr := readZipEntryLimited(f, 256)
			if readErr != nil {
				return false
			}
			if strings.TrimSpace(string(body)) != MIMETypeASiCS {
				return false
			}
			hasMIME = true
		case strings.ToLower(EntryXMLSignature), strings.ToLower(EntryBinarySign):
			if hasMIME {
				return true
			}
		}
	}
	return hasMIME
}

func SignedDetect(data []byte) bool {
	if !Detect(data) {
		return false
	}
	if _, err := ExtractXAdESSignature(data); err == nil {
		return true
	}
	_, err := ExtractCAdESSignature(data)
	return err == nil
}

// ExtractCAdESSignature devuelve la firma CMS de un contenedor CAdES-ASiC-S
// (META-INF/signature.p7s).
func ExtractCAdESSignature(data []byte) ([]byte, error) {
	files, err := abrirEntradas(data)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Name == EntryBinarySign {
			return readZipEntryLimited(f, maxEntryRead)
		}
	}
	return nil, errors.New("el contenedor ASiC-S no contiene META-INF/signature.p7s")
}

// CreateCAdESContainer crea un contenedor CAdES-ASiC-S como AutoFirma Java:
// mimetype, el dato firmado y META-INF/signature.p7s (CMS explícita).
func CreateCAdESContainer(cmsDER, payload []byte, dataFilename string) ([]byte, error) {
	return createContainer(EntryBinarySign, cmsDER, payload, dataFilename)
}

func ExtractXAdESSignature(data []byte) ([]byte, error) {
	files, err := abrirEntradas(data)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Name == EntryXMLSignature {
			return readZipEntryLimited(f, maxEntryRead)
		}
	}
	return nil, ErrNoASiCSignature
}

// ExtractData devuelve el único objeto de datos de un ASiC-S: la única
// entrada fuera de META-INF/ que no es mimetype. Con más de una, el
// verificador no puede saber cuál muestra quien abre el contenedor y la firma
// no cubriría las demás, así que se rechaza (ETSI EN 319 162-1, ASiC-S).
func ExtractData(data []byte) ([]byte, string, error) {
	files, err := abrirEntradas(data)
	if err != nil {
		return nil, "", err
	}
	var dato *zip.File
	for _, f := range files {
		if f.Name == EntryMIMEType || strings.HasPrefix(f.Name, "META-INF/") || strings.HasSuffix(f.Name, "/") || f.FileInfo().IsDir() {
			continue
		}
		if dato != nil {
			return nil, "", ErrManyDataObjects
		}
		dato = f
	}
	if dato == nil {
		return nil, "", ErrNoASiCData
	}
	body, readErr := readZipEntryLimited(dato, maxEntryRead)
	if readErr != nil {
		return nil, "", readErr
	}
	return body, dato.Name, nil
}

func CreateXAdESContainer(signatureXML, payload []byte, dataFilename string) ([]byte, error) {
	return createContainer(EntryXMLSignature, signatureXML, payload, dataFilename)
}

func createContainer(signatureEntry string, signature, payload []byte, dataFilename string) ([]byte, error) {
	if len(signature) == 0 {
		return nil, errors.New("la firma no puede estar vacia")
	}
	if len(payload) == 0 {
		return nil, errors.New("los datos del contenedor no pueden estar vacios")
	}
	dataFilename = EnsureDataFilename(dataFilename)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zw.SetComment("mimetype=" + MIMETypeASiCS); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := writeStoredZipEntry(zw, EntryMIMEType, []byte(MIMETypeASiCS)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := writeZipEntry(zw, dataFilename, payload); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := writeZipEntry(zw, signatureEntry, signature); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeStoredZipEntry(zw *zip.Writer, name string, data []byte) error {
	size := uint64(len(data))
	header := &zip.FileHeader{
		Name:               strings.ReplaceAll(name, "\\", "/"),
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   size,
		UncompressedSize64: size,
	}
	w, err := zw.CreateRaw(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(strings.ReplaceAll(name, "\\", "/"))
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func normalizeName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
}

func readZipEntryLimited(f *zip.File, maxSize uint64) ([]byte, error) {
	if f.UncompressedSize64 > maxSize {
		return nil, ErrEntryTooLarge
	}
	if maxSize >= math.MaxInt64 {
		return nil, fmt.Errorf("limite de lectura ZIP fuera de rango: %d", maxSize)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	lr := io.LimitReader(rc, int64(maxSize)+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) > maxSize {
		return nil, ErrEntryTooLarge
	}
	return data, nil
}

func MIMETypeForFilename(name string) string {
	switch strings.ToLower(path.Ext(strings.TrimSpace(name))) {
	case ".xml":
		return "application/xml"
	case ".xsig":
		return "application/xml"
	case ".txt":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func OutputFilename(base string) string {
	trimmed := strings.TrimSpace(base)
	if trimmed == "" {
		return "documento.asics"
	}
	return strings.TrimSuffix(trimmed, path.Ext(trimmed)) + ".asics"
}

func OutputNameFromInput(base string) string {
	trimmed := strings.TrimSpace(path.Base(base))
	if trimmed == "" {
		return "documento.asics"
	}
	return strings.TrimSuffix(trimmed, path.Ext(trimmed)) + ".asics"
}

func OutputNameForPayload(name string) string {
	return EnsureDataFilename(name)
}

func EnsureDataFilename(name string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if trimmed := strings.TrimSpace(path.Base(normalized)); trimmed != "" &&
		trimmed != "." &&
		trimmed != "/" &&
		!strings.EqualFold(trimmed, EntryMIMEType) {
		return trimmed
	}
	return defaultDataName
}

func DebugSummary(data []byte) (string, error) {
	sig, err := ExtractXAdESSignature(data)
	if err != nil {
		return "", err
	}
	payload, name, err := ExtractData(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("asic payload=%s bytes=%d sig=%d", name, len(payload), len(sig)), nil
}
