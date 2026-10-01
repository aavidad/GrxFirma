// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build cgo && linux

package pkcs11store

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/miekg/pkcs11"
)

type nativeModule struct {
	ctx *pkcs11.Ctx
	pin *pinBridge
}

func loadNativeModule(path string) (module, error) {
	// Nunca se usa la búsqueda implícita del cargador ni rutas del cwd.
	if !filepath.IsAbs(path) {
		return nil, ErrModuloNoDisponible
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, ErrModuloNoDisponible
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrModuloNoDisponible
	}
	pin, err := openPINBridge(resolved)
	if err != nil {
		return nil, err
	}
	// Validar primero C_GetFunctionList evita el camino de error del cargador
	// externo que no libera el dlopen si falta ese símbolo.
	ctx := pkcs11.New(resolved)
	if ctx == nil {
		pin.close()
		return nil, ErrModuloNoDisponible
	}
	return &nativeModule{ctx: ctx, pin: pin}, nil
}

func nativeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var code pkcs11.Error
	if !errors.As(err, &code) {
		return &ModuleError{Operation: operation}
	}
	var cause error
	switch uint(code) {
	case pkcs11.CKR_PIN_INCORRECT, pkcs11.CKR_PIN_INVALID, pkcs11.CKR_PIN_LEN_RANGE:
		cause = ErrPINIncorrect
	case pkcs11.CKR_PIN_LOCKED:
		cause = ErrPINLocked
	case pkcs11.CKR_PIN_EXPIRED:
		cause = ErrPINExpired
	case pkcs11.CKR_TOKEN_NOT_PRESENT, pkcs11.CKR_TOKEN_NOT_RECOGNIZED, pkcs11.CKR_DEVICE_REMOVED, pkcs11.CKR_SESSION_CLOSED, pkcs11.CKR_SESSION_HANDLE_INVALID:
		cause = ErrTokenUnavailable
	case pkcs11.CKR_USER_ALREADY_LOGGED_IN:
		cause = errAlreadyLoggedIn
	case pkcs11.CKR_MECHANISM_INVALID, pkcs11.CKR_MECHANISM_PARAM_INVALID, pkcs11.CKR_KEY_TYPE_INCONSISTENT, pkcs11.CKR_KEY_FUNCTION_NOT_PERMITTED:
		cause = ErrMechanismUnsupported
	}
	return &ModuleError{Operation: operation, Code: uint(code), cause: cause}
}
func (m *nativeModule) Initialize() error { return nativeError("Initialize", m.ctx.Initialize()) }
func (m *nativeModule) Finalize() error   { return nativeError("Finalize", m.ctx.Finalize()) }
func (m *nativeModule) Destroy()          { m.pin.close(); m.ctx.Destroy() }
func (m *nativeModule) Slots() ([]uint, error) {
	slots, err := m.ctx.GetSlotList(true)
	return slots, nativeError("GetSlotList", err)
}
func (m *nativeModule) TokenInfo(slot uint) (tokenInfo, error) {
	info, err := m.ctx.GetTokenInfo(slot)
	return tokenInfo{label: trimPKCS11String(info.Label), manufacturer: trimPKCS11String(info.ManufacturerID), model: trimPKCS11String(info.Model), serial: trimPKCS11String(info.SerialNumber), initialized: info.Flags&pkcs11.CKF_TOKEN_INITIALIZED != 0, loginRequired: info.Flags&pkcs11.CKF_LOGIN_REQUIRED != 0, protectedAuthentication: info.Flags&pkcs11.CKF_PROTECTED_AUTHENTICATION_PATH != 0, minPIN: info.MinPinLen, maxPIN: info.MaxPinLen}, nativeError("GetTokenInfo", err)
}
func (m *nativeModule) OpenSession(slot uint) (sessionHandle, error) {
	session, err := m.ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION)
	return sessionHandle(session), nativeError("OpenSession", err)
}
func (m *nativeModule) CloseSession(session sessionHandle) error {
	return nativeError("CloseSession", m.ctx.CloseSession(pkcs11.SessionHandle(session)))
}
func (m *nativeModule) FindInit(session sessionHandle, query objectQuery) error {
	class := uint(pkcs11.CKO_CERTIFICATE)
	if query.private {
		class = pkcs11.CKO_PRIVATE_KEY
	}
	attrs := []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_CLASS, class)}
	if query.id != nil {
		attrs = append(attrs, pkcs11.NewAttribute(pkcs11.CKA_ID, query.id))
	}
	if query.signing {
		attrs = append(attrs, pkcs11.NewAttribute(pkcs11.CKA_SIGN, true))
	}
	return nativeError("FindObjectsInit", m.ctx.FindObjectsInit(pkcs11.SessionHandle(session), attrs))
}
func (m *nativeModule) Find(session sessionHandle, count int) ([]objectHandle, error) {
	handles, _, err := m.ctx.FindObjects(pkcs11.SessionHandle(session), count)
	result := make([]objectHandle, len(handles))
	for i, handle := range handles {
		result[i] = objectHandle(handle)
	}
	return result, nativeError("FindObjects", err)
}
func (m *nativeModule) FindFinal(session sessionHandle) error {
	return nativeError("FindObjectsFinal", m.ctx.FindObjectsFinal(pkcs11.SessionHandle(session)))
}
func (m *nativeModule) Certificate(session sessionHandle, object objectHandle) (der, id []byte, err error) {
	attrs, err := m.ctx.GetAttributeValue(pkcs11.SessionHandle(session), pkcs11.ObjectHandle(object), []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_VALUE, nil), pkcs11.NewAttribute(pkcs11.CKA_ID, nil)})
	if err != nil {
		return nil, nil, nativeError("GetAttributeValue(certificate)", err)
	}
	for _, attr := range attrs {
		switch attr.Type {
		case pkcs11.CKA_VALUE:
			der = attr.Value
		case pkcs11.CKA_ID:
			id = attr.Value
		}
	}
	return der, id, nil
}
func (m *nativeModule) AlwaysAuthenticate(session sessionHandle, object objectHandle) (bool, error) {
	attrs, err := m.ctx.GetAttributeValue(pkcs11.SessionHandle(session), pkcs11.ObjectHandle(object), []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_ALWAYS_AUTHENTICATE, nil)})
	if err != nil {
		var code pkcs11.Error
		if errors.As(err, &code) && uint(code) == pkcs11.CKR_ATTRIBUTE_TYPE_INVALID {
			return false, nil
		}
		return false, nativeError("GetAttributeValue(always_authenticate)", err)
	}
	if len(attrs) != 1 || len(attrs[0].Value) != 1 {
		return false, ErrLimit
	}
	return attrs[0].Value[0] != 0, nil
}
func (m *nativeModule) Mechanisms(slot uint) ([]mechanism, error) {
	mechanisms, err := m.ctx.GetMechanismList(slot)
	if err != nil {
		return nil, nativeError("GetMechanismList", err)
	}
	if len(mechanisms) > maxObjects {
		return nil, ErrLimit
	}
	var result []mechanism
	for _, candidate := range mechanisms {
		if candidate == nil {
			continue
		}
		switch candidate.Mechanism {
		case pkcs11.CKM_RSA_PKCS:
			result = append(result, rsaPKCS)
		case pkcs11.CKM_ECDSA:
			result = append(result, ecdsaRaw)
		}
	}
	return result, nil
}
func (m *nativeModule) Login(session sessionHandle, pin []byte, specific bool) error {
	user := uint(pkcs11.CKU_USER)
	if specific {
		user = pkcs11.CKU_CONTEXT_SPECIFIC
	}
	code := m.pin.login(uint(session), user, pin)
	if code == pkcs11.CKR_OK {
		return nil
	}
	return nativeError("Login", pkcs11.Error(code))
}
func (m *nativeModule) Logout(session sessionHandle) error {
	return nativeError("Logout", m.ctx.Logout(pkcs11.SessionHandle(session)))
}
func (m *nativeModule) SignInit(session sessionHandle, algorithm mechanism, key objectHandle) error {
	var mechanismID uint
	switch algorithm {
	case rsaPKCS:
		mechanismID = pkcs11.CKM_RSA_PKCS
	case ecdsaRaw:
		mechanismID = pkcs11.CKM_ECDSA
	default:
		return ErrMechanismUnsupported
	}
	return nativeError("SignInit", m.ctx.SignInit(pkcs11.SessionHandle(session), []*pkcs11.Mechanism{pkcs11.NewMechanism(mechanismID, nil)}, pkcs11.ObjectHandle(key)))
}
func (m *nativeModule) Sign(session sessionHandle, input []byte) ([]byte, error) {
	result, err := m.ctx.Sign(pkcs11.SessionHandle(session), input)
	return result, nativeError("Sign", err)
}
