//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	scardScopeUser    = 0
	scardStatePresent = 0x20
	scardNoReaders    = 0x8010002E
	// SCARD_E_NO_SERVICE / SCARD_E_SERVICE_STOPPED: Windows detiene el servicio
	// de tarjetas inteligentes cuando no hay ningún lector conectado.
	scardNoService      = 0x8010001D
	scardServiceStopped = 0x8010001E
)

var (
	winscardDLL           = syscall.NewLazyDLL("winscard.dll")
	scardEstablishContext = winscardDLL.NewProc("SCardEstablishContext")
	scardReleaseContext   = winscardDLL.NewProc("SCardReleaseContext")
	scardListReaders      = winscardDLL.NewProc("SCardListReadersW")
	scardGetStatusChange  = winscardDLL.NewProc("SCardGetStatusChangeW")
)

type scardReaderState struct {
	reader       *uint16
	userData     uintptr
	currentState uint32
	eventState   uint32
	atrLength    uint32
	atr          [36]byte
}

func detectSmartcards(ctx context.Context) ([]smartcardReader, error) {
	var handle uintptr
	code, _, _ := scardEstablishContext.Call(scardScopeUser, 0, 0, uintptr(unsafe.Pointer(&handle)))
	if code == scardNoService || code == scardServiceStopped {
		return []smartcardReader{}, nil
	}
	if code != 0 {
		return nil, fmt.Errorf("WinSCard: no se pudo abrir el contexto (%#x)", code)
	}
	defer scardReleaseContext.Call(handle)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var count uint32
	code, _, _ = scardListReaders.Call(handle, 0, 0, uintptr(unsafe.Pointer(&count)))
	if code == scardNoReaders || code == scardNoService || code == scardServiceStopped {
		return []smartcardReader{}, nil
	}
	if code != 0 {
		return nil, fmt.Errorf("WinSCard: no se pudieron enumerar lectores (%#x)", code)
	}
	if count == 0 || count > 32768 {
		return nil, fmt.Errorf("WinSCard: longitud de lectores inválida")
	}
	buffer := make([]uint16, count)
	code, _, _ = scardListReaders.Call(handle, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&count)))
	if code != 0 {
		return nil, fmt.Errorf("WinSCard: no se pudieron leer lectores (%#x)", code)
	}
	if count > uint32(len(buffer)) {
		return nil, fmt.Errorf("WinSCard: respuesta de lectores inválida")
	}
	var names []string
	for start, i := 0, 0; i < int(count); i++ {
		if buffer[i] != 0 {
			continue
		}
		if i == start {
			break
		}
		names = append(names, string(utf16.Decode(buffer[start:i])))
		start = i + 1
	}
	if len(names) == 0 {
		return []smartcardReader{}, nil
	}
	states := make([]scardReaderState, len(names))
	pointers := make([][]uint16, len(names))
	for i, name := range names {
		pointers[i] = utf16.Encode([]rune(name + "\x00"))
		states[i].reader = &pointers[i][0]
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Timeout 0: consulta instantánea, nunca se espera una inserción.
	code, _, _ = scardGetStatusChange.Call(handle, 0, uintptr(unsafe.Pointer(&states[0])), uintptr(len(states)))
	runtime.KeepAlive(pointers)
	if code != 0 {
		return nil, fmt.Errorf("WinSCard: no se pudo consultar el estado (%#x)", code)
	}
	readers := make([]smartcardReader, len(names))
	for i, state := range states {
		present := state.eventState&scardStatePresent != 0
		length := min(int(state.atrLength), len(state.atr))
		readers[i] = smartcardReader{
			Name: names[i], Present: present,
			IsDNIe: present && classifyDNIeATR(state.atr[:length]),
		}
	}
	return readers, nil
}
