// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/ports"
)

func guardarDesdeWeb(t *testing.T, destino string, approval *userApprovalStub) string {
	t.Helper()
	handler := &legacyWebSocketHandler{
		savePicker: func(context.Context, string, string) (string, error) { return destino, nil },
	}
	if approval != nil {
		handler.approval = approval
	}
	return handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion:         afirmauri.OperacionSave,
		OrigenSolicitante: "https://sede.example",
		LegacyParams: mapToValues(map[string]string{
			"dat": base64.StdEncoding.EncodeToString([]byte("contenido")),
		}),
	})
}

func noExiste(t *testing.T, ruta string) {
	t.Helper()
	if _, err := os.Stat(ruta); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s no debía escribirse", ruta)
	}
}

// aprobacionConTituloStub registra el título con el que se pide la confirmación.
type aprobacionConTituloStub struct {
	userApprovalStub
	titulos []string
}

func (a *aprobacionConTituloStub) ConTitulo(titulo string) ports.UserApproval {
	a.titulos = append(a.titulos, titulo)
	return &a.userApprovalStub
}

// A-09: la confirmación de guardado no sale como «Confirmación de firma».
func TestGuardadoWeb_ConfirmacionConTituloPropio(t *testing.T) {
	destino := filepath.Join(carpetaGuardadoWeb(t), "informe.docx")
	approval := &aprobacionConTituloStub{}
	handler := &legacyWebSocketHandler{
		savePicker: func(context.Context, string, string) (string, error) { return destino, nil },
	}
	handler.approval = approval
	if resp := handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion:         afirmauri.OperacionSave,
		OrigenSolicitante: "https://sede.example",
		LegacyParams: mapToValues(map[string]string{
			"dat": base64.StdEncoding.EncodeToString([]byte("contenido")),
		}),
	}); resp != "CANCEL" {
		t.Fatalf("handleSave() = %q", resp)
	}
	if approval.calls != 1 || len(approval.titulos) != 1 {
		t.Fatalf("la confirmación de guardado no pidió su propio título: llamadas=%d títulos=%q", approval.calls, approval.titulos)
	}
	titulo := approval.titulos[0]
	if titulo == claveTituloGuardado || titulo == "" || titulo == tl("Confirmación de firma") {
		t.Fatalf("título de la confirmación de guardado = %q", titulo)
	}
}

// Lo que ya se bloqueaba antes del lote 4 sigue bloqueado, con interfaz o
// sin ella, y no se ofrece a confirmar.
func TestGuardadoWeb_BloqueaEjecutablesDeSiempre(t *testing.T) {
	dir := carpetaGuardadoWeb(t)
	for _, nombre := range []string{"x.exe", "x.js", "x.lnk", "x.ps1", "x.sh", "x.desktop", "x.hta"} {
		destino := filepath.Join(dir, nombre)
		approval := &userApprovalStub{approved: true}
		if resp := guardarDesdeWeb(t, destino, approval); resp != "SAF_05: Tipo de fichero no permitido por seguridad" {
			t.Fatalf("%s: handleSave() = %q", nombre, resp)
		}
		noExiste(t, destino)
		if approval.calls != 0 {
			t.Fatalf("%s: un tipo bloqueado no se ofrece a confirmar", nombre)
		}
		if resp := guardarDesdeWeb(t, destino, nil); resp != "SAF_05: Tipo de fichero no permitido por seguridad" {
			t.Fatalf("%s sin interfaz: handleSave() = %q", nombre, resp)
		}
		noExiste(t, destino)
	}
}

// aprobacionRegistrada guarda el texto que se enseña al confirmar.
type aprobacionRegistrada struct {
	userApprovalStub
	mensajes []string
}

func (a *aprobacionRegistrada) Request(ctx context.Context, mensaje string) (bool, error) {
	a.mensajes = append(a.mensajes, mensaje)
	return a.userApprovalStub.Request(ctx, mensaje)
}

// Lote 13c (A-09): los tipos que el lote 4 añadió al bloqueo (ofimática con
// macros, imágenes de disco, guiones...) se podían guardar antes; ahora piden
// confirmación con el aviso de macros o código, y se guardan si se acepta.
func TestGuardadoWeb_TiposConRiesgoPidenConfirmacionConAviso(t *testing.T) {
	dir := carpetaGuardadoWeb(t)
	aviso := tl(claveConfirmarGuardadoConRiesgo, ".docm", "x")
	normal := tl(claveConfirmarGuardado, ".docm", "x")
	if aviso == claveConfirmarGuardadoConRiesgo || aviso == normal {
		t.Fatalf("falta el texto propio del aviso: %q", aviso)
	}
	for _, nombre := range []string{"ayuda.chm", "disco.iso", "macro.xll", "x.img", "x.vhdx", "x.docm", "x.xlsm", "x.dotm", "x.py", "x.appinstaller", "x.library-ms"} {
		destino := filepath.Join(dir, nombre)
		rechazo := &aprobacionRegistrada{}
		if resp := guardarDesdeWebCon(t, destino, rechazo); resp != "CANCEL" {
			t.Fatalf("%s rechazado: handleSave() = %q", nombre, resp)
		}
		noExiste(t, destino)
		if len(rechazo.mensajes) != 1 || !strings.Contains(rechazo.mensajes[0], "macro") && !strings.Contains(rechazo.mensajes[0], "código") {
			t.Fatalf("%s: la confirmación debía llevar el aviso de macros o código: %q", nombre, rechazo.mensajes)
		}
		acepto := &aprobacionRegistrada{userApprovalStub: userApprovalStub{approved: true}}
		if resp := guardarDesdeWebCon(t, destino, acepto); resp != "SAVE_OK" {
			t.Fatalf("%s aceptado: handleSave() = %q", nombre, resp)
		}
	}
}

// Lote 13c (A-09): sin interfaz para confirmar (headless), lo que antes del
// lote 4 se guardaba se vuelve a guardar, con aviso en el registro y, en
// Windows, con la marca de procedencia.
func TestGuardadoWeb_SinInterfazGuardaConAviso(t *testing.T) {
	dir := carpetaGuardadoWeb(t)
	for _, nombre := range []string{"informe.docx", "x.docm", "x.iso", "x.raro"} {
		destino := filepath.Join(dir, nombre)
		if resp := guardarDesdeWeb(t, destino, nil); resp != "SAVE_OK" {
			t.Fatalf("%s sin aprobador: handleSave() = %q", nombre, resp)
		}
		destino2 := filepath.Join(dir, "h-"+nombre)
		if resp := guardarDesdeWebCon(t, destino2, aprobacionHeadlessDePrueba{}); resp != "SAVE_OK" {
			t.Fatalf("%s headless: handleSave() = %q", nombre, resp)
		}
	}
}

type aprobacionHeadlessDePrueba struct{}

func (aprobacionHeadlessDePrueba) Request(context.Context, string) (bool, error) {
	return false, errors.New("sin interfaz")
}
func (aprobacionHeadlessDePrueba) SinInterfaz() bool { return true }

func guardarDesdeWebCon(t *testing.T, destino string, approval ports.UserApproval) string {
	t.Helper()
	handler := &legacyWebSocketHandler{
		savePicker: func(context.Context, string, string) (string, error) { return destino, nil },
		approval:   approval,
	}
	return handler.handleSave(context.Background(), afirmauri.Solicitud{
		Operacion:         afirmauri.OperacionSave,
		OrigenSolicitante: "https://sede.example",
		LegacyParams: mapToValues(map[string]string{
			"dat": base64.StdEncoding.EncodeToString([]byte("contenido")),
		}),
	})
}

// A-09: fuera de la lista de firmas y datos se pide confirmación explícita.
func TestGuardadoWeb_TipoNoHabitualPideConfirmacion(t *testing.T) {
	dir := carpetaGuardadoWeb(t)

	destino := filepath.Join(dir, "informe.docx")
	approval := &userApprovalStub{approved: false}
	if resp := guardarDesdeWeb(t, destino, approval); resp != "CANCEL" {
		t.Fatalf("rechazada la confirmación: handleSave() = %q", resp)
	}
	if approval.calls != 1 {
		t.Fatalf("confirmaciones pedidas: %d", approval.calls)
	}
	noExiste(t, destino)

	approval = &userApprovalStub{approved: true}
	if resp := guardarDesdeWeb(t, destino, approval); resp != "SAVE_OK" {
		t.Fatalf("confirmado: handleSave() = %q", resp)
	}
}

// A-09: las firmas y los datos habituales se guardan sin preguntar más.
func TestGuardadoWeb_TiposHabitualesNoPreguntan(t *testing.T) {
	dir := carpetaGuardadoWeb(t)
	for _, nombre := range []string{"doc.pdf", "f.csig", "f.xsig", "f.xml", "f.p7s", "f.asice", "datos.json"} {
		approval := &userApprovalStub{}
		if resp := guardarDesdeWeb(t, filepath.Join(dir, nombre), approval); resp != "SAVE_OK" {
			t.Fatalf("%s: handleSave() = %q", nombre, resp)
		}
		if approval.calls != 0 {
			t.Fatalf("%s: no debía pedir confirmación", nombre)
		}
	}
}

// A-09: la marca de descarga de Internet lleva zona 3 y solo un origen limpio.
func TestContenidoZoneIdentifier(t *testing.T) {
	c := contenidoZoneIdentifier("https://sede.example")
	if !strings.Contains(c, "[ZoneTransfer]\r\nZoneId=3\r\n") || !strings.Contains(c, "ReferrerUrl=https://sede.example\r\n") {
		t.Fatalf("contenido inesperado: %q", c)
	}
	for _, malo := range []string{"https://a.example\r\nZoneId=0", "javascript:alert(1)", "https://a.example/\u202e", ""} {
		c := contenidoZoneIdentifier(malo)
		if strings.Contains(c, "ReferrerUrl") || strings.Count(c, "ZoneId=") != 1 || !strings.Contains(c, "ZoneId=3") {
			t.Fatalf("origen %q: contenido %q", malo, c)
		}
	}
}
