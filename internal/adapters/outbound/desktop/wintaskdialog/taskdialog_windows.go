// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package wintaskdialog

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	configSize = 160
	buttonSize = 12
	maxButtons = 8
	maxRadios  = 128

	tdfAllowDialogCancellation = 0x0008
	tdfNoDefaultRadioButton    = 0x4000
	tdfSizeToContent           = 0x01000000

	tdnCreated            = 0
	tdnButtonClicked      = 2
	tdnDestroyed          = 5
	tdnRadioButtonClicked = 6

	wmUser         = 0x0400
	tdmClickButton = wmUser + 102
	bmGetCheck     = 0x00F0
	bstChecked     = 1
)

var (
	comctl32        = windows.NewLazySystemDLL("comctl32.dll")
	user32          = windows.NewLazySystemDLL("user32.dll")
	procTaskDialog  = comctl32.NewProc("TaskDialogIndirect")
	procPostMessage = user32.NewProc("PostMessageW")
	procForeground  = user32.NewProc("SetForegroundWindow")
	procMessageBeep = user32.NewProc("MessageBeep")
	procWindowPID   = user32.NewProc("GetWindowThreadProcessId")
	procEnumChild   = user32.NewProc("EnumChildWindows")
	procClassName   = user32.NewProc("GetClassNameW")
	procWindowText  = user32.NewProc("GetWindowTextW")
	procWindowLong  = user32.NewProc("GetWindowLongW")
	procSendMessage = user32.NewProc("SendMessageW")
	callback        = windows.NewCallback(callbackProc)
	enumCallback    = windows.NewCallback(enumChildProc)
)

// Los callbacks de Windows reciben un identificador, nunca un puntero de Go.
var (
	statesMu sync.Mutex
	states   = map[uintptr]*state{}
	nextID   uintptr
)

func registerState(st *state) uintptr {
	statesMu.Lock()
	defer statesMu.Unlock()
	nextID++
	states[nextID] = st
	return nextID
}

func unregisterState(id uintptr) {
	statesMu.Lock()
	defer statesMu.Unlock()
	delete(states, id)
}

func lookupState(id uintptr) *state {
	statesMu.Lock()
	defer statesMu.Unlock()
	return states[id]
}

type state struct {
	radioIDs         []int32
	radioLabels      []string
	hwnd             atomic.Uintptr
	created          chan struct{}
	acceptNeedsRadio int32
	selectedRadio    atomic.Int32
	foreign          atomic.Bool
}

// Available indica si TaskDialogIndirect está disponible (Common Controls v6).
func Available() bool {
	return procTaskDialog.Find() == nil
}

// Show muestra el diálogo y devuelve el botón y la opción elegidos.
func Show(ctx context.Context, spec Spec) (int32, int32, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if len(spec.Buttons) == 0 || len(spec.Buttons) > maxButtons || len(spec.Radios) > maxRadios {
		return 0, 0, errors.New("diálogo nativo sin acciones o con demasiadas opciones")
	}
	if !Available() {
		return 0, 0, errors.New("TaskDialog no disponible: falta Common Controls v6 en el manifiesto")
	}
	title, err := windows.UTF16FromString(sanitize(spec.Title, 256))
	if err != nil {
		return 0, 0, err
	}
	instruction, err := windows.UTF16FromString(sanitize(spec.Instruction, 512))
	if err != nil {
		return 0, 0, err
	}
	content, err := windows.UTF16FromString(sanitizeMultiline(spec.Content, 4096))
	if err != nil {
		return 0, 0, err
	}
	buttons, buttonLabels, err := encodeChoices(spec.Buttons)
	if err != nil {
		return 0, 0, err
	}
	radios, radioLabels, err := encodeChoices(spec.Radios)
	if err != nil {
		return 0, 0, err
	}

	st := &state{created: make(chan struct{}), acceptNeedsRadio: spec.AcceptNeedsRadio}
	for _, radio := range spec.Radios {
		st.radioIDs = append(st.radioIDs, radio.ID)
		st.radioLabels = append(st.radioLabels, sanitize(radio.Label, 768))
	}
	flags := uint32(tdfAllowDialogCancellation | tdfSizeToContent)
	if len(radios) > 0 {
		flags |= tdfNoDefaultRadioButton
	}
	var cfg [configSize]byte
	putU32(&cfg, 0, configSize)
	putU32(&cfg, 20, flags)
	putPtr(&cfg, 28, uintptr(unsafe.Pointer(&title[0])))
	putPtr(&cfg, 44, uintptr(unsafe.Pointer(&instruction[0])))
	putPtr(&cfg, 52, uintptr(unsafe.Pointer(&content[0])))
	putU32(&cfg, 60, uint32(len(spec.Buttons))) // #nosec G115 -- acotado por maxButtons.
	putPtr(&cfg, 64, uintptr(unsafe.Pointer(&buttons[0])))
	putU32(&cfg, 72, uint32(spec.DefaultButton)) // #nosec G115 -- reinterpretación del int32 de la API Win32.
	if len(radios) > 0 {
		putU32(&cfg, 76, uint32(len(spec.Radios))) // #nosec G115 -- acotado por maxRadios.
		putPtr(&cfg, 80, uintptr(unsafe.Pointer(&radios[0])))
	}
	putPtr(&cfg, 140, callback)
	id := registerState(st)
	defer unregisterState(id)
	putPtr(&cfg, 148, id)

	done := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-st.created:
		}
		select {
		case <-done:
		case <-ctx.Done():
			if hwnd := st.hwnd.Load(); hwnd != 0 {
				_, _, _ = procPostMessage.Call(hwnd, tdmClickButton, uintptr(IDCancel), 0)
			}
		}
	}()

	var selectedButton, selectedRadio, verification int32
	runtime.LockOSThread()
	result, _, callErr := procTaskDialog.Call(
		uintptr(unsafe.Pointer(&cfg)),
		uintptr(unsafe.Pointer(&selectedButton)),
		uintptr(unsafe.Pointer(&selectedRadio)),
		uintptr(unsafe.Pointer(&verification)),
	)
	runtime.UnlockOSThread()
	close(done)
	runtime.KeepAlive(cfg)
	runtime.KeepAlive(st)
	runtime.KeepAlive(title)
	runtime.KeepAlive(instruction)
	runtime.KeepAlive(content)
	runtime.KeepAlive(buttons)
	runtime.KeepAlive(buttonLabels)
	runtime.KeepAlive(radios)
	runtime.KeepAlive(radioLabels)
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}
	if st.foreign.Load() {
		return 0, 0, errors.New("Windows creó el diálogo fuera de este proceso")
	}
	hresult := uint32(result) // #nosec G115 -- un HRESULT ocupa 32 bits.
	if hresult&0x80000000 != 0 {
		return 0, 0, fmt.Errorf("TaskDialogIndirect devolvió HRESULT 0x%08X: %v", hresult, callErr)
	}
	if chosen := st.selectedRadio.Load(); chosen != 0 {
		selectedRadio = chosen
	}
	return selectedButton, selectedRadio, nil
}

// CheckedRadio devuelve el identificador de la opción marcada en un
// TaskDialog, consultando el estado real de sus controles. TaskDialog crea
// cada opción como un botón con identificador de control 0, así que la opción
// se reconoce por su texto (la etiqueta que se le pasó) y, si hay etiquetas
// repetidas, por su orden de creación.
func CheckedRadio(dialog uintptr, ids []int32, labels []string) int32 {
	return checkedRadio(dialog, ids, labels)
}

func checkedRadio(dialog uintptr, ids []int32, labels []string) int32 {
	if len(ids) == 0 || len(labels) != len(ids) {
		return 0
	}
	query := &radioQuery{ids: ids, labels: labels}
	id := registerRadioQuery(query)
	defer unregisterRadioQuery(id)
	_, _, _ = procEnumChild.Call(dialog, enumCallback, id)
	return query.checked
}

type radioQuery struct {
	ids     []int32
	labels  []string
	ordinal int
	checked int32
}

var (
	queriesMu sync.Mutex
	queries   = map[uintptr]*radioQuery{}
	nextQuery uintptr
)

func registerRadioQuery(q *radioQuery) uintptr {
	queriesMu.Lock()
	defer queriesMu.Unlock()
	nextQuery++
	queries[nextQuery] = q
	return nextQuery
}

func unregisterRadioQuery(id uintptr) {
	queriesMu.Lock()
	defer queriesMu.Unlock()
	delete(queries, id)
}

const (
	gwlStyle          = ^uintptr(15) // -16
	bsTypeMask        = 0x0F
	bsRadioButton     = 0x04
	bsAutoRadioButton = 0x09
)

func enumChildProc(child uintptr, data uintptr) uintptr {
	queriesMu.Lock()
	q := queries[data]
	queriesMu.Unlock()
	if q == nil {
		return 0
	}
	if windowClass(child) != "Button" {
		return 1
	}
	style, _, _ := procWindowLong.Call(child, gwlStyle)
	if kind := style & bsTypeMask; kind != bsRadioButton && kind != bsAutoRadioButton {
		return 1
	}
	ordinal := q.ordinal
	q.ordinal++
	state, _, _ := procSendMessage.Call(child, bmGetCheck, 0, 0)
	if state != bstChecked {
		return 1
	}
	text := windowText(child)
	match := -1
	for i, label := range q.labels {
		if label == text {
			if match >= 0 {
				match = -1 // etiqueta repetida: se decide por el orden
				break
			}
			match = i
		}
	}
	if match < 0 && ordinal < len(q.ids) {
		match = ordinal
	}
	if match >= 0 {
		q.checked = q.ids[match]
	}
	return 0
}

func windowClass(hwnd uintptr) string {
	buf := make([]uint16, 64)
	n, _, _ := procClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func windowText(hwnd uintptr) string {
	buf := make([]uint16, 1024)
	n, _, _ := procWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func callbackProc(hwnd uintptr, notification uint32, wParam uintptr, _ uintptr, data uintptr) uintptr {
	if data == 0 {
		return 0
	}
	st := lookupState(data)
	if st == nil {
		return 0
	}
	switch notification {
	case tdnCreated:
		var pid uint32
		_, _, _ = procWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != windows.GetCurrentProcessId() {
			st.foreign.Store(true)
			_, _, _ = procPostMessage.Call(hwnd, tdmClickButton, uintptr(IDCancel), 0)
			return 0
		}
		st.hwnd.Store(hwnd)
		_, _, _ = procForeground.Call(hwnd)
		close(st.created)
	case tdnRadioButtonClicked:
		st.selectedRadio.Store(int32(wParam)) // #nosec G115 -- identificador de opción declarado por nosotros (int32).
	case tdnButtonClicked:
		// El botón de aceptar exige haber elegido una opción explícitamente.
		// Se consulta el estado real de los controles: un lector de pantalla
		// que marca la opción mediante UI Automation no genera
		// TDN_RADIO_BUTTON_CLICKED y, sin esta consulta, no podría aceptar.
		if st.acceptNeedsRadio != 0 && int32(wParam) == st.acceptNeedsRadio { // #nosec G115 -- identificador de botón propio (int32).
			if checked := checkedRadio(hwnd, st.radioIDs, st.radioLabels); checked != 0 {
				st.selectedRadio.Store(checked)
			}
			if st.selectedRadio.Load() == 0 {
				_, _, _ = procMessageBeep.Call(0x00000030)
				return 1
			}
		}
	case tdnDestroyed:
	}
	return 0
}

func encodeChoices(choices []Choice) ([]byte, [][]uint16, error) {
	if len(choices) == 0 {
		return nil, nil, nil
	}
	seen := make(map[int32]struct{}, len(choices))
	out := make([]byte, len(choices)*buttonSize)
	labels := make([][]uint16, 0, len(choices))
	for i, choice := range choices {
		if choice.ID <= 0 {
			return nil, nil, errors.New("identificador de opción inválido")
		}
		if _, dup := seen[choice.ID]; dup {
			return nil, nil, errors.New("identificador de opción duplicado")
		}
		seen[choice.ID] = struct{}{}
		label, err := windows.UTF16FromString(sanitize(choice.Label, 768))
		if err != nil {
			return nil, nil, err
		}
		labels = append(labels, label)
		offset := i * buttonSize
		binary.LittleEndian.PutUint32(out[offset:offset+4], uint32(choice.ID))
		binary.LittleEndian.PutUint64(out[offset+4:offset+12], uint64(uintptr(unsafe.Pointer(&labels[len(labels)-1][0]))))
	}
	return out, labels, nil
}

func putU32(cfg *[configSize]byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(cfg[offset:offset+4], value)
}

func putPtr(cfg *[configSize]byte, offset int, value uintptr) {
	binary.LittleEndian.PutUint64(cfg[offset:offset+8], uint64(value))
}

// sanitize elimina controles (incluidos los de dirección bidireccional que
// permitirían disfrazar un origen) y limita la longitud.
func sanitize(value string, max int) string {
	return clean(value, max, false)
}

func sanitizeMultiline(value string, max int) string {
	return clean(value, max, true)
}

func clean(value string, max int, multiline bool) string {
	var b strings.Builder
	count := 0
	for _, r := range value {
		if count >= max {
			break
		}
		switch {
		case r == '\n' && multiline:
		case unicode.IsControl(r), unicode.Is(unicode.Bidi_Control, r), r == 0x200B, r == 0xFEFF:
			r = ' '
		}
		b.WriteRune(r)
		count++
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return " "
	}
	return out
}
