// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"context"
	"errors"
	"strings"
)

// Programas que pueden atender las firmas que piden los portales mediante el
// protocolo afirma://.
const (
	ProgramaAfirmaGrxFirma  = "grxfirma"
	ProgramaAfirmaAutoFirma = "autofirma"
	ProgramaAfirmaOtro      = "other"
	ProgramaAfirmaNinguno   = "none"
	// ProgramaAfirmaPreguntar deja GrxFirma registrado y, al llegar cada
	// enlace, pregunta si firmar con GrxFirma o con AutoFirma.
	ProgramaAfirmaPreguntar = "preguntar"
)

// NormalizarProgramaAfirma devuelve el valor canónico de una elección
// (grxfirma, autofirma o preguntar) y si es válida.
func NormalizarProgramaAfirma(valor string) (string, bool) {
	switch v := strings.ToLower(strings.TrimSpace(valor)); v {
	case ProgramaAfirmaGrxFirma, ProgramaAfirmaAutoFirma, ProgramaAfirmaPreguntar:
		return v, true
	}
	return "", false
}

// PoliticaProgramaAfirma es lo que fija la política de máquina
// (manejador_afirma) por encima de la elección del usuario.
type PoliticaProgramaAfirma struct {
	// Valor es grxfirma, autofirma o preguntar; "" si la política no lo fija.
	Valor string
	// Ilegible: la política existe pero no se pudo leer o el valor no se
	// entiende. Se mantiene lo de antes (la elección del usuario) con aviso.
	Ilegible bool
}

// ProgramaAfirmaEfectivo es lo que se aplica: la política, si fija un valor;
// si no, la elección del usuario; y si no eligió nada, GrxFirma.
func ProgramaAfirmaEfectivo(politica PoliticaProgramaAfirma, preferencia string) string {
	if v, ok := NormalizarProgramaAfirma(politica.Valor); ok && !politica.Ilegible {
		return v
	}
	if v, ok := NormalizarProgramaAfirma(preferencia); ok {
		return v
	}
	return ProgramaAfirmaGrxFirma
}

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
	// Politica es el valor que fija la política de máquina ("" si no fija
	// ninguno). Con política, el ajuste es de solo lectura.
	Politica string
	// PoliticaIlegible: hay política pero no se pudo leer este ajuste.
	PoliticaIlegible bool
	// Efectiva es lo que se aplica (ProgramaAfirmaEfectivo).
	Efectiva string
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
	// ErrProtocoloAfirmaPolitica: la política de máquina fija el programa y
	// el usuario no lo puede cambiar.
	ErrProtocoloAfirmaPolitica = errors.New("la política del equipo fija el programa de afirma://")
)
