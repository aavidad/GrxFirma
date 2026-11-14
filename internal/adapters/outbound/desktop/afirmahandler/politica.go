// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package afirmahandler

import (
	"context"

	"grxfirma/internal/ports"
)

// conPolitica aplica la política de máquina manejador_afirma sobre un
// selector: la muestra en el estado y no deja elegir otra cosa.
type conPolitica struct {
	inner ports.ProtocoloAfirma
	leer  func() ports.PoliticaProgramaAfirma
}

// ConPolitica envuelve un selector con el lector de la política de máquina.
// La política se lee en cada llamada, así que un cambio de GPO se nota sin
// reiniciar. Con leer nil no hay política.
func ConPolitica(inner ports.ProtocoloAfirma, leer func() ports.PoliticaProgramaAfirma) ports.ProtocoloAfirma {
	if leer == nil {
		leer = func() ports.PoliticaProgramaAfirma { return ports.PoliticaProgramaAfirma{} }
	}
	return conPolitica{inner: inner, leer: leer}
}

// valorPolitica es el programa que fija la política, o "" si no fija ninguno
// (sin política, o ilegible: entonces se mantiene la elección del usuario).
func valorPolitica(p ports.PoliticaProgramaAfirma) string {
	if p.Ilegible {
		return ""
	}
	v, _ := ports.NormalizarProgramaAfirma(p.Valor)
	return v
}

// CompletarEstado rellena Politica, PoliticaIlegible y Efectiva.
//
// Efectiva es lo que pasa cuando una web pide firmar ahora: lo que fija la
// política o, si no, lo que eligió el usuario mientras GrxFirma recibe los
// enlaces. Si los recibe AutoFirma u otro programa, es ese. AutoFirma y
// «preguntar» sin AutoFirma instalado acaban firmando con GrxFirma.
func CompletarEstado(e *ports.EstadoProtocoloAfirma, p ports.PoliticaProgramaAfirma) {
	e.Politica = valorPolitica(p)
	e.PoliticaIlegible = p.Ilegible
	switch {
	case !e.Soportado:
		e.Efectiva = e.Actual
		return
	case e.Politica != "" && (e.Actual == ports.ProgramaAfirmaGrxFirma || e.Actual == e.Politica):
		e.Efectiva = e.Politica
	case e.Actual == ports.ProgramaAfirmaGrxFirma:
		e.Efectiva = ports.ProgramaAfirmaEfectivo(ports.PoliticaProgramaAfirma{}, e.Preferencia)
	default:
		e.Efectiva = e.Actual
	}
	if e.Efectiva != ports.ProgramaAfirmaGrxFirma && e.Actual == ports.ProgramaAfirmaGrxFirma && !e.AutoFirmaInstalada {
		e.Efectiva = ports.ProgramaAfirmaGrxFirma
	}
}

func (c conPolitica) Estado(ctx context.Context) (ports.EstadoProtocoloAfirma, error) {
	politica := c.leer()
	estado, err := c.inner.Estado(ctx)
	CompletarEstado(&estado, politica)
	return estado, err
}

// Elegir rechaza con ErrProtocoloAfirmaPolitica todo lo que no sea el valor
// de la política. Elegir ese mismo valor sí vale: aplica lo que manda la
// organización (por ejemplo, si AutoFirma se adueñó del protocolo).
func (c conPolitica) Elegir(ctx context.Context, programa string) (ports.EstadoProtocoloAfirma, error) {
	politica := c.leer()
	if fijo := valorPolitica(politica); fijo != "" {
		if pedido, _ := ports.NormalizarProgramaAfirma(programa); pedido != fijo {
			return ports.EstadoProtocoloAfirma{}, ports.ErrProtocoloAfirmaPolitica
		}
	}
	estado, err := c.inner.Elegir(ctx, programa)
	if err != nil {
		return estado, err
	}
	CompletarEstado(&estado, politica)
	return estado, nil
}
