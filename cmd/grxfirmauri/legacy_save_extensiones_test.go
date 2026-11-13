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

// A-09: tipos que Windows abre o monta y que la lista negra no cubría.
func TestGuardadoWeb_BloqueaTiposPeligrososNuevos(t *testing.T) {
	dir := t.TempDir()
	for _, nombre := range []string{"ayuda.chm", "disco.iso", "macro.xll", "x.img", "x.vhdx", "x.docm", "x.appinstaller", "x.library-ms"} {
		destino := filepath.Join(dir, nombre)
		approval := &userApprovalStub{approved: true}
		if resp := guardarDesdeWeb(t, destino, approval); resp != "SAF_05: Tipo de fichero no permitido por seguridad" {
			t.Fatalf("%s: handleSave() = %q", nombre, resp)
		}
		noExiste(t, destino)
		if approval.calls != 0 {
			t.Fatalf("%s: un tipo bloqueado no se ofrece a confirmar", nombre)
		}
	}
}

// A-09: fuera de la lista de firmas y datos se pide confirmación explícita.
func TestGuardadoWeb_TipoNoHabitualPideConfirmacion(t *testing.T) {
	dir := t.TempDir()

	destino := filepath.Join(dir, "informe.docx")
	approval := &userApprovalStub{approved: false}
	if resp := guardarDesdeWeb(t, destino, approval); resp != "CANCEL" {
		t.Fatalf("rechazada la confirmación: handleSave() = %q", resp)
	}
	if approval.calls != 1 {
		t.Fatalf("confirmaciones pedidas: %d", approval.calls)
	}
	noExiste(t, destino)

	if resp := guardarDesdeWeb(t, destino, nil); !strings.HasPrefix(resp, "SAF_05") {
		t.Fatalf("sin aprobador debía fallar cerrado: %q", resp)
	}
	noExiste(t, destino)

	approval = &userApprovalStub{approved: true}
	if resp := guardarDesdeWeb(t, destino, approval); resp != "SAVE_OK" {
		t.Fatalf("confirmado: handleSave() = %q", resp)
	}
}

// A-09: las firmas y los datos habituales se guardan sin preguntar más.
func TestGuardadoWeb_TiposHabitualesNoPreguntan(t *testing.T) {
	dir := t.TempDir()
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
	for _, malo := range []string{"https://a.example\r\nZoneId=0", "javascript:alert(1)", "https://a.example/‮", ""} {
		c := contenidoZoneIdentifier(malo)
		if strings.Contains(c, "ReferrerUrl") || strings.Count(c, "ZoneId=") != 1 || !strings.Contains(c, "ZoneId=3") {
			t.Fatalf("origen %q: contenido %q", malo, c)
		}
	}
}
