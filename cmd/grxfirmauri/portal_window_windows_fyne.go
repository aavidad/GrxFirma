// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build fyne_gui && windows

package main

import (
	"os"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	portalUser32          = windows.NewLazySystemDLL("user32.dll")
	portalFindWindow      = portalUser32.NewProc("FindWindowW")
	portalGetForeground   = portalUser32.NewProc("GetForegroundWindow")
	portalGetWindowPID    = portalUser32.NewProc("GetWindowThreadProcessId")
	portalIsWindow        = portalUser32.NewProc("IsWindow")
	portalIsVisible       = portalUser32.NewProc("IsWindowVisible")
	portalGetRect         = portalUser32.NewProc("GetWindowRect")
	portalGetMonitor      = portalUser32.NewProc("MonitorFromWindow")
	portalGetMonitorInfo  = portalUser32.NewProc("GetMonitorInfoW")
	portalSetWindowPos    = portalUser32.NewProc("SetWindowPos")
	portalSetForeground   = portalUser32.NewProc("SetForegroundWindow")
	portalBringToTop      = portalUser32.NewProc("BringWindowToTop")
	portalAttachInput     = portalUser32.NewProc("AttachThreadInput")
	portalFlashWindow     = portalUser32.NewProc("FlashWindowEx")
	portalCurrentThreadID = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	portalAllowForeground = portalUser32.NewProc("AllowSetForegroundWindow")
)

type portalFlashInfo struct {
	Size    uint32
	Window  uintptr
	Flags   uint32
	Count   uint32
	Timeout uint32
}

type portalWinRect struct{ Left, Top, Right, Bottom int32 }
type portalWinMonitorInfo struct {
	Size    uint32
	Monitor portalWinRect
	Work    portalWinRect
	Flags   uint32
}

func capturePortalForeground() string {
	hwnd, _, _ := portalGetForeground.Call()
	if hwnd == 0 || portalWindowBelongsToSelf(hwnd) {
		return ""
	}
	return strconv.FormatUint(uint64(hwnd), 10)
}

func placePortalWaitingWindow(previous string, generation uint64) {
	go func() {
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			if portalWaitingGeneration.Load() != generation {
				return
			}
			hwnd := portalFindOwnWindow()
			if hwnd != 0 {
				portalWaitingEffectMu.Lock()
				if portalWaitingGeneration.Load() != generation {
					portalWaitingEffectMu.Unlock()
					return
				}
				if portalPositionPassive(hwnd) {
					restorePortalForeground(previous)
					portalWaitingEffectMu.Unlock()
					return
				}
				portalWaitingEffectMu.Unlock()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

func portalFindOwnWindow() uintptr {
	for _, title := range []string{"GrxFirma — Firma web", "GrxFirma — Solicitud web"} {
		name, err := windows.UTF16PtrFromString(title)
		if err != nil {
			continue
		}
		hwnd, _, _ := portalFindWindow.Call(0, uintptr(unsafe.Pointer(name)))
		if !portalWindowBelongsToSelf(hwnd) {
			continue
		}
		visible, _, _ := portalIsVisible.Call(hwnd)
		if visible != 0 {
			return hwnd
		}
	}
	return 0
}

// La petición llega desde un navegador, por lo que SetForegroundWindow por sí
// solo puede ser rechazado. La espera y el aviso de entrega no llaman aquí.
func focusPortalActionWindow(generation uint64) {
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) && portalWaitingGeneration.Load() == generation {
			if hwnd := portalFindOwnWindow(); hwnd != 0 {
				focusWindowsActionWindow(hwnd, func() bool {
					return portalWaitingGeneration.Load() == generation
				})
				return
			}
			time.Sleep(40 * time.Millisecond)
		}
	}()
}

func focusWindowsActionWindow(hwnd uintptr, stillNeeded func() bool) {
	go func() {
		for attempt := 0; attempt < 6; attempt++ {
			if !portalWindowBelongsToSelf(hwnd) || (stillNeeded != nil && !stillNeeded()) {
				return
			}
			visible, _, _ := portalIsVisible.Call(hwnd)
			if visible != 0 && (portalTryFocusActionWindow(hwnd, attempt == 5) || attempt == 5) {
				if active, _, _ := portalGetForeground.Call(); active != hwnd &&
					(stillNeeded == nil || stillNeeded()) {
					info := portalFlashInfo{Size: uint32(unsafe.Sizeof(portalFlashInfo{})), Window: hwnd,
						Flags: 0x0002 | 0x000C, Count: 3} // FLASHW_TRAY | FLASHW_TIMERNOFG
					_, _, _ = portalFlashWindow.Call(uintptr(unsafe.Pointer(&info)))
				}
				return
			}
			time.Sleep(75 * time.Millisecond)
		}
	}()
}

func portalTryFocusActionWindow(hwnd uintptr, useTopmost bool) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	current, _, _ := portalCurrentThreadID.Call()
	foreground, _, _ := portalGetForeground.Call()
	foregroundThread, _, _ := portalGetWindowPID.Call(foreground, 0)
	if foregroundThread != 0 && foregroundThread != current {
		if ok, _, _ := portalAttachInput.Call(current, foregroundThread, 1); ok != 0 {
			defer portalAttachInput.Call(current, foregroundThread, 0)
		}
	}
	if useTopmost {
		// El estado «siempre encima» dura solo este intento de activación.
		_, _, _ = portalSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x0001|0x0002|0x0010)
		defer portalSetWindowPos.Call(hwnd, ^uintptr(1), 0, 0, 0, 0, 0x0001|0x0002|0x0010)
	}
	_, _, _ = portalSetForeground.Call(hwnd)
	_, _, _ = portalBringToTop.Call(hwnd)
	active, _, _ := portalGetForeground.Call()
	return active == hwnd
}

func portalWindowBelongsToSelf(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	valid, _, _ := portalIsWindow.Call(hwnd)
	if valid == 0 {
		return false
	}
	var pid uint32
	_, _, _ = portalGetWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid == uint32(os.Getpid())
}

func portalPositionPassive(hwnd uintptr) bool {
	var rect portalWinRect
	if ok, _, _ := portalGetRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return false
	}
	width, height := rect.Right-rect.Left, rect.Bottom-rect.Top
	if width < 200 || height < 100 {
		return false
	}
	monitor, _, _ := portalGetMonitor.Call(hwnd, 2) // MONITOR_DEFAULTTONEAREST
	if monitor == 0 {
		return false
	}
	info := portalWinMonitorInfo{Size: uint32(unsafe.Sizeof(portalWinMonitorInfo{}))}
	if ok, _, _ := portalGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false
	}
	x, y := info.Work.Right-width-16, info.Work.Bottom-height-16
	if x < info.Work.Left {
		x = info.Work.Left
	}
	if y < info.Work.Top {
		y = info.Work.Top
	}
	// HWND_NOTOPMOST, SWP_NOSIZE y SWP_NOACTIVATE.
	_, _, _ = portalSetWindowPos.Call(hwnd, ^uintptr(1), uintptr(x), uintptr(y), 0, 0, 0x0001|0x0010)
	return true
}

func restorePortalForeground(previous string) {
	id, err := strconv.ParseUint(previous, 10, 64)
	if err != nil || id == 0 {
		return
	}
	hwnd := uintptr(id)
	valid, _, _ := portalIsWindow.Call(hwnd)
	if valid == 0 {
		return
	}
	active, _, _ := portalGetForeground.Call()
	if active == hwnd || (active != 0 && !portalWindowBelongsToSelf(active)) {
		return
	}
	var pid uint32
	_, _, _ = portalGetWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		_, _, _ = portalAllowForeground.Call(uintptr(pid))
	}
	_, _, _ = portalSetForeground.Call(hwnd)
}
