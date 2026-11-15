//go:build windows

// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package settingsfile_test

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procCreateRestrictedToken = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateRestrictedToken")

const disableMaxPrivilege = 0x1

// exigirElevado salta la prueba fuera de un proceso elevado: solo así se
// reproduce el propietario Administradores que deja la CLI elevada.
func exigirElevado(t *testing.T) {
	t.Helper()
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("necesita un proceso elevado (como la CLI abierta como administrador)")
	}
}

// comoUsuarioSinElevar ejecuta fn con un token como el que Windows da al mismo
// usuario sin elevar: Administradores solo para denegar, sin privilegios y con
// integridad media. El hilo queda fijo para que la suplantación valga en todas
// las aperturas de fichero de fn.
func comoUsuarioSinElevar(t *testing.T, fn func()) {
	t.Helper()
	var process windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT, &process); err != nil {
		t.Fatal("OpenProcessToken:", err)
	}
	defer process.Close()
	var disable []windows.SIDAndAttributes
	for _, text := range []string{"S-1-5-32-544", "S-1-5-114"} {
		sid, err := windows.StringToSid(text)
		if err != nil {
			t.Fatal(err)
		}
		disable = append(disable, windows.SIDAndAttributes{Sid: sid})
	}
	var restricted windows.Token
	if r, _, err := procCreateRestrictedToken.Call(uintptr(process), disableMaxPrivilege,
		uintptr(len(disable)), uintptr(unsafe.Pointer(&disable[0])), 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&restricted))); r == 0 {
		t.Fatal("CreateRestrictedToken:", err)
	}
	defer restricted.Close()
	medium, err := windows.CreateWellKnownSid(windows.WinMediumLabelSid)
	if err != nil {
		t.Fatal(err)
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: medium, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err := windows.SetTokenInformation(restricted, windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
		t.Fatal("bajar la integridad:", err)
	}
	var impersonation windows.Token
	if err := windows.DuplicateTokenEx(restricted, windows.TOKEN_IMPERSONATE|windows.TOKEN_QUERY, nil,
		windows.SecurityImpersonation, windows.TokenImpersonation, &impersonation); err != nil {
		t.Fatal("DuplicateTokenEx:", err)
	}
	defer impersonation.Close()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.SetThreadToken(nil, impersonation); err != nil {
		t.Fatal("SetThreadToken:", err)
	}
	defer func() {
		if err := windows.RevertToSelf(); err != nil {
			t.Error("RevertToSelf:", err)
		}
	}()
	fn()
}
