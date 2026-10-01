// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package certpicker

import (
	"context"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/domain"
)

var etiquetasPosicionSello = map[string]string{
	"superior-izquierda": "Arriba a la izquierda",
	"superior-centro":    "Arriba en el centro",
	"superior-derecha":   "Arriba a la derecha",
	"inferior-izquierda": "Abajo a la izquierda",
	"inferior-centro":    "Abajo en el centro",
	"inferior-derecha":   "Abajo a la derecha",
}

type eleccionSello struct {
	posicion, pagina string
	err              error
}

// ElegirPosicionSello deja al usuario situar la firma visible cuando la web
// lo pide (visibleSignature=want), como AutoFirma Java.
func (f *FyneSelector) ElegirPosicionSello(ctx context.Context) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	a := f.app
	if a == nil {
		if a = fyne.CurrentApp(); a == nil {
			a = app.NewWithID(fyneSelectorAppID)
		}
	}
	resultado := make(chan eleccionSello, 1)
	var once sync.Once
	responder := func(e eleccionSello) { once.Do(func() { resultado <- e }) }
	var w fyne.Window
	fyne.DoAndWait(func() {
		w = a.NewWindow(tp("Firma visible"))
		etiquetas := make([]string, 0, len(domain.PosicionesSello))
		porEtiqueta := map[string]string{}
		for _, p := range domain.PosicionesSello {
			e := tp(etiquetasPosicionSello[p])
			etiquetas = append(etiquetas, e)
			porEtiqueta[e] = p
		}
		ultima, primera := tp("Última página (lo habitual)"), tp("Primera página")
		posiciones := widget.NewRadioGroup(etiquetas, nil)
		paginas := widget.NewRadioGroup([]string{ultima, primera}, nil)
		paginas.SetSelected(ultima)
		colocar := widget.NewButton(tp("Colocar la firma aquí"), func() {
			pagina := "-1"
			if paginas.Selected == primera {
				pagina = "1"
			}
			responder(eleccionSello{posicion: porEtiqueta[posiciones.Selected], pagina: pagina})
			w.Close()
		})
		colocar.Importance = widget.HighImportance
		colocar.Disable()
		posiciones.OnChanged = func(s string) {
			if s != "" {
				colocar.Enable()
			}
		}
		sinSello := widget.NewButton(tp("Firmar sin sello visible"), func() {
			responder(eleccionSello{err: ErrSinSelloVisible})
			w.Close()
		})
		cancelar := widget.NewButton(tp("Cancelar"), func() {
			responder(eleccionSello{err: ErrSeleccionCancelada})
			w.Close()
		})
		explicacion := widget.NewLabel(tp("La web ha pedido que elijas dónde aparece la firma en el PDF. Se añadirá un sello con tu nombre y la fecha. También puedes firmar sin sello visible: la firma es igual de válida."))
		explicacion.Wrapping = fyne.TextWrapWord
		w.SetContent(container.NewPadded(container.NewVBox(
			widget.NewLabelWithStyle(tp("¿Dónde quieres colocar la firma visible?"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			explicacion,
			posiciones,
			widget.NewLabelWithStyle(tp("¿En qué página?"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			paginas,
			container.NewHBox(colocar, sinSello, cancelar),
		)))
		w.SetCloseIntercept(func() {
			responder(eleccionSello{err: ErrSeleccionCancelada})
			w.Close()
		})
		w.Resize(fyne.NewSize(560, 520))
		w.CenterOnScreen()
		w.Show()
	})
	select {
	case e := <-resultado:
		return e.posicion, e.pagina, e.err
	case <-ctx.Done():
		fyne.Do(func() { w.Close() })
		return "", "", ctx.Err()
	}
}
