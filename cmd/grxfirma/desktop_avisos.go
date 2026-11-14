// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"strings"

	"grxfirma/internal/adapters/inbound/common/cli"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/application"
)

// avisosVerificacionDesktop es lo que la interfaz Fyne añade al resultado de
// una verificación (lote 17b): los avisos traducidos, entre ellos lo que se
// dio por válido porque el usuario lo aceptó en este equipo, y, si aceptarlo
// bastaría, la orden de la CLI que lo hace. Fyne no gestiona la lista: eso
// queda en la CLI, Qt y WinUI.
func avisosVerificacionDesktop(loc *localizador.Localizador, resultado application.VerifyResult, firma, original string) string {
	var b strings.Builder
	if len(resultado.Verification.Warnings) > 0 {
		b.WriteString("\n**" + escaparMarkdownDesktop(loc.T("Advertencias")) + "**\n\n")
		for _, aviso := range resultado.Verification.Warnings {
			b.WriteString("- " + escaparMarkdownDesktop(loc.T(aviso)) + "\n")
		}
	}
	if len(resultado.ConfianzaForzada.Forzable) == 0 || strings.ContainsAny(firma+original, "\r\n") {
		return b.String()
	}
	b.WriteString("\n" + escaparMarkdownDesktop(loc.T("verify.forced_trust.cli.hint_command")) + "\n\n")
	b.WriteString("```\n" + cli.OrdenAceptarConfianza(firma, original) + "\n```\n")
	return b.String()
}

// escaparMarkdownDesktop evita que un texto del catálogo o del motor cambie
// el formato del resultado de Fyne, que se pinta como Markdown.
func escaparMarkdownDesktop(texto string) string {
	var b strings.Builder
	for _, r := range texto {
		if strings.ContainsRune("\\`*_[]<>#|", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
