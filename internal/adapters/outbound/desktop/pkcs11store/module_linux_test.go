// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build cgo && linux

package pkcs11store

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miekg/pkcs11"
)

func buildNativeTestModule(t *testing.T) string {
	t.Helper()
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc no disponible para el doble nativo")
	}
	// Solo se consulta la dependencia ya compilada; GOPROXY=off evita red.
	query := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/miekg/pkcs11")
	query.Env = append(query.Environ(), "GOPROXY=off")
	include, err := query.Output()
	if err != nil {
		t.Fatalf("localizar headers PKCS#11: %v", err)
	}
	path := filepath.Join(t.TempDir(), "pin-module.so")
	build := exec.Command(compiler, "-shared", "-fPIC", "-I", strings.TrimSpace(string(include)), "testdata/pin_module.c", "-o", path)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compilar doble nativo: %v: %s", err, output)
	}
	return path
}

func TestNativeTokenInitializedFlag(t *testing.T) {
	m, err := loadNativeModule(buildNativeTestModule(t))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Destroy()
	for _, slot := range []uint{1, 2} {
		info, err := m.TokenInfo(slot)
		if err != nil || info.initialized != (slot == 1) || !info.loginRequired {
			t.Fatalf("slot=%d initialized=%v loginRequired=%v err=%v", slot, info.initialized, info.loginRequired, err)
		}
	}
	if _, err := m.TokenInfo(3); !errors.Is(err, ErrTokenUnavailable) {
		t.Fatalf("error nativo ocultado: %v", err)
	}
}

func TestPINBridgeUsesNativeABIWithoutSecretCopies(t *testing.T) {
	path := buildNativeTestModule(t)
	bridge, err := openPINBridge(path)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	pin := []byte{1, 0, 2, 3}
	if code := bridge.login(42, pkcs11.CKU_USER, pin); code != pkcs11.CKR_OK {
		t.Fatalf("Login CK_RV=%x", code)
	}
	if pin[0] != 7 {
		t.Fatal("el puente creó una copia del PIN")
	}
	clear(pin)
	if code := bridge.login(43, pkcs11.CKU_CONTEXT_SPECIFIC, nil); code != pkcs11.CKR_OK {
		t.Fatalf("PIN protegido no nulo: %x", code)
	}
	if code := bridge.login(0, pkcs11.CKU_USER, []byte("sintetico")); code != pkcs11.CKR_PIN_INCORRECT {
		t.Fatalf("código alterado: %x", code)
	}
}

func TestNativeErrorsAreTypedAndDoNotExposeDriverText(t *testing.T) {
	for _, tc := range []struct {
		code uint
		want error
	}{
		{pkcs11.CKR_PIN_INCORRECT, ErrPINIncorrect}, {pkcs11.CKR_PIN_LOCKED, ErrPINLocked}, {pkcs11.CKR_PIN_EXPIRED, ErrPINExpired},
		{pkcs11.CKR_TOKEN_NOT_PRESENT, ErrTokenUnavailable}, {pkcs11.CKR_DEVICE_REMOVED, ErrTokenUnavailable},
		{pkcs11.CKR_MECHANISM_INVALID, ErrMechanismUnsupported},
	} {
		err := nativeError("Login", pkcs11.Error(tc.code))
		var typed *ModuleError
		if !errors.Is(err, tc.want) || !errors.As(err, &typed) || typed.Code != tc.code {
			t.Fatalf("error=%v", err)
		}
	}
	if err := nativeError("Login", errors.New("texto privado del driver")); strings.Contains(err.Error(), "privado") {
		t.Fatal("texto del driver expuesto")
	}
	if _, err := loadNativeModule("modulo-relativo.so"); !errors.Is(err, ErrModuloNoDisponible) {
		t.Fatal(err)
	}
}
