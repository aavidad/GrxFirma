// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog

import "fmt"

type promptMessage struct {
	Headline       string
	OriginLabel    string
	OriginValue    string
	PrimaryMessage string
	RiskMessage    string
	ResidentRisk   string
	Question       string
}

func buildPromptMessage(origin string) promptMessage {
	return promptMessage{
		Headline:       tt("Portal u origen desconocido"),
		OriginLabel:    tt("Portal/origen:"),
		OriginValue:    origin,
		PrimaryMessage: tt("Este portal quiere iniciar una operación de firma electrónica con GrxFirma, pero todavía no está marcado como origen confiable."),
		RiskMessage:    tt("Si continúas, este sitio podrá pedir operaciones de firma o acceso al flujo local en esta sesión."),
		ResidentRisk:   tt("Si GrxFirma está residente, aceptar sin revisar el origen reduce la barrera entre el navegador y el agente local. Verifica bien el portal antes de continuar."),
		Question:       tt("¿Qué quieres hacer con este portal?"),
	}
}

// BuildPromptMessageForUI expone el copy canónico del diálogo TOFU/origen
// desconocido para reutilizarlo desde el cableado Fyne del flujo web.
func BuildPromptMessageForUI(origin string) struct {
	Headline       string
	OriginLabel    string
	OriginValue    string
	PrimaryMessage string
	RiskMessage    string
	ResidentRisk   string
	Question       string
} {
	msg := buildPromptMessage(origin)
	return struct {
		Headline       string
		OriginLabel    string
		OriginValue    string
		PrimaryMessage string
		RiskMessage    string
		ResidentRisk   string
		Question       string
	}{
		Headline:       msg.Headline,
		OriginLabel:    msg.OriginLabel,
		OriginValue:    msg.OriginValue,
		PrimaryMessage: msg.PrimaryMessage,
		RiskMessage:    msg.RiskMessage,
		ResidentRisk:   msg.ResidentRisk,
		Question:       msg.Question,
	}
}

func buildPromptReport(origin string) string {
	msg := buildPromptMessage(origin)
	return fmt.Sprintf("%s\n\n%s %s\n\n%s\n\n%s\n%s\n%s\n\n%s",
		msg.Headline,
		msg.OriginLabel,
		msg.OriginValue,
		msg.PrimaryMessage,
		tt("Riesgo si continúas"),
		msg.RiskMessage,
		msg.ResidentRisk,
		msg.Question,
	)
}
