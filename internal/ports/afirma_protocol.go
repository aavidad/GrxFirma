// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"errors"
)

// Programas que pueden atender las firmas que piden los portales mediante el
// protocolo afirma://.
const (
	ProgramaAfirmaGrxFirma  = "grxfirma"
	ProgramaAfirmaAutoFirma = "autofirma"
	ProgramaAfirmaOtro      = "other"
	ProgramaAfirmaNinguno   = "none"
)

// EstadoProtocoloAfirma describe, leído del sistema, qué programa abre los
// enlaces afirma:// y qué opciones puede elegir la persona.
type EstadoProtocoloAfirma struct {
	// Soportado es falso en las plataformas sin selector.
	Soportado bool
	// Actual es uno de los ProgramaAfirma*.
	Actual string
	// RutaActual es el ejecutable que atiende el protocolo cuando Actual es
	// ProgramaAfirmaOtro (o AutoFirma), para poder nombrarlo.
	RutaActual string
	// GrxFirmaInstalada indica si el componente afirma:// de GrxFirma existe.
	GrxFirmaInstalada bool
	// AutoFirmaInstalada indica si AutoFirma (la aplicación Java del
	// Gobierno) tiene registrado el protocolo para el equipo.
	AutoFirmaInstalada bool
	RutaAutoFirma      string
	// Preferencia es la última elección guardada ("" si nunca se eligió).
	Preferencia string
}

// ProtocoloAfirma consulta y cambia el programa que atiende afirma://.
type ProtocoloAfirma interface {
	Estado(ctx context.Context) (EstadoProtocoloAfirma, error)
	Elegir(ctx context.Context, programa string) (EstadoProtocoloAfirma, error)
}

var (
	// ErrProtocoloAfirmaNoSoportado: la plataforma no tiene selector.
	ErrProtocoloAfirmaNoSoportado = errors.New("selector de afirma:// no disponible en esta plataforma")
	// ErrProtocoloAfirmaAjeno: otro programa cambió el registro de afirma://
	// y GrxFirma no lo toca.
	ErrProtocoloAfirmaAjeno = errors.New("el registro de afirma:// pertenece a otro programa")
	// ErrAutoFirmaNoInstalada: no hay un AutoFirma registrado para el equipo.
	ErrAutoFirmaNoInstalada = errors.New("AutoFirma no está instalado")
	// ErrGrxFirmaAfirmaNoInstalada: falta el componente afirma:// de GrxFirma.
	ErrGrxFirmaAfirmaNoInstalada = errors.New("el componente afirma:// de GrxFirma no está instalado")
	// ErrProgramaAfirmaDesconocido: el programa pedido no es una opción válida.
	ErrProgramaAfirmaDesconocido = errors.New("programa de afirma:// no válido")
)
