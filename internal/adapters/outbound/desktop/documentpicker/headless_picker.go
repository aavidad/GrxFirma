// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !fyne_gui

package documentpicker

import (
	"context"
	"errors"
)

// NewHeadless crea un selector documental sin GUI.
func NewHeadless() *Picker {
	return Nuevo(ResolveFunc(func(context.Context) (SelectedDocument, error) {
		return SelectedDocument{}, errors.New("selector documental no disponible sin interfaz gráfica")
	}))
}
