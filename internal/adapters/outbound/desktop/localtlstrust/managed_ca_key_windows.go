// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package localtlstrust

import (
	"bytes"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var managedCAKeyHeader = []byte("GRXFIRMA-CA-DPAPI-1\x00")

func protectManagedCAKey(key []byte) ([]byte, error) {
	ciphertext, err := callManagedCAKeyDPAPI(key, true)
	if err != nil {
		return nil, err
	}
	defer clearManagedCABytes(ciphertext)
	stored := make([]byte, 0, len(managedCAKeyHeader)+len(ciphertext))
	stored = append(stored, managedCAKeyHeader...)
	stored = append(stored, ciphertext...)
	return stored, nil
}

func unprotectManagedCAKey(stored []byte) ([]byte, error) {
	if !bytes.HasPrefix(stored, managedCAKeyHeader) {
		return nil, ErrManagedCAKeyLegacy
	}
	ciphertext := stored[len(managedCAKeyHeader):]
	if len(ciphertext) == 0 {
		return nil, errors.New("localtlstrust: blob DPAPI de CA vacío")
	}
	return callManagedCAKeyDPAPI(ciphertext, false)
}

func callManagedCAKeyDPAPI(input []byte, protect bool) ([]byte, error) {
	if len(input) == 0 || len(input) > maxManagedCAKeyBytes*2 {
		return nil, errors.New("localtlstrust: tamaño de blob DPAPI de CA inválido")
	}
	in := windows.DataBlob{Size: uint32(len(input)), Data: &input[0]} // #nosec G115 -- acotado a 32 KiB.
	var out windows.DataBlob
	var err error
	if protect {
		description, descErr := windows.UTF16PtrFromString("GrxFirma local TLS CA")
		if descErr != nil {
			return nil, descErr
		}
		err = windows.CryptProtectData(&in, description, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, fmt.Errorf("localtlstrust: DPAPI de clave CA: %w", err)
	}
	if out.Data == nil || out.Size == 0 {
		return nil, errors.New("localtlstrust: DPAPI devolvió un resultado vacío")
	}
	buffer := unsafe.Slice(out.Data, out.Size)
	result := append([]byte(nil), buffer...)
	clearManagedCABytes(buffer)
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return result, nil
}
