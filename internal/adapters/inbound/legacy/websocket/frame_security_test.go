// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package websocket

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteServerTextFrame_CodificaLimitesSinTruncar(t *testing.T) {
	tests := []struct {
		name   string
		length int
		header []byte
	}{
		{name: "cero", length: 0, header: []byte{0x81, 0}},
		{name: "ultimo corto", length: 125, header: []byte{0x81, 125}},
		{name: "primer uint16", length: 126, header: []byte{0x81, 126, 0, 126}},
		{name: "ultimo uint16", length: 65535, header: []byte{0x81, 126, 255, 255}},
		{name: "primer uint64", length: 65536, header: []byte{0x81, 127, 0, 0, 0, 0, 0, 1, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte{'x'}, tt.length)
			var out bytes.Buffer
			if err := writeServerTextFrame(&out, string(payload)); err != nil {
				t.Fatalf("writeServerTextFrame() error = %v", err)
			}
			if !bytes.HasPrefix(out.Bytes(), tt.header) {
				t.Fatalf("cabecera=%v, se esperaba prefijo %v", out.Bytes()[:len(tt.header)], tt.header)
			}
			if got := out.Len(); got != len(tt.header)+tt.length {
				t.Fatalf("longitud del frame=%d, want %d", got, len(tt.header)+tt.length)
			}
		})
	}
}

func TestWriteServerTextFrame_RechazaPayloadFueraDelLimite(t *testing.T) {
	payload := strings.Repeat("x", websocketMaxFrame+1)
	var out bytes.Buffer
	err := writeServerTextFrame(&out, payload)
	if err == nil || !strings.Contains(err.Error(), "demasiado grande") {
		t.Fatalf("writeServerTextFrame() error=%v, se esperaba límite de tamaño", err)
	}
	if out.Len() != 0 {
		t.Fatalf("se escribió un frame parcial de %d bytes", out.Len())
	}
}

func TestWebSocketPayloadLengthConversions_RechazanFueraDeRango(t *testing.T) {
	for _, length := range []int{-1, 126} {
		if _, err := websocketPayloadLengthByte(length); err == nil {
			t.Errorf("websocketPayloadLengthByte(%d) debería fallar", length)
		}
	}
	for _, length := range []int{-1, 65536} {
		if _, err := websocketPayloadLengthUint16(length); err == nil {
			t.Errorf("websocketPayloadLengthUint16(%d) debería fallar", length)
		}
	}
}
