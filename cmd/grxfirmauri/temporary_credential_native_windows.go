// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui && amd64

package main

import (
	"context"
	"errors"
	"runtime"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"grxfirma/presentation/desktop/certpicker"
)

const temporaryCredentialCaption = "GrxFirma — Contraseña del archivo P12/PFX"

var (
	procPromptTemporaryCredential      = windows.NewLazySystemDLL("credui.dll").NewProc("CredUIPromptForCredentialsW")
	procEnumTemporaryCredentialWindows = user32Native.NewProc("EnumThreadWindows")
	procTemporaryCredentialWindowText  = user32Native.NewProc("GetWindowTextW")
	procTemporaryCredentialWindowClass = user32Native.NewProc("GetClassNameW")
	procTemporaryCredentialWindowOwner = user32Native.NewProc("GetWindow")
	cancelTemporaryCredentialCallback  = windows.NewCallback(func(hwnd, owner uintptr) uintptr {
		actualOwner, _, _ := procTemporaryCredentialWindowOwner.Call(hwnd, 4) // GW_OWNER
		// Sin padre explícito, CredUI puede crear un propietario oculto. El
		// hilo permanece dedicado al prompt hasta terminar la cancelación.
		if hwnd != owner && (owner == 0 || actualOwner == owner) && nativeWindowOwnedByCurrentProcess(hwnd) {
			var title [256]uint16
			var class [32]uint16
			procTemporaryCredentialWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
			procTemporaryCredentialWindowClass.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
			if windows.UTF16ToString(title[:]) == temporaryCredentialCaption || windows.UTF16ToString(class[:]) == "#32770" {
				postNativeWindowMessage(hwnd, wmClose, 0)
			}
		}
		return 1
	})
)

type temporaryCredentialUIInfo struct {
	Size    uint32
	Parent  uintptr
	Message *uint16
	Caption *uint16
	Banner  uintptr
}

// solicitarPasswordTemporalNativo usa CredUI sin guardar credenciales. La clave
// pasa directamente a buffers UTF-16 borrables y después a UTF-8, nunca a argv.
func solicitarPasswordTemporalNativo(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := procPromptTemporaryCredential.Find(); err != nil {
		return nil, errors.New("Windows no dispone del diálogo protegido de contraseña.")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	message, _ := windows.UTF16PtrFromString("Introduzca la contraseña del P12/PFX para esta sesión de firma. No se instalará el certificado ni se guardará la contraseña.")
	caption, _ := windows.UTF16PtrFromString(temporaryCredentialCaption)
	target, _ := windows.UTF16PtrFromString("GrxFirma/P12-temporal")
	info := temporaryCredentialUIInfo{
		Size:    uint32(unsafe.Sizeof(temporaryCredentialUIInfo{})),
		Parent:  validatedNativeShellWindow(),
		Message: message,
		Caption: caption,
	}
	// El límite nativo CredUI es 256 caracteres, más el terminador.
	var username [514]uint16
	var password [257]uint16
	defer clear(username[:])
	defer clear(password[:])
	var save int32
	done := make(chan struct{})
	cancelDone := make(chan struct{})
	threadID := windows.GetCurrentThreadId()
	go func() {
		defer close(cancelDone)
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			default:
			}
			// Solo diálogos del hilo del prompt y su propietario; nunca otras apps.
			procEnumTemporaryCredentialWindows.Call(uintptr(threadID), cancelTemporaryCredentialCallback, info.Parent)
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
	result, _, _ := procPromptTemporaryCredential.Call(
		uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(target)), 0, 0,
		uintptr(unsafe.Pointer(&username[0])), uintptr(len(username)),
		uintptr(unsafe.Pointer(&password[0])), uintptr(len(password)),
		uintptr(unsafe.Pointer(&save)),
		0x40000|0x00080|0x00002|0x00200, // GENERIC, ALWAYS_SHOW, DO_NOT_PERSIST, PASSWORD_ONLY_OK
	)
	close(done)
	<-cancelDone
	runtime.KeepAlive(info)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result == uintptr(windows.ERROR_CANCELLED) {
		return nil, certpicker.ErrSeleccionCancelada
	}
	if result != 0 {
		return nil, errors.New("Windows no pudo recoger la contraseña del P12/PFX.")
	}
	return temporaryPasswordUTF8(password[:])
}

func temporaryPasswordUTF8(password []uint16) ([]byte, error) {
	result := make([]byte, 0, len(password)*3)
	for index := 0; index < len(password); index++ {
		unit := password[index]
		if unit == 0 {
			return result, nil
		}
		value := rune(unit)
		if utf16.IsSurrogate(value) {
			if index+1 >= len(password) {
				clear(result)
				return nil, errors.New("La contraseña contiene texto Unicode inválido.")
			}
			index++
			value = utf16.DecodeRune(value, rune(password[index]))
			if value == utf8.RuneError {
				clear(result)
				return nil, errors.New("La contraseña contiene texto Unicode inválido.")
			}
		}
		result = utf8.AppendRune(result, value)
	}
	clear(result)
	return nil, errors.New("Windows devolvió una contraseña sin terminador.")
}

func mostrarAvisoCredencialTemporalNativo(ctx context.Context, title, detail string) {
	_, _, _ = showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:         "GrxFirma — Certificado P12/PFX",
		Instruction:   title,
		Content:       detail,
		Buttons:       []nativeDialogChoice{{ID: nativeButtonCancel, Label: "Volver a los certificados"}},
		DefaultButton: nativeButtonCancel,
	})
}
