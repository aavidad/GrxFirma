// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows && fyne_gui

package main

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	pfdDoubleBuffer  = 0x00000001
	pfdDrawToWindow  = 0x00000004
	pfdSupportOpenGL = 0x00000020
	pfdGenericFormat = 0x00000040
	pfdGenericAccel  = 0x00001000
	pfdTypeRGBA      = 0
	pfdMainPlane     = 0
	glVersion        = 0x1F02
	glRenderer       = 0x1F01

	wsPopup         = 0x80000000
	mbOK            = 0x00000000
	mbIconError     = 0x00000010
	mbSetForeground = 0x00010000
)

var (
	user32Graphics   = windows.NewLazySystemDLL("user32.dll")
	gdi32Graphics    = windows.NewLazySystemDLL("gdi32.dll")
	openGL32Graphics = windows.NewLazySystemDLL("opengl32.dll")

	procCreateWindowExW   = user32Graphics.NewProc("CreateWindowExW")
	procDestroyWindow     = user32Graphics.NewProc("DestroyWindow")
	procGetDC             = user32Graphics.NewProc("GetDC")
	procReleaseDC         = user32Graphics.NewProc("ReleaseDC")
	procMessageBoxW       = user32Graphics.NewProc("MessageBoxW")
	procChoosePixelFormat = gdi32Graphics.NewProc("ChoosePixelFormat")
	procDescribePixelFmt  = gdi32Graphics.NewProc("DescribePixelFormat")
	procSetPixelFormat    = gdi32Graphics.NewProc("SetPixelFormat")
	procWGLCreateContext  = openGL32Graphics.NewProc("wglCreateContext")
	procWGLDeleteContext  = openGL32Graphics.NewProc("wglDeleteContext")
	procWGLMakeCurrent    = openGL32Graphics.NewProc("wglMakeCurrent")
	procGLGetString       = openGL32Graphics.NewProc("glGetString")
)

type pixelFormatDescriptor struct {
	Size           uint16
	Version        uint16
	Flags          uint32
	PixelType      uint8
	ColorBits      uint8
	RedBits        uint8
	RedShift       uint8
	GreenBits      uint8
	GreenShift     uint8
	BlueBits       uint8
	BlueShift      uint8
	AlphaBits      uint8
	AlphaShift     uint8
	AccumBits      uint8
	AccumRedBits   uint8
	AccumGreenBits uint8
	AccumBlueBits  uint8
	AccumAlphaBits uint8
	DepthBits      uint8
	StencilBits    uint8
	AuxBuffers     uint8
	LayerType      uint8
	Reserved       uint8
	LayerMask      uint32
	VisibleMask    uint32
	DamageMask     uint32
}

func platformGraphicsReadiness() error {
	capability, err := probeWindowsOpenGL()
	if err != nil {
		return wrapGraphicsReadinessError("no se pudo crear un contexto OpenGL 2.1", err)
	}
	return validateGraphicsCapability(capability)
}

func probeWindowsOpenGL() (graphicsCapability, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	staticClass, err := windows.UTF16PtrFromString("STATIC")
	if err != nil {
		return graphicsCapability{}, err
	}
	windowName, err := windows.UTF16PtrFromString("GrxFirma graphics readiness")
	if err != nil {
		return graphicsCapability{}, err
	}
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(staticClass)),
		uintptr(unsafe.Pointer(windowName)),
		wsPopup,
		0,
		0,
		1,
		1,
		0,
		0,
		0,
		0,
	)
	if hwnd == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("CreateWindowExW", callErr)
	}
	defer procDestroyWindow.Call(hwnd) //nolint:errcheck

	hdc, _, callErr := procGetDC.Call(hwnd)
	if hdc == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("GetDC", callErr)
	}
	defer procReleaseDC.Call(hwnd, hdc) //nolint:errcheck

	pfd := pixelFormatDescriptor{
		Size:        uint16(unsafe.Sizeof(pixelFormatDescriptor{})),
		Version:     1,
		Flags:       pfdDrawToWindow | pfdSupportOpenGL | pfdDoubleBuffer,
		PixelType:   pfdTypeRGBA,
		ColorBits:   24,
		AlphaBits:   8,
		DepthBits:   24,
		StencilBits: 8,
		LayerType:   pfdMainPlane,
	}
	pixelFormat, _, callErr := procChoosePixelFormat.Call(hdc, uintptr(unsafe.Pointer(&pfd)))
	if pixelFormat == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("ChoosePixelFormat", callErr)
	}
	var selectedPFD pixelFormatDescriptor
	described, _, callErr := procDescribePixelFmt.Call(
		hdc,
		pixelFormat,
		unsafe.Sizeof(selectedPFD),
		uintptr(unsafe.Pointer(&selectedPFD)),
	)
	if described == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("DescribePixelFormat", callErr)
	}
	if err := validateFramebufferCapability(framebufferCapability{
		DrawToWindow:   selectedPFD.Flags&pfdDrawToWindow != 0,
		SupportsOpenGL: selectedPFD.Flags&pfdSupportOpenGL != 0,
		RGBA:           selectedPFD.PixelType == pfdTypeRGBA,
		DoubleBuffered: selectedPFD.Flags&pfdDoubleBuffer != 0,
		SoftwareOnly: selectedPFD.Flags&pfdGenericFormat != 0 &&
			selectedPFD.Flags&pfdGenericAccel == 0,
	}); err != nil {
		return graphicsCapability{}, err
	}
	ok, _, callErr := procSetPixelFormat.Call(hdc, pixelFormat, uintptr(unsafe.Pointer(&selectedPFD)))
	if ok == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("SetPixelFormat", callErr)
	}

	glContext, _, callErr := procWGLCreateContext.Call(hdc)
	if glContext == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("wglCreateContext", callErr)
	}
	defer procWGLDeleteContext.Call(glContext) //nolint:errcheck

	ok, _, callErr = procWGLMakeCurrent.Call(hdc, glContext)
	if ok == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("wglMakeCurrent", callErr)
	}
	defer procWGLMakeCurrent.Call(0, 0) //nolint:errcheck

	versionPtr, _, callErr := procGLGetString.Call(glVersion)
	if versionPtr == 0 {
		return graphicsCapability{}, windowsGraphicsCallError("glGetString(GL_VERSION)", callErr)
	}
	rendererPtr, _, _ := procGLGetString.Call(glRenderer)
	return graphicsCapability{
		Version:  windowsCString(versionPtr, 160),
		Renderer: windowsCString(rendererPtr, 160),
	}, nil
}

func windowsCString(ptr uintptr, maxLen int) string {
	if ptr == 0 || maxLen <= 0 {
		return ""
	}
	data := make([]byte, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		value := *(*byte)(unsafe.Pointer(ptr + uintptr(i)))
		if value == 0 {
			break
		}
		data = append(data, value)
	}
	return string(data)
}

func windowsGraphicsCallError(operation string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("%s devolvió un resultado vacío", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func platformGraphicsFailureDialog(message string) error {
	title, err := windows.UTF16PtrFromString(tl(graphicsFailureTitleID))
	if err != nil {
		return err
	}
	body, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return err
	}
	result, _, callErr := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(body)),
		uintptr(unsafe.Pointer(title)),
		mbOK|mbIconError|mbSetForeground,
	)
	if result == 0 {
		return windowsGraphicsCallError("MessageBoxW", callErr)
	}
	return nil
}
