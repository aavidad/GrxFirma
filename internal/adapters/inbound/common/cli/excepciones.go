// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/appdirs"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/excepciones"
)

var errCLIActivationFlag = errors.New("excepciones.cli.id_requerido")

func (a *Adaptador) excepcionLocalizador() ports.Localizador {
	if a.Localizador != nil {
		return a.Localizador
	}
	return localizador.Detectar()
}

// ejecutarActivarExcepcion solo recibe un identificador local del parser de
// argumentos. Ninguna petición de un portal puede invocar esta operación.
func (a *Adaptador) ejecutarActivarExcepcion(ctx context.Context, cfg configCLI) int {
	id := strings.TrimSpace(cfg.excepcionInsegura)
	loc := a.excepcionLocalizador()
	if id == "" {
		fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), loc.T("excepciones.cli.id_requerido"))
		return 1
	}
	e, ok := excepciones.Find(id)
	if !ok {
		fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), loc.T("excepciones.cli.id_requerido"))
		return 1
	}
	configDir := strings.TrimSpace(a.ConfigDir)
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), a.mensajeErrorExcepcion(err))
			return 1
		}
		configDir = appdirs.Config(home)
	}

	if block := excepciones.DescribeAt(configDir, id); block != nil && !block.Activable {
		fmt.Fprintln(a.Stderr, block.Error())
		return 1
	}
	// Los tres textos y la versión se muestran completos antes de pedir
	// confirmación. La clave de riesgo siempre procede del catálogo cerrado.
	fmt.Fprintln(a.Stdout, loc.T(e.TitleKey))
	fmt.Fprintln(a.Stdout, loc.T(e.RiskKey))
	fmt.Fprintln(a.Stdout, loc.T(e.AcceptanceKey))
	fmt.Fprintln(a.Stdout, excepciones.AcceptanceVersion)
	if !cfg.aceptoAvisoConfianza {
		if !a.terminalInteractiva() {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), loc.T("excepciones.cli.confirmacion_requerida", e.ID))
			return 1
		}
		fmt.Fprint(a.Stdout, loc.T("excepciones.cli.confirmar"))
		if !a.confirmacionPositiva() {
			fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), loc.T("excepciones.cli.cancelada"))
			return 1
		}
	}
	_, err := excepciones.ActivateLocal(excepciones.ForLocalCLI(ctx), configDir, excepciones.ActivationRequest{
		ID: id, Accepted: true, AcceptanceVersion: excepciones.AcceptanceVersion,
	})
	if err != nil {
		fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), a.mensajeErrorExcepcion(err))
		return 1
	}
	fmt.Fprintln(a.Stdout, loc.T("excepciones.cli.activada", id))
	return 0
}

// La ayuda de terminal pertenece al adaptador CLI; Block.Error es común a todos los canales.
func (a *Adaptador) mensajeErrorExcepcion(err error) string {
	if err == nil {
		return ""
	}
	var block *excepciones.Block
	if !errors.As(err, &block) {
		if a.ConfigDir != "" {
			block = excepciones.FromText(err.Error(), excepciones.WithConfigDir(context.Background(), a.ConfigDir))
		} else {
			block = excepciones.FromText(err.Error())
		}
	}
	message := err.Error()
	if block != nil && block.Activable && !block.ConfigUnavailable {
		instruction := a.excepcionLocalizador().T("excepciones.cli.como_activar")
		message += "\n" + strings.ReplaceAll(instruction, "{id}", block.ID)
	}
	return message
}
