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
	"path/filepath"
	"strings"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

// Errores cuyo texto es una clave de catálogo; errorConfianzaForzada la traduce.
var (
	errClaveFaltaEntradaConfianza = errors.New("verify.forced_trust.cli.error.missing_input")
	errClaveConfirmacionConfianza = errors.New("verify.forced_trust.cli.error.needs_confirmation")
)

// ConfianzaForzadaUseCase gestiona lo que el usuario acepta en su equipo al
// verificar firmas (lote 14).
type ConfianzaForzadaUseCase interface {
	Listar(ctx context.Context) (domain.ConfianzaForzada, error)
	Aceptar(ctx context.Context, obstaculo domain.ObstaculoConfianza) (domain.AceptacionConfianza, error)
	Quitar(ctx context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error)
}

// contextoConfianzaForzada adjunta al contexto lo que el usuario aceptó en
// su equipo, para comprobaciones locales que no pasan por la verificación de
// firmas (Veri*Factu). Si no se puede leer, cuenta como prohibido.
func (a *Adaptador) contextoConfianzaForzada(ctx context.Context) (context.Context, *domain.RegistroConfianzaForzada) {
	if a.ConfianzaForzada == nil {
		return ctx, nil
	}
	confianza, err := a.ConfianzaForzada.Listar(ctx)
	if err != nil {
		confianza = domain.ConfianzaForzada{Prohibida: true}
	}
	registro := domain.NuevoRegistroConfianzaForzada(confianza)
	return domain.ConRegistroConfianzaForzada(ctx, registro), registro
}

// auditarConfianzaForzadaAplicada registra en la auditoría lo aceptado que se
// aplicó, si el caso de uso lo admite.
func (a *Adaptador) auditarConfianzaForzadaAplicada(ctx context.Context, operacion string, registro *domain.RegistroConfianzaForzada) {
	auditor, ok := a.ConfianzaForzada.(interface {
		RegistrarAplicadas(context.Context, string, []domain.ObstaculoConfianza)
	})
	if ok && registro != nil {
		auditor.RegistrarAplicadas(ctx, operacion, registro.Aplicadas())
	}
}

// escribirPistaConfianzaForzada dice cómo aceptar en este equipo los
// obstáculos que impiden dar la firma por válida, si eso bastaría.
func (a *Adaptador) escribirPistaConfianzaForzada(cfg configCLI, r application.ResultadoConfianzaForzada) {
	if a.ConfianzaForzada == nil || len(r.Forzable) == 0 {
		return
	}
	fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.hint_command", "verify.forced_trust.cli.hint_command"))
	fmt.Fprintf(a.Stdout, "  %s\n", OrdenAceptarConfianza(cfg.entrada, cfg.original))
}

// OrdenAceptarConfianza es la orden de la CLI que acepta en este equipo los
// obstáculos de una firma, con las rutas citadas para la terminal. La usa
// también la interfaz Fyne, que no gestiona la lista (lote 17b).
func OrdenAceptarConfianza(entrada, original string) string {
	orden := "grxfirma -modo-cli -aceptar-confianza -entrada " + citarArgumentoCLI(entrada)
	if strings.TrimSpace(original) != "" {
		orden += " -documento-original " + citarArgumentoCLI(original)
	}
	return orden
}

func citarArgumentoCLI(valor string) string {
	if valor != "" && !strings.ContainsAny(valor, " \t\"'\\$`") {
		return valor
	}
	return "'" + strings.ReplaceAll(valor, "'", `'\''`) + "'"
}

func confianzaForzadaJSONCLI(r application.ResultadoConfianzaForzada) map[string]any {
	return map[string]any{
		"aplicada":  obstaculosJSONCLI(r.Aplicada),
		"forzable":  obstaculosJSONCLI(r.Forzable),
		"prohibida": r.Prohibida,
	}
}

func obstaculosJSONCLI(lista []domain.ObstaculoConfianza) []map[string]string {
	out := make([]map[string]string, 0, len(lista))
	for _, o := range lista {
		out = append(out, map[string]string{"tipo": string(o.Tipo), "huella": o.Huella, "sujeto": o.Sujeto})
	}
	return out
}

func (a *Adaptador) errorConfianzaForzada(err error) int {
	mensaje := err.Error()
	if strings.HasPrefix(mensaje, "verify.forced_trust.") {
		mensaje = a.clave(mensaje)
	}
	fmt.Fprintln(a.Stderr, a.t("cli.error.prefix", "error:"), mensaje)
	return 1
}

func (a *Adaptador) ejecutarConfianzaForzadaListar(ctx context.Context, cfg configCLI) int {
	if a.ConfianzaForzada == nil {
		return a.errorConfianzaForzada(application.ErrConfianzaForzadaNoConfigurada)
	}
	confianza, err := a.ConfianzaForzada.Listar(ctx)
	if err != nil {
		return a.errorConfianzaForzada(err)
	}
	if cfg.json {
		entradas := make([]map[string]string, 0, len(confianza.Aceptaciones))
		for _, e := range confianza.Aceptaciones {
			entradas = append(entradas, map[string]string{
				"tipo": string(e.Tipo), "huella": e.Huella, "sujeto": e.Sujeto,
				"aceptada_en": e.AceptadaEn.Format("2006-01-02T15:04:05Z07:00"), "aviso": e.Aviso,
			})
		}
		if err := escribirJSON(a.Stdout, map[string]any{"prohibida": confianza.Prohibida, "entradas": entradas}); err != nil {
			return a.errorConfianzaForzada(err)
		}
		return 0
	}
	if confianza.Prohibida {
		fmt.Fprintln(a.Stdout, a.t(domain.ClaveConfianzaForzadaProhibida, domain.ClaveConfianzaForzadaProhibida))
	}
	if len(confianza.Aceptaciones) == 0 {
		fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.list_empty", "verify.forced_trust.cli.list_empty"))
		return 0
	}
	for _, e := range confianza.Aceptaciones {
		fmt.Fprintf(a.Stdout, "- %s\n", a.clave(etiquetaTipoConfianza(e.Tipo)))
		fmt.Fprintf(a.Stdout, "  %s: %s\n", a.t("verify.forced_trust.cli.type", "verify.forced_trust.cli.type"), e.Tipo)
		if e.Sujeto != "" {
			fmt.Fprintf(a.Stdout, "  %s: %s\n", a.t("verify.forced_trust.cli.subject", "verify.forced_trust.cli.subject"), e.Sujeto)
		}
		if e.Huella != "" {
			fmt.Fprintf(a.Stdout, "  %s: %s\n", a.t("verify.forced_trust.cli.fingerprint", "verify.forced_trust.cli.fingerprint"), e.Huella)
		}
		fmt.Fprintf(a.Stdout, "  %s: %s\n", a.t("verify.forced_trust.cli.accepted_at", "verify.forced_trust.cli.accepted_at"), e.AceptadaEn.Local().Format("2006-01-02 15:04"))
	}
	return 0
}

func etiquetaTipoConfianza(tipo domain.TipoConfianzaForzada) string {
	return "verify.forced_trust.label." + string(tipo)
}

// ejecutarConfianzaForzadaAceptar acepta en este equipo:
//   - con -entrada, los obstáculos de esa firma (todos, o los de
//     -tipo-confianza), solo si con ellos la firma saldría válida;
//   - sin -entrada, el tipo de -tipo-confianza (con -huella si es por
//     certificado).
//
// Antes muestra el aviso y pide confirmación; sin terminal exige
// -acepto-el-aviso.
func (a *Adaptador) ejecutarConfianzaForzadaAceptar(ctx context.Context, cfg configCLI) int {
	if a.ConfianzaForzada == nil {
		return a.errorConfianzaForzada(application.ErrConfianzaForzadaNoConfigurada)
	}
	tipoPedido := domain.TipoConfianzaForzada(strings.TrimSpace(cfg.tipoConfianza))
	if tipoPedido != "" && !tipoPedido.Valido() {
		return a.errorConfianzaForzada(domain.ErrConfianzaTipoDesconocido)
	}
	var obstaculos []domain.ObstaculoConfianza
	if strings.TrimSpace(cfg.entrada) != "" {
		encontrados, rc := a.obstaculosDeFirma(ctx, cfg)
		if rc != 0 {
			return rc
		}
		for _, o := range encontrados {
			if tipoPedido == "" || o.Tipo == tipoPedido {
				obstaculos = append(obstaculos, o)
			}
		}
		if len(obstaculos) == 0 {
			fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.nothing_to_accept", "verify.forced_trust.cli.nothing_to_accept"))
			return 0
		}
	} else {
		if tipoPedido == "" {
			return a.errorConfianzaForzada(errClaveFaltaEntradaConfianza)
		}
		obstaculos = []domain.ObstaculoConfianza{{Tipo: tipoPedido, Huella: cfg.huellaConfianza}}
	}

	for _, o := range obstaculos {
		fmt.Fprintf(a.Stdout, "%s\n", a.clave(etiquetaTipoConfianza(o.Tipo)))
		fmt.Fprintf(a.Stdout, "%s\n", a.clave(o.Tipo.ClaveAviso()))
		if o.Sujeto != "" {
			fmt.Fprintf(a.Stdout, "  %s: %s\n", a.t("verify.forced_trust.cli.subject", "verify.forced_trust.cli.subject"), o.Sujeto)
		}
		if o.Huella != "" {
			fmt.Fprintf(a.Stdout, "  %s: %s\n", a.t("verify.forced_trust.cli.fingerprint", "verify.forced_trust.cli.fingerprint"), o.Huella)
		}
		fmt.Fprintln(a.Stdout)
	}
	if !cfg.aceptoAvisoConfianza {
		if !a.terminalInteractiva() {
			return a.errorConfianzaForzada(errClaveConfirmacionConfianza)
		}
		fmt.Fprint(a.Stdout, a.t("verify.forced_trust.cli.confirm", "verify.forced_trust.cli.confirm"))
		if !a.confirmacionPositiva() {
			fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.cancelled", "verify.forced_trust.cli.cancelled"))
			return 0
		}
	}
	for _, o := range obstaculos {
		if _, err := a.ConfianzaForzada.Aceptar(ctx, o); err != nil {
			return a.errorConfianzaForzada(err)
		}
	}
	fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.accepted", "verify.forced_trust.cli.accepted"))
	return 0
}

// obstaculosDeFirma verifica la firma indicada y devuelve los obstáculos que
// bastaría con aceptar para darla por válida.
func (a *Adaptador) obstaculosDeFirma(ctx context.Context, cfg configCLI) ([]domain.ObstaculoConfianza, int) {
	if a.Verificar == nil {
		return nil, a.errorConfianzaForzada(application.ErrConfianzaForzadaNoConfigurada)
	}
	datos, err := a.LeerFichero(cfg.entrada)
	if err != nil {
		a.escribirErrorCLI(a.t("Entrada", "Entrada"), err)
		return nil, 1
	}
	firmado, err := domain.NewDocument(filepath.Base(cfg.entrada), datos, inferirTipoMIME(cfg.entrada))
	if err != nil {
		a.escribirErrorCLI(a.t("Documento", "Documento"), err)
		return nil, 1
	}
	cmd := application.VerifyCommand{SignedDocument: firmado}
	if strings.TrimSpace(cfg.original) != "" {
		original, err := a.LeerFichero(cfg.original)
		if err != nil {
			a.escribirErrorCLI(a.t("Original", "Original"), err)
			return nil, 1
		}
		doc, err := domain.NewDocument(filepath.Base(cfg.original), original, inferirTipoMIME(cfg.original))
		if err != nil {
			a.escribirErrorCLI(a.t("Original", "Original"), err)
			return nil, 1
		}
		cmd.OriginalDocument = &doc
	}
	resultado, err := a.Verificar.Execute(ctx, cmd)
	if err != nil {
		a.escribirErrorCLI(a.t("Validar", "Validar"), err)
		return nil, 1
	}
	if resultado.ConfianzaForzada.Prohibida {
		return nil, a.errorConfianzaForzada(domain.ErrConfianzaProhibida)
	}
	return resultado.ConfianzaForzada.Forzable, 0
}

func (a *Adaptador) ejecutarConfianzaForzadaQuitar(ctx context.Context, cfg configCLI) int {
	if a.ConfianzaForzada == nil {
		return a.errorConfianzaForzada(application.ErrConfianzaForzadaNoConfigurada)
	}
	tipo := domain.TipoConfianzaForzada(strings.TrimSpace(cfg.tipoConfianza))
	quitada, err := a.ConfianzaForzada.Quitar(ctx, tipo, cfg.huellaConfianza)
	if err != nil {
		return a.errorConfianzaForzada(err)
	}
	if !quitada {
		fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.not_found", "verify.forced_trust.cli.not_found"))
		return 0
	}
	fmt.Fprintln(a.Stdout, a.t("verify.forced_trust.cli.removed", "verify.forced_trust.cli.removed"))
	return 0
}

// clave traduce una clave de catálogo; sin traducción devuelve la clave.
func (a *Adaptador) clave(id string) string {
	if a.Localizador != nil {
		if traducido := a.Localizador.T(id); traducido != "" {
			return traducido
		}
	}
	return id
}

// terminalInteractiva indica si entrada y salida son una terminal.
func (a *Adaptador) terminalInteractiva() bool {
	if a.esInteractivo != nil {
		return a.esInteractivo()
	}
	in, ok := a.Stdin.(*os.File)
	if !ok {
		return false
	}
	out, ok := a.Stdout.(*os.File)
	if !ok {
		return false
	}
	inInfo, err := in.Stat()
	if err != nil {
		return false
	}
	outInfo, err := out.Stat()
	if err != nil {
		return false
	}
	return inInfo.Mode()&os.ModeCharDevice != 0 && outInfo.Mode()&os.ModeCharDevice != 0
}
