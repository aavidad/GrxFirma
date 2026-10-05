// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package documentpicker

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

const fyneDocumentPickerAppID = "io.github.aavidad.grxfirma.afirmauri"

// NewFyne crea un selector documental interactivo basado en Fyne.
func NewFyne() *Picker {
	return Nuevo(fyneResolver{})
}

// NewFyneWithParent crea un selector que reutiliza la ventana indicada cuando
// esta disponible. Esto evita crear una ventana GLFW secundaria que pueda
// quedar oculta detrás del navegador o sobrevivir al cierre del app principal.
func NewFyneWithParent(parent func() fyne.Window) *Picker {
	return Nuevo(fyneResolver{parent: parent})
}

type fyneResolver struct {
	parent func() fyne.Window
}

type resolveResult struct {
	document SelectedDocument
	err      error
}

func (r fyneResolver) ResolveDocument(ctx context.Context) (SelectedDocument, error) {
	return r.ResolveDocumentFiltered(ctx, ports.DocumentFilter{})
}

// fyneExtensionFilter traduce el filtro del puerto al de Fyne (".pdf").
func fyneExtensionFilter(filter ports.DocumentFilter) storage.FileFilter {
	if len(filter.Extensions) == 0 {
		return nil
	}
	exts := make([]string, 0, len(filter.Extensions))
	for _, ext := range filter.Extensions {
		ext = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
		if ext != "" {
			exts = append(exts, "."+ext)
		}
	}
	if len(exts) == 0 {
		return nil
	}
	return storage.NewExtensionFileFilter(exts)
}

func (r fyneResolver) ResolveDocumentFiltered(ctx context.Context, filter ports.DocumentFilter) (SelectedDocument, error) {
	select {
	case <-ctx.Done():
		return SelectedDocument{}, ctx.Err()
	default:
	}

	var w fyne.Window
	if r.parent != nil {
		w = r.parent()
	}

	a := fyne.CurrentApp()
	ownsApp := false
	if a == nil && w == nil {
		a = app.NewWithID(fyneDocumentPickerAppID)
		applyDocumentPickerAppIcon(a)
		ownsApp = true
	}
	if a == nil {
		return SelectedDocument{}, errors.New("desktop-document-picker: ventana Fyne sin aplicación activa")
	}

	ownsWindow := w == nil
	createWindow := func() {
		w = a.NewWindow("Seleccionar documento")
		w.Resize(fyne.NewSize(980, 680))
		w.CenterOnScreen()
	}
	if ownsWindow {
		if ownsApp {
			createWindow()
		} else {
			fyne.DoAndWait(createWindow)
		}
	}

	resultCh := make(chan resolveResult, 1)
	done := make(chan struct{})
	var doneOnce sync.Once
	var fd *dialog.FileDialog
	emit := func(res resolveResult) {
		doneOnce.Do(func() {
			resultCh <- res
			close(done)
			fyne.Do(func() {
				if fd != nil {
					fd.Hide()
				}
				if ownsWindow {
					w.SetCloseIntercept(nil)
					w.Close()
				}
			})
		})
	}

	showPicker := func() {
		if ownsWindow {
			w.SetCloseIntercept(func() {
				emit(resolveResult{err: ErrSeleccionCancelada})
			})
		}
		w.Show()
		w.RequestFocus()
		fd = dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				emit(resolveResult{err: err})
				return
			}
			if reader == nil {
				emit(resolveResult{err: ErrSeleccionCancelada})
				return
			}
			defer reader.Close()

			data, readErr := io.ReadAll(reader)
			if readErr != nil {
				emit(resolveResult{err: readErr})
				return
			}

			nombre := reader.URI().Name()
			if strings.TrimSpace(nombre) == "" {
				nombre = filepath.Base(reader.URI().Path())
			}
			mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(nombre)))
			if strings.TrimSpace(mimeType) == "" && len(data) > 0 {
				mimeType = http.DetectContentType(data)
			}

			emit(resolveResult{document: SelectedDocument{
				Name:     nombre,
				Content:  data,
				MIMEType: mimeType,
			}})
		}, w)
		if extFilter := fyneExtensionFilter(filter); extFilter != nil {
			fd.SetFilter(extFilter)
		}
		if downloads, ok := fynePickerStartLocation(); ok {
			fd.SetLocation(downloads)
		}
		fd.Resize(fyne.NewSize(900, 600))
		fd.Show()
	}
	if ownsApp {
		showPicker()
	} else {
		fyne.DoAndWait(showPicker)
	}

	go func() {
		select {
		case <-ctx.Done():
			emit(resolveResult{err: ctx.Err()})
		case <-done:
		}
	}()

	if ownsApp {
		a.Run()
	}

	res := <-resultCh
	return res.document, res.err
}

func fynePickerStartLocation() (fyne.ListableURI, bool) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil, false
	}
	for _, candidate := range []string{filepath.Join(home, "Descargas"), filepath.Join(home, "Downloads"), home} {
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}
		loc, err := storage.ListerForURI(storage.NewFileURI(candidate))
		if err == nil && loc != nil {
			return loc, true
		}
	}
	return nil, false
}

func applyDocumentPickerAppIcon(a fyne.App) {
	if a == nil {
		return
	}
	exePath, _ := os.Executable()
	binDir := filepath.Dir(exePath)
	for _, candidate := range []string{
		filepath.Join(binDir, "../lib/grxfirma/gui-qml/assets/logo_firma_grxfirma_final.png"),
		filepath.Join(binDir, "../lib64/grxfirma/gui-qml/assets/logo_firma_grxfirma_final.png"),
		filepath.Join(binDir, "assets/logo_firma_grxfirma_final.png"),
		filepath.Join(filepath.Dir(binDir), "cmd/gui-qml/assets/logo_firma_grxfirma_final.png"),
		filepath.Join(filepath.Dir(filepath.Dir(binDir)), "cmd/gui-qml/assets/logo_firma_grxfirma_final.png"),
		filepath.Join(".", "cmd/gui-qml/assets/logo_firma_grxfirma_final.png"),
	} {
		data, err := securefile.ReadFileLimit(candidate, 5*1024*1024)
		if err != nil || len(data) == 0 {
			continue
		}
		a.SetIcon(fyne.NewStaticResource("logo_firma_grxfirma_final.png", data))
		return
	}
}
