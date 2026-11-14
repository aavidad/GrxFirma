// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"fmt"

	"grxfirma/internal/ports"
)

// WithProtocoloAfirma habilita ver y elegir el programa que abre las firmas
// de las webs (afirma://). Solo la CLI local lo recibe: ningún canal web
// (afirma://, WebSocket, REST ni host nativo) puede cambiarlo.
func (a *Adaptador) WithProtocoloAfirma(p ports.ProtocoloAfirma) *Adaptador {
	a.ProtocoloAfirma = p
	return a
}

// claveEstadoProgramaFirmaWeb es la frase que dice qué pasa cuando una web
// pide firmar.
func claveEstadoProgramaFirmaWeb(e ports.EstadoProtocoloAfirma) string {
	switch e.Efectiva {
	case ports.ProgramaAfirmaGrxFirma:
		return "protocolo.estado.grxfirma"
	case ports.ProgramaAfirmaAutoFirma:
		return "protocolo.estado.autofirma"
	case ports.ProgramaAfirmaPreguntar:
		return "protocolo.estado.preguntar"
	case ports.ProgramaAfirmaOtro:
		return "protocolo.estado.otro"
	}
	return "protocolo.estado.ninguno"
}

func clavePoliticaProgramaFirmaWeb(politica string) string {
	return "protocolo.politica." + politica
}

func (a *Adaptador) escribirEstadoProgramaFirmaWeb(cfg configCLI, e ports.EstadoProtocoloAfirma) int {
	if cfg.json {
		if err := escribirJSON(a.Stdout, map[string]any{
			"supported":          e.Soportado,
			"current":            e.Actual,
			"effective":          e.Efectiva,
			"preference":         e.Preferencia,
			"policy":             e.Politica,
			"policyUnreadable":   e.PoliticaIlegible,
			"grxfirmaInstalled":  e.GrxFirmaInstalada,
			"autofirmaInstalled": e.AutoFirmaInstalada,
			"autofirmaPath":      e.RutaAutoFirma,
		}); err != nil {
			return a.errorProgramaFirmaWeb(err)
		}
		return 0
	}
	if !e.Soportado {
		fmt.Fprintln(a.Stdout, a.t("protocolo.error.no_disponible", "protocolo.error.no_disponible"))
		return 0
	}
	if clave := claveEstadoProgramaFirmaWeb(e); e.Efectiva == ports.ProgramaAfirmaOtro {
		fmt.Fprintln(a.Stdout, a.t(clave, clave, e.RutaActual))
	} else {
		fmt.Fprintln(a.Stdout, a.clave(clave))
	}
	if e.Politica != "" {
		fmt.Fprintln(a.Stdout, a.clave(clavePoliticaProgramaFirmaWeb(e.Politica)))
		if e.Politica != ports.ProgramaAfirmaGrxFirma && !e.AutoFirmaInstalada {
			fmt.Fprintln(a.Stdout, a.t("protocolo.politica.sin_autofirma", "protocolo.politica.sin_autofirma"))
		}
	}
	if e.PoliticaIlegible {
		fmt.Fprintln(a.Stdout, a.t("protocolo.politica_ilegible", "protocolo.politica_ilegible"))
	}
	if e.AutoFirmaInstalada {
		fmt.Fprintln(a.Stdout, a.t("cli.web_signing.autofirma_at", "AutoFirma: %s", e.RutaAutoFirma))
	} else {
		fmt.Fprintln(a.Stdout, a.t("cli.web_signing.autofirma_missing", "cli.web_signing.autofirma_missing"))
	}
	return 0
}

func (a *Adaptador) errorProgramaFirmaWeb(err error) int {
	key := "protocolo.error.generico"
	switch {
	case errors.Is(err, ports.ErrProtocoloAfirmaNoSoportado):
		key = "protocolo.error.no_disponible"
	case errors.Is(err, ports.ErrProtocoloAfirmaPolitica):
		key = "protocolo.error.politica"
	case errors.Is(err, ports.ErrProtocoloAfirmaAjeno):
		key = "protocolo.error.ajeno"
	case errors.Is(err, ports.ErrAutoFirmaNoInstalada):
		key = "cli.web_signing.autofirma_missing"
	case errors.Is(err, ports.ErrGrxFirmaAfirmaNoInstalada):
		key = "protocolo.error.grxfirma_no_instalada"
	case errors.Is(err, ports.ErrProgramaAfirmaDesconocido):
		key = "cli.web_signing.invalid"
	}
	fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), a.clave(key))
	return 1
}

func (a *Adaptador) ejecutarProgramaFirmaWebVer(ctx context.Context, cfg configCLI) int {
	if a.ProtocoloAfirma == nil {
		return a.errorProgramaFirmaWeb(ports.ErrProtocoloAfirmaNoSoportado)
	}
	estado, err := a.ProtocoloAfirma.Estado(ctx)
	if err != nil {
		return a.errorProgramaFirmaWeb(err)
	}
	return a.escribirEstadoProgramaFirmaWeb(cfg, estado)
}

func (a *Adaptador) ejecutarProgramaFirmaWebElegir(ctx context.Context, cfg configCLI) int {
	programa, ok := ports.NormalizarProgramaAfirma(cfg.programaFirmaWeb)
	if !ok {
		return a.errorProgramaFirmaWeb(ports.ErrProgramaAfirmaDesconocido)
	}
	if a.ProtocoloAfirma == nil {
		return a.errorProgramaFirmaWeb(ports.ErrProtocoloAfirmaNoSoportado)
	}
	estado, err := a.ProtocoloAfirma.Elegir(ctx, programa)
	if err != nil {
		return a.errorProgramaFirmaWeb(err)
	}
	return a.escribirEstadoProgramaFirmaWeb(cfg, estado)
}
