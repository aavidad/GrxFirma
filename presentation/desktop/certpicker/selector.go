// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package certpicker proporciona un selector de certificado para el escritorio.
// La implementación con Fyne solo existe en fyne_selector.go; el resto del paquete
// no importa fyne.io y puede compilarse sin display gráfico.
package certpicker

import (
	"context"
	"errors"

	"grxfirma/internal/domain"
)

// ErrSeleccionCancelada se retorna cuando el usuario cancela la selección de certificado.
var ErrSeleccionCancelada = errors.New(tp("selección cancelada por el usuario"))

var ErrSeleccionInteractivaNecesaria = errors.New("seleccione la tarjeta en una interfaz gráfica antes de autorizar su uso")

// Las acciones se resuelven en el runtime, que custodia la credencial temporal.
var ErrCargarCertificado = errors.New("cargar certificado temporal")
var ErrActualizarCertificados = errors.New("actualizar catálogo de certificados")

// SupportsCredentialLoading distingue los selectores que permiten recuperar un
// catálogo vacío y deben ofrecer alternativas incluso con un único certificado.
func SupportsCredentialLoading(selector CertSelector) bool {
	capable, ok := selector.(interface{ SupportsCredentialLoading() bool })
	return ok && capable.SupportsCredentialLoading()
}

// ClearCredentials retira las identidades temporales al cerrar la sesión de firma.
func ClearCredentials(selector CertSelector) {
	if temporary, ok := selector.(interface{ ClearCredentials() }); ok {
		temporary.ClearCredentials()
	}
}

// BeginCredentialOperation mantiene la identidad cargada disponible hasta que
// termina su firma, aunque lleguen otras peticiones mientras el usuario elige.
func BeginCredentialOperation(ctx context.Context, selector CertSelector) (func(), error) {
	if scoped, ok := selector.(interface {
		BeginCredentialOperation(context.Context) (func(), error)
	}); ok {
		return scoped.BeginCredentialOperation(ctx)
	}
	return func() {}, nil
}

// ModoRecuerdo indica si el certificado seleccionado debe recordarse.
type ModoRecuerdo string

const (
	NoRecordar      ModoRecuerdo = ""
	RecordarSesion  ModoRecuerdo = "sesion"
	RecordarSiempre ModoRecuerdo = "siempre"
)

// ResultadoSeleccion devuelve el certificado elegido y el modo de recuerdo asociado.
type ResultadoSeleccion struct {
	Certificado domain.CertificateRef
	Recuerdo    ModoRecuerdo
}

// CertSelector permite al usuario seleccionar un certificado de una lista.
type CertSelector interface {
	Select(ctx context.Context, certs []domain.CertificateRef) (ResultadoSeleccion, error)
}
