// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
)

// smartcardReader comunica solo el estado necesario para guiar al usuario.
// El ATR nunca cruza el protocolo IPC.
type smartcardReader struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	IsDNIe  bool   `json:"isDnie"`
}

type smartcardStatus struct {
	Readers []smartcardReader `json:"readers"`
}

type smartcardDetector func(context.Context) ([]smartcardReader, error)

// La secuencia histórica DNIe publicada por la Policía contiene 00 6A DNIe.
// No se infiere DNIe del nombre del lector ni de un ATR de otra tarjeta.
func classifyDNIeATR(atr []byte) bool {
	return len(atr) >= 12 && atr[0] == 0x3B &&
		bytes.Contains(atr, []byte{0x00, 0x6A, 'D', 'N', 'I', 'e'})
}

func handleSmartcardStatus(ctx context.Context, detector smartcardDetector) respuesta {
	const action = "smartcard_status"
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	readers, err := detector(ctx)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if readers == nil {
		readers = []smartcardReader{}
	}
	return respuesta{OK: true, Action: action, Data: smartcardStatus{Readers: readers}}
}
