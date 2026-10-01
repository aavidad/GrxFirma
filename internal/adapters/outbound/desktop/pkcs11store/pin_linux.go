// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build cgo && linux

package pkcs11store

/*
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include <dlfcn.h>

// Prefijo estable de CK_FUNCTION_LIST (PKCS#11 2.x/3.x, ABI Unix).
// No se accede a la representación privada del contexto de miekg/pkcs11.
typedef unsigned long af2_ulong;
typedef af2_ulong (*af2_login_fn)(af2_ulong, af2_ulong, unsigned char*, af2_ulong);
typedef void (*af2_function)(void);
typedef struct {
    unsigned char major, minor;
} af2_version;
typedef struct {
    af2_version version;
    af2_function initialize, finalize, get_info, get_function_list;
    af2_function get_slot_list, get_slot_info, get_token_info;
    af2_function get_mechanism_list, get_mechanism_info;
    af2_function init_token, init_pin, set_pin;
    af2_function open_session, close_session, close_all_sessions;
    af2_function get_session_info, get_operation_state, set_operation_state;
    af2_login_fn login;
} af2_function_list_prefix;
typedef af2_ulong (*af2_get_functions_fn)(af2_function_list_prefix**);
typedef struct { void *library; af2_login_fn login; } af2_pin_bridge;

static af2_pin_bridge *af2_open_pin_bridge(const char *path) {
    void *library = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (!library) return NULL;
    af2_get_functions_fn get_functions = (af2_get_functions_fn)dlsym(library, "C_GetFunctionList");
    af2_function_list_prefix *functions = NULL;
    if (!get_functions || get_functions(&functions) != 0 || !functions ||
        (functions->version.major != 2 && functions->version.major != 3) || !functions->login) {
        dlclose(library); return NULL;
    }
    af2_pin_bridge *bridge = calloc(1, sizeof(*bridge));
    if (!bridge) { dlclose(library); return NULL; }
    bridge->library = library; bridge->login = functions->login;
    return bridge;
}
static void af2_close_pin_bridge(af2_pin_bridge *bridge) {
    if (bridge) { dlclose(bridge->library); free(bridge); }
}
static af2_ulong af2_login(af2_pin_bridge *bridge, af2_ulong session,
                         af2_ulong user, unsigned char *pin, af2_ulong size) {
    return bridge->login(session, user, pin, size);
}
*/
import "C"

import (
	"runtime"
	"unsafe"
)

type pinBridge struct{ native *C.af2_pin_bridge }

func openPINBridge(path string) (*pinBridge, error) {
	name := C.CString(path) // solo ruta local; nunca PIN
	defer C.free(unsafe.Pointer(name))
	native := C.af2_open_pin_bridge(name)
	if native == nil {
		return nil, ErrModuloNoDisponible
	}
	return &pinBridge{native: native}, nil
}
func (b *pinBridge) close() { C.af2_close_pin_bridge(b.native); b.native = nil }
func (b *pinBridge) login(session, user uint, pin []byte) uint {
	var pointer *C.uchar
	if len(pin) > 0 {
		pointer = (*C.uchar)(unsafe.Pointer(&pin[0]))
	}
	code := C.af2_login(b.native, C.af2_ulong(session), C.af2_ulong(user), pointer, C.af2_ulong(len(pin)))
	runtime.KeepAlive(pin)
	return uint(code)
}
