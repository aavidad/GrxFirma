// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build darwin && cgo

package proxysecretstore

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static OSStatus afv2_copy_default_keychain() {
	SecKeychainRef keychain = NULL;
	OSStatus status = SecKeychainCopyDefault(&keychain);
	if (status == errSecSuccess && keychain != NULL) {
		CFRelease(keychain);
	}
	return status;
}

static OSStatus afv2_add_generic_password(const char* service, const char* account, const void* passwordData, UInt32 passwordLen) {
	return SecKeychainAddGenericPassword(
		NULL,
		(UInt32)strlen(service), service,
		(UInt32)strlen(account), account,
		passwordLen, passwordData,
		NULL
	);
}

static OSStatus afv2_find_generic_password(const char* service, const char* account, UInt32* passwordLen, void** passwordData, SecKeychainItemRef* itemRef) {
	return SecKeychainFindGenericPassword(
		NULL,
		(UInt32)strlen(service), service,
		(UInt32)strlen(account), account,
		passwordLen, passwordData,
		itemRef
	);
}

static OSStatus afv2_delete_generic_password(const char* service, const char* account) {
	UInt32 passwordLen = 0;
	void* passwordData = NULL;
	SecKeychainItemRef itemRef = NULL;
	OSStatus status = afv2_find_generic_password(service, account, &passwordLen, &passwordData, &itemRef);
	if (status != errSecSuccess) {
		return status;
	}
	if (passwordData != NULL) {
		SecKeychainItemFreeContent(NULL, passwordData);
	}
	status = SecKeychainItemDelete(itemRef);
	if (itemRef != NULL) {
		CFRelease(itemRef);
	}
	return status;
}
*/
import "C"

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"grxfirma/internal/ports"
)

const darwinServiceName = "grxfirma-proxy"

type darwinKeychainBackend interface {
	Store(service, account string, payload []byte) error
	Load(service, account string) ([]byte, error)
	Delete(service, account string) error
	Status(context.Context) (ports.ProxySecretStoreStatus, error)
}

type Store struct {
	idgen   func() (string, error)
	service string
	backend darwinKeychainBackend
}

func New() *Store {
	return &Store{
		idgen:   randomID,
		service: darwinServiceName,
		backend: darwinKeychainBackendCgo{},
	}
}

func (s *Store) Store(ctx context.Context, realm string, material ports.ProxySecretMaterial) (ports.ProxySecretDescriptor, error) {
	if s == nil || s.idgen == nil || s.backend == nil {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: backend no configurado")
	}
	realm = strings.TrimSpace(realm)
	if realm == "" {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: realm vacio")
	}
	if len(material.Password) == 0 {
		return ports.ProxySecretDescriptor{}, errors.New("proxysecretstore: password vacio")
	}

	id, err := s.idgen()
	if err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	id, err = normalizeSecretID(id)
	if err != nil {
		return ports.ProxySecretDescriptor{}, err
	}
	env := secretEnvelope{
		Realm:       realm,
		Username:    material.Username,
		PasswordB64: base64.StdEncoding.EncodeToString(material.Password),
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: serializando secreto: %w", err)
	}
	defer zeroSecretBytes(raw)
	if err := s.backend.Delete(s.service, id); err != nil && !isDarwinNotFound(err) {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: preparando entrada keychain: %w", err)
	}
	if err := s.backend.Store(s.service, id, raw); err != nil {
		return ports.ProxySecretDescriptor{}, fmt.Errorf("proxysecretstore: guardando secreto en Keychain: %w", err)
	}
	return ports.ProxySecretDescriptor{
		ID:       id,
		Realm:    realm,
		Username: material.Username,
	}, nil
}

func (s *Store) Load(_ context.Context, id string) (ports.ProxySecretMaterial, error) {
	if s == nil || s.backend == nil {
		return ports.ProxySecretMaterial{}, errors.New("proxysecretstore: backend no configurado")
	}
	id, err := normalizeSecretID(id)
	if err != nil {
		return ports.ProxySecretMaterial{}, err
	}
	raw, err := s.backend.Load(s.service, id)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: cargando secreto desde Keychain: %w", err)
	}
	defer zeroSecretBytes(raw)
	var env secretEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: parseando secreto: %w", err)
	}
	password, err := base64.StdEncoding.DecodeString(env.PasswordB64)
	if err != nil {
		return ports.ProxySecretMaterial{}, fmt.Errorf("proxysecretstore: decodificando password: %w", err)
	}
	return ports.ProxySecretMaterial{
		Realm:    env.Realm,
		Username: env.Username,
		Password: password,
	}, nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	if s == nil || s.backend == nil {
		return errors.New("proxysecretstore: backend no configurado")
	}
	id, err := normalizeSecretID(id)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(s.service, id); err != nil && !isDarwinNotFound(err) {
		return fmt.Errorf("proxysecretstore: eliminando secreto Keychain: %w", err)
	}
	return nil
}

func (s *Store) Status(ctx context.Context) (ports.ProxySecretStoreStatus, error) {
	if s == nil || s.backend == nil {
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   "keychain",
			Reason:    "proxysecretstore: backend no configurado",
		}, nil
	}
	return s.backend.Status(ctx)
}

type darwinKeychainBackendCgo struct{}

func (darwinKeychainBackendCgo) Store(service, account string, payload []byte) error {
	svc := C.CString(service)
	acc := C.CString(account)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	var dataPtr unsafe.Pointer
	if len(payload) > 0 {
		dataPtr = unsafe.Pointer(&payload[0])
	}
	status := C.afv2_add_generic_password(svc, acc, dataPtr, C.UInt32(len(payload)))
	return darwinStatusError("anadiendo item generico al Keychain", status)
}

func (darwinKeychainBackendCgo) Load(service, account string) ([]byte, error) {
	svc := C.CString(service)
	acc := C.CString(account)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	var length C.UInt32
	var data unsafe.Pointer
	// SecKeychainItemRef deriva de CFTypeRef y CGo lo representa como uintptr.
	var itemRef C.SecKeychainItemRef
	status := C.afv2_find_generic_password(svc, acc, &length, &data, &itemRef)
	if itemRef != 0 {
		defer C.CFRelease(C.CFTypeRef(itemRef))
	}
	if status != C.errSecSuccess {
		return nil, darwinStatusError("buscando item generico en Keychain", status)
	}
	defer C.SecKeychainItemFreeContent(nil, data)
	return C.GoBytes(data, C.int(length)), nil
}

func (darwinKeychainBackendCgo) Delete(service, account string) error {
	svc := C.CString(service)
	acc := C.CString(account)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))

	status := C.afv2_delete_generic_password(svc, acc)
	return darwinStatusError("eliminando item generico del Keychain", status)
}

func (darwinKeychainBackendCgo) Status(context.Context) (ports.ProxySecretStoreStatus, error) {
	status := C.afv2_copy_default_keychain()
	if status != C.errSecSuccess {
		err := darwinStatusError("consultando Keychain por defecto", status)
		return ports.ProxySecretStoreStatus{
			Available: false,
			Platform:  runtime.GOOS,
			Backend:   "keychain",
			Reason:    classifyDarwinStatusReason(err),
		}, nil
	}
	return ports.ProxySecretStoreStatus{
		Available: true,
		Platform:  runtime.GOOS,
		Backend:   "keychain",
	}, nil
}

type darwinStatusErr struct {
	status C.OSStatus
	msg    string
	op     string
}

func (e darwinStatusErr) Error() string {
	msg := strings.TrimSpace(e.msg)
	if msg == "" {
		msg = fmt.Sprintf("OSStatus %d", int32(e.status))
	}
	return fmt.Sprintf("proxysecretstore: %s: %s", e.op, msg)
}

func (e darwinStatusErr) Status() C.OSStatus { return e.status }

func darwinStatusError(op string, status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	msg := ""
	if cf := C.SecCopyErrorMessageString(status, nil); cf != 0 {
		msg = cfStringToGoString(cf)
		C.CFRelease(C.CFTypeRef(cf))
	}
	return darwinStatusErr{
		status: status,
		msg:    msg,
		op:     op,
	}
}

func cfStringToGoString(value C.CFStringRef) string {
	if value == 0 {
		return ""
	}
	length := C.CFStringGetLength(value)
	maxSize := C.CFStringGetMaximumSizeForEncoding(length, C.kCFStringEncodingUTF8) + 1
	if maxSize <= 1 {
		return ""
	}
	buffer := C.malloc(C.size_t(maxSize))
	if buffer == nil {
		return ""
	}
	defer C.free(buffer)
	if C.CFStringGetCString(value, (*C.char)(buffer), maxSize, C.kCFStringEncodingUTF8) == 0 {
		return ""
	}
	return C.GoString((*C.char)(buffer))
}

func isDarwinNotFound(err error) bool {
	if err == nil {
		return false
	}
	var statusErr darwinStatusErr
	if errors.As(err, &statusErr) {
		return statusErr.status == C.errSecItemNotFound
	}
	return false
}

func classifyDarwinStatusReason(err error) string {
	if err == nil {
		return ""
	}
	raw := strings.TrimSpace(err.Error())
	if raw == "" {
		return "proxysecretstore: estado del backend Keychain desconocido"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "user interaction is not allowed"):
		return "proxysecretstore: el Keychain requiere interaccion del usuario o esta bloqueado"
	case strings.Contains(lower, "not available"), strings.Contains(lower, "keychain"):
		return raw
	default:
		return raw
	}
}

var _ ports.ProxySecretStore = (*Store)(nil)
