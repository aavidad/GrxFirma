// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package originvalidator implementa la validación estricta de origen para
// solicitudes afirma://, comprobando el formato de la URL de origen y
// consultando la política de confianza configurada.
package originvalidator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const longitudMaximaOrigen = 253

// Errores de validación de formato exportados.
var (
	// ErrOrigenVacio se devuelve cuando el origen es una cadena vacía o solo espacios.
	ErrOrigenVacio = errors.New("el origen no puede estar vacio")

	// ErrEsquemaNoPermitido se devuelve cuando el esquema no es https ni http.
	ErrEsquemaNoPermitido = errors.New("esquema no permitido: solo se admiten https y http")

	// ErrOrigenConPath se devuelve cuando el origen incluye path, query o fragment.
	ErrOrigenConPath = errors.New("el origen no debe incluir path, query ni fragment")

	// ErrOrigenConCredenciales se devuelve cuando el origen incluye credenciales (user:pass@host).
	ErrOrigenConCredenciales = errors.New("el origen no debe incluir credenciales")

	// ErrOrigenDemasiadoLargo se devuelve cuando el origen supera 253 caracteres.
	ErrOrigenDemasiadoLargo = errors.New("el origen supera la longitud maxima permitida de 253 caracteres")

	// ErrHTTPSoloLocalhost se devuelve cuando se usa http con un host que no es localhost ni 127.0.0.1.
	ErrHTTPSoloLocalhost = errors.New("el esquema http solo esta permitido para localhost y 127.0.0.1")

	// ErrOrigenRechazado se devuelve cuando la política de confianza deniega o no ha aprobado el origen.
	ErrOrigenRechazado = errors.New("el origen ha sido rechazado por la politica de confianza")
)

// Validator valida y autoriza orígenes de solicitudes afirma://.
type Validator struct {
	policy ports.TrustPolicy
}

// New construye un Validator con la política de confianza indicada.
func New(policy ports.TrustPolicy) *Validator {
	return &Validator{policy: policy}
}

// Validate valida el formato del origen y evalúa la política de confianza.
// Retorna error si el origen tiene formato inválido o es rechazado por la política.
// origen debe ser una URL absoluta con esquema https:// o http://localhost.
func (v *Validator) Validate(ctx context.Context, origen string) error {
	if err := validarFormato(origen); err != nil {
		return err
	}

	decision, err := v.policy.Evaluate(ctx, origen)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrOrigenRechazado, err)
	}

	if decision.Status != domain.TrustAllowed {
		return fmt.Errorf("%w: %s (estado: %s)", ErrOrigenRechazado, origen, decision.Status)
	}

	return nil
}

// validarFormato aplica todas las reglas de validación estricta de formato sobre el origen.
func validarFormato(origen string) error {
	if strings.TrimSpace(origen) == "" {
		return ErrOrigenVacio
	}

	if len(origen) > longitudMaximaOrigen {
		return ErrOrigenDemasiadoLargo
	}

	u, err := url.Parse(origen)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrEsquemaNoPermitido, err)
	}

	// Debe ser URL absoluta (tiene esquema y host).
	if u.Scheme == "" || u.Host == "" {
		return ErrEsquemaNoPermitido
	}

	esquema := strings.ToLower(u.Scheme)
	if esquema != "https" && esquema != "http" {
		return ErrEsquemaNoPermitido
	}

	// Sin credenciales.
	if u.User != nil {
		return ErrOrigenConCredenciales
	}

	// Sin path significativo, query ni fragment.
	// El path puede ser "" o "/" (algunos parsers añaden "/" al host puro).
	pathLimpio := strings.TrimRight(u.Path, "/")
	if pathLimpio != "" || u.RawQuery != "" || u.Fragment != "" {
		return ErrOrigenConPath
	}

	// http solo para localhost / 127.0.0.1.
	if esquema == "http" {
		host := u.Hostname()
		if !esLocalhost(host) {
			return ErrHTTPSoloLocalhost
		}
	}

	return nil
}

// esLocalhost devuelve true si el host es "localhost", "127.0.0.1" o cualquier
// otra dirección de loopback IPv4/IPv6.
func esLocalhost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}
