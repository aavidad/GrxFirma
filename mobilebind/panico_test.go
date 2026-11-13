// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

// Prueba de regresión de H-10: gomobile no convierte un pánico de Go en una
// excepción de Java o Swift; si un analizador del núcleo entra en pánico, el
// proceso Android o iOS termina. La fachada debe devolver un error saneado.

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"grxfirma/internal/application"
)

const secretoPanico = "detalle-interno-del-panico"

type signServicePanico struct{}

func (signServicePanico) Execute(context.Context, application.SignCommand) (application.SignResult, error) {
	panic(secretoPanico)
}

type verifyServicePanico struct{}

func (verifyServicePanico) Execute(context.Context, application.VerifyCommand) (application.VerifyResult, error) {
	panic(secretoPanico)
}

type importServicePanico struct{}

func (importServicePanico) Execute(context.Context, application.ImportCertificateCommand) (application.ImportCertificateResult, error) {
	panic(secretoPanico)
}

func comprobarErrorSaneado(t *testing.T, operacion string, llamar func() (string, error)) {
	t.Helper()
	var (
		out string
		err error
	)
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("%s: el pánico salió de la fachada: %v", operacion, p)
			}
		}()
		out, err = llamar()
	}()
	if err == nil || out != "" {
		t.Fatalf("%s: se esperaba error y salida vacía, out=%q err=%v", operacion, out, err)
	}
	if strings.Contains(err.Error(), secretoPanico) {
		t.Fatalf("%s: el error expone el detalle del pánico: %v", operacion, err)
	}
}

func TestFacadePanicoEnServicioDevuelveError(t *testing.T) {
	contenido := base64.StdEncoding.EncodeToString([]byte("hola"))
	facade := newFacade(signServicePanico{}, verifyServicePanico{}, nil).withImportService(importServicePanico{})

	comprobarErrorSaneado(t, "SignJSON", func() (string, error) {
		return facade.SignJSON(`{"name":"demo.txt","content_base64":"` + contenido + `","mime_type":"text/plain","format":"CAdES","action":"sign","certificate_id":"cert-1"}`)
	})
	comprobarErrorSaneado(t, "VerifyJSON", func() (string, error) {
		return facade.VerifyJSON(`{"name":"firma.p7s","content_base64":"` + contenido + `","mime_type":"application/pkcs7-signature"}`)
	})
	comprobarErrorSaneado(t, "ImportCertificateJSON", func() (string, error) {
		return facade.ImportCertificateJSON(`{"data_base64":"` + base64.StdEncoding.EncodeToString([]byte("P12")) + `","password":"secret"}`)
	})
	comprobarErrorSaneado(t, "ImportCertificateBytesJSON", func() (string, error) {
		return facade.ImportCertificateBytesJSON([]byte("P12"), "secret")
	})

	// El mutex de operaciones queda libre tras el pánico: otra llamada no se bloquea.
	comprobarErrorSaneado(t, "VerifyJSON (segunda llamada)", func() (string, error) {
		return facade.VerifyJSON(`{"name":"firma.p7s","content_base64":"` + contenido + `","mime_type":"application/pkcs7-signature"}`)
	})
}
