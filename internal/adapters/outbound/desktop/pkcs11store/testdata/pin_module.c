// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Doble nativo sin dispositivos ni claves. El header de la dependencia fija la
// tabla ABI independientemente del prefijo usado por nuestro puente.
#include "pkcs11go.h"
#include <string.h>

static CK_RV test_token_info(CK_SLOT_ID slot, CK_TOKEN_INFO_PTR info) {
    memset(info, 0, sizeof(*info));
    if (slot != 1 && slot != 2)
        return CKR_TOKEN_NOT_RECOGNIZED;
    info->flags = CKF_LOGIN_REQUIRED;
    if (slot == 1)
        info->flags |= CKF_TOKEN_INITIALIZED;
    return CKR_OK;
}

static CK_RV test_login(CK_SESSION_HANDLE session, CK_USER_TYPE user,
                        CK_UTF8CHAR_PTR pin, CK_ULONG size) {
    if (session == 42 && user == CKU_USER && pin && size == 4 &&
        pin[0] == 1 && pin[1] == 0 && pin[2] == 2 && pin[3] == 3) {
        pin[0] = 7; // prueba que no se ha fabricado una copia CString
        return CKR_OK;
    }
    if (session == 43 && user == CKU_CONTEXT_SPECIFIC && !pin && size == 0)
        return CKR_OK;
    return CKR_PIN_INCORRECT;
}
static CK_FUNCTION_LIST functions = {
    .version = { 2, 40 },
    .C_GetTokenInfo = test_token_info,
    .C_Login = test_login
};
CK_RV C_GetFunctionList(CK_FUNCTION_LIST_PTR_PTR output) {
    *output = &functions;
    return CKR_OK;
}
