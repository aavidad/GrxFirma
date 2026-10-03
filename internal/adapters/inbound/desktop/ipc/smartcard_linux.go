//go:build linux && cgo

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

/*
#cgo LDFLAGS: -ldl
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <dlfcn.h>

typedef unsigned long DWORD;
typedef long SCARDCONTEXT;
typedef long LONG;
typedef struct {
    const char *szReader;
    void *pvUserData;
    DWORD dwCurrentState;
    DWORD dwEventState;
    DWORD cbAtr;
    unsigned char rgbAtr[33];
} reader_state;
// Acota en C la longitud del ATR al búfer de 33 bytes que define PC/SC.
static int grx_atr_len(DWORD n) { return n > 33 ? 33 : (int)n; }
typedef LONG (*establish_fn)(DWORD, const void *, const void *, SCARDCONTEXT *);
typedef LONG (*release_fn)(SCARDCONTEXT);
typedef LONG (*list_fn)(SCARDCONTEXT, const char *, char *, DWORD *);
typedef LONG (*status_fn)(SCARDCONTEXT, DWORD, reader_state *, DWORD);
typedef struct {
    char name[512];
    DWORD state;
    DWORD atr_len;
    unsigned char atr[36];
} reader_result;

// Carga pcsc-lite en tiempo de ejecución: no requiere sus cabeceras de desarrollo.
// ABI Linux de wintypes.h: DWORD=unsigned long y SCARDCONTEXT=long.
// La consulta no espera a que se inserte una tarjeta.
static int detect_readers(reader_result *out, int capacity, int *count, uint32_t *error_code) {
    void *lib = dlopen("libpcsclite.so.1", RTLD_NOW | RTLD_LOCAL);
    if (!lib) return -1;
    establish_fn establish = (establish_fn)dlsym(lib, "SCardEstablishContext");
    release_fn release = (release_fn)dlsym(lib, "SCardReleaseContext");
    list_fn list = (list_fn)dlsym(lib, "SCardListReaders");
    status_fn status = (status_fn)dlsym(lib, "SCardGetStatusChange");
    if (!establish || !release || !list || !status) { dlclose(lib); return -1; }
    SCARDCONTEXT context = 0;
    uint32_t result = (uint32_t)establish(2, NULL, NULL, &context);
    if (result == 0x8010001D || result == 0x8010001E) { dlclose(lib); return 0; }
    if (result) { *error_code = result; dlclose(lib); return -2; }
    DWORD length = 0;
    result = (uint32_t)list(context, NULL, NULL, &length);
    if (result == 0x8010002E) { release(context); dlclose(lib); return 0; }
    if (result || length < 2 || length > 65536) {
        *error_code = result; release(context); dlclose(lib); return -2;
    }
    char *names = (char *)calloc(length, 1);
    if (!names) { release(context); dlclose(lib); return -1; }
    result = (uint32_t)list(context, NULL, names, &length);
    if (result) { *error_code = result; free(names); release(context); dlclose(lib); return -2; }
    reader_state states[32] = {0};
    size_t offset = 0;
    while (offset < length && names[offset] && *count < capacity) {
        size_t remaining = length - offset;
        size_t size = strnlen(names + offset, remaining);
        if (size == remaining || size >= sizeof(out[0].name)) {
            free(names); release(context); dlclose(lib); return -1;
        }
        memcpy(out[*count].name, names + offset, size);
        states[*count].szReader = out[*count].name;
        (*count)++;
        offset += size + 1;
    }
    if (offset < length && names[offset]) {
        free(names); release(context); dlclose(lib); return -1;
    }
    if (*count) {
        result = (uint32_t)status(context, 0, states, (DWORD)*count);
        if (result) { *error_code = result; free(names); release(context); dlclose(lib); return -2; }
        for (int i = 0; i < *count; i++) {
            out[i].state = states[i].dwEventState;
            out[i].atr_len = states[i].cbAtr < 33 ? states[i].cbAtr : 33;
            memcpy(out[i].atr, states[i].rgbAtr, out[i].atr_len);
        }
    }
    free(names);
    release(context);
    dlclose(lib);
    return 0;
}
*/
import "C"

import (
	"context"
	"errors"
	"unsafe"
)

const smartcardAvailable = true

func detectSmartcards(ctx context.Context) ([]smartcardReader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var found [32]C.reader_result
	var count C.int
	var code C.uint32_t
	status := C.detect_readers(&found[0], 32, &count, &code)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if status == -1 {
		return nil, errors.New("smartcard.pcsc_unavailable")
	}
	if status != 0 {
		return nil, errors.New("smartcard.pcsc_error")
	}
	result := make([]smartcardReader, 0, int(count))
	for _, reader := range found[:int(count)] {
		name := C.GoString(&reader.name[0])
		atrLen := min(int(C.grx_atr_len(reader.atr_len)), len(reader.atr))
		atr := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(&reader.atr[0])), atrLen)...)
		present := reader.state&0x20 != 0 // SCARD_STATE_PRESENT
		result = append(result, smartcardReader{Name: name, Present: present, IsDNIe: present && classifyDNIeATR(atr)})
	}
	return result, nil
}
