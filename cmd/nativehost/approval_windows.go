// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package main

import (
	"context"

	"golang.org/x/sys/windows"

	"grxfirma/internal/adapters/outbound/desktop/wintaskdialog"
)

const botonFirmarNativo int32 = 1001

// solicitarAprobacionWindows pide confirmación con un diálogo nativo. Antes
// se lanzaba PowerShell con -ExecutionPolicy Bypass y -EncodedCommand, que
// muchas Administraciones bloquean y que los EDR señalan como sospechoso.
// El botón por defecto es "Cancelar": un Enter accidental nunca firma.
func solicitarAprobacionWindows(ctx context.Context, titulo, mensaje string) (bool, error) {
	if wintaskdialog.Available() {
		boton, _, err := wintaskdialog.Show(ctx, wintaskdialog.Spec{
			Title:         titulo,
			Instruction:   titulo,
			Content:       mensaje,
			Buttons:       []wintaskdialog.Choice{{ID: botonFirmarNativo, Label: "Firmar"}, {ID: wintaskdialog.IDCancel, Label: "Cancelar"}},
			DefaultButton: wintaskdialog.IDCancel,
		})
		if err != nil {
			return false, err
		}
		return boton == botonFirmarNativo, nil
	}
	texto, err := windows.UTF16PtrFromString(mensaje)
	if err != nil {
		return false, err
	}
	cabecera, err := windows.UTF16PtrFromString(titulo)
	if err != nil {
		return false, err
	}
	resultado := make(chan int32, 1)
	go func() {
		r, _ := windows.MessageBox(0, texto, cabecera,
			windows.MB_OKCANCEL|windows.MB_ICONWARNING|windows.MB_DEFBUTTON2|windows.MB_TOPMOST|windows.MB_SETFOREGROUND)
		resultado <- r
	}()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-resultado:
		return r == 1, nil // IDOK
	}
}
