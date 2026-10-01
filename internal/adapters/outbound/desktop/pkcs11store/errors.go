// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11store

import (
	"errors"
	"fmt"
)

// ErrModuloNoDisponible se retorna cuando el módulo PKCS#11 no se puede cargar.
var ErrModuloNoDisponible = errors.New("módulo PKCS#11 no disponible")

// ErrCGONoDisponible identifica un binario que se compiló sin el soporte CGo
// requerido por la implementación PKCS#11. Envuelve ErrModuloNoDisponible para
// que los consumidores que ya comprobaban ese error mantengan su comportamiento.
var ErrCGONoDisponible = fmt.Errorf("%w: binario compilado sin CGo", ErrModuloNoDisponible)

var (
	ErrClosed                  = errors.New("PKCS#11: almacén o identidad cerrados")
	ErrLimit                   = errors.New("PKCS#11: límite de datos del módulo excedido")
	ErrIdentityNotFound        = errors.New("PKCS#11: identidad seleccionada no disponible")
	ErrAmbiguousIdentity       = errors.New("PKCS#11: asociación certificado y clave ambigua")
	ErrAmbiguousSigningRequest = errors.New("PKCS#11: la firma requiere certificado concreto y algoritmo; use KeyFor y crypto.Signer")
	ErrMechanismUnsupported    = errors.New("PKCS#11: algoritmo o mecanismo de firma no soportado")
	ErrPINUnavailable          = errors.New("PKCS#11: entrada local de PIN no disponible o cancelada")
	ErrPINIncorrect            = errors.New("PKCS#11: PIN incorrecto; no se reintenta automáticamente")
	ErrPINLocked               = errors.New("PKCS#11: PIN bloqueado")
	ErrPINExpired              = errors.New("PKCS#11: PIN caducado")
	ErrTokenUnavailable        = errors.New("PKCS#11: token retirado o sesión no disponible")
	ErrSignatureInvalid        = errors.New("PKCS#11: la firma del token no corresponde al certificado seleccionado")
	errAlreadyLoggedIn         = errors.New("PKCS#11: sesión ya autenticada")
)

// ModuleError conserva únicamente operación y código CK_RV; nunca datos de
// atributos, rutas, PIN ni mensajes libres del driver.
type ModuleError struct {
	Operation string
	Code      uint
	cause     error
}

func (e *ModuleError) Error() string {
	return fmt.Sprintf("PKCS#11 %s: CK_RV=0x%x", e.Operation, e.Code)
}
func (e *ModuleError) Unwrap() error { return e.cause }
