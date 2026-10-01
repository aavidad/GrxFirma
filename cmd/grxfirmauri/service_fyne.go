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
	"grxfirma/internal/appdirs"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri/triphase"
	legacyws "grxfirma/internal/adapters/inbound/legacy/websocket"
	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/application"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
)

type aprobacionInteractiva struct {
	titulo string
}

func (a aprobacionInteractiva) Request(ctx context.Context, message string) (bool, error) {
	portalActionUI()
	if _, parent := currentProtocolUIContext(); parent != nil {
		resultado := make(chan bool, 1)

		titulo := newServiceConfirmText(a.titulo, 22, true)
		texto := newServiceConfirmText(avisos.ConAvisos(message), 18, false)
		pregunta := newServiceConfirmText(tl("¿Quieres continuar con esta operación?"), 18, true)

		content := container.NewVBox(
			titulo,
			widget.NewCard("", "", container.NewPadded(texto)),
			pregunta,
		)

		var d *dialog.CustomDialog
		enviar := func(ok bool) {
			select {
			case resultado <- ok:
			default:
			}
			if d != nil {
				d.Hide()
			}
		}

		fyne.DoAndWait(func() {
			btnAceptar := widget.NewButton(tl("Aceptar"), func() { enviar(true) })
			btnCancelar := widget.NewButton(tl("Cancelar"), func() { enviar(false) })
			btnAceptar.Importance = widget.HighImportance

			botones := container.NewGridWithColumns(2, btnAceptar, btnCancelar)
			full := container.NewBorder(nil, botones, nil, nil, content)

			d = dialog.NewCustom(a.titulo, tl("Cerrar"), full, parent)
			d.Resize(fyne.NewSize(860, 430))
			d.SetOnClosed(func() {
				select {
				case resultado <- false:
				default:
				}
			})
			d.Show()
		})

		go func() {
			<-ctx.Done()
			fyne.Do(func() {
				if d != nil {
					d.Hide()
				}
			})
		}()

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case ok := <-resultado:
			return ok, nil
		}
	}

	aplicarEscalaProtocolUI("")
	preset := loadLegacyUISizePreset("")
	aplicacion := newLegacyFyneApp()
	ventana := aplicacion.NewWindow(a.titulo)
	ventana.Resize(preset.serviceSize)
	ventana.CenterOnScreen()

	resultado := make(chan bool, 1)
	enviar := func(ok bool) {
		select {
		case resultado <- ok:
		default:
		}
		ventana.Close()
	}

	titulo := newServiceConfirmText(a.titulo, 22, true)
	texto := newServiceConfirmText(message, 18, false)
	pregunta := newServiceConfirmText(tl("¿Quieres continuar con esta operación?"), 18, true)
	botones := container.NewHBox(
		widget.NewButton(tl("Aceptar"), func() { enviar(true) }),
		widget.NewButton(tl("Cancelar"), func() { enviar(false) }),
	)
	ventana.SetContent(container.NewBorder(nil, botones, nil, nil, container.NewPadded(container.NewVBox(titulo, texto, pregunta))))
	ventana.SetCloseIntercept(func() { enviar(false) })

	go func() {
		<-ctx.Done()
		enviar(false)
	}()

	ventana.ShowAndRun()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case ok := <-resultado:
		return ok, nil
	}
}

func newServiceConfirmText(text string, size float32, bold bool) fyne.CanvasObject {
	lbl := canvas.NewText(wrapServiceConfirmText(text, 92), theme.Color(theme.ColorNameForeground))
	lbl.TextSize = size
	lbl.TextStyle = fyne.TextStyle{Bold: bold}
	return lbl
}

func wrapServiceConfirmText(text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" || width < 8 {
		return text
	}
	paragraphs := strings.Split(text, "\n")
	out := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		lines := make([]string, 0, 4)
		for _, word := range words[1:] {
			if len([]rune(line))+1+len([]rune(word)) > width {
				lines = append(lines, line)
				line = word
				continue
			}
			line += " " + word
		}
		lines = append(lines, line)
		out = append(out, strings.Join(lines, "\n"))
	}
	return strings.Join(out, "\n")
}

func maybeRunDesktopService(ctx context.Context, stderr io.Writer) (bool, int) {
	return runDesktopService(ctx, stderr, false, true)
}

func runExplicitDesktopService(ctx context.Context, stderr io.Writer) int {
	_, code := runDesktopService(ctx, stderr, true, false)
	return code
}

func runDesktopService(ctx context.Context, stderr io.Writer, force, governance bool) (bool, int) {
	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	setProtocolLocalizer(configDir)
	cfg, err := config.Load(configDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", tl("Error cargando certificados"), err)
		return true, 1
	}
	if !cfg.PermiteWebSocket() {
		if force {
			_, _ = fmt.Fprintln(stderr, tl("WebSocket deshabilitado por configuración o política"))
			return true, 1
		}
		return false, 0
	}
	if !cfg.WebsocketHabilitado && !force {
		return false, 0
	}
	if force {
		cfg.WebsocketHabilitado = true
	}
	if err := requireInteractiveGraphics(stderr); err != nil {
		return true, graphicsUnavailableExitCode
	}
	if err := prepareStartupLocalTLSTrust(ctx, configDir, stderr); err != nil {
		return true, 1
	}

	aplicarEscalaProtocolUI(configDir)
	preset := loadLegacyUISizePreset(configDir)
	p12Dir := cfg.DirectorioP12
	if p12Dir == "" {
		p12Dir = filepath.Join(configDir, "pkcs12")
	}
	if override := os.Getenv("GRXFIRMA_PKCS12_DIR"); override != "" {
		p12Dir = override
	}
	p12Password := os.Getenv("GRXFIRMA_PKCS12_PASSWORD")
	proxyDiag := diagnosticarClienteHTTPRuntimeSeguro(configDir)
	if msg := formatRuntimeProxyStartupNotice(proxyDiag); msg != "" {
		_, _ = fmt.Fprintf(stderr, "%s\n", msg)
	}

	websocketAdapter, auditor, err := construirServicioWebSocket(configDir, p12Dir, p12Password)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", tl("Preparando servicio local..."), err)
		return true, 1
	}

	serviceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var srv *legacyws.TLSServer
	if governance {
		srv, err = legacyws.StartTLSServerWithGovernance(
			serviceCtx,
			cfg,
			websocketAdapter,
			filepath.Join(configDir, "tls"),
			aprobacionInteractiva{titulo: tl("Activación del WebSocket local")},
			auditor,
		)
	} else {
		srv, err = legacyws.StartTLSServer(
			serviceCtx,
			cfg,
			websocketAdapter,
			filepath.Join(configDir, "tls"),
		)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", tl("Preparando servicio local..."), err)
		return true, 1
	}
	if srv == nil {
		return true, 0
	}
	cleanupControl, err := startLegacyLaunchControlServer(serviceCtx, nil, configDir, cfg, websocketAdapter)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: %v\n", err)
		cleanupControl = func() {}
	}
	defer cleanupControl()

	aplicacion := newLegacyFyneApp()
	ventana := aplicacion.NewWindow(tl("GrxFirma — WebSocket activo"))
	setProtocolUIContext(aplicacion, ventana)
	residentOption := newTrayResidentOption(configDir)
	traySupported, cleanupTray := installLegacyTrayMenu(aplicacion, ventana, cancel)
	defer cleanupTray()
	ventana.Resize(preset.serviceSize)
	ventana.CenterOnScreen()

	indicador := widget.NewRichTextFromMarkdown(detailWithStartupTLSTrustNotice(renderEstadoWebSocketActivo(srv.Addr)))
	indicador.Wrapping = fyne.TextWrapWord
	residenteCheck := widget.NewCheck(tl("Mantener este servicio en segundo plano al cerrar"), func(checked bool) {
		residentOption.Set(checked)
	})
	residenteCheck.SetChecked(residentOption.Enabled())
	detener := widget.NewButton(tl("Detener servidor"), func() {
		cancel()
		ventana.Close()
	})
	ocultar := widget.NewButton(tl("Ocultar a bandeja"), func() {
		ventana.Hide()
	})
	ocultar.Importance = widget.LowImportance
	ocultar.Hide()
	if traySupported {
		ocultar.Show()
	}
	bodyContent := fyne.CanvasObject(container.NewPadded(indicador))
	if protocolDebugEnabled() {
		logConsole := newShellLogConsole(serviceCtx)
		bodyContent = container.NewPadded(container.NewVBox(indicador, logConsole))
	}
	ventana.SetContent(container.NewBorder(
		nil,
		container.NewVBox(residenteCheck, container.NewHBox(detener, ocultar)),
		nil,
		nil,
		bodyContent,
	))
	ventana.SetCloseIntercept(func() {
		if traySupported && residentOption.Enabled() {
			ventana.Hide()
			return
		}
		cancel()
		ventana.Close()
	})

	go func() {
		<-serviceCtx.Done()
		fyne.DoAndWait(func() {
			clearProtocolUIContext()
			ventana.SetCloseIntercept(nil)
			ventana.Close()
		})
	}()

	ventana.SetOnClosed(func() {
		if _, current := currentProtocolUIContext(); current == ventana {
			clearProtocolUIContext()
		}
	})
	if !governance {
		_, _ = fmt.Fprintf(stderr, "%s wss://%s\n", tl("GrxFirma — WebSocket activo"), srv.Addr)
		if traySupported {
			ventana.Hide()
		} else {
			ventana.Show()
		}
		aplicacion.Run()
		return true, 0
	}

	ventana.ShowAndRun()
	return true, 0
}

func renderEstadoWebSocketActivo(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	return fmt.Sprintf(
		"## %s\n\n**%s** ACTIVO\n\n**%s** `wss://%s`\n\n%s\n\n%s",
		tl("SERVIDOR WEBSOCKET ACTIVO"),
		tl("Estado:"),
		tl("Dirección:"),
		addr,
		tl("El servicio permanecerá activo mientras esta ventana siga abierta."),
		tl("Cierra esta ventana o pulsa `Detener servidor` para desactivarlo."),
	)
}

func newSignApproval() ports.UserApproval {
	if nativeProtocolUIEnabled() {
		return newNativeWindowsApproval(tl("Confirmación de firma"))
	}
	return aprobacionInteractiva{titulo: tl("Confirmación de firma")}
}

func construirServicioWebSocket(configDir, p12Dir, p12Password string) (*legacyws.Adaptador, *application.AuditUseCase, error) {
	runtime, err := construirRuntimeAfirmaURI(configDir, p12Dir, p12Password)
	if err != nil {
		return nil, nil, err
	}
	auditLogger, err := auditlog.NewDefault()
	if err != nil {
		return nil, nil, err
	}
	auditor := application.NuevoAuditUseCase(relojReal{}, auditLogger)
	approval := newSignApproval()
	firmarDirecto := application.NuevoSignDocumentUseCase(
		runtime.catalogo,
		runtime.claves,
		runtime.motor,
		approval,
		auditor,
		nil,
	)
	if firmarDirecto == nil {
		return nil, nil, errors.New("no se pudo construir el caso de uso de firma para WebSocket")
	}
	batchUC := application.NuevoProcessBatchUseCase(
		runtime.catalogo,
		runtime.claves,
		runtime.motor,
		approval,
		auditor,
		nil,
	)
	triphaseExec := triphase.New(runtime.clienteHTTP)
	legacyHandler := newLegacyWebSocketHandler(
		firmarDirecto,
		batchUC,
		runtime.catalogo,
		runtime.selector,
		newLegacyDocumentPicker(),
		runtime.claves,
		triphaseExec,
		triphase.NewBatch(triphaseExec),
	).withApproval(approval)
	return legacyws.New(runtime.parser, firmarDirecto, runtime.orquestador).
		WithTrustPolicy(runtime.trustPolicy).
		WithLegacyHandler(legacyHandler), auditor, nil
}
