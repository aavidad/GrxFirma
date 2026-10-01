// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package asiccontainer

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"testing"
)

func TestCreateAndExtractXAdESContainer(t *testing.T) {
	container, err := CreateXAdESContainer([]byte("<sig/>"), []byte("hola"), "factura.xml")
	if err != nil {
		t.Fatalf("CreateXAdESContainer error = %v", err)
	}
	if !Detect(container) {
		t.Fatal("Detect(container) = false, want true")
	}
	if !SignedDetect(container) {
		t.Fatal("SignedDetect(container) = false, want true")
	}
	sig, err := ExtractXAdESSignature(container)
	if err != nil {
		t.Fatalf("ExtractXAdESSignature error = %v", err)
	}
	if string(sig) != "<sig/>" {
		t.Fatalf("firma inesperada: %q", string(sig))
	}
	payload, name, err := ExtractData(container)
	if err != nil {
		t.Fatalf("ExtractData error = %v", err)
	}
	if string(payload) != "hola" || name != "factura.xml" {
		t.Fatalf("payload inesperado: %q / %q", string(payload), name)
	}
}

func TestCreateXAdESContainerStoresMIMETypeFirst(t *testing.T) {
	cases := []struct {
		name                string
		payloadName         string
		expectedPayloadName string
		payload             []byte
	}{
		{
			name:                "xml",
			payloadName:         "factura.xml",
			expectedPayloadName: "factura.xml",
			payload:             []byte(`<?xml version="1.0"?><factura/>`),
		},
		{
			name:                "binario",
			payloadName:         "documento.bin",
			expectedPayloadName: "documento.bin",
			payload:             []byte{0x00, 0x01, 0xff, '<', 'x', 0x00},
		},
		{
			name:                "ruta_windows",
			payloadName:         `C:\documentos\documento.bin`,
			expectedPayloadName: "documento.bin",
			payload:             []byte{0x00, 0x01, 0x02},
		},
		{
			name:                "nombre_reservado",
			payloadName:         "mimetype",
			expectedPayloadName: defaultDataName,
			payload:             []byte{0xff, 0x00, 0xff},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			container, err := CreateXAdESContainer(
				[]byte("<sig/>"),
				tc.payload,
				tc.payloadName,
			)
			if err != nil {
				t.Fatalf("CreateXAdESContainer error = %v", err)
			}

			reader, err := zip.NewReader(bytes.NewReader(container), int64(len(container)))
			if err != nil {
				t.Fatalf("zip.NewReader error = %v", err)
			}
			if len(reader.File) != 3 {
				t.Fatalf("entradas ZIP = %d, esperaba 3", len(reader.File))
			}
			assertStoredMIMETypeLocalHeader(t, container)
			mimetype := reader.File[0]
			if mimetype.Name != EntryMIMEType {
				t.Fatalf("primera entrada = %q, esperaba %q", mimetype.Name, EntryMIMEType)
			}
			if mimetype.Method != zip.Store {
				t.Fatalf("método de mimetype = %d, esperaba Store (%d)", mimetype.Method, zip.Store)
			}
			stream, err := mimetype.Open()
			if err != nil {
				t.Fatalf("mimetype.Open error = %v", err)
			}
			content, readErr := io.ReadAll(stream)
			closeErr := stream.Close()
			if readErr != nil {
				t.Fatalf("leyendo mimetype: %v", readErr)
			}
			if closeErr != nil {
				t.Fatalf("cerrando mimetype: %v", closeErr)
			}
			if string(content) != MIMETypeASiCS {
				t.Fatalf("contenido mimetype = %q, esperaba %q", content, MIMETypeASiCS)
			}

			payload, name, err := ExtractData(container)
			if err != nil {
				t.Fatalf("ExtractData error = %v", err)
			}
			if name != tc.expectedPayloadName || !bytes.Equal(payload, tc.payload) {
				t.Fatalf("payload extraído inesperado: nombre=%q datos=%x", name, payload)
			}
		})
	}
}

func assertStoredMIMETypeLocalHeader(t *testing.T, container []byte) {
	t.Helper()

	const (
		localHeaderLength = 30
		localSignature    = 0x04034b50
	)
	name := []byte(EntryMIMEType)
	content := []byte(MIMETypeASiCS)
	contentOffset := localHeaderLength + len(name)
	if len(container) < contentOffset+len(content) {
		t.Fatalf("cabecera local ASiC truncada: %d bytes", len(container))
	}
	if got := binary.LittleEndian.Uint32(container[0:4]); got != localSignature {
		t.Fatalf("firma de cabecera local = %#x, esperaba %#x", got, localSignature)
	}
	if flags := binary.LittleEndian.Uint16(container[6:8]); flags != 0 {
		t.Fatalf("flags de mimetype = %#x, esperaba 0", flags)
	}
	if method := binary.LittleEndian.Uint16(container[8:10]); method != zip.Store {
		t.Fatalf("método local de mimetype = %d, esperaba Store (%d)", method, zip.Store)
	}
	if got := binary.LittleEndian.Uint32(container[14:18]); got != crc32.ChecksumIEEE(content) {
		t.Fatalf("CRC local de mimetype = %#x, inesperado", got)
	}
	if got := binary.LittleEndian.Uint32(container[18:22]); got != uint32(len(content)) {
		t.Fatalf("tamaño comprimido local = %d, esperaba %d", got, len(content))
	}
	if got := binary.LittleEndian.Uint32(container[22:26]); got != uint32(len(content)) {
		t.Fatalf("tamaño original local = %d, esperaba %d", got, len(content))
	}
	if got := binary.LittleEndian.Uint16(container[26:28]); got != uint16(len(name)) {
		t.Fatalf("longitud del nombre local = %d, esperaba %d", got, len(name))
	}
	if got := binary.LittleEndian.Uint16(container[28:30]); got != 0 {
		t.Fatalf("longitud extra local = %d, esperaba 0", got)
	}
	if got := container[localHeaderLength:contentOffset]; !bytes.Equal(got, name) {
		t.Fatalf("nombre local = %q, esperaba %q", got, name)
	}
	if got := container[contentOffset : contentOffset+len(content)]; !bytes.Equal(got, content) {
		t.Fatalf("contenido local de mimetype = %q, esperaba %q", got, content)
	}
}
