// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/certpicker"
)

const desktopFyneMaxDocumentBytes = 50 * 1024 * 1024

type desktopManualUI struct {
	app       fyne.App
	window    fyne.Window
	servicios *serviciosGrxFirma

	signInput   *widget.Entry
	signOutput  *widget.Entry
	signFormat  *widget.Select
	signAction  *widget.Select
	signStatus  *widget.RichText
	signButton  *widget.Button
	toolsStatus *widget.RichText

	verifyInput    *widget.Entry
	verifyOriginal *widget.Entry
	verifyStatus   *widget.RichText
	verifyButton   *widget.Button
}

type opcionAccionDesktop struct {
	etiqueta string
	valor    string
}

var accionesDesktop = []opcionAccionDesktop{
	{etiqueta: "Firmar", valor: "sign"},
	{etiqueta: "Cofirmar", valor: "cosign"},
	{etiqueta: "Contrafirmar", valor: "countersign"},
}

func maybeRunDesktopApp(ctx context.Context, stderr io.Writer, frontend, rutaP12, password, rutaCert, rutaClave string) (bool, int) {
	if frontend == "qt" {
		return maybeRunQtDesktopApp(stderr, rutaP12, password)
	}
	desktopDebugf("arranque desktop manual")
	aplicarEscalaDesktop()
	servicios, err := construirServicios(rutaP12, password, rutaCert, rutaClave)
	if err != nil {
		desktopDebugf("error bootstrap desktop: %v", err)
		_, _ = fmt.Fprintf(stderr, "error arrancando la app desktop: %v\n", err)
		return true, 1
	}
	ui := newDesktopManualUI(servicios)
	return true, ui.run(ctx)
}

func newDesktopManualUI(servicios *serviciosGrxFirma) *desktopManualUI {
	executablePath, _ := os.Executable()
	ui := &desktopManualUI{
		app:       app.NewWithID("org.dipgra.grxfirma.desktop"),
		servicios: servicios,
	}
	for _, iconPath := range []string{
		filepath.Join(filepath.Dir(executablePath), "assets/grxfirma-icono-256.png"),
		"/usr/share/icons/hicolor/256x256/apps/grxfirma.png",
		filepath.Join(os.Getenv("HOME"), ".local/share/icons/hicolor/256x256/apps/grxfirma.png"),
		"assets/branding/grxfirma-icono-256.png",
	} {
		iconData, err := securefile.ReadFileLimit(iconPath, 5*1024*1024)
		if err == nil && len(iconData) > 0 {
			ui.app.SetIcon(fyne.NewStaticResource("grxfirma-icono-256.png", iconData))
			break
		}
	}
	ui.window = ui.app.NewWindow("GrxFirma — escritorio")
	ui.window.Resize(fyne.NewSize(1960, 1440))
	ui.window.CenterOnScreen()
	ui.window.SetContent(ui.buildContent())
	return ui
}

func (ui *desktopManualUI) run(ctx context.Context) int {
	desktopDebugf("ui desktop creada")
	go func() {
		<-ctx.Done()
		fyne.Do(func() {
			ui.window.Close()
		})
	}()
	ui.window.ShowAndRun()
	return 0
}

func (ui *desktopManualUI) buildContent() fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItem("Firmar", ui.buildSignTab()),
		container.NewTabItem("Verificar", ui.buildVerifyTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	return container.NewBorder(
		widget.NewRichTextFromMarkdown("## GrxFirma\nCliente desktop manual para firmar y verificar documentos locales."),
		nil,
		nil,
		nil,
		tabs,
	)
}

func (ui *desktopManualUI) buildSignTab() fyne.CanvasObject {
	ui.signInput = widget.NewEntry()
	ui.signInput.SetPlaceHolder("/ruta/documento.pdf")
	ui.signOutput = widget.NewEntry()
	ui.signOutput.SetPlaceHolder("/ruta/salida")
	ui.signFormat = widget.NewSelect([]string{"Auto", "CAdES", "XAdES", "PAdES", "XMLdSig", "ODF", "OOXML", "FacturaE", "ASiC-XAdES"}, func(_ string) {
		ui.actualizarSalidaSugerida()
	})
	ui.signFormat.SetSelected("Auto")
	ui.signAction = widget.NewSelect(etiquetasAccionDesktop(), nil)
	ui.signAction.SetSelected("Firmar")
	ui.signStatus = widget.NewRichTextFromMarkdown("Selecciona un documento y pulsa **Firmar y guardar**.")
	ui.toolsStatus = widget.NewRichTextFromMarkdown("Puedes importar un `P12/PFX` o abrir el gestor de certificados del navegador o del sistema.")

	abrirEntrada := widget.NewButton("Examinar documento", func() {
		ui.selectFile(ui.signInput, func(path string) {
			ui.actualizarSalidaSugerida()
			ui.setSignStatus(fmt.Sprintf("Documento seleccionado: `%s`", path))
		})
	})
	abrirSalida := widget.NewButton("Elegir salida", func() {
		sugerida := strings.TrimSpace(ui.signOutput.Text)
		if sugerida == "" {
			sugerida = construirSalidaDesktop(strings.TrimSpace(ui.signInput.Text), ui.formatoFirmaSeleccionado())
		}
		ui.selectSavePath(ui.signOutput, sugerida)
	})
	ui.signButton = widget.NewButton("Firmar y guardar", func() {
		ui.firmarDocumento()
	})
	ui.signButton.Importance = widget.HighImportance
	botonImportarCert := widget.NewButton("Importar P12/PFX…", func() {
		ui.importarCertificado()
	})
	botonGestionarCert := widget.NewButton("Gestionar certificados", func() {
		ui.abrirGestorCertificados()
	})

	form := widget.NewForm(
		widget.NewFormItem("Documento", container.NewBorder(nil, nil, nil, abrirEntrada, ui.signInput)),
		widget.NewFormItem("Formato", ui.signFormat),
		widget.NewFormItem("Acción", ui.signAction),
		widget.NewFormItem("Salida", container.NewBorder(nil, nil, nil, abrirSalida, ui.signOutput)),
	)

	return container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), ui.signButton),
		nil,
		nil,
		container.NewVBox(
			container.NewHBox(botonImportarCert, botonGestionarCert),
			ui.toolsStatus,
			widget.NewSeparator(),
			form,
			widget.NewSeparator(),
			ui.signStatus,
		),
	)
}

func (ui *desktopManualUI) buildVerifyTab() fyne.CanvasObject {
	ui.verifyInput = widget.NewEntry()
	ui.verifyInput.SetPlaceHolder("/ruta/firma.pdf | .xsig | .csig")
	ui.verifyOriginal = widget.NewEntry()
	ui.verifyOriginal.SetPlaceHolder("Opcional: documento original para firmas detached")
	ui.verifyStatus = widget.NewRichTextFromMarkdown("Selecciona una firma para verificarla.")

	abrirFirma := widget.NewButton("Examinar firma", func() {
		ui.selectFile(ui.verifyInput, func(path string) {
			if sugerido := inferirOriginalRelacionado(path); sugerido != "" && strings.TrimSpace(ui.verifyOriginal.Text) == "" {
				ui.verifyOriginal.SetText(sugerido)
			}
			ui.setVerifyStatus(fmt.Sprintf("Firma seleccionada: `%s`", path))
		})
	})
	abrirOriginal := widget.NewButton("Examinar original", func() {
		ui.selectFile(ui.verifyOriginal, nil)
	})
	ui.verifyButton = widget.NewButton("Verificar", func() {
		ui.verificarDocumento()
	})
	ui.verifyButton.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Firma", container.NewBorder(nil, nil, nil, abrirFirma, ui.verifyInput)),
		widget.NewFormItem("Original", container.NewBorder(nil, nil, nil, abrirOriginal, ui.verifyOriginal)),
	)

	return container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), ui.verifyButton),
		nil,
		nil,
		container.NewVBox(
			form,
			widget.NewSeparator(),
			ui.verifyStatus,
		),
	)
}

func (ui *desktopManualUI) actualizarSalidaSugerida() {
	entrada := strings.TrimSpace(ui.signInput.Text)
	if entrada == "" {
		return
	}
	actual := strings.TrimSpace(ui.signOutput.Text)
	if actual != "" && actual != construirSalidaDesktop(entrada, ui.formatoFirmaSeleccionado()) {
		return
	}
	ui.signOutput.SetText(construirSalidaDesktop(entrada, ui.formatoFirmaSeleccionado()))
}

func (ui *desktopManualUI) formatoFirmaSeleccionado() domain.SignatureFormat {
	return domain.SignatureFormat(inferirFormatoDesktop(ui.signFormat.Selected, ui.signInput.Text))
}

func (ui *desktopManualUI) selectFile(entry *widget.Entry, onPick func(string)) {
	dlg := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if rc == nil {
			return
		}
		path := rc.URI().Path()
		_ = rc.Close()
		entry.SetText(path)
		if onPick != nil {
			onPick(path)
		}
	}, ui.window)
	dlg.Show()
}

func (ui *desktopManualUI) chooseFile(filter storage.FileFilter) (string, error) {
	resultado := make(chan struct {
		path string
		err  error
	}, 1)
	fyne.Do(func() {
		dlg := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				resultado <- struct {
					path string
					err  error
				}{"", err}
				return
			}
			if rc == nil {
				resultado <- struct {
					path string
					err  error
				}{"", certpicker.ErrSeleccionCancelada}
				return
			}
			path := rc.URI().Path()
			_ = rc.Close()
			resultado <- struct {
				path string
				err  error
			}{path, nil}
		}, ui.window)
		if filter != nil {
			dlg.SetFilter(filter)
		}
		dlg.Show()
	})
	res := <-resultado
	return res.path, res.err
}

func (ui *desktopManualUI) pedirPasswordImportacion() (string, error) {
	resultado := make(chan struct {
		password string
		err      error
	}, 1)
	fyne.Do(func() {
		entry := widget.NewPasswordEntry()
		form := widget.NewForm(widget.NewFormItem("Contraseña", entry))
		dlg := dialog.NewCustomConfirm(
			"Contraseña del P12/PFX",
			"Continuar",
			"Cancelar",
			form,
			func(ok bool) {
				if !ok {
					resultado <- struct {
						password string
						err      error
					}{"", certpicker.ErrSeleccionCancelada}
					return
				}
				resultado <- struct {
					password string
					err      error
				}{entry.Text, nil}
			},
			ui.window,
		)
		dlg.Show()
	})
	res := <-resultado
	return res.password, res.err
}

func (ui *desktopManualUI) selectSavePath(entry *widget.Entry, sugerida string) {
	dlg := dialog.NewFileSave(func(wc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if wc == nil {
			return
		}
		path := wc.URI().Path()
		_ = wc.Close()
		entry.SetText(path)
	}, ui.window)
	if sugerida != "" {
		dlg.SetFileName(filepath.Base(sugerida))
		if dir := filepath.Dir(sugerida); dir != "" && dir != "." {
			if lister, err := storage.ListerForURI(storage.NewFileURI(dir)); err == nil {
				dlg.SetLocation(lister)
			}
		}
	}
	dlg.Show()
}

func (ui *desktopManualUI) firmarDocumento() {
	entrada := strings.TrimSpace(ui.signInput.Text)
	if entrada == "" {
		ui.setSignStatus("Selecciona primero un documento de entrada.")
		return
	}
	desktopDebugf("firma solicitada input=%s formato=%s accion=%s", entrada, ui.signFormat.Selected, valorAccionDesktop(ui.signAction.Selected))
	ui.signButton.Disable()
	ui.setSignStatus("Listando certificados y preparando la firma…")

	go func() {
		defer fyne.Do(func() {
			ui.signButton.Enable()
		})

		ctx := context.Background()
		cert, err := ui.seleccionarCertificado(ctx)
		if err != nil {
			desktopDebugf("error seleccionando certificado: %v", err)
			ui.setSignStatus("No se pudo seleccionar el certificado: " + err.Error())
			return
		}
		desktopDebugf("certificado seleccionado id=%s subject=%s", cert.ID, cert.Subject)

		data, err := securefile.ReadFileLimit(entrada, desktopFyneMaxDocumentBytes)
		if err != nil {
			desktopDebugf("error leyendo entrada: %v", err)
			ui.setSignStatus("No se pudo leer el documento: " + err.Error())
			return
		}

		cmd, err := application.NewSignCommand(
			filepath.Base(entrada),
			data,
			inferirTipoMIMEDesktop(entrada),
			string(ui.formatoFirmaSeleccionado()),
			valorAccionDesktop(ui.signAction.Selected),
			cert.ID,
			nil,
		)
		if err != nil {
			desktopDebugf("error construyendo comando de firma: %v", err)
			ui.setSignStatus("No se pudo construir la operación de firma: " + err.Error())
			return
		}

		resultado, err := ui.servicios.firmar.Execute(ctx, cmd)
		if err != nil {
			desktopDebugf("error ejecutando firma: %v", err)
			ui.setSignStatus("La firma ha fallado: " + err.Error())
			return
		}

		rutaSalida := strings.TrimSpace(ui.signOutput.Text)
		if rutaSalida == "" {
			rutaSalida = construirSalidaDesktop(entrada, resultado.Result.Format)
		}
		writer := filesystem.NuevoEscritorResultado(filesystem.PoliticaRenombrar)
		rutaFinal, err := writer.Escribir(rutaSalida, resultado.Result.Data)
		if err != nil {
			desktopDebugf("firma generada pero error guardando: %v", err)
			ui.setSignStatus("La firma se generó pero no se pudo guardar: " + err.Error())
			return
		}
		desktopDebugf("firma completada output=%s formato=%s", rutaFinal, resultado.Result.Format)

		fyne.Do(func() {
			ui.signOutput.SetText(rutaFinal)
			ui.verifyInput.SetText(rutaFinal)
			if resultado.Result.Format != domain.FormatPAdES {
				ui.verifyOriginal.SetText(entrada)
			} else {
				ui.verifyOriginal.SetText("")
			}
		})
		ui.setSignStatus(fmt.Sprintf(
			"Firma completada.\n\n- Certificado: `%s`\n- Formato: `%s`\n- Salida: `%s`",
			resultado.CertificateUsed.Subject,
			resultado.Result.Format,
			rutaFinal,
		))
	}()
}

func (ui *desktopManualUI) verificarDocumento() {
	firma := strings.TrimSpace(ui.verifyInput.Text)
	if firma == "" {
		ui.setVerifyStatus("Selecciona primero una firma a verificar.")
		return
	}
	desktopDebugf("verificación solicitada firma=%s original=%s", firma, strings.TrimSpace(ui.verifyOriginal.Text))
	ui.verifyButton.Disable()
	ui.setVerifyStatus("Verificando firma…")

	go func() {
		defer fyne.Do(func() {
			ui.verifyButton.Enable()
		})

		ctx := context.Background()
		signedData, err := securefile.ReadFileLimit(firma, desktopFyneMaxDocumentBytes)
		if err != nil {
			desktopDebugf("error leyendo firma: %v", err)
			ui.setVerifyStatus("No se pudo leer la firma: " + err.Error())
			return
		}
		signedDoc, err := domain.NewDocument(filepath.Base(firma), signedData, inferirTipoMIMEDesktop(firma))
		if err != nil {
			desktopDebugf("error preparando firma: %v", err)
			ui.setVerifyStatus("No se pudo preparar la firma: " + err.Error())
			return
		}

		var original *domain.Document
		if rutaOriginal := strings.TrimSpace(ui.verifyOriginal.Text); rutaOriginal != "" {
			data, err := securefile.ReadFileLimit(rutaOriginal, desktopFyneMaxDocumentBytes)
			if err != nil {
				desktopDebugf("error leyendo original: %v", err)
				ui.setVerifyStatus("No se pudo leer el original: " + err.Error())
				return
			}
			doc, err := domain.NewDocument(filepath.Base(rutaOriginal), data, inferirTipoMIMEDesktop(rutaOriginal))
			if err != nil {
				desktopDebugf("error preparando original: %v", err)
				ui.setVerifyStatus("No se pudo preparar el original: " + err.Error())
				return
			}
			original = &doc
		}

		resultado, err := ui.servicios.verificar.Execute(ctx, application.VerifyCommand{
			SignedDocument:   signedDoc,
			OriginalDocument: original,
		})
		if err != nil {
			desktopDebugf("error verificando: %v", err)
			mensaje := "La verificación ha fallado: " + err.Error()
			if original == nil && strings.Contains(strings.ToLower(err.Error()), "detached") {
				mensaje += "\n\nSugerencia: selecciona también el documento original."
			}
			ui.setVerifyStatus(mensaje)
			return
		}
		desktopDebugf("verificación completada valida=%t razon=%s", resultado.Verification.Valid, resultado.Verification.Reason)

		var detalle strings.Builder
		if resultado.Verification.Valid {
			detalle.WriteString("## Firma válida\n\n")
		} else {
			detalle.WriteString("## Firma no válida\n\n")
		}
		if resultado.Verification.Reason != "" {
			detalle.WriteString("- Motivo: `" + resultado.Verification.Reason + "`\n")
		}
		for _, firmante := range resultado.Firmantes {
			detalle.WriteString("- Firmante: `" + firmante.Subject + "`\n")
		}
		for _, linea := range resultado.Verification.Details {
			detalle.WriteString("- " + linea + "\n")
		}
		ui.setVerifyStatus(detalle.String())
	}()
}

func (ui *desktopManualUI) seleccionarCertificado(ctx context.Context) (domain.CertificateRef, error) {
	for intento := 0; intento < 2; intento++ {
		refs, err := ui.servicios.catalogo.List(ctx)
		if err != nil {
			return domain.CertificateRef{}, err
		}
		elegibles := make([]domain.CertificateRef, 0, len(refs))
		for _, ref := range refs {
			key, err := ui.servicios.claves.KeyFor(ctx, ref)
			if err == nil {
				elegibles = append(elegibles, ref)
			}
			ports.CloseSigningKey(key)
		}
		if len(elegibles) == 0 {
			reintentar, err := ui.resolverSinCertificados()
			if err != nil {
				return domain.CertificateRef{}, err
			}
			if reintentar {
				continue
			}
			return domain.CertificateRef{}, fmt.Errorf("no hay certificados de firma disponibles")
		}
		resultado, err := certpicker.New().WithApp(ui.app).Select(ctx, elegibles)
		if err != nil {
			return domain.CertificateRef{}, err
		}
		return resultado.Certificado, nil
	}
	return domain.CertificateRef{}, fmt.Errorf("no hay certificados de firma disponibles")
}

func (ui *desktopManualUI) setSignStatus(markdown string) {
	fyne.Do(func() {
		ui.signStatus.ParseMarkdown(markdown)
	})
}

func (ui *desktopManualUI) setVerifyStatus(markdown string) {
	fyne.Do(func() {
		ui.verifyStatus.ParseMarkdown(markdown)
	})
}

func (ui *desktopManualUI) setToolsStatus(markdown string) {
	if ui.toolsStatus == nil {
		return
	}
	fyne.Do(func() {
		ui.toolsStatus.ParseMarkdown(markdown)
	})
}

func (ui *desktopManualUI) importarCertificado() {
	ui.setToolsStatus("Selecciona un contenedor `P12/PFX` para importarlo al perfil local de GrxFirma.")
	ruta, err := ui.chooseFile(storage.NewExtensionFileFilter([]string{".p12", ".pfx"}))
	if err != nil {
		if !errors.Is(err, certpicker.ErrSeleccionCancelada) {
			ui.setToolsStatus("No se pudo abrir el selector de fichero: " + err.Error())
		}
		return
	}
	password, err := ui.pedirPasswordImportacion()
	if err != nil {
		if !errors.Is(err, certpicker.ErrSeleccionCancelada) {
			ui.setToolsStatus("No se pudo recoger la contraseña del contenedor: " + err.Error())
		}
		return
	}

	importador := pkcs12importer.New()
	identidad, err := importador.ImportP12File(context.Background(), ruta, password)
	if err != nil {
		ui.setToolsStatus("No se pudo importar el certificado: " + err.Error())
		return
	}
	if err := os.MkdirAll(ui.servicios.directorioP12, 0o700); err != nil {
		ui.setToolsStatus("No se pudo preparar el directorio de certificados: " + err.Error())
		return
	}
	data, err := securefile.ReadFileLimit(ruta, 16*1024*1024)
	if err != nil {
		ui.setToolsStatus("No se pudo leer el contenedor seleccionado: " + err.Error())
		return
	}
	destino := filepath.Join(ui.servicios.directorioP12, nombreDestinoP12Desktop(identidad.Reference))
	if err := securefile.WriteFileAtomic(destino, data, 0o600); err != nil {
		ui.setToolsStatus("No se pudo guardar el contenedor importado: " + err.Error())
		return
	}
	if err := ui.recargarServicios(); err != nil {
		ui.setToolsStatus("El certificado se guardó, pero no se pudo recargar el catálogo: " + err.Error())
		return
	}
	ui.setToolsStatus(fmt.Sprintf("Certificado importado correctamente.\n\n- Titular: `%s`\n- Huella: `%s`\n- Ruta: `%s`", identidad.Reference.Subject, identidad.Reference.Fingerprint, destino))
}

func (ui *desktopManualUI) recargarServicios() error {
	servicios, err := construirServicios(ui.servicios.rutaP12, ui.servicios.password, ui.servicios.rutaCert, ui.servicios.rutaClave)
	if err != nil {
		return err
	}
	ui.servicios = servicios
	return nil
}

func (ui *desktopManualUI) abrirGestorCertificados() {
	gestores := gestoresCertificadosDesktop(ui.servicios.directorioP12)
	if len(gestores) == 0 {
		ui.setToolsStatus("No se ha encontrado un gestor de certificados conocido. Revisa tu navegador o la carpeta local de P12 de GrxFirma.")
		return
	}
	fyne.Do(func() {
		contenido := container.NewVBox(widget.NewRichTextFromMarkdown("Selecciona dónde quieres gestionar tus certificados."))
		dlg := dialog.NewCustom("Gestores de certificados", "Cancelar", contenido, ui.window)
		for _, gestor := range gestores {
			g := gestor
			contenido.Add(widget.NewButton(g.etiqueta, func() {
				dlg.Hide()
				go func() {
					cmd := exec.Command(g.comando[0], g.comando[1:]...)
					if err := cmd.Start(); err != nil {
						ui.setToolsStatus("No se pudo abrir el gestor de certificados: " + err.Error())
						return
					}
					ui.setToolsStatus("Se ha abierto `" + g.etiqueta + "` para gestionar certificados.")
				}()
			}))
		}
		dlg.Show()
	})
}

func (ui *desktopManualUI) resolverSinCertificados() (bool, error) {
	type accionSinCertificados string
	const (
		accionImportar  accionSinCertificados = "importar"
		accionGestionar accionSinCertificados = "gestionar"
		accionCancelar  accionSinCertificados = "cancelar"
	)

	resultado := make(chan accionSinCertificados, 1)
	fyne.DoAndWait(func() {
		contenido := container.NewVBox(
			widget.NewRichTextFromMarkdown("## No hay certificados disponibles\n\nPuedes importar un `P12/PFX` al perfil local de GrxFirma o abrir el gestor de certificados del navegador o del sistema."),
		)
		dlg := dialog.NewCustom("Sin certificados", "Cancelar", contenido, ui.window)
		contenido.Add(widget.NewButton("Importar P12/PFX…", func() {
			resultado <- accionImportar
			dlg.Hide()
		}))
		contenido.Add(widget.NewButton("Gestionar certificados", func() {
			resultado <- accionGestionar
			dlg.Hide()
		}))
		dlg.SetOnClosed(func() {
			select {
			case resultado <- accionCancelar:
			default:
			}
		})
		dlg.Show()
	})
	switch <-resultado {
	case accionImportar:
		ui.importarCertificado()
		return true, nil
	case accionGestionar:
		ui.abrirGestorCertificados()
		return false, certpicker.ErrSeleccionCancelada
	default:
		return false, certpicker.ErrSeleccionCancelada
	}
}

func desktopDebugf(format string, args ...any) {
	if !logging.DebugAllowed() {
		return
	}
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("GRXFIRMA_DESKTOP_DEBUG")))
	if raw != "1" && raw != "true" && raw != "yes" {
		return
	}
	f, err := os.OpenFile("/tmp/grxfirma-desktop.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func aplicarEscalaDesktop() {
	if strings.TrimSpace(os.Getenv("FYNE_SCALE")) != "" {
		return
	}
	escala := strings.TrimSpace(os.Getenv("GRXFIRMA_DESKTOP_SCALE"))
	if escala == "" {
		escala = "4.0"
	}
	_ = os.Setenv("FYNE_SCALE", escala)
}

func etiquetasAccionDesktop() []string {
	out := make([]string, 0, len(accionesDesktop))
	for _, accion := range accionesDesktop {
		out = append(out, accion.etiqueta)
	}
	return out
}

func valorAccionDesktop(etiqueta string) string {
	for _, accion := range accionesDesktop {
		if accion.etiqueta == etiqueta {
			return accion.valor
		}
	}
	return "sign"
}
