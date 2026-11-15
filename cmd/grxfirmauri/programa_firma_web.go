// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/desktop/afirmahandler"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
)

// Lote 15: con «Preguntar cada vez» (o la política de máquina que lo fija),
// GrxFirma pregunta con qué programa firmar antes de atender el enlace; con
// AutoFirma elegido por la preferencia o la política, se lo pasa a AutoFirma.
// Nunca se reenvía en silencio por otra causa: si algo falla, firma GrxFirma,
// como hasta ahora.

// Modos internos para mostrar los diálogos en un proceso aparte (fuera de
// Windows): Fyne no puede arrancar dos veces en el mismo proceso, y después
// del diálogo GrxFirma abre su propia ventana. No llevan el enlace.
const (
	flagElegirProgramaFirmaWeb = "--elegir-programa-firma-web"
	flagSinRecordar            = "--sin-recordar"
	flagFalloReenvioAutoFirma  = "--fallo-reenvio-autofirma"
)

// Respuestas del proceso del diálogo, una por línea.
const (
	respuestaRecordar = "recordar"
	respuestaCancelar = "cancelar"
)

var errDialogoNoDisponible = errors.New("diálogo de elección no disponible")

type eleccionProgramaFirmaWeb struct {
	programa string // grxfirma o autofirma
	recordar bool
}

var (
	nuevoSelectorFirmaWeb = func() ports.ProtocoloAfirma {
		return afirmahandler.ConPolitica(afirmahandler.New(), config.LectorPoliticaProgramaAfirma)
	}
	preguntarProgramaFirmaWeb = preguntarProgramaFirmaWebPlataforma
	preguntarFalloReenvio     = preguntarFalloReenvioPlataforma
	reenviarAutoFirma         = afirmahandler.Reenviar
)

// decidirProgramaFirmaWeb devuelve handled=true si el enlace ya se atendió
// (reenviado a AutoFirma, o cancelado tras un fallo del reenvío). Con
// handled=false, sigue GrxFirma.
func decidirProgramaFirmaWeb(ctx context.Context, rawURI string) (bool, int) {
	if os.Getenv(afirmahandler.EnvReenvioAfirma) == "1" {
		// AutoFirma (o lo que se registró como tal) devolvió el enlace a
		// GrxFirma: se firma aquí para no entrar en un bucle.
		slog.WarnContext(ctx, "afirma_reenvio_devuelto_a_grxfirma")
		return false, 0
	}
	sel := nuevoSelectorFirmaWeb()
	estado, err := sel.Estado(ctx)
	if err != nil {
		slog.WarnContext(ctx, "afirma_programa_firma_web_estado_fallido", "error", err)
		return false, 0
	}
	if estado.PoliticaIlegible {
		message := tl("excepciones.politica_ilegible.aviso")
		avisos.Registrar(message, "")
		setStartupTLSTrustNotice(detailWithStartupTLSTrustNotice(message))
		_, _ = fmt.Fprintln(os.Stderr, message)
		presentProtocolNoticePlatform(ctx, message)
	}
	switch estado.Efectiva {
	case ports.ProgramaAfirmaAutoFirma:
		return reenviarOFirmarConGrxFirma(ctx, sel, rawURI, "preferencia")
	case ports.ProgramaAfirmaPreguntar:
		if afirmahandler.ValidarURIReenvio(rawURI) != nil {
			slog.WarnContext(ctx, "afirma_programa_firma_web_enlace_no_reenviable", "esquema", afirmahandler.EsquemaURI(rawURI))
			return false, 0
		}
		if _, err := afirmahandler.EjecutableAutoFirma(ctx, sel); err != nil {
			slog.WarnContext(ctx, "afirma_programa_firma_web_sin_autofirma", "error", err)
			return false, 0
		}
		puedeRecordar := estado.Politica == ""
		eleccion, err := preguntarProgramaFirmaWeb(ctx, puedeRecordar)
		if err != nil {
			slog.WarnContext(ctx, "afirma_programa_firma_web_dialogo_fallido", "error", err)
			return false, 0
		}
		slog.InfoContext(ctx, "afirma_programa_firma_web_elegido", "programa", eleccion.programa, "recordar", eleccion.recordar && puedeRecordar)
		if eleccion.recordar && puedeRecordar {
			if _, err := sel.Elegir(ctx, eleccion.programa); err != nil {
				slog.WarnContext(ctx, "afirma_programa_firma_web_recordar_fallido", "error", err)
			}
		}
		if eleccion.programa != ports.ProgramaAfirmaAutoFirma {
			return false, 0
		}
		return reenviarOFirmarConGrxFirma(ctx, sel, rawURI, "dialogo")
	}
	return false, 0
}

// reenviarOFirmarConGrxFirma pasa el enlace a AutoFirma. Si no se puede,
// ofrece firmar con GrxFirma o cancelar.
func reenviarOFirmarConGrxFirma(ctx context.Context, sel ports.ProtocoloAfirma, rawURI, motivo string) (bool, int) {
	exe, err := afirmahandler.EjecutableAutoFirma(ctx, sel)
	if err == nil {
		err = reenviarAutoFirma(ctx, exe, rawURI)
	}
	if err == nil {
		slog.InfoContext(ctx, "afirma_reenviado_a_autofirma", "motivo", motivo)
		message := tl("excepciones.autofirma.aviso")
		avisos.Registrar(message, "")
		setStartupTLSTrustNotice(detailWithStartupTLSTrustNotice(message))
		_, _ = fmt.Fprintln(os.Stderr, message)
		presentProtocolNoticePlatform(ctx, message)
		return true, 0
	}
	slog.WarnContext(ctx, "afirma_reenvio_no_realizado", "motivo", motivo, "error", err)
	firmarAqui, errUI := preguntarFalloReenvio(ctx)
	if errUI != nil || firmarAqui {
		return false, 0
	}
	return true, 1
}

// ejecutarModoDialogoFirmaWeb atiende los modos internos de los diálogos.
func ejecutarModoDialogoFirmaWeb(ctx context.Context, args []string, stdout io.Writer) (bool, int) {
	switch {
	case len(args) >= 1 && len(args) <= 2 && args[0] == flagElegirProgramaFirmaWeb:
		puedeRecordar := true
		if len(args) == 2 {
			if args[1] != flagSinRecordar {
				return true, 2
			}
			puedeRecordar = false
		}
		eleccion, err := mostrarEleccionProgramaFirmaWeb(ctx, puedeRecordar)
		if err != nil {
			return true, 1
		}
		_, _ = fmt.Fprintln(stdout, formatearEleccionFirmaWeb(eleccion))
		return true, 0
	case len(args) == 1 && args[0] == flagFalloReenvioAutoFirma:
		firmarAqui, err := mostrarFalloReenvio(ctx)
		if err != nil {
			return true, 1
		}
		respuesta := respuestaCancelar
		if firmarAqui {
			respuesta = ports.ProgramaAfirmaGrxFirma
		}
		_, _ = fmt.Fprintln(stdout, respuesta)
		return true, 0
	}
	return false, 0
}

func formatearEleccionFirmaWeb(e eleccionProgramaFirmaWeb) string {
	if e.recordar {
		return e.programa + " " + respuestaRecordar
	}
	return e.programa
}

// analizarEleccionFirmaWeb solo acepta las respuestas exactas del diálogo.
func analizarEleccionFirmaWeb(salida string) (eleccionProgramaFirmaWeb, error) {
	campos := strings.Fields(salida)
	if len(campos) == 0 || len(campos) > 2 || (len(campos) == 2 && campos[1] != respuestaRecordar) {
		return eleccionProgramaFirmaWeb{}, errDialogoNoDisponible
	}
	switch campos[0] {
	case ports.ProgramaAfirmaGrxFirma, ports.ProgramaAfirmaAutoFirma:
		return eleccionProgramaFirmaWeb{programa: campos[0], recordar: len(campos) == 2}, nil
	}
	return eleccionProgramaFirmaWeb{}, errDialogoNoDisponible
}
