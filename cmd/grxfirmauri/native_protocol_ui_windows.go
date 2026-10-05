// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui && amd64

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/adapters/outbound/common/securefile"
	desktopdocumentpicker "grxfirma/internal/adapters/outbound/desktop/documentpicker"
	"grxfirma/internal/adapters/outbound/desktop/wintaskdialog"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
	"grxfirma/presentation/desktop/certpicker"
	"grxfirma/presentation/desktop/progressdialog"
	"grxfirma/presentation/desktop/trustdialog"
)

const (
	nativeUIForceEnvironment = "GRXFIRMA_WINDOWS_NATIVE_UI"

	nativeButtonAccept              = 1101
	nativeButtonOnce                = 1102
	nativeButtonCancel              = 1103
	nativeButtonLoadCertificate     = 1104
	nativeButtonRefreshCertificates = 1105
	nativeRadioBase                 = 2000

	idCancel = 2

	tdfAllowDialogCancellation  = 0x0008
	tdfPositionRelativeToWindow = 0x1000
	tdfNoDefaultRadioButton     = 0x4000
	tdfSizeToContent            = 0x01000000

	tdnCreated              = 0
	tdnButtonClicked        = 2
	tdnDestroyed            = 5
	tdnRadioButtonClicked   = 6
	monitorDefaultToNearest = 2
	swHide                  = 0
	swShowNoActivate        = 4
	swShow                  = 5
	swpNoSize               = 0x0001
	swpNoActivate           = 0x0010
	spiGetWorkArea          = 0x0030
	nimAdd                  = 0
	nimModify               = 1
	nimDelete               = 2
	nifIcon                 = 0x00000002
	nifTip                  = 0x00000004
	nifInfo                 = 0x00000010
	niifInfo                = 0x00000001

	wmUser  = 0x0400
	wmClose = 0x0010

	tdmClickButton    = wmUser + 102
	tdmNavigatePage   = wmUser + 101
	tdmSetElementText = wmUser + 108

	tdeContent         = 0
	tdeMainInstruction = 3

	ofnOverwritePrompt     = 0x00000002
	ofnNoChangeDir         = 0x00000008
	ofnEnableHook          = 0x00000020
	ofnAllowMultiSelect    = 0x00000200
	ofnPathMustExist       = 0x00000800
	ofnFileMustExist       = 0x00001000
	ofnNoReadOnlyReturn    = 0x00008000
	ofnExplorer            = 0x00080000
	ofnDontAddToRecent     = 0x02000000
	ofnForceFileSystem     = 0x10000000
	wmInitDialog           = 0x0110
	nativeMaxCertificates  = 64
	nativeMaxDocumentBytes = 64 * 1024 * 1024

	nativeTaskDialogConfigSize = 160
	nativeTaskDialogButtonSize = 12
)

var (
	nativeProtocolFallback      atomic.Bool
	nativeProtocolShellHWND     atomic.Uintptr
	nativeProtocolFailure       atomic.Bool
	nativeProtocolComplete      atomic.Bool
	nativeProtocolShellState    atomic.Pointer[nativeTaskDialogState]
	nativeProtocolPreviousHWND  atomic.Uintptr
	nativeProtocolNotifyHWND    atomic.Uintptr
	nativeProtocolSuccessShown  atomic.Bool
	nativeProtocolDeliveredKind atomic.Value
	nativeProtocolErrorText     atomic.Pointer[nativeProtocolErrorState]

	comctl32Native = windows.NewLazySystemDLL("comctl32.dll")
	comdlg32Native = windows.NewLazySystemDLL("comdlg32.dll")
	user32Native   = windows.NewLazySystemDLL("user32.dll")

	procTaskDialogIndirectNative  = comctl32Native.NewProc("TaskDialogIndirect")
	procGetOpenFileNameWNative    = comdlg32Native.NewProc("GetOpenFileNameW")
	procGetSaveFileNameWNative    = comdlg32Native.NewProc("GetSaveFileNameW")
	procCommDlgExtendedError      = comdlg32Native.NewProc("CommDlgExtendedError")
	procSendMessageWNative        = user32Native.NewProc("SendMessageW")
	procPostMessageWNative        = user32Native.NewProc("PostMessageW")
	procIsWindowNative            = user32Native.NewProc("IsWindow")
	procGetWindowThreadProcessID  = user32Native.NewProc("GetWindowThreadProcessId")
	procGetParentNative           = user32Native.NewProc("GetParent")
	procGetDlgItemNative          = user32Native.NewProc("GetDlgItem")
	procSetWindowTextWNative      = user32Native.NewProc("SetWindowTextW")
	procSetForegroundWindowNative = user32Native.NewProc("SetForegroundWindow")
	procGetForegroundWindowNative = user32Native.NewProc("GetForegroundWindow")
	procSetWindowPosNative        = user32Native.NewProc("SetWindowPos")
	procShowWindowNative          = user32Native.NewProc("ShowWindow")
	procGetWindowRectNative       = user32Native.NewProc("GetWindowRect")
	procMonitorFromWindowNative   = user32Native.NewProc("MonitorFromWindow")
	procGetMonitorInfoWNative     = user32Native.NewProc("GetMonitorInfoW")
	procAllowSetForegroundNative  = user32Native.NewProc("AllowSetForegroundWindow")
	procLoadIconWNative           = user32Native.NewProc("LoadIconW")
	procMessageBeepNative         = user32Native.NewProc("MessageBeep")
	procShellNotifyIconWNative    = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")

	nativeTaskDialogCallback = windows.NewCallback(nativeTaskDialogCallbackProc)
	nativeFileDialogHook     = windows.NewCallback(nativeFileDialogHookProc)
)

type nativeTaskDialogConfig [nativeTaskDialogConfigSize]byte

type nativeDialogChoice struct {
	ID    int32
	Label string
}

type nativeDialogSpec struct {
	Title            string
	Instruction      string
	Content          string
	Buttons          []nativeDialogChoice
	DefaultButton    int32
	RadioButtons     []nativeDialogChoice
	AcceptNeedsRadio int32
	OnCreated        func(uintptr)
	OnDestroyed      func(uintptr)
	LegacyShell      bool
	PassiveShell     bool
}

type nativeTaskDialogState struct {
	radioIDs         []int32
	radioLabels      []string
	created          chan uintptr
	acceptNeedsRadio int32
	selectedRadio    atomic.Int32
	invalidWindow    atomic.Bool
	onCreated        func(uintptr)
	onDestroyed      func(uintptr)
	passiveShell     bool
}

type nativeRect struct{ Left, Top, Right, Bottom int32 }

type nativeMonitorInfo struct {
	Size    uint32
	Monitor nativeRect
	Work    nativeRect
	Flags   uint32
}

// NOTIFYICONDATAW x64: el icono solo anuncia la entrega; no recibe acciones.
type nativeNotifyIconData struct {
	Size            uint32
	_               uint32
	Window          uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	_               uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Timeout         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            [16]byte
	BalloonIcon     uintptr
}

type nativeProtocolErrorState struct {
	status string
	detail string
}

type nativeOpenFileName struct {
	Size             uint32
	Owner            uintptr
	Instance         uintptr
	Filter           *uint16
	CustomFilter     *uint16
	MaxCustomFilter  uint32
	FilterIndex      uint32
	File             *uint16
	MaxFile          uint32
	FileTitle        *uint16
	MaxFileTitle     uint32
	InitialDirectory *uint16
	Title            *uint16
	Flags            uint32
	FileOffset       uint16
	FileExtension    uint16
	DefaultExtension *uint16
	CustomData       uintptr
	Hook             uintptr
	TemplateName     *uint16
	Reserved         uintptr
	Reserved32       uint32
	FlagsEx          uint32
}

type nativeFileDialogState struct {
	created       chan uintptr
	invalidWindow atomic.Bool
}

type nativeProtocolPolicyError struct {
	message string
}

func (e *nativeProtocolPolicyError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

type nativeWindowsApproval struct {
	title string
}

type nativeWindowsCertSelector struct{}

type nativeWindowsTrustUI struct{}

type nativeWindowsProgressProvider struct{}

type nativeWindowsProgressReporter struct {
	title string
}

func enableNativeProtocolUIFallback(error) bool {
	if err := procTaskDialogIndirectNative.Find(); err != nil {
		return false
	}
	if err := procGetOpenFileNameWNative.Find(); err != nil {
		return false
	}
	if err := procGetSaveFileNameWNative.Find(); err != nil {
		return false
	}
	nativeProtocolFallback.Store(true)
	return true
}

func disableNativeProtocolUIFallback() {
	nativeProtocolFallback.Store(false)
	nativeProtocolShellHWND.Store(0)
	nativeProtocolPreviousHWND.Store(0)
}

func nativeProtocolUIEnabled() bool {
	return nativeProtocolFallback.Load()
}

func nativeProtocolUIForced() bool {
	switch strings.ToLower(strings.TrimSpace(
		os.Getenv(nativeUIForceEnvironment),
	)) {
	case "1", "true", "yes", "si", "on":
		return true
	default:
		return false
	}
}

func validateNativeProtocolOperation(
	operation afirmauri.TipoOperacion,
) error {
	if !nativeProtocolUIEnabled() {
		return nil
	}
	switch operation {
	case afirmauri.OperacionFirma,
		afirmauri.OperacionLote,
		afirmauri.OperacionSelectCert,
		afirmauri.OperacionSave,
		afirmauri.OperacionLoad,
		afirmauri.OperacionSignSave:
		return nil
	default:
		return &nativeProtocolPolicyError{message: fmt.Sprintf(
			"Fallo local: la operación %q no está disponible en la interfaz nativa de compatibilidad. No se ha ejecutado ninguna acción.",
			sanitizeNativeDialogText(string(operation), 64),
		)}
	}
}

func newNativeWindowsApproval(title string) ports.UserApproval {
	return nativeWindowsApproval{title: title}
}

func (a nativeWindowsApproval) Request(
	ctx context.Context,
	message string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	button, _, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       a.title,
		Instruction: "Confirma la operación de firma",
		Content: sanitizeNativeDialogText(avisos.ConAvisos(message), 2048) +
			"\r\n\r\nContinúa solo si reconoce el documento, el certificado y la solicitud del portal.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonAccept, Label: "Continuar y firmar"},
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton: nativeButtonCancel,
	})
	if err != nil {
		return false, err
	}
	return button == nativeButtonAccept, nil
}

func newNativeWindowsCertSelector() certpicker.CertSelector {
	return nativeWindowsCertSelector{}
}

func (nativeWindowsCertSelector) Select(
	ctx context.Context,
	certificates []domain.CertificateRef,
) (certpicker.ResultadoSeleccion, error) {
	if err := ctx.Err(); err != nil {
		return certpicker.ResultadoSeleccion{}, err
	}
	if len(certificates) > nativeMaxCertificates {
		return certpicker.ResultadoSeleccion{}, fmt.Errorf(
			"fallo local: hay %d certificados y la interfaz nativa solo puede presentar %d de forma segura; no se ha seleccionado ninguno",
			len(certificates),
			nativeMaxCertificates,
		)
	}

	seenIDs := make(map[string]struct{}, len(certificates))
	radios := make([]nativeDialogChoice, 0, len(certificates))
	defer clear(radios)
	for index, certificate := range certificates {
		id := strings.TrimSpace(certificate.ID)
		if id == "" {
			return certpicker.ResultadoSeleccion{},
				errors.New("fallo local: el catálogo contiene un certificado sin identificador estable")
		}
		if _, exists := seenIDs[id]; exists {
			return certpicker.ResultadoSeleccion{},
				errors.New("fallo local: el catálogo contiene identificadores de certificado duplicados")
		}
		seenIDs[id] = struct{}{}
		radios = append(radios, nativeDialogChoice{
			ID:    nativeRadioBase + int32(index),
			Label: nativeCertificateLabel(certificate),
		})
	}

	button, radio, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Seleccionar certificado",
		Instruction: "Elige el certificado para esta operación",
		Content:     "Selecciona un certificado del sistema o abre un archivo P12/PFX para esta operación. No hay ningún certificado preseleccionado.\r\n\r\nOtras formas de firmar: Cl@ve Firma requiere que la sede ofrezca esa integración. DNIe y tokens dependen del almacén y del dispositivo; aún no están validados aquí.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonAccept, Label: "Usar el certificado seleccionado"},
			{ID: nativeButtonLoadCertificate, Label: "Usar un archivo P12/PFX…"},
			{ID: nativeButtonRefreshCertificates, Label: "Actualizar certificados"},
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton:    nativeButtonCancel,
		RadioButtons:     radios,
		AcceptNeedsRadio: nativeButtonAccept,
	})
	if err != nil {
		return certpicker.ResultadoSeleccion{}, err
	}
	if button == nativeButtonLoadCertificate {
		return certpicker.ResultadoSeleccion{}, certpicker.ErrCargarCertificado
	}
	if button == nativeButtonRefreshCertificates {
		return certpicker.ResultadoSeleccion{}, certpicker.ErrActualizarCertificados
	}
	if button != nativeButtonAccept {
		return certpicker.ResultadoSeleccion{},
			certpicker.ErrSeleccionCancelada
	}
	index := int(radio - nativeRadioBase)
	if index < 0 || index >= len(certificates) {
		return certpicker.ResultadoSeleccion{},
			errors.New("fallo local: no se confirmó un certificado válido")
	}
	return certpicker.ResultadoSeleccion{
		Certificado: certificates[index],
		Recuerdo:    certpicker.NoRecordar,
	}, nil
}

// nativeButtonSinSello firma sin sello visible cuando la web pide situarlo.
const nativeButtonSinSello = 1106

var nativeEtiquetasPosicionSello = map[string]string{
	"superior-izquierda": "Arriba a la izquierda",
	"superior-centro":    "Arriba en el centro",
	"superior-derecha":   "Arriba a la derecha",
	"inferior-izquierda": "Abajo a la izquierda",
	"inferior-centro":    "Abajo en el centro",
	"inferior-derecha":   "Abajo a la derecha",
}

// ElegirPosicionSello deja al usuario situar la firma visible cuando la web
// lo pide (visibleSignature=want), como AutoFirma Java.
func (nativeWindowsCertSelector) ElegirPosicionSello(ctx context.Context) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	posiciones := make([]nativeDialogChoice, 0, len(domain.PosicionesSello))
	for i, p := range domain.PosicionesSello {
		posiciones = append(posiciones, nativeDialogChoice{ID: nativeRadioBase + int32(i), Label: nativeEtiquetasPosicionSello[p]}) // #nosec G115 -- seis posiciones fijas.
	}
	button, radio, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Firma visible",
		Instruction: "¿Dónde quieres colocar la firma visible?",
		Content:     "La web ha pedido que elijas dónde aparece la firma en el PDF. Se añadirá un sello con tu nombre y la fecha en la posición que elijas. También puedes firmar sin sello visible: la firma es igual de válida.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonAccept, Label: "Colocar la firma aquí"},
			{ID: nativeButtonSinSello, Label: "Firmar sin sello visible"},
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton:    nativeButtonCancel,
		RadioButtons:     posiciones,
		AcceptNeedsRadio: nativeButtonAccept,
	})
	if err != nil {
		return "", "", err
	}
	switch button {
	case nativeButtonSinSello:
		return "", "", certpicker.ErrSinSelloVisible
	case nativeButtonAccept:
	default:
		return "", "", certpicker.ErrSeleccionCancelada
	}
	indice := int(radio - nativeRadioBase)
	if indice < 0 || indice >= len(domain.PosicionesSello) {
		return "", "", errors.New("fallo local: no se confirmó una posición válida")
	}
	button, radio, err = showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Firma visible",
		Instruction: "¿En qué página?",
		Content:     "La firma visible se colocará " + strings.ToLower(nativeEtiquetasPosicionSello[domain.PosicionesSello[indice]]) + " de la página que elijas.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonAccept, Label: "Continuar"},
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton: nativeButtonCancel,
		RadioButtons: []nativeDialogChoice{
			{ID: nativeRadioBase, Label: "Última página (lo habitual)"},
			{ID: nativeRadioBase + 1, Label: "Primera página"},
		},
		AcceptNeedsRadio: nativeButtonAccept,
	})
	if err != nil {
		return "", "", err
	}
	if button != nativeButtonAccept {
		return "", "", certpicker.ErrSeleccionCancelada
	}
	pagina := "-1"
	if radio == nativeRadioBase+1 {
		pagina = "1"
	}
	return domain.PosicionesSello[indice], pagina, nil
}

func newNativeWindowsTrustUI() trustdialog.TrustUIProvider {
	return nativeWindowsTrustUI{}
}

func (nativeWindowsTrustUI) PedirDecision(
	ctx context.Context,
	origin string,
) (trustdialog.DecisionUnicaVez, error) {
	if err := ctx.Err(); err != nil {
		return trustdialog.Rechazar, err
	}
	message := trustdialog.BuildPromptMessageForUI(
		sanitizeNativeDialogText(origin, 512),
	)
	content := strings.Join([]string{
		message.OriginLabel + " " + message.OriginValue,
		message.PrimaryMessage,
		"Riesgo si continúas: " + message.RiskMessage,
		message.ResidentRisk,
		message.Question,
	}, "\r\n\r\n")
	button, _, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Autorización del portal",
		Instruction: message.Headline,
		Content:     content,
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonAccept, Label: "Confiar siempre"},
			{ID: nativeButtonOnce, Label: "Confiar solo esta vez"},
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton: nativeButtonCancel,
	})
	if err != nil {
		return trustdialog.Rechazar, err
	}
	switch button {
	case nativeButtonAccept:
		return trustdialog.ConfiarSiempre, nil
	case nativeButtonOnce:
		return trustdialog.ConfiarEstaVez, nil
	default:
		return trustdialog.Rechazar, nil
	}
}

func newNativeWindowsProgressProvider() progressdialog.ProgressProvider {
	return nativeWindowsProgressProvider{}
}

func (nativeWindowsProgressProvider) MostrarProgreso(
	_ context.Context,
	title string,
) progressdialog.Reporter {
	reporter := &nativeWindowsProgressReporter{
		title: sanitizeNativeDialogText(title, 256),
	}
	updateNativeProtocolUI(reporter.title, "Preparando la operación…")
	return reporter
}

func (r *nativeWindowsProgressReporter) SetMensaje(message string) {
	updateNativeProtocolUI(
		r.title,
		sanitizeNativeDialogText(message, 1024),
	)
}

func (*nativeWindowsProgressReporter) SetProgreso(float64) {}

func (r *nativeWindowsProgressReporter) Cerrar() {
	updateNativeProtocolUI(r.title, "Operación local finalizada.")
}

func newNativeWindowsDocumentPicker() ports.DocumentPicker {
	return desktopdocumentpicker.Nuevo(
		desktopdocumentpicker.ResolveFilteredFunc(
			func(ctx context.Context, filter ports.DocumentFilter) (
				desktopdocumentpicker.SelectedDocument,
				error,
			) {
				paths, err := selectNativeWindowsLoadPaths(
					ctx,
					"",
					strings.Join(filter.Extensions, ","),
					false,
				)
				if err != nil {
					if errors.Is(err, errLegacyLoadCanceled) {
						return desktopdocumentpicker.SelectedDocument{},
							desktopdocumentpicker.ErrSeleccionCancelada
					}
					return desktopdocumentpicker.SelectedDocument{}, err
				}
				if len(paths) != 1 {
					return desktopdocumentpicker.SelectedDocument{},
						errors.New("fallo local: el selector no devolvió un único documento")
				}
				resolved, ok := resolveUserChosenReadablePath(paths[0])
				if !ok {
					return desktopdocumentpicker.SelectedDocument{},
						errors.New("fallo local: el documento está fuera del ámbito local permitido")
				}
				data, err := securefile.ReadFileLimit(
					resolved,
					nativeMaxDocumentBytes,
				)
				if err != nil {
					return desktopdocumentpicker.SelectedDocument{}, err
				}
				name := filepath.Base(resolved)
				mimeType := mime.TypeByExtension(
					strings.ToLower(filepath.Ext(name)),
				)
				if mimeType == "" {
					mimeType = http.DetectContentType(data)
				}
				return desktopdocumentpicker.SelectedDocument{
					Name:     name,
					Content:  data,
					MIMEType: mimeType,
				}, nil
			},
		),
	)
}

func runNativeDirectProtocolUIShell(
	parent context.Context,
	stderr io.Writer,
	rawURI string,
) int {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	nativeProtocolFailure.Store(false)
	nativeProtocolComplete.Store(false)
	nativeProtocolSuccessShown.Store(false)
	nativeProtocolErrorText.Store(nil)

	result := make(chan int, 1)
	var startOnce sync.Once
	startHandler := func(uintptr) {
		startOnce.Do(func() {
			go func() {
				code := handleProtocolRequestNoGrace(
					ctx,
					newProtocolTraceWriter(stderr),
					rawURI,
				)
				publishNativeDirectProtocolResult(
					result,
					code,
					func(publishedCode int) {
						if publishedCode == 0 {
							// Las operaciones de firma/certificado ya ocultaron la
							// ventana al confirmar la entrega al portal.
							hwnd := validatedNativeShellWindow()
							foreground, _, _ := procGetForegroundWindowNative.Call()
							_, _, _ = procShowWindowNative.Call(hwnd, swHide)
							if nativeWindowOwnedByCurrentProcess(foreground) {
								restoreNativePortalForeground()
							}
							// El TaskDialog queda oculto mientras Windows presenta
							// el aviso de bandeja. Su HWND sigue válido ese plazo.
							time.AfterFunc(3*time.Second, func() {
								postNativeWindowMessage(
									validatedNativeShellWindow(),
									tdmClickButton,
									idCancel,
								)
							})
							return
						}
						if !nativeProtocolFailure.Load() {
							reportNativeProtocolFailure(
								"protocol",
								errors.New("la operación no se ha completado"),
							)
						}
						setNativeTaskDialogButtonText(
							validatedNativeShellWindow(),
							nativeButtonCancel,
							"Cerrar",
						)
					},
				)
			}()
		})
	}

	_, _, dialogErr := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Solicitud web",
		Instruction: "Preparando la solicitud del portal…",
		Content:     "La operación permanece bajo control local. Puede cancelarla en cualquier momento.",
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonCancel, Label: "Cancelar"},
		},
		DefaultButton: nativeButtonCancel,
		PassiveShell:  true,
		OnCreated: func(hwnd uintptr) {
			nativeProtocolShellHWND.Store(hwnd)
			startHandler(hwnd)
		},
		OnDestroyed: func(hwnd uintptr) {
			deleteNativePortalNotification(hwnd)
			nativeProtocolShellHWND.CompareAndSwap(hwnd, 0)
		},
	})
	cancel()
	if dialogErr != nil {
		return 1
	}
	select {
	case code := <-result:
		return code
	default:
		return 1
	}
}

func publishNativeDirectProtocolResult(
	result chan<- int,
	code int,
	afterPublish func(int),
) {
	result <- code
	if afterPublish != nil {
		afterPublish(code)
	}
}

func waitNativeLegacyLaunchUI(
	ctx context.Context,
	cancel context.CancelFunc,
	mode string,
	address string,
) int {
	detail := "El canal local está activo y espera la operación del portal."
	if safeAddress := sanitizeLocalProtocolAddress(address); safeAddress != "" {
		detail += "\r\n\r\nCanal local: " + safeAddress
	}
	button, _, err := showNativeTaskDialog(ctx, nativeDialogSpec{
		Title:       "GrxFirma — Firma web",
		Instruction: "Esperando al portal…",
		Content: detail + "\r\n\r\nModo: " +
			sanitizeNativeDialogText(mode, 32),
		Buttons: []nativeDialogChoice{
			{ID: nativeButtonCancel, Label: "Cancelar solicitud web"},
		},
		DefaultButton: nativeButtonCancel,
		LegacyShell:   true,
		PassiveShell:  true,
		OnCreated: func(hwnd uintptr) {
			nativeProtocolShellHWND.Store(hwnd)
			if nativeProtocolComplete.Load() {
				if nativeProtocolFailure.Load() {
					go func() {
						navigateNativeProtocolPage(hwnd, true)
						nativePortalErrorUI()
						if state := nativeProtocolErrorText.Load(); state != nil {
							updateNativeProtocolUI(state.status, state.detail)
						}
					}()
				} else {
					kind, _ := nativeProtocolDeliveredKind.Load().(string)
					hideNativePortalSuccess(hwnd, kind)
				}
			}
		},
		OnDestroyed: func(hwnd uintptr) {
			deleteNativePortalNotification(hwnd)
			nativeProtocolShellHWND.CompareAndSwap(hwnd, 0)
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		if cancel != nil {
			cancel()
		}
		return 1
	}
	if button == nativeButtonCancel && cancel != nil {
		cancel()
	}
	return 0
}

func setNativeProtocolCompletionUI(completed bool) {
	if !completed {
		nativeProtocolFailure.Store(false)
		nativeProtocolSuccessShown.Store(false)
		nativeProtocolErrorText.Store(nil)
	}
	if nativeProtocolComplete.Swap(completed) == completed {
		return
	}
	if hwnd := validatedNativeShellWindow(); hwnd != 0 {
		if !completed {
			showNativePassiveShell(hwnd)
		}
		navigateNativeProtocolPage(hwnd, completed)
	}
}

// nativePortalDeliveredUI conserva el servicio durante el plazo de nuevas
// operaciones, pero retira inmediatamente la ventana del navegador.
func nativePortalDeliveredUI(kind string) {
	nativeProtocolDeliveredKind.Store(kind)
	if nativeProtocolComplete.Swap(true) {
		return
	}
	hideNativePortalSuccess(validatedNativeShellWindow(), kind)
}

func hideNativePortalSuccess(hwnd uintptr, kind string) {
	if hwnd == 0 {
		return
	}
	if nativeProtocolSuccessShown.Swap(true) {
		return
	}
	showNativePortalNotification(hwnd, kind)
	foreground, _, _ := procGetForegroundWindowNative.Call()
	_, _, _ = procShowWindowNative.Call(hwnd, swHide)
	if nativeWindowOwnedByCurrentProcess(foreground) {
		restoreNativePortalForeground()
	}
}

func nativePortalErrorUI() {
	nativeProtocolFailure.Store(true)
	hwnd := validatedNativeShellWindow()
	if hwnd == 0 {
		return
	}
	setNativeTaskDialogButtonText(hwnd, nativeButtonCancel, "Cerrar")
	_, _, _ = procShowWindowNative.Call(hwnd, swShow)
	focusWindowsActionWindow(hwnd, nil)
}

func showNativePassiveShell(hwnd uintptr) {
	if !nativeWindowOwnedByCurrentProcess(hwnd) {
		return
	}
	positionNativePassiveShell(hwnd)
	_, _, _ = procShowWindowNative.Call(hwnd, swShowNoActivate)
	restoreNativePortalForeground()
}

func positionNativePassiveShell(hwnd uintptr) {
	if !nativeWindowOwnedByCurrentProcess(hwnd) {
		return
	}
	var rect nativeRect
	gotRect, _, _ := procGetWindowRectNative.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if gotRect == 0 {
		return
	}
	width, height := rect.Right-rect.Left, rect.Bottom-rect.Top
	if width <= 0 || height <= 0 {
		return
	}
	monitor, _, _ := procMonitorFromWindowNative.Call(hwnd, monitorDefaultToNearest)
	var work nativeRect
	if monitor != 0 {
		info := nativeMonitorInfo{Size: uint32(unsafe.Sizeof(nativeMonitorInfo{}))}
		if ok, _, _ := procGetMonitorInfoWNative.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
			work = info.Work
		}
	}
	if work.Right <= work.Left || work.Bottom <= work.Top {
		// En entornos sin monitor identificable, Windows aporta el área primaria.
		_, _, _ = user32Native.NewProc("SystemParametersInfoW").Call(
			spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0,
		)
	}
	if work.Right <= work.Left || work.Bottom <= work.Top {
		return
	}
	x := work.Right - width - 16
	y := work.Bottom - height - 16
	if x < work.Left {
		x = work.Left
	}
	if y < work.Top {
		y = work.Top
	}
	// HWND_NOTOPMOST (-2) quita cualquier estado heredado «siempre encima».
	_, _, _ = procSetWindowPosNative.Call(hwnd, ^uintptr(1), uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoActivate)
}

func restoreNativePortalForeground() {
	previous := nativeProtocolPreviousHWND.Load()
	if previous == 0 || nativeWindowOwnedByCurrentProcess(previous) {
		return
	}
	active, _, _ := procGetForegroundWindowNative.Call()
	if active != 0 && active != previous && !nativeWindowOwnedByCurrentProcess(active) {
		return
	}
	valid, _, _ := procIsWindowNative.Call(previous)
	if valid == 0 {
		return
	}
	var processID uint32
	_, _, _ = procGetWindowThreadProcessID.Call(previous, uintptr(unsafe.Pointer(&processID)))
	if processID != 0 {
		_, _, _ = procAllowSetForegroundNative.Call(uintptr(processID))
	}
	_, _, _ = procSetForegroundWindowNative.Call(previous)
}

func showNativePortalNotification(hwnd uintptr, kind string) {
	if !nativeWindowOwnedByCurrentProcess(hwnd) || unsafe.Sizeof(nativeNotifyIconData{}) != 976 {
		return
	}
	message := "Firma entregada al portal: consulte el resultado en la web"
	if kind == "certificate" {
		message = "Certificado entregado al portal: consulte el resultado en la web"
	}
	data := nativeNotifyIconData{Size: uint32(unsafe.Sizeof(nativeNotifyIconData{})), Window: hwnd, ID: 1,
		Flags: nifIcon | nifTip | nifInfo, InfoFlags: niifInfo}
	data.Icon, _, _ = procLoadIconWNative.Call(0, 32512) // IDI_APPLICATION
	copy(data.Tip[:], windows.StringToUTF16("GrxFirma"))
	copy(data.Info[:], windows.StringToUTF16(message))
	copy(data.InfoTitle[:], windows.StringToUTF16("GrxFirma — Portal"))
	command := uintptr(nimAdd)
	if nativeProtocolNotifyHWND.Load() == hwnd {
		command = nimModify
	}
	if ok, _, _ := procShellNotifyIconWNative.Call(command, uintptr(unsafe.Pointer(&data))); ok != 0 {
		nativeProtocolNotifyHWND.Store(hwnd)
	}
}

func deleteNativePortalNotification(hwnd uintptr) {
	if !nativeProtocolNotifyHWND.CompareAndSwap(hwnd, 0) {
		return
	}
	data := nativeNotifyIconData{Size: uint32(unsafe.Sizeof(nativeNotifyIconData{})), Window: hwnd, ID: 1}
	_, _, _ = procShellNotifyIconWNative.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
}

// TaskDialog no admite cambiar de forma fiable la etiqueta de un botón.
// La nueva página usa el mismo HWND y el mismo callback del diálogo original.
func navigateNativeProtocolPage(hwnd uintptr, completed bool) {
	state := nativeProtocolShellState.Load()
	if state == nil || !nativeWindowOwnedByCurrentProcess(hwnd) {
		return
	}
	buttonText := "Cancelar solicitud web"
	instructionText := "Procesando solicitud…"
	contentText := "La web ha enviado una nueva operación."
	if completed {
		buttonText = "Cerrar"
		instructionText = "Operación no completada"
		contentText = "Consulte el error de la operación antes de cerrar esta ventana."
	}
	title, _ := windows.UTF16FromString("GrxFirma — Firma web")
	instruction, _ := windows.UTF16FromString(instructionText)
	content, _ := windows.UTF16FromString(contentText)
	buttons, labels, err := nativeTaskDialogButtons([]nativeDialogChoice{{ID: nativeButtonCancel, Label: buttonText}})
	if err != nil {
		return
	}
	defer zeroNativeUTF16(title)
	defer zeroNativeUTF16(instruction)
	defer zeroNativeUTF16(content)
	defer clear(buttons)
	defer zeroNativeUTF16Slices(labels)
	var cfg nativeTaskDialogConfig
	cfg.setUint32(0, nativeTaskDialogConfigSize)
	cfg.setUint32(20, tdfAllowDialogCancellation|tdfSizeToContent)
	cfg.setPointer(28, uintptr(unsafe.Pointer(&title[0])))
	cfg.setPointer(44, uintptr(unsafe.Pointer(&instruction[0])))
	cfg.setPointer(52, uintptr(unsafe.Pointer(&content[0])))
	cfg.setUint32(60, 1)
	cfg.setPointer(64, uintptr(unsafe.Pointer(&buttons[0])))
	cfg.setInt32(72, nativeButtonCancel)
	cfg.setPointer(140, nativeTaskDialogCallback)
	cfg.setPointer(148, uintptr(unsafe.Pointer(state)))
	_, _, _ = procSendMessageWNative.Call(hwnd, tdmNavigatePage, 0, uintptr(unsafe.Pointer(&cfg)))
	if !completed {
		positionNativePassiveShell(hwnd)
	}
	runtime.KeepAlive(cfg)
	runtime.KeepAlive(state)
	runtime.KeepAlive(buttons)
	runtime.KeepAlive(labels)
}

func updateNativeProtocolUI(status, detail string) {
	status = sanitizeNativeDialogText(status, 512)
	detail = sanitizeNativeDialogText(detail, 2048)
	if nativeProtocolFailure.Load() {
		nativeProtocolErrorText.Store(&nativeProtocolErrorState{status: status, detail: detail})
	}
	hwnd := validatedNativeShellWindow()
	if hwnd == 0 {
		return
	}
	setNativeTaskDialogText(
		hwnd,
		tdeMainInstruction,
		status,
	)
	setNativeTaskDialogText(
		hwnd,
		tdeContent,
		detail,
	)
}

func reportNativeProtocolFailure(action string, err error) {
	if !nativeProtocolUIEnabled() || err == nil {
		return
	}
	nativePortalErrorUI()
	if nativeProtocolShellState.Load() != nil {
		setNativeProtocolCompletionUI(true)
	}
	updateNativeProtocolUI(
		"Operación no completada",
		nativeSafeFailureDetail(action, err),
	)
	if nativeProtocolShellState.Load() == nil {
		setNativeTaskDialogButtonText(
			validatedNativeShellWindow(),
			nativeButtonCancel,
			"Cerrar",
		)
	}
}

func showNativeTaskDialog(
	ctx context.Context,
	spec nativeDialogSpec,
) (int32, int32, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if len(spec.Buttons) == 0 {
		return 0, 0, errors.New("diálogo nativo sin acciones")
	}

	title, err := windows.UTF16FromString(
		sanitizeNativeDialogText(spec.Title, 256),
	)
	if err != nil {
		return 0, 0, err
	}
	defer zeroNativeUTF16(title)
	instruction, err := windows.UTF16FromString(
		sanitizeNativeDialogText(spec.Instruction, 512),
	)
	if err != nil {
		return 0, 0, err
	}
	defer zeroNativeUTF16(instruction)
	content, err := windows.UTF16FromString(
		sanitizeNativeDialogText(spec.Content, 4096),
	)
	if err != nil {
		return 0, 0, err
	}
	defer zeroNativeUTF16(content)
	buttons, buttonLabels, err := nativeTaskDialogButtons(spec.Buttons)
	if err != nil {
		return 0, 0, err
	}
	defer clear(buttons)
	defer zeroNativeUTF16Slices(buttonLabels)
	radios, radioLabels, err := nativeTaskDialogButtons(
		spec.RadioButtons,
	)
	if err != nil {
		return 0, 0, err
	}
	defer clear(radios)
	defer zeroNativeUTF16Slices(radioLabels)

	state := &nativeTaskDialogState{
		created:          make(chan uintptr, 1),
		acceptNeedsRadio: spec.AcceptNeedsRadio,
		onCreated:        spec.OnCreated,
		onDestroyed:      spec.OnDestroyed,
		passiveShell:     spec.PassiveShell,
	}
	if spec.PassiveShell {
		nativeProtocolPreviousHWND.Store(0)
		previous, _, _ := procGetForegroundWindowNative.Call()
		if previous != 0 && !nativeWindowOwnedByCurrentProcess(previous) {
			nativeProtocolPreviousHWND.Store(previous)
		}
	}
	if spec.LegacyShell {
		nativeProtocolShellState.Store(state)
		defer nativeProtocolShellState.CompareAndSwap(state, nil)
	}
	for _, radio := range spec.RadioButtons {
		state.radioIDs = append(state.radioIDs, radio.ID)
		state.radioLabels = append(state.radioLabels, sanitizeNativeDialogText(radio.Label, 768))
	}
	flags := uint32(
		tdfAllowDialogCancellation |
			tdfSizeToContent,
	)
	parentWindow := validatedNativeShellWindow()
	if parentWindow != 0 {
		flags |= tdfPositionRelativeToWindow
	}
	if len(radios) > 0 {
		flags |= tdfNoDefaultRadioButton
	}
	var cfg nativeTaskDialogConfig
	cfg.setUint32(0, nativeTaskDialogConfigSize)
	cfg.setPointer(4, parentWindow)
	cfg.setUint32(20, flags)
	cfg.setPointer(28, uintptr(unsafe.Pointer(&title[0])))
	cfg.setPointer(
		44,
		uintptr(unsafe.Pointer(&instruction[0])),
	)
	cfg.setPointer(52, uintptr(unsafe.Pointer(&content[0])))
	cfg.setUint32(
		60,
		uint32(len(buttons)/nativeTaskDialogButtonSize),
	)
	if len(buttons) > 0 {
		cfg.setPointer(64, uintptr(unsafe.Pointer(&buttons[0])))
	}
	cfg.setInt32(72, spec.DefaultButton)
	cfg.setUint32(
		76,
		uint32(len(radios)/nativeTaskDialogButtonSize),
	)
	if len(radios) > 0 {
		cfg.setPointer(80, uintptr(unsafe.Pointer(&radios[0])))
	}
	cfg.setInt32(88, 0)
	cfg.setPointer(140, nativeTaskDialogCallback)
	cfg.setPointer(148, uintptr(unsafe.Pointer(state)))
	cfg.setUint32(156, 0)

	done := make(chan struct{})
	go cancelNativeWindowOnContext(
		ctx,
		state.created,
		done,
		tdmClickButton,
		idCancel,
	)
	var selectedButton int32
	var selectedRadio int32
	var verification int32
	result, _, callErr := procTaskDialogIndirectNative.Call(
		uintptr(unsafe.Pointer(&cfg)),
		uintptr(unsafe.Pointer(&selectedButton)),
		uintptr(unsafe.Pointer(&selectedRadio)),
		uintptr(unsafe.Pointer(&verification)),
	)
	close(done)
	runtime.KeepAlive(cfg)
	runtime.KeepAlive(state)
	runtime.KeepAlive(buttons)
	runtime.KeepAlive(buttonLabels)
	runtime.KeepAlive(radios)
	runtime.KeepAlive(radioLabels)
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}
	if state.invalidWindow.Load() {
		return 0, 0,
			errors.New("fallo local: Windows creó un diálogo fuera de la instancia de GrxFirma")
	}
	if int32(result) < 0 {
		return 0, 0, fmt.Errorf(
			"TaskDialogIndirect devolvió HRESULT 0x%08X: %w",
			uint32(result),
			nativeWindowsCallError("TaskDialogIndirect", callErr),
		)
	}
	if chosen := state.selectedRadio.Load(); chosen != 0 {
		selectedRadio = chosen
	}
	return selectedButton, selectedRadio, nil
}

func nativeTaskDialogCallbackProc(
	hwnd uintptr,
	notification uint32,
	wParam uintptr,
	_ uintptr,
	callbackData uintptr,
) uintptr {
	if callbackData == 0 {
		return 0
	}
	state := (*nativeTaskDialogState)(unsafe.Pointer(callbackData))
	switch notification {
	case tdnCreated:
		if !nativeWindowOwnedByCurrentProcess(hwnd) {
			state.invalidWindow.Store(true)
			postNativeWindowMessage(
				hwnd,
				tdmClickButton,
				idCancel,
			)
			return 0
		}
		if state.passiveShell {
			positionNativePassiveShell(hwnd)
			restoreNativePortalForeground()
		} else {
			focusWindowsActionWindow(hwnd, nil)
		}
		select {
		case state.created <- hwnd:
		default:
		}
		if state.onCreated != nil {
			state.onCreated(hwnd)
		}
	case tdnRadioButtonClicked:
		state.selectedRadio.Store(int32(wParam))
	case tdnButtonClicked:
		if int32(wParam) == state.acceptNeedsRadio &&
			state.acceptNeedsRadio != 0 {
			// Un lector de pantalla que marca la opción por UI Automation no
			// genera TDN_RADIO_BUTTON_CLICKED: se consulta el estado real.
			if checked := wintaskdialog.CheckedRadio(hwnd, state.radioIDs, state.radioLabels); checked != 0 {
				state.selectedRadio.Store(checked)
			}
			if state.selectedRadio.Load() == 0 {
				_, _, _ = procMessageBeepNative.Call(0x00000030)
				return 1
			}
		}
	case tdnDestroyed:
		if state.onDestroyed != nil {
			state.onDestroyed(hwnd)
		}
		if !state.passiveShell && validatedNativeShellWindow() != 0 {
			restoreNativePortalForeground()
		}
	}
	return 0
}

func nativeTaskDialogButtons(
	choices []nativeDialogChoice,
) ([]byte, [][]uint16, error) {
	if len(choices) == 0 {
		return nil, nil, nil
	}
	seen := make(map[int32]struct{}, len(choices))
	buttons := make(
		[]byte,
		len(choices)*nativeTaskDialogButtonSize,
	)
	labels := make([][]uint16, 0, len(choices))
	for index, choice := range choices {
		if choice.ID <= 0 {
			return nil, nil,
				errors.New("identificador de acción nativa inválido")
		}
		if _, exists := seen[choice.ID]; exists {
			return nil, nil,
				errors.New("identificador de acción nativa duplicado")
		}
		seen[choice.ID] = struct{}{}
		label, err := windows.UTF16FromString(
			sanitizeNativeDialogText(choice.Label, 768),
		)
		if err != nil {
			return nil, nil, err
		}
		labels = append(labels, label)
		offset := index * nativeTaskDialogButtonSize
		binary.LittleEndian.PutUint32(
			buttons[offset:offset+4],
			uint32(choice.ID),
		)
		binary.LittleEndian.PutUint64(
			buttons[offset+4:offset+12],
			uint64(uintptr(unsafe.Pointer(
				&labels[len(labels)-1][0],
			))),
		)
	}
	return buttons, labels, nil
}

func (c *nativeTaskDialogConfig) setUint32(
	offset int,
	value uint32,
) {
	binary.LittleEndian.PutUint32(c[offset:offset+4], value)
}

func (c *nativeTaskDialogConfig) setInt32(
	offset int,
	value int32,
) {
	c.setUint32(offset, uint32(value))
}

func (c *nativeTaskDialogConfig) setPointer(
	offset int,
	value uintptr,
) {
	binary.LittleEndian.PutUint64(
		c[offset:offset+8],
		uint64(value),
	)
}

func selectNativeWindowsLoadPaths(
	ctx context.Context,
	initialPath string,
	extensions string,
	multiple bool,
) ([]string, error) {
	buffer := make([]uint16, 32*1024)
	flags := uint32(
		ofnNoChangeDir |
			ofnEnableHook |
			ofnPathMustExist |
			ofnFileMustExist |
			ofnExplorer |
			ofnDontAddToRecent |
			ofnForceFileSystem,
	)
	if multiple {
		flags |= ofnAllowMultiSelect
	}
	paths, err := runNativeFileDialog(
		ctx,
		false,
		buffer,
		initialPath,
		extensions,
		flags,
	)
	if err != nil {
		if errors.Is(err, errNativeDialogCanceled) {
			return nil, errLegacyLoadCanceled
		}
		return nil, err
	}
	if !multiple && len(paths) != 1 {
		return nil, errors.New(
			"fallo local: el selector devolvió una selección incoherente",
		)
	}
	return paths, nil
}

func selectNativeWindowsSaveTarget(
	ctx context.Context,
	defaultPath string,
	extensions string,
) (string, error) {
	buffer := make([]uint16, 32*1024)
	copyUTF16Buffer(buffer, defaultPath)
	paths, err := runNativeFileDialog(
		ctx,
		true,
		buffer,
		filepath.Dir(defaultPath),
		extensions,
		ofnOverwritePrompt|
			ofnNoChangeDir|
			ofnEnableHook|
			ofnPathMustExist|
			ofnNoReadOnlyReturn|
			ofnExplorer|
			ofnDontAddToRecent|
			ofnForceFileSystem,
	)
	if err != nil {
		if errors.Is(err, errNativeDialogCanceled) {
			return "", errLegacySaveCanceled
		}
		return "", err
	}
	if len(paths) != 1 {
		return "", errors.New(
			"fallo local: el selector de destino devolvió una ruta incoherente",
		)
	}
	return paths[0], nil
}

var errNativeDialogCanceled = errors.New(
	"diálogo nativo cancelado por el usuario",
)

func runNativeFileDialog(
	ctx context.Context,
	save bool,
	buffer []uint16,
	initialPath string,
	extensions string,
	flags uint32,
) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(buffer) < 2 {
		return nil, errors.New("buffer de selector nativo inválido")
	}
	defer zeroNativeUTF16(buffer)
	filter, defaultExtension := nativeFileDialogFilter(extensions)
	defer zeroNativeUTF16(filter)
	titleText := "Seleccionar fichero"
	if save {
		titleText = "Guardar resultado"
	}
	title, err := windows.UTF16PtrFromString(titleText)
	if err != nil {
		return nil, err
	}
	initialDirectory := safeNativeInitialDirectory(initialPath)
	var initialDirectoryPtr *uint16
	var initialDirectoryUTF16 []uint16
	if initialDirectory != "" {
		initialDirectoryUTF16, err = windows.UTF16FromString(
			initialDirectory,
		)
		if err != nil {
			return nil, err
		}
		defer zeroNativeUTF16(initialDirectoryUTF16)
		initialDirectoryPtr = &initialDirectoryUTF16[0]
	}
	var defaultExtensionPtr *uint16
	if defaultExtension != "" {
		defaultExtensionPtr, err = windows.UTF16PtrFromString(
			defaultExtension,
		)
		if err != nil {
			return nil, err
		}
	}

	state := &nativeFileDialogState{
		created: make(chan uintptr, 1),
	}
	config := nativeOpenFileName{
		Size:             uint32(unsafe.Sizeof(nativeOpenFileName{})),
		Owner:            validatedNativeShellWindow(),
		Filter:           &filter[0],
		FilterIndex:      1,
		File:             &buffer[0],
		MaxFile:          uint32(len(buffer)),
		InitialDirectory: initialDirectoryPtr,
		Title:            title,
		Flags:            flags,
		DefaultExtension: defaultExtensionPtr,
		CustomData:       uintptr(unsafe.Pointer(state)),
		Hook:             nativeFileDialogHook,
	}

	done := make(chan struct{})
	go cancelNativeWindowOnContext(
		ctx,
		state.created,
		done,
		wmClose,
		0,
	)
	runtime.LockOSThread()
	var result uintptr
	var callErr error
	if save {
		result, _, callErr = procGetSaveFileNameWNative.Call(
			uintptr(unsafe.Pointer(&config)),
		)
	} else {
		result, _, callErr = procGetOpenFileNameWNative.Call(
			uintptr(unsafe.Pointer(&config)),
		)
	}
	runtime.UnlockOSThread()
	close(done)
	runtime.KeepAlive(config)
	runtime.KeepAlive(state)
	runtime.KeepAlive(filter)
	runtime.KeepAlive(buffer)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if state.invalidWindow.Load() {
		return nil, errors.New(
			"fallo local: Windows creó un selector fuera de la instancia de GrxFirma",
		)
	}
	if result == 0 {
		extended, _, _ := procCommDlgExtendedError.Call()
		if extended == 0 {
			return nil, errNativeDialogCanceled
		}
		return nil, fmt.Errorf(
			"fallo local: el selector nativo devolvió el código 0x%X: %w",
			extended,
			nativeWindowsCallError("CommonDialog", callErr),
		)
	}
	return parseNativeFileDialogBuffer(buffer)
}

func nativeFileDialogHookProc(
	hwnd uintptr,
	message uint32,
	_ uintptr,
	lParam uintptr,
) uintptr {
	if message != wmInitDialog || lParam == 0 {
		return 0
	}
	config := (*nativeOpenFileName)(unsafe.Pointer(lParam))
	if config.CustomData == 0 {
		return 0
	}
	state := (*nativeFileDialogState)(
		unsafe.Pointer(config.CustomData),
	)
	dialog := config.Owner
	parent, _, _ := procGetParentNative.Call(hwnd)
	if parent != 0 {
		dialog = parent
	}
	if !nativeWindowOwnedByCurrentProcess(dialog) {
		state.invalidWindow.Store(true)
		postNativeWindowMessage(dialog, wmClose, 0)
		return 0
	}
	select {
	case state.created <- dialog:
	default:
	}
	focusWindowsActionWindow(dialog, nil)
	return 0
}

func cancelNativeWindowOnContext(
	ctx context.Context,
	created <-chan uintptr,
	done <-chan struct{},
	message uint32,
	wParam uintptr,
) {
	select {
	case hwnd := <-created:
		select {
		case <-ctx.Done():
			postNativeWindowMessage(hwnd, message, wParam)
		case <-done:
		}
	case <-done:
	}
}

func postNativeWindowMessage(
	hwnd uintptr,
	message uint32,
	wParam uintptr,
) {
	if !nativeWindowOwnedByCurrentProcess(hwnd) {
		return
	}
	_, _, _ = procPostMessageWNative.Call(
		hwnd,
		uintptr(message),
		wParam,
		0,
	)
}

func setNativeTaskDialogText(
	hwnd uintptr,
	element uintptr,
	text string,
) {
	if !nativeWindowOwnedByCurrentProcess(hwnd) {
		return
	}
	encoded, err := windows.UTF16FromString(text)
	if err != nil {
		return
	}
	defer zeroNativeUTF16(encoded)
	_, _, _ = procSendMessageWNative.Call(
		hwnd,
		tdmSetElementText,
		element,
		uintptr(unsafe.Pointer(&encoded[0])),
	)
	runtime.KeepAlive(encoded)
}

func setNativeTaskDialogButtonText(
	hwnd uintptr,
	buttonID int32,
	text string,
) {
	if !nativeWindowOwnedByCurrentProcess(hwnd) || buttonID <= 0 {
		return
	}
	button, _, _ := procGetDlgItemNative.Call(
		hwnd,
		uintptr(buttonID),
	)
	if !nativeWindowOwnedByCurrentProcess(button) {
		return
	}
	encoded, err := windows.UTF16FromString(
		sanitizeNativeDialogText(text, 64),
	)
	if err != nil {
		return
	}
	defer zeroNativeUTF16(encoded)
	_, _, _ = procSetWindowTextWNative.Call(
		button,
		uintptr(unsafe.Pointer(&encoded[0])),
	)
	runtime.KeepAlive(encoded)
}

func validatedNativeShellWindow() uintptr {
	hwnd := nativeProtocolShellHWND.Load()
	if !nativeWindowOwnedByCurrentProcess(hwnd) {
		nativeProtocolShellHWND.CompareAndSwap(hwnd, 0)
		return 0
	}
	return hwnd
}

func nativeWindowOwnedByCurrentProcess(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	valid, _, _ := procIsWindowNative.Call(hwnd)
	if valid == 0 {
		return false
	}
	var processID uint32
	threadID, _, _ := procGetWindowThreadProcessID.Call(
		hwnd,
		uintptr(unsafe.Pointer(&processID)),
	)
	return threadID != 0 && processID == uint32(os.Getpid())
}

func sanitizeNativeDialogText(raw string, maximumRunes int) string {
	if maximumRunes <= 0 {
		return ""
	}
	raw = strings.TrimSpace(raw)
	var output strings.Builder
	count := 0
	for _, value := range raw {
		if count >= maximumRunes {
			break
		}
		switch {
		case value == '\n':
			output.WriteString("\r\n")
		case value == '\r':
			continue
		case value == '\t':
			output.WriteRune(' ')
		case unicode.IsControl(value) || unicode.Is(unicode.Cf, value):
			output.WriteRune(' ')
		default:
			output.WriteRune(value)
		}
		count++
	}
	return strings.TrimSpace(output.String())
}

func nativeCertificateLabel(certificate domain.CertificateRef) string {
	subject := sanitizeNativeDialogText(
		certificate.Subject,
		220,
	)
	if subject == "" {
		subject = "Titular no disponible"
	}
	issuer := sanitizeNativeDialogText(
		certificate.Issuer,
		160,
	)
	if issuer == "" {
		issuer = "Emisor no disponible"
	}
	expiry := "caducidad no disponible"
	if !certificate.NotAfter.IsZero() {
		expiry = certificate.NotAfter.Local().Format("02/01/2006")
		if dias, ok := application.DiasHastaCaducidad(certificate, time.Now()); ok && dias >= 0 && dias <= application.DiasAvisoCaducidad {
			expiry += fmt.Sprintf(" (¡atención: quedan %d días!)", dias)
		}
	}
	fingerprint := sanitizeNativeFingerprint(
		certificate.Fingerprint,
	)
	if fingerprint == "" {
		fingerprint = "no disponible"
	}
	return sanitizeNativeDialogText(
		fmt.Sprintf(
			"%s\r\nEmisor: %s · Caduca: %s · Huella SHA-256: %s",
			subject,
			issuer,
			expiry,
			fingerprint,
		),
		640,
	)
}

func sanitizeNativeFingerprint(raw string) string {
	var output strings.Builder
	for _, value := range strings.ToUpper(raw) {
		if (value >= '0' && value <= '9') ||
			(value >= 'A' && value <= 'F') {
			output.WriteRune(value)
			if output.Len() >= 64 {
				break
			}
		}
	}
	return output.String()
}

func sanitizeLocalProtocolAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "wss://")
	raw = strings.TrimPrefix(raw, "https://")
	host, port, found := strings.Cut(raw, ":")
	if !found || port == "" {
		return ""
	}
	if host != "127.0.0.1" &&
		!strings.EqualFold(host, "localhost") &&
		host != "[::1]" {
		return ""
	}
	for _, value := range port {
		if value < '0' || value > '9' {
			return ""
		}
	}
	return host + ":" + port
}

func nativeFileDialogFilter(
	rawExtensions string,
) ([]uint16, string) {
	extensions := normalizedNativeExtensions(rawExtensions)
	if len(extensions) == 0 {
		return nativeUTF16Filter(
			tl(legacyFiltroTodosID),
			"*.*",
		), ""
	}
	patterns := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		patterns = append(patterns, "*."+extension)
	}
	pattern := strings.Join(patterns, ";")
	return nativeUTF16Filter(
		tl(legacyFiltroPermitidosID, pattern),
		pattern,
		tl(legacyFiltroTodosID),
		"*.*",
	), extensions[0]
}

func normalizedNativeExtensions(raw string) []string {
	seen := make(map[string]struct{})
	output := make([]string, 0, 8)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), ".")
		part = strings.ToLower(part)
		if part == "" || len(part) > 16 {
			continue
		}
		valid := true
		for _, value := range part {
			if !((value >= 'a' && value <= 'z') ||
				(value >= '0' && value <= '9')) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		output = append(output, part)
		if len(output) >= 32 {
			break
		}
	}
	return output
}

func nativeUTF16Filter(parts ...string) []uint16 {
	output := make([]uint16, 0, 128)
	for _, part := range parts {
		encoded, _ := windows.UTF16FromString(part)
		output = append(output, encoded...)
	}
	output = append(output, 0)
	return output
}

func safeNativeInitialDirectory(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.HasPrefix(raw, `\\`) {
		candidate := filepath.Clean(raw)
		if info, err := os.Stat(candidate); err == nil {
			if !info.IsDir() {
				candidate = filepath.Dir(candidate)
			}
			if info, err = os.Stat(candidate); err == nil &&
				info.IsDir() &&
				filepath.IsAbs(candidate) {
				return candidate
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, candidate := range []string{
		filepath.Join(home, "Descargas"),
		filepath.Join(home, "Downloads"),
		home,
	} {
		if info, statErr := os.Stat(candidate); statErr == nil &&
			info.IsDir() {
			return candidate
		}
	}
	return ""
}

func copyUTF16Buffer(buffer []uint16, value string) {
	if len(buffer) == 0 {
		return
	}
	encoded, err := windows.UTF16FromString(
		sanitizeNativeDialogText(value, len(buffer)-1),
	)
	if err != nil {
		return
	}
	copy(buffer, encoded)
}

func parseNativeFileDialogBuffer(
	buffer []uint16,
) ([]string, error) {
	parts := make([]string, 0, 4)
	start := 0
	for index, value := range buffer {
		if value != 0 {
			continue
		}
		if index == start {
			break
		}
		parts = append(parts, windows.UTF16ToString(
			buffer[start:index],
		))
		start = index + 1
		if len(parts) > 128 {
			return nil, errors.New(
				"fallo local: el selector devolvió demasiados ficheros",
			)
		}
	}
	if len(parts) == 0 {
		return nil, errNativeDialogCanceled
	}
	if len(parts) == 1 {
		return []string{filepath.Clean(parts[0])}, nil
	}
	directory := filepath.Clean(parts[0])
	output := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		if filepath.IsAbs(name) || filepath.Base(name) != name {
			return nil, errors.New(
				"fallo local: el selector devolvió un nombre de fichero no seguro",
			)
		}
		output = append(output, filepath.Join(directory, name))
	}
	return output, nil
}

func nativeWindowsCallError(
	operation string,
	err error,
) error {
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return fmt.Errorf("%s devolvió un resultado vacío", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func nativeSafeFailureDetail(action string, err error) string {
	var policyError *nativeProtocolPolicyError
	if errors.As(err, &policyError) {
		return sanitizeNativeDialogText(policyError.Error(), 1024)
	}
	if isLegacyDocumentoNoPDF(err) {
		return sanitizeNativeDialogText(tl(legacyDocumentoNoPDFDetailID), 1024)
	}

	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "sha-1") ||
		strings.Contains(lower, "sha1") {
		return "Fallo local de seguridad: la solicitud exige SHA-1, un algoritmo obsoleto e inseguro. La operación se ha rechazado y no se ha firmado ningún documento."
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return "La operación se ha cancelado antes de completarse. No se ha firmado ningún documento."
	}

	diagnostic := application.BuildGuidedDiagnostic(
		sanitizeNativeDialogText(action, 48),
		err.Error(),
	)
	parts := []string{
		diagnostic.UserMessage,
		diagnostic.ResponsibilityMessage,
		diagnostic.SuggestedAction,
	}
	if code := sanitizeNativeFailureCode(diagnostic.FailureCode); code != "" {
		parts = append(parts, "Código de diagnóstico: "+code)
	}
	return sanitizeNativeDialogText(
		strings.Join(parts, "\r\n\r\n"),
		2048,
	)
}

func sanitizeNativeFailureCode(raw string) string {
	var output strings.Builder
	for _, value := range strings.ToUpper(strings.TrimSpace(raw)) {
		if (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') ||
			value == '_' || value == '-' {
			output.WriteRune(value)
		}
		if output.Len() >= 24 {
			break
		}
	}
	return output.String()
}

func zeroNativeUTF16(buffer []uint16) {
	clear(buffer)
	runtime.KeepAlive(buffer)
}

func zeroNativeUTF16Slices(buffers [][]uint16) {
	for _, buffer := range buffers {
		zeroNativeUTF16(buffer)
	}
	clear(buffers)
	runtime.KeepAlive(buffers)
}

var (
	_ ports.UserApproval              = nativeWindowsApproval{}
	_ certpicker.CertSelector         = nativeWindowsCertSelector{}
	_ trustdialog.TrustUIProvider     = nativeWindowsTrustUI{}
	_ progressdialog.ProgressProvider = nativeWindowsProgressProvider{}
	_ progressdialog.Reporter         = (*nativeWindowsProgressReporter)(nil)
)
