// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestClassifyDNIeATR(t *testing.T) {
	for _, tc := range []struct {
		name string
		atr  []byte
		want bool
	}{
		{"dnie oficial", []byte{0x3B, 0x7F, 0x38, 0, 0, 0, 0x6A, 'D', 'N', 'I', 'e', 0x10}, true},
		{"otra tarjeta", []byte{0x3B, 0x7F, 0x38, 0, 0, 0, 0x6A, 'T', 'E', 'S', 'T', 0x10}, false},
		{"texto sin ATR", []byte("DNIe"), false},
		{"sin firma histórica", []byte{0x3B, 0x7F, 0x38, 'D', 'N', 'I', 'e', 0, 0, 0, 0, 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyDNIeATR(tc.atr); got != tc.want {
				t.Fatalf("classifyDNIeATR = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestHandleSmartcardStatus(t *testing.T) {
	m := &Manejador{smartcardDetector: func(context.Context) ([]smartcardReader, error) {
		return []smartcardReader{{Name: "Lector de prueba", Present: true, IsDNIe: true}}, nil
	}}
	response := m.despachar(context.Background(), peticion{Action: "smartcard_status"})
	if !response.OK || response.Action != "smartcard_status" {
		t.Fatalf("respuesta inesperada: %+v", response)
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"readers":[{"name":"Lector de prueba","present":true,"isDnie":true}]}` {
		t.Fatalf("contrato inesperado: %s", encoded)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "atr") {
		t.Fatal("el ATR no debe exponerse por IPC")
	}
	failed := handleSmartcardStatus(context.Background(), func(context.Context) ([]smartcardReader, error) {
		return nil, errors.New("falló WinSCard")
	})
	if failed.OK || failed.Error == "" {
		t.Fatalf("error del detector perdido: %+v", failed)
	}
	empty := handleSmartcardStatus(context.Background(), func(context.Context) ([]smartcardReader, error) {
		return nil, nil
	})
	if encoded, err := json.Marshal(empty.Data); err != nil || string(encoded) != `{"readers":[]}` {
		t.Fatalf("sin lectores: %s, %v", encoded, err)
	}
}
