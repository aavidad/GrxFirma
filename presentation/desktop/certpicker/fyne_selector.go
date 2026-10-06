// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package certpicker

import (
	"context"
	"errors"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/domain"
)

// FyneSelector es una implementación de CertSelector que muestra una ventana modal
// con Fyne v2 para que el usuario seleccione un certificado de una lista.
type FyneSelector struct {
	credentialActions bool
	app               fyne.App
	window            fyne.Window
	host              *fyne.Container
	restore           func()
}

const fyneSelectorAppID = "io.github.aavidad.grxfirma.afirmauri"

// New crea un FyneSelector listo para usar.
func New() *FyneSelector {
	return &FyneSelector{}
}

// WithApp asocia una aplicación Fyne existente al selector.
func (f *FyneSelector) WithApp(a fyne.App) *FyneSelector {
	f.app = a
	return f
}

// WithCredentialActions permite que el consumidor atienda carga y refresco locales.
func (f *FyneSelector) WithCredentialActions() *FyneSelector {
	f.credentialActions = true
	return f
}

// WithWindow reutiliza una ventana Fyne existente.
func (f *FyneSelector) WithWindow(w fyne.Window) *FyneSelector {
	f.window = w
	return f
}

// WithHost reutiliza un contenedor superior de una ventana existente.
func (f *FyneSelector) WithHost(host *fyne.Container, restore func()) *FyneSelector {
	f.host = host
	f.restore = restore
	return f
}

// Select abre una ventana modal con la lista de certificados disponibles.
// El usuario puede filtrar por texto y seleccionar uno con un solo clic.
// Si ctx se cancela antes de que el usuario elija, cierra la ventana y retorna ctx.Err().
// Si certs está vacío retorna error inmediatamente.
func (f *FyneSelector) Select(ctx context.Context, certs []domain.CertificateRef) (ResultadoSeleccion, error) {
	select {
	case <-ctx.Done():
		return ResultadoSeleccion{}, ctx.Err()
	default:
	}

	if len(certs) == 0 && !f.credentialActions {
		return ResultadoSeleccion{}, errors.New(tp("no hay certificados disponibles"))
	}
	slog.Info("certpicker_select_start", "count", len(certs), "reuse_window", f.window != nil, "reuse_host", f.window != nil && f.host != nil)

	var resultado ResultadoSeleccion
	var resultErr error
	done := make(chan struct{})
	a := f.app
	ownsApp := false
	reuseWindow := f.window != nil
	if a == nil {
		a = fyne.CurrentApp()
		if a == nil {
			a = app.NewWithID(fyneSelectorAppID)
			applySelectorAppIcon(a)
			ownsApp = true
		}
	}
	var w fyne.Window
	var previousTitle string
	var previousFixed bool
	var previousSize fyne.Size
	restoreWindow := func() {}
	reuseHost := reuseWindow && f.host != nil
	if reuseWindow {
		w = f.window
		previousTitle = w.Title()
		previousFixed = w.FixedSize()
		previousSize = w.Canvas().Size()
		restoreWindow = func() {
			slog.Info("certpicker_restore_window", "reuse_host", reuseHost)
			if f.restore != nil {
				f.restore()
			} else {
				fyne.Do(func() {
					w.SetTitle(previousTitle)
					w.SetFixedSize(previousFixed)
					if previousSize.Width > 0 && previousSize.Height > 0 {
						w.Resize(previousSize)
					}
				})
			}
		}
	}
	createWindow := func() {
		if !reuseWindow {
			w = a.NewWindow(tp("Seleccionar certificado"))
		}
		slog.Info("certpicker_create_window", "reuse_window", reuseWindow)
		w.SetTitle(tp("Seleccionar certificado"))
		w.Resize(fyne.NewSize(1180, 760))
		w.SetFixedSize(false)
		w.CenterOnScreen()
	}
	if ownsApp {
		createWindow()
	} else {
		fyne.DoAndWait(createWindow)
	}
	var cerrarVentanaOnce sync.Once
	var cerrarDoneOnce sync.Once

	cerrarDone := func() {
		cerrarDoneOnce.Do(func() {
			close(done)
		})
	}
	cerrarVentana := func() {
		cerrarVentanaOnce.Do(func() {
			slog.Info("certpicker_close", "reuse_window", reuseWindow)
			if reuseWindow {
				restoreWindow()
				cerrarDone()
				return
			}
			fyne.Do(func() {
				w.SetCloseIntercept(nil)
				w.Close()
			})
		})
	}

	colorTitulo := color.NRGBA{R: 241, G: 245, B: 249, A: 255}
	colorTextoPrincipal := color.NRGBA{R: 248, G: 250, B: 252, A: 255}
	colorTextoSecundario := color.NRGBA{R: 203, G: 213, B: 225, A: 255}
	colorFondoTarjeta := color.NRGBA{R: 30, G: 41, B: 59, A: 255}
	colorBordeTarjeta := color.NRGBA{R: 71, G: 85, B: 105, A: 255}
	colorFondoSeleccion := color.NRGBA{R: 12, G: 74, B: 110, A: 255}
	colorBordeSeleccion := color.NRGBA{R: 14, G: 116, B: 144, A: 255}

	filtered := make([]domain.CertificateRef, len(certs))
	copy(filtered, certs)
	recordarSeleccion := widget.NewCheck("", nil)
	recordarLabel := canvas.NewText(tp("Recordar esta selección"), colorTitulo)
	recordarLabel.TextSize = 24
	recordarLabel.TextStyle = fyne.TextStyle{Bold: true}
	modoRecordar := RecordarSesion
	opcionSesion := widget.NewCheck("", nil)
	opcionSesionLabel := canvas.NewText(tp("Solo esta sesión"), colorTextoPrincipal)
	opcionSesionLabel.TextSize = 22
	opcionSiempre := widget.NewCheck("", nil)
	opcionSiempreLabel := canvas.NewText(tp("Siempre"), colorTextoPrincipal)
	opcionSiempreLabel.TextSize = 22
	opcionSesion.SetChecked(true)
	opcionSiempre.Disable()
	opcionSesion.Disable()
	opcionSesion.OnChanged = func(activo bool) {
		if !activo {
			if !opcionSiempre.Checked {
				opcionSesion.SetChecked(true)
			}
			return
		}
		modoRecordar = RecordarSesion
		if opcionSiempre.Checked {
			opcionSiempre.SetChecked(false)
		}
	}
	opcionSiempre.OnChanged = func(activo bool) {
		if !activo {
			if !opcionSesion.Checked {
				opcionSiempre.SetChecked(true)
			}
			return
		}
		modoRecordar = RecordarSiempre
		if opcionSesion.Checked {
			opcionSesion.SetChecked(false)
		}
	}
	recordarSeleccion.OnChanged = func(activo bool) {
		if activo {
			opcionSesion.Enable()
			opcionSiempre.Enable()
		} else {
			opcionSesion.Disable()
			opcionSiempre.Disable()
		}
	}

	selectedIdx := -1
	selectByID := func(id string) {
		selectedIdx = -1
		for i, cert := range filtered {
			if cert.ID == id {
				selectedIdx = i
				break
			}
		}
	}
	confirmarSeleccion := func() {
		if selectedIdx < 0 || selectedIdx >= len(filtered) {
			return
		}
		slog.Info("certpicker_confirm_selection", "certificate_id", filtered[selectedIdx].ID)
		resultado.Certificado = filtered[selectedIdx]
		if recordarSeleccion.Checked {
			if modoRecordar == RecordarSiempre {
				resultado.Recuerdo = RecordarSiempre
			} else {
				resultado.Recuerdo = RecordarSesion
			}
		} else {
			resultado.Recuerdo = NoRecordar
		}
		resultErr = nil
		cerrarVentana()
	}

	titulo := canvas.NewText(tp("CERTIFICADOS"), colorTitulo)
	titulo.TextSize = 20
	titulo.TextStyle = fyne.TextStyle{Bold: true}

	certList := widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject {
			estado := canvas.NewText(tp("Activo"), color.NRGBA{R: 23, G: 132, B: 78, A: 255})
			estado.TextSize = 12
			estado.TextStyle = fyne.TextStyle{Bold: true}

			subject := canvas.NewText("-", colorTextoPrincipal)
			subject.TextSize = 16
			subject.TextStyle = fyne.TextStyle{Bold: true}

			issuer := canvas.NewText("-", colorTextoSecundario)
			issuer.TextSize = 15

			expiry := canvas.NewText("-", colorTextoSecundario)
			expiry.TextSize = 15

			top := container.NewBorder(nil, nil, nil, estado, subject)
			card := container.NewVBox(top, issuer, expiry)
			bg := canvas.NewRectangle(colorFondoTarjeta)
			bg.CornerRadius = 12
			border := canvas.NewRectangle(colorBordeTarjeta)
			border.StrokeColor = colorBordeTarjeta
			border.StrokeWidth = 1
			border.CornerRadius = 12

			return container.NewStack(
				bg,
				border,
				container.NewPadded(card),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			stack := obj.(*fyne.Container)
			bg := stack.Objects[0].(*canvas.Rectangle)
			border := stack.Objects[1].(*canvas.Rectangle)
			cardWrap := stack.Objects[2].(*fyne.Container)
			card := cardWrap.Objects[0].(*fyne.Container)
			top := card.Objects[0].(*fyne.Container)
			subject := top.Objects[0].(*canvas.Text)
			estado := top.Objects[1].(*canvas.Text)
			issuer := card.Objects[1].(*canvas.Text)
			expiry := card.Objects[2].(*canvas.Text)
			if id < len(filtered) {
				c := filtered[id]
				subject.Text = c.Subject
				issuer.Text = c.Issuer
				expiry.Text = tp("Válido hasta %s", c.NotAfter.Format("02/01/2006"))
				if c.IsExpired(time.Now()) {
					estado.Text = tp("Caducado")
					estado.Color = color.NRGBA{R: 184, G: 28, B: 28, A: 255}
				} else if c.SigningKeyNeedsUnlock {
					estado.Text = tp("Requiere autorización")
					estado.Color = color.NRGBA{R: 253, G: 230, B: 138, A: 255}
				} else {
					estado.Text = tp("Activo")
					estado.Color = color.NRGBA{R: 23, G: 132, B: 78, A: 255}
				}
				if selectedIdx == id {
					bg.FillColor = colorFondoSeleccion
					border.StrokeColor = colorBordeSeleccion
					border.StrokeWidth = 2
					subject.Color = colorTextoPrincipal
					issuer.Color = colorTextoSecundario
					expiry.Color = colorTextoSecundario
					if c.IsExpired(time.Now()) {
						estado.Color = color.NRGBA{R: 254, G: 202, B: 202, A: 255}
					} else if c.SigningKeyNeedsUnlock {
						estado.Color = color.NRGBA{R: 253, G: 230, B: 138, A: 255}
					} else {
						estado.Color = color.NRGBA{R: 187, G: 247, B: 208, A: 255}
					}
				} else {
					bg.FillColor = colorFondoTarjeta
					border.StrokeColor = colorBordeTarjeta
					border.StrokeWidth = 1
					subject.Color = colorTextoPrincipal
					issuer.Color = colorTextoSecundario
					expiry.Color = colorTextoSecundario
				}
				bg.Refresh()
				border.Refresh()
				subject.Refresh()
				estado.Refresh()
				issuer.Refresh()
				expiry.Refresh()
			}
		},
	)
	detalleTitulo := canvas.NewText(tp("Detalles del certificado"), colorTitulo)
	detalleTitulo.TextSize = 16
	detalleTitulo.TextStyle = fyne.TextStyle{Bold: true}

	detalleTitularTitulo := canvas.NewText(tp("Titular"), colorTitulo)
	detalleTitularTitulo.TextSize = 13
	detalleTitularTitulo.TextStyle = fyne.TextStyle{Bold: true}
	detalleTitularValor := canvas.NewText(tp("Selecciona un certificado de la lista."), colorTextoPrincipal)
	detalleTitularValor.TextSize = 16

	detalleEmisorTitulo := canvas.NewText(tp("Emisor"), colorTitulo)
	detalleEmisorTitulo.TextSize = 13
	detalleEmisorTitulo.TextStyle = fyne.TextStyle{Bold: true}
	detalleEmisorValor := canvas.NewText("-", colorTextoPrincipal)
	detalleEmisorValor.TextSize = 16

	detalleSerieTitulo := canvas.NewText(tp("Identificador"), colorTitulo)
	detalleSerieTitulo.TextSize = 13
	detalleSerieTitulo.TextStyle = fyne.TextStyle{Bold: true}
	detalleSerieValor := canvas.NewText("-", colorTextoPrincipal)
	detalleSerieValor.TextSize = 16

	detalleValidezTitulo := canvas.NewText(tp("Válido hasta"), colorTitulo)
	detalleValidezTitulo.TextSize = 13
	detalleValidezTitulo.TextStyle = fyne.TextStyle{Bold: true}
	detalleValidezValor := canvas.NewText("-", colorTextoPrincipal)
	detalleValidezValor.TextSize = 16

	detalleHuellaTitulo := canvas.NewText(tp("Huella"), colorTitulo)
	detalleHuellaTitulo.TextSize = 13
	detalleHuellaTitulo.TextStyle = fyne.TextStyle{Bold: true}
	detalleHuellaValor := canvas.NewText("-", colorTextoPrincipal)
	detalleHuellaValor.TextSize = 16

	actualizarDetalles := func() {
		if selectedIdx < 0 || selectedIdx >= len(filtered) {
			detalleTitularValor.Text = tp("Selecciona un certificado de la lista.")
			detalleEmisorValor.Text = "-"
			detalleSerieValor.Text = "-"
			detalleValidezValor.Text = "-"
			detalleValidezValor.Color = colorTextoPrincipal
			detalleHuellaValor.Text = "-"
			detalleTitularValor.Refresh()
			detalleEmisorValor.Refresh()
			detalleSerieValor.Refresh()
			detalleValidezValor.Refresh()
			detalleHuellaValor.Refresh()
			return
		}
		cert := filtered[selectedIdx]
		detalleTitularValor.Text = cert.Subject
		detalleEmisorValor.Text = cert.Issuer
		detalleSerieValor.Text = cert.ID
		detalleValidezValor.Text = cert.NotAfter.Format("02/01/2006 15:04")
		if cert.IsExpired(time.Now()) {
			detalleValidezValor.Color = color.NRGBA{R: 254, G: 202, B: 202, A: 255}
		} else {
			detalleValidezValor.Color = color.NRGBA{R: 134, G: 239, B: 172, A: 255}
		}
		detalleHuellaValor.Text = cert.Fingerprint
		detalleTitularValor.Refresh()
		detalleEmisorValor.Refresh()
		detalleSerieValor.Refresh()
		detalleValidezValor.Refresh()
		detalleHuellaValor.Refresh()
	}
	certList.OnSelected = func(id widget.ListItemID) {
		selectedIdx = id
		actualizarDetalles()
		certList.Refresh()
	}

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(tp("Filtrar por nombre o emisor..."))

	helpLabel := canvas.NewText(tp("Selecciona un certificado y pulsa “Usar certificado”."), colorTextoPrincipal)
	if len(certs) == 0 {
		helpLabel.Text = tp("No hay certificados. Puedes abrir un archivo P12/PFX.")
	}
	helpLabel.TextSize = 12

	searchEntry.OnChanged = func(query string) {
		var selectedID string
		if selectedIdx >= 0 && selectedIdx < len(filtered) {
			selectedID = filtered[selectedIdx].ID
		}
		query = strings.ToLower(query)
		filtered = filtered[:0]
		for _, c := range certs {
			if query == "" ||
				strings.Contains(strings.ToLower(c.Subject), query) ||
				strings.Contains(strings.ToLower(c.Issuer), query) {
				filtered = append(filtered, c)
			}
		}
		selectByID(selectedID)
		actualizarDetalles()
		certList.UnselectAll()
		if selectedIdx >= 0 {
			certList.Select(selectedIdx)
		}
		certList.Refresh()
	}

	usarBtn := widget.NewButtonWithIcon(tp("Usar certificado"), theme.ConfirmIcon(), confirmarSeleccion)
	usarBtn.Importance = widget.HighImportance
	if len(certs) == 0 {
		usarBtn.Disable()
	}
	credentialButtons := container.NewVBox()
	if f.credentialActions {
		otherMethods := widget.NewLabel(tp("Otras formas de firmar: certificado del sistema o P12/PFX. Cl@ve Firma requiere que la sede ofrezca esa integración. DNIe y tokens dependen del almacén y del dispositivo; aún no están validados aquí."))
		otherMethods.Wrapping = fyne.TextWrapWord
		credentialButtons.Add(widget.NewButton(tp("Usar un archivo P12/PFX…"), func() {
			resultErr = ErrCargarCertificado
			cerrarVentana()
		}))
		credentialButtons.Add(widget.NewButton(tp("Actualizar certificados"), func() {
			resultErr = ErrActualizarCertificados
			cerrarVentana()
		}))
		credentialButtons.Add(otherMethods)
	}

	cancelBtn := widget.NewButton(tp("Cancelar"), func() {
		resultErr = ErrSeleccionCancelada
		cerrarVentana()
	})
	cancelBtn.Importance = widget.DangerImportance

	cabecera := container.NewVBox(titulo)
	if solicitud, ok := ContextoSolicitudDe(ctx); ok {
		origen := solicitud.Origen
		if origen == "" {
			origen = tp("origen no identificado")
		}
		aviso := widget.NewLabel(tp("Solicitado por") + ": " + origen + "\n" + tp("Operación") + ": " + solicitud.Operacion)
		aviso.Wrapping = fyne.TextWrapWord
		aviso.TextStyle = fyne.TextStyle{Bold: true}
		cabecera.Add(aviso)
	}
	cabecera.Add(searchEntry)
	panelIzquierdo := container.NewBorder(
		cabecera,
		nil,
		nil,
		nil,
		certList,
	)

	cardDetalleBg := canvas.NewRectangle(colorFondoTarjeta)
	cardDetalleBg.CornerRadius = 12
	cardDetalleBorder := canvas.NewRectangle(color.Transparent)
	cardDetalleBorder.StrokeColor = colorBordeTarjeta
	cardDetalleBorder.StrokeWidth = 1
	cardDetalleBorder.CornerRadius = 12
	panelDetalle := container.NewStack(
		cardDetalleBg,
		cardDetalleBorder,
		container.NewPadded(container.NewVBox(
			detalleTitulo,
			detalleTitularTitulo,
			detalleTitularValor,
			detalleEmisorTitulo,
			detalleEmisorValor,
			detalleSerieTitulo,
			detalleSerieValor,
			detalleValidezTitulo,
			detalleValidezValor,
			detalleHuellaTitulo,
			detalleHuellaValor,
			widget.NewSeparator(),
			container.NewHBox(recordarSeleccion, recordarLabel),
			container.NewHBox(opcionSesion, opcionSesionLabel),
			container.NewHBox(opcionSiempre, opcionSiempreLabel),
			widget.NewSeparator(),
			helpLabel,
			credentialButtons,
			container.NewHBox(usarBtn, cancelBtn),
		)),
	)

	contenidoCentral := container.NewHSplit(
		container.NewPadded(panelIzquierdo),
		container.NewPadded(container.NewVScroll(panelDetalle)),
	)
	contenidoCentral.Offset = 0.47
	content := container.NewPadded(contenidoCentral)
	configureWindow := func() {
		if reuseHost {
			f.host.Objects = []fyne.CanvasObject{content}
			f.host.Refresh()
			w.SetTitle(tp("Seleccionar certificado"))
			w.Resize(fyne.NewSize(1180, 900))
			return
		}
		w.SetContent(content)
		if reuseWindow {
			return
		}
		w.SetCloseIntercept(func() {
			if resultErr == nil {
				resultErr = ErrSeleccionCancelada
			}
			w.SetCloseIntercept(nil)
			w.Close()
		})
		w.SetOnClosed(func() {
			cerrarDone()
		})
	}
	if ownsApp {
		configureWindow()
	} else {
		fyne.DoAndWait(configureWindow)
	}

	go func() {
		select {
		case <-ctx.Done():
			resultErr = ctx.Err()
			cerrarVentana()
		case <-done:
		}
	}()

	if ownsApp {
		w.Show()
		a.Run()
	} else if !reuseWindow {
		fyne.Do(func() {
			w.Show()
		})
	}

	<-done

	return resultado, resultErr
}

func applySelectorAppIcon(a fyne.App) {
	if a == nil {
		return
	}
	for _, candidate := range selectorIconCandidates() {
		data, err := os.ReadFile(candidate)
		if err != nil || len(data) == 0 {
			continue
		}
		a.SetIcon(fyne.NewStaticResource("grxfirma-icono-256.png", data))
		return
	}
}

func selectorIconCandidates() []string {
	exePath, _ := os.Executable()
	binDir := filepath.Dir(exePath)
	return []string{
		filepath.Join(binDir, "../lib/grxfirma/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(binDir, "../lib64/grxfirma/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(binDir, "assets/grxfirma-icono-256.png"),
		filepath.Join(filepath.Dir(binDir), "cmd/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(filepath.Dir(filepath.Dir(binDir)), "cmd/gui-qml/assets/grxfirma-icono-256.png"),
		filepath.Join(".", "cmd/gui-qml/assets/grxfirma-icono-256.png"),
	}
}
