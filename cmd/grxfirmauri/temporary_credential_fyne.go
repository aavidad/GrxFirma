// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"errors"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"grxfirma/presentation/desktop/certpicker"
)

func credentialLoadingAvailable() bool { return true }

func solicitarCredencialTemporal(ctx context.Context) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	paths, err := selectLegacyLoadPaths(ctx, "", "p12,pfx", false)
	if errors.Is(err, errLegacyLoadCanceled) {
		return nil, nil, certpicker.ErrSeleccionCancelada
	}
	if err != nil || len(paths) != 1 {
		return nil, nil, errors.New("No se pudo seleccionar el archivo P12/PFX.")
	}
	data, err := readTemporaryCredential(paths[0])
	if err != nil {
		return nil, nil, err
	}
	var password []byte
	if nativeProtocolUIEnabled() {
		password, err = solicitarPasswordTemporalNativo(ctx)
	} else {
		password, err = solicitarPasswordTemporalFyne(ctx)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		clear(data)
		clear(password)
		return nil, nil, err
	}
	return data, password, nil
}

// solicitarPasswordTemporalFyne devuelve un buffer borrable, sin persistencia.
// Fyne conserva strings internamente; se vacía el campo en todos los cierres.
func solicitarPasswordTemporalFyne(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	portalActionUI()
	a, _ := currentProtocolUIContext()
	if a == nil {
		a = fyne.CurrentApp()
	}
	if a == nil {
		return nil, errors.New("No está disponible la ventana local para introducir la contraseña.")
	}
	type answer struct {
		password []byte
		err      error
	}
	result := make(chan answer, 1)
	done := make(chan struct{})
	var once sync.Once
	var finish func(bool)
	fyne.DoAndWait(func() {
		w := a.NewWindow(tl("Contraseña del archivo P12/PFX"))
		entry := widget.NewPasswordEntry()
		entry.SetPlaceHolder(tl("Contraseña del archivo P12/PFX"))
		warning := widget.NewLabel("")
		finish = func(accept bool) {
			if accept && len(entry.Text) > maxTemporaryPasswordBytes {
				warning.SetText(tl("La contraseña es demasiado larga (máximo 4096 bytes)."))
				return
			}
			once.Do(func() {
				response := answer{err: certpicker.ErrSeleccionCancelada}
				if accept {
					response = answer{password: []byte(entry.Text)}
				}
				entry.SetText("")
				w.SetCloseIntercept(nil)
				w.Close()
				result <- response
				close(done)
			})
		}
		entry.OnSubmitted = func(string) { finish(true) }
		w.SetCloseIntercept(func() { finish(false) })
		help := widget.NewLabel(tl("El certificado se usará durante esta sesión de firma. No se instalará ni se guardará la contraseña."))
		help.Wrapping = fyne.TextWrapWord
		w.SetContent(container.NewVBox(help, entry, warning, container.NewHBox(
			widget.NewButton(tl("Continuar"), func() { finish(true) }),
			widget.NewButton(tl("Cancelar"), func() { finish(false) }),
		)))
		w.Resize(fyne.NewSize(560, 220))
		w.CenterOnScreen()
		w.Show()
		w.RequestFocus()
		w.Canvas().Focus(entry)
	})
	go func() {
		select {
		case <-ctx.Done():
			fyne.Do(func() { finish(false) })
		case <-done:
		}
	}()
	response := <-result
	if err := ctx.Err(); err != nil {
		clear(response.password)
		return nil, err
	}
	return response.password, response.err
}

// mostrarAvisoCredencialTemporal mantiene visible el error antes del reintento.
func mostrarAvisoCredencialTemporal(ctx context.Context, title, detail string) {
	if ctx.Err() != nil {
		return
	}
	if nativeProtocolUIEnabled() {
		mostrarAvisoCredencialTemporalNativo(ctx, title, detail)
		return
	}
	a, parent := currentProtocolUIContext()
	if a == nil {
		a = fyne.CurrentApp()
	}
	if a == nil {
		updateProtocolUI(title, detail)
		return
	}
	done := make(chan struct{})
	var once sync.Once
	var notice dialog.Dialog
	fyne.DoAndWait(func() {
		owned := parent == nil
		if owned {
			parent = a.NewWindow(title)
		}
		notice = dialog.NewInformation(title, detail, parent)
		notice.SetOnClosed(func() {
			once.Do(func() {
				if owned {
					parent.Close()
				}
				close(done)
			})
		})
		if owned {
			parent.Show()
		}
		notice.Show()
	})
	select {
	case <-ctx.Done():
		fyne.DoAndWait(func() { notice.Hide() })
	case <-done:
	}
}
