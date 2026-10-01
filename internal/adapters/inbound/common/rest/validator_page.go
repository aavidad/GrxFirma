// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html"
	"html/template"
)

//go:embed webvalidator.html.tmpl
var validatorPageTemplateRaw string

var validatorPageTemplate = template.Must(template.New("webvalidator").Funcs(template.FuncMap{
	"json": func(v any) template.JS {
		raw, err := json.Marshal(v)
		if err != nil {
			return template.JS(`{}`)
		}
		return template.JS(raw) // #nosec G203 -- json.Marshal escapa el contenido para el contexto JavaScript.
	},
}).Parse(validatorPageTemplateRaw))

func consoleValidatorPage(lang string, t func(string, ...any) string) string {
	t = wrapWebLocalizer(lang, t)
	data := signerPageData{
		Lang:      lang,
		Messages:  validatorPageMessages(t),
		Languages: pageLanguageOptions(lang, t),
	}
	var buf bytes.Buffer
	if err := validatorPageTemplate.Execute(&buf, data); err != nil {
		return "<!doctype html><html lang=\"" + html.EscapeString(lang) + "\"><body>" +
			html.EscapeString(t("No se pudo conectar con el backend local.")) + "</body></html>"
	}
	return buf.String()
}

func validatorPageMessages(t func(string, ...any) string) map[string]string {
	return map[string]string{
		"title":                 t("Validación avanzada"),
		"subtitle":              t("Comprueba firmas, certificados y huellas sin enviar documentos fuera de este equipo."),
		"languageSelector":      t("Idioma"),
		"openConsole":           t("Consola completa"),
		"openSigner":            t("Firmador web"),
		"openOpenAPI":           t("OpenAPI"),
		"connectionChecking":    t("Comprobando conexión con el backend local..."),
		"connectionReady":       t("Backend local disponible."),
		"connectionError":       t("No se pudo conectar con el backend local."),
		"authTitle":             t("Autenticación"),
		"bearerToken":           t("Bearer token"),
		"bearerHelp":            t("Requerido salvo autenticación por certificado. Se envía como Bearer."),
		"signatureTitle":        t("Validación rápida"),
		"signatureHelp":         t("Comprueba una firma ya existente desde fichero cargado o ruta local usando el mismo backend de verificación avanzada."),
		"signedFile":            t("Fichero"),
		"originalFile":          t("Original"),
		"selectFile":            t("Seleccionar fichero..."),
		"selectOriginal":        t("Seleccionar original..."),
		"verifyNow":             t("Verificar ahora"),
		"clear":                 t("Limpiar"),
		"result":                t("Resultado"),
		"emptyVerification":     t("Aún no se ha ejecutado una validación rápida en esta sesión."),
		"status":                t("Estado"),
		"reason":                t("Motivo"),
		"format":                t("Formato"),
		"coverage":              t("Cobertura"),
		"integrity":             t("Integridad"),
		"certificate":           t("Certificado"),
		"trust":                 t("Confianza"),
		"signers":               t("Firmantes"),
		"downloadReport":        t("Guardar informe JSON"),
		"certificateTitle":      t("Verificación de certificado"),
		"certificateHelp":       t("Selecciona un certificado del catálogo local para revisar su vigencia y estado de revocación."),
		"loadCertificates":      t("Cargar certificados"),
		"certificateSelection":  t("Certificado seleccionado"),
		"noCertificates":        t("Sin certificados cargados."),
		"validateCertificate":   t("Revisar certificados"),
		"certificateValidity":   t("Caducidad"),
		"signature":             t("Firma"),
		"issuer":                t("Emisor"),
		"fingerprint":           t("Huella"),
		"onlineCheck":           t("Comprobar revocación online"),
		"onlineNotChecked":      t("Todavía no se ha lanzado la comprobación online."),
		"onlineChecking":        t("Comprobando..."),
		"hashTitle":             t("Huellas e integridad"),
		"hashHelp":              t("Crea o comprueba huellas de ficheros y manifiestos de directorio."),
		"dataFile":              t("Entrada"),
		"hashFile":              t("Huella o manifiesto"),
		"selectHash":            t("Seleccionar huella"),
		"algorithm":             t("Algoritmo"),
		"createHash":            t("Crear huella"),
		"checkHash":             t("Comprobar huella"),
		"expectedHash":          t("Esperada: "),
		"actualHash":            t("Actual: "),
		"valid":                 t("Válido"),
		"invalid":               t("No válido"),
		"notAvailable":          t("No disponible"),
		"requestFailed":         t("La operación no se pudo completar."),
		"fileTooLarge":          t("El fichero supera el tamaño máximo permitido."),
		"selectRequired":        t("Selecciona primero los ficheros necesarios."),
		"reportFilename":        t("informe-validacion.json"),
		"certificateReportName": t("informe-certificado.json"),
		"hashReportName":        t("informe-huella.json"),
	}
}
