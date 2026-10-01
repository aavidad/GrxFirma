// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"strings"
)

//go:embed websigner.html.tmpl
var signerPageTemplateRaw string

var signerPageTemplate = template.Must(template.New("websigner").Funcs(template.FuncMap{
	"json": func(v any) template.JS {
		raw, err := json.Marshal(v)
		if err != nil {
			return template.JS(`{}`)
		}
		return template.JS(raw) // #nosec G203 -- raw procede exclusivamente de json.Marshal, que escapa contenido para contexto JavaScript.
	},
}).Parse(signerPageTemplateRaw))

type signerPageData struct {
	Lang      string
	Messages  map[string]string
	Languages []pageLanguageOption
}

type pageLanguageOption struct {
	Code string
	Name string
}

func consoleSignerPage(lang string, t func(string, ...any) string) string {
	t = wrapWebLocalizer(lang, t)
	data := signerPageData{
		Lang:      lang,
		Messages:  signerPageMessages(t),
		Languages: pageLanguageOptions(lang, t),
	}
	var buf bytes.Buffer
	if err := signerPageTemplate.Execute(&buf, data); err != nil {
		return "<!doctype html><html lang=\"" + html.EscapeString(lang) + "\"><body>" + html.EscapeString(t("No se pudo conectar con el backend local.")) + "</body></html>"
	}
	return buf.String()
}

func pageLanguageOptions(currentLang string, t func(string, ...any) string) []pageLanguageOption {
	codes := []string{"es", "ca", "va", "eu", "gl", "en", "de", "fr", "pt", "it", "zh"}
	out := make([]pageLanguageOption, 0, len(codes))
	for _, code := range codes {
		name := t("language." + code)
		if name == "language."+code {
			name = fallbackLanguageLabel(currentLang, code)
		}
		out = append(out, pageLanguageOption{
			Code: code,
			Name: name,
		})
	}
	return out
}

func languageSelectorHTML(basePath, currentLang string, t func(string, ...any) string) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="language-picker">`)
	buf.WriteString(`<label for="languageSelect">`)
	buf.WriteString(html.EscapeString(t("Idioma")))
	buf.WriteString(`</label><select id="languageSelect" onchange="window.grxfirmaSwitchWebLanguage && window.grxfirmaSwitchWebLanguage(this.value)">`)
	for _, item := range pageLanguageOptions(currentLang, t) {
		selected := ""
		if item.Code == currentLang {
			selected = ` selected`
		}
		fmt.Fprintf(&buf, `<option value="%s"%s>%s</option>`, html.EscapeString(item.Code), selected, html.EscapeString(item.Name))
	}
	buf.WriteString(`</select></div>`)
	return buf.String()
}

func fallbackLanguageLabel(currentLang, code string) string {
	normalized := strings.TrimSpace(strings.ToLower(currentLang))
	if normalized == "es" {
		switch code {
		case "es":
			return "Español"
		case "ca":
			return "Catalán"
		case "va":
			return "Valenciano"
		case "eu":
			return "Euskera"
		case "gl":
			return "Gallego"
		case "en":
			return "Inglés"
		case "de":
			return "Alemán"
		case "fr":
			return "Francés"
		case "pt":
			return "Portugués"
		case "it":
			return "Italiano"
		case "zh":
			return "Chino"
		}
	}
	switch code {
	case "es":
		return "Spanish"
	case "ca":
		return "Catalan"
	case "va":
		return "Valencian"
	case "eu":
		return "Basque"
	case "gl":
		return "Galician"
	case "en":
		return "English"
	case "de":
		return "German"
	case "fr":
		return "French"
	case "pt":
		return "Portuguese"
	case "it":
		return "Italian"
	case "zh":
		return "Chinese"
	default:
		return code
	}
}

func signerPageMessages(t func(string, ...any) string) map[string]string {
	return map[string]string{
		"title":                                   t("Firmador web"),
		"subtitle":                                t("Interfaz web local de GrxFirma para firmar documentos y configurar sello visible PAdES sin depender de la extensión completa."),
		"languageSelector":                        t("Idioma"),
		"openConsole":                             t("Consola completa"),
		"openValidator":                           t("Validación avanzada"),
		"openHashTools":                           t("Utilidades de huellas"),
		"openOpenAPI":                             t("OpenAPI"),
		"connectionReady":                         t("Backend local disponible."),
		"connectionChecking":                      t("Comprobando conexión con el backend local..."),
		"connectionError":                         t("No se pudo conectar con el backend local."),
		"fileCardTitle":                           t("Documento(s)"),
		"fileHelp":                                t("Selecciona varios ficheros sueltos, una carpeta completa, arrástralos aquí o usa una ruta local. Para PDFs con sello visible se recomienda PAdES."),
		"dropLabel":                               t("Arrastra aquí uno o varios ficheros o pulsa para seleccionarlos"),
		"selectFile":                              t("Seleccionar fichero(s)"),
		"selectDirectory":                         t("Seleccionar carpeta"),
		"clearFile":                               t("Quitar"),
		"localPath":                               t("Ruta local"),
		"localPathPlaceholder":                    t("/ruta/al/documento.pdf"),
		"outputPath":                              t("Ruta de salida"),
		"outputPathPlaceholder":                   t("/ruta/de/salida/opcional"),
		"saveToDisk":                              t("Guardar en disco si se usa ruta"),
		"returnB64":                               t("Devolver contenido firmado en base64"),
		"overwrite":                               t("Sobrescribir salida"),
		"localPathHelp":                           t("Si indicas una ruta local, el firmador usará el backend sobre ese fichero sin necesidad de subirlo al navegador."),
		"selectionTypeTitle":                      t("Tipo de selección"),
		"selectionTypePending":                    t("Pendiente de selección"),
		"selectionTypeBatch":                      t("Lote o selección múltiple"),
		"selectionTypePDF":                        t("PDF"),
		"selectionTypeXML":                        t("XML"),
		"selectionTypeODF":                        t("ODF"),
		"selectionTypeOOXML":                      t("OOXML"),
		"selectionTypeCMS":                        t("CMS/CAdES"),
		"selectionTypeGeneric":                    t("Documento genérico"),
		"certificateCardTitle":                    t("Certificado y firma"),
		"refreshCertificates":                     t("Recargar certificados"),
		"clearSelectedCertificate":                t("Quitar selección"),
		"certificateInventoryTitle":               t("Inventario"),
		"certificateInventoryEmpty":               t("Sin certificados cargados."),
		"certificateInventoryLoaded":              t("certificado(s) cargado(s)"),
		"certificateInventoryUsable":              t("Aptos para firma: "),
		"autoSelectCertificate":                   t("Seleccionar automáticamente"),
		"certificateSubjectFilter":                t("Filtro por titular"),
		"certificateSubjectFilterPlaceholder":     t("Ana, representante, sello..."),
		"certificateIssuerFilter":                 t("Filtro por emisor"),
		"certificateIssuerFilterPlaceholder":      t("FNMT, AC, CA..."),
		"clearCertificateFilters":                 t("Mostrar todos"),
		"onlyUsableCertificates":                  t("Solo utilizables"),
		"onlyValidCertificates":                   t("Ocultar caducados"),
		"certificateFilterMeta":                   t("Mostrando %visible% de %total% certificado(s)."),
		"certificateFilterEmptyCatalog":           t("Aún no hay catálogo cargado para aplicar filtros."),
		"certificateFilterNoMatches":              t("Ningún certificado coincide con los filtros actuales."),
		"certificateFilterNoMatchesShort":         t("Sin coincidencias"),
		"certificateFilterSelectionHidden":        t("La selección actual queda fuera de los filtros, pero se mantiene como referencia."),
		"certificateFilterSelectionVisible":       t("La selección actual es coherente con los filtros visibles."),
		"rememberSelectedCertificate":             t("Recordar certificado seleccionado"),
		"certificatePreferenceTitle":              t("Preferencia de certificado"),
		"certificatePreferenceDisabled":           t("La selección actual no se recordará entre sesiones."),
		"certificatePreferenceNoSelection":        t("Activa la preferencia y selecciona un certificado para reutilizarlo en la siguiente sesión."),
		"certificatePreferenceStored":             t("Se reutilizará en la próxima sesión si sigue disponible."),
		"certificatePreferenceRestored":           t("Restaurado desde la sesión anterior."),
		"certificatePreferenceMissing":            t("La preferencia guardada ya no está disponible en el catálogo actual."),
		"certificateAutoSelectIdle":               t("Puedes pedir al backend que seleccione automáticamente el certificado más adecuado según los filtros visibles."),
		"certificateAutoSelectRunning":            t("Seleccionando certificado automáticamente..."),
		"certificateAutoSelectDone":               t("Certificado seleccionado automáticamente:"),
		"certificateAutoSelectError":              t("No se pudo seleccionar certificado automáticamente:"),
		"certificate":                             t("Certificado"),
		"certificateStatusTitle":                  t("Estado"),
		"certificateStatusNone":                   t("Sin certificado seleccionado."),
		"certificateStatusNotUsable":              t("El certificado seleccionado no es apto para firma."),
		"certificateStatusExpired":                t("El certificado seleccionado está caducado o expirado."),
		"certificateStatusReady":                  t("El certificado seleccionado está listo para firmar."),
		"certificateExpiryTitle":                  t("Caducidad"),
		"certificateExpiryEmpty":                  t("Sin información de caducidad."),
		"certificateExpirySoon":                   t("Caduca en días: "),
		"certificateExpiryOk":                     t("Sin caducidad próxima."),
		"certificateDetailsEmpty":                 t("Sin detalles de certificado."),
		"certificateExportPending":                t("Selecciona un certificado para exportar su resumen visible como JSON."),
		"certificateExportReady":                  t("Descargar resumen JSON del certificado"),
		"certificateExportFilenameFallback":       t("certificado"),
		"certificateExportFilenameSuffix":         t("_certificado.json"),
		"certificateCopyReady":                    t("Copiar JSON del certificado"),
		"certificateCopyDone":                     t("JSON del certificado copiado al portapapeles."),
		"certificateCopyError":                    t("No se pudo copiar el JSON del certificado."),
		"certificateFingerprintCopyReady":         t("Copiar huella"),
		"certificateFingerprintCopyDone":          t("Huella del certificado copiada al portapapeles."),
		"certificateFingerprintCopyError":         t("No se pudo copiar la huella del certificado."),
		"certificateValidity":                     t("Validez"),
		"certificateSerial":                       t("Serie"),
		"certificateFingerprint":                  t("Huella"),
		"formatOptionAuto":                        t("AUTO"),
		"formatOptionPAdES":                       t("PAdES"),
		"formatOptionCAdES":                       t("CAdES"),
		"formatOptionXAdES":                       t("XAdES"),
		"formatOptionXMLdSig":                     t("XMLdSig"),
		"formatOptionODF":                         t("ODF"),
		"formatOptionOOXML":                       t("OOXML"),
		"formatOptionFacturaE":                    t("FacturaE"),
		"formatOptionASiCXAdES":                   t("ASiC-XAdES"),
		"format":                                  t("Formato"),
		"action":                                  t("Acción"),
		"actionSign":                              t("Firmar"),
		"actionCosign":                            t("Cofirma"),
		"actionCountersign":                       t("Contrafirma"),
		"actionHelp":                              t("Permite firmar, cofirmar o contrafirmar directamente desde el firmador web local."),
		"strictCompat":                            t("Compatibilidad estricta"),
		"allowInvalidPDF":                         t("Permitir PDF inválido"),
		"signProfile":                             t("Perfil de firma"),
		"signProfileBaseline":                     t("Baseline B"),
		"signProfileT":                            t("Baseline T"),
		"signProfileLT":                           t("CAdES-LT"),
		"signProfileLTA":                          t("CAdES-LTA"),
		"signProfileHelp":                         t("Baseline B/T aplica a PAdES, CAdES y XAdES. LT/LTA solo aplica de forma efectiva a CAdES."),
		"tsaUrl":                                  t("URL TSA"),
		"tsaUrlPlaceholder":                       t("https://tsa.ejemplo/"),
		"tsaUrlHelp":                              t("Opcional. Si indicas una TSA, el backend la usará para perfiles con sello de tiempo."),
		"tsaUrlNotConfigured":                     t("Sin TSA configurada"),
		"padesSubFilter":                          t("Subfiltro PAdES"),
		"padesSubFilterETSI":                      t("ETSI.CAdES.detached"),
		"padesSubFilterAdobe":                     t("adbe.pkcs7.detached"),
		"padesSubFilterHelp":                      t("Solo aplica a PAdES. Permite elegir entre el subfiltro ETSI recomendado o el modo Adobe compatible."),
		"enableMultiCosign":                       t("Cofirma múltiple guiada"),
		"enableMultiCosignHelp":                   t("Encadena una firma inicial y varias cofirmas sobre un único documento usando el backend REST actual."),
		"multiCosignAdditionalSigners":            t("Firmantes adicionales"),
		"multiCosignSelectAll":                    t("Seleccionar todos"),
		"multiCosignClear":                        t("Limpiar"),
		"multiCosignTitle":                        t("Plan de cofirma múltiple"),
		"multiCosignPrimary":                      t("Firmante principal"),
		"multiCosignAdditional":                   t("Cofirmantes"),
		"multiCosignDisabled":                     t("La cofirma múltiple guiada está desactivada."),
		"multiCosignDisabledShort":                t("desactivada"),
		"multiCosignEnabled":                      t("activa"),
		"multiCosignUnsupported":                  t("La cofirma múltiple guiada solo está disponible para documento único en acciones firmar o cofirmar."),
		"multiCosignNeedPrimary":                  t("Selecciona primero el certificado principal."),
		"multiCosignNoAdditional":                 t("Selecciona al menos un firmante adicional para la cofirma múltiple."),
		"multiCosignExecutedTitle":                t("Cofirma múltiple aplicada"),
		"multiCosignExecutedChain":                t("Cadena de firmantes"),
		"multiCosignMemoryOnlyNotice":             t("En este modo guiado la salida final se mantiene en memoria/descarga; la persistencia en disco específica requiere un endpoint nativo de cofirma múltiple."),
		"multiCosignOutputPolicyTitle":            t("Entrega efectiva en cofirma múltiple"),
		"multiCosignOutputPolicyActive":           t("Mientras la cofirma múltiple guiada esté activa, la salida final se entregará en memoria/base64 y se bloquearán temporalmente las opciones de guardado en disco."),
		"multiCosignOutputPolicyRestore":          t("Al desactivar la cofirma múltiple guiada se restaurarán tus preferencias previas de salida."),
		"multiCosignOutputPolicyInactive":         t("La política especial de salida para cofirma múltiple no está activa."),
		"multiCosignLabel":                        t("Cofirma múltiple"),
		"operationStrategySignMulti":              t("Se generará una firma inicial y después se añadirán cofirmas encadenadas sobre el mismo artefacto."),
		"operationStrategyCosignMulti":            t("Se añadirá una cofirma y, a continuación, se encadenarán cofirmas adicionales sobre el mismo artefacto."),
		"signingPlan":                             t("Plan de firma"),
		"guidedAssistantTitle":                    t("Siguiente paso"),
		"guidedAssistantNeedSource":               t("Selecciona primero un fichero, una carpeta o una ruta local para preparar la operación."),
		"guidedAssistantNeedSourceHelp":           t("Puedes arrastrar documentos al área superior o indicar una ruta local sin subir el fichero al navegador."),
		"guidedAssistantNeedCertificate":          t("Selecciona un certificado antes de firmar o validar la participación del firmante."),
		"guidedAssistantNeedCertificateHelp":      t("El bloque de certificado permite filtrar, autoseleccionar o recordar el firmante activo entre sesiones."),
		"guidedAssistantNeedCoSigner":             t("Añade al menos un cofirmante adicional para completar la cofirma múltiple guiada."),
		"guidedAssistantNeedCoSignerHelp":         t("El certificado actual actuará como firmante principal y los adicionales se encadenarán sobre el mismo artefacto."),
		"guidedAssistantNeedPades":                t("Cambia a PAdES o desactiva el sello visible para evitar una configuración incompatible."),
		"guidedAssistantNeedPadesHelp":            t("El sello visible solo se aplica cuando el formato efectivo de firma es PAdES."),
		"guidedAssistantInvalidPages":             t("Revisa la selección de páginas del sello visible antes de firmar."),
		"guidedAssistantInvalidPagesHelp":         t("Usa valores como 1, 2,4-6 o all para indicar páginas válidas."),
		"guidedAssistantReady":                    t("Todo listo para firmar con la configuración actual."),
		"guidedAssistantReadyHelp":                t("Puedes lanzar la operación ahora o seguir afinando sello visible, perfil o metadatos antes de firmar."),
		"verificationPlanTitle":                   t("Verificacion posterior"),
		"verificationPlanPending":                 t("Aun no hay una seleccion preparada para estimar la verificacion automatica posterior."),
		"verificationPlanSign":                    t("Tras firmar, se validara automaticamente el artefacto resultante."),
		"verificationPlanCosign":                  t("Tras cofirmar, se validara el artefacto final manteniendo la firma principal existente."),
		"verificationPlanCountersign":             t("Tras contrafirmar, se validara el artefacto final con la nueva contrafirma aplicada."),
		"verificationPlanUploadRef":               t("El documento cargado en el navegador quedara disponible como referencia local durante la autoverificacion."),
		"verificationPlanBatchRef":                t("Cada elemento cargado en el navegador quedara disponible como referencia local durante la autoverificacion del lote."),
		"verificationPlanLocalRef":                t("Con ruta local, la autoverificacion se hara sobre el artefacto firmado final disponible, sin reenviar el original por separado."),
		"verificationPlanFormatRef":               t("Si el formato y la operacion aprovechan un original de referencia, se usara automaticamente cuando este disponible."),
		"verificationPlanArtifactOnly":            t("La validacion se centrara en la integridad y confianza del artefacto firmado final."),
		"certificateRoleTitle":                    t("Rol del certificado"),
		"certificateRolePending":                  t("Selecciona un certificado para ver cómo participará en la operación elegida."),
		"certificateRoleSelected":                 t("Certificado operativo"),
		"certificateRoleDescription":              t("Participación"),
		"certificateRoleNotUsable":                t("El certificado seleccionado no es apto para asumir la operación elegida."),
		"certificateRoleSign":                     t("Actuará como firmante inicial del artefacto resultante."),
		"certificateRoleCosign":                   t("Se añadirá como cofirmante, manteniendo la firma ya existente."),
		"certificateRoleCountersign":              t("Aplicará una contrafirma sobre la firma ya presente."),
		"operationStrategyTitle":                  t("Estrategia de operación"),
		"operationStrategy":                       t("Estrategia"),
		"operationStrategyPending":                t("Aún no hay una selección preparada para determinar la estrategia efectiva."),
		"operationStrategySignSingle":             t("Se generará una firma nueva sobre el documento seleccionado."),
		"operationStrategySignBatch":              t("Se generará una firma nueva por cada elemento del lote seleccionado."),
		"operationStrategyCosignSingle":           t("Se añadirá una cofirma al artefacto ya firmado, manteniendo la firma existente."),
		"operationStrategyCosignBatch":            t("Se intentará añadir una cofirma sobre cada elemento del lote seleccionado."),
		"operationStrategyCountersignSingle":      t("Se aplicará una contrafirma sobre la firma ya presente en el artefacto seleccionado."),
		"operationStrategyCountersignBatch":       t("Se intentará aplicar una contrafirma sobre cada elemento del lote seleccionado."),
		"selectionType":                           t("Selección"),
		"filesSelectedCount":                      t("ficheros seleccionados"),
		"source":                                  t("Origen"),
		"outputMode":                              t("Entrega"),
		"sealStatus":                              t("Sello visible"),
		"uploadedSelection":                       t("Selección cargada en navegador"),
		"localPathMode":                           t("Ruta local"),
		"pendingSelection":                        t("Pendiente de selección"),
		"deliveryDiskAndBase64":                   t("disco + base64"),
		"deliveryDiskOnly":                        t("solo disco"),
		"deliveryMemoryAndBase64":                 t("memoria + base64"),
		"deliveryMemoryOnly":                      t("solo memoria"),
		"outputDeliveryTitle":                     t("Persistencia efectiva"),
		"outputDeliveryPending":                   t("Aun no hay una seleccion preparada para calcular la persistencia real del resultado."),
		"outputDeliveryLocalAuto":                 t("Con ruta local, el backend podra guardar el firmado en disco aunque no indiques ruta de salida explicita."),
		"outputDeliveryLocalExplicit":             t("La firma se guardara en la ruta de salida indicada y, si activas base64, tambien volvera al navegador."),
		"outputDeliveryUploadExplicit":            t("La firma cargada desde navegador solo podra guardarse en disco porque has indicado una ruta de salida explicita."),
		"outputDeliveryUploadNeedPath":            t("Con un fichero cargado en el navegador, marcar guardar en disco requiere indicar una ruta de salida explicita."),
		"outputDeliveryBatchMemory":               t("En lote cargado desde navegador, la salida se entregara por respuesta web; para persistencia en disco hacen falta rutas por elemento."),
		"outputDeliveryMemory":                    t("La salida quedara disponible en memoria o base64, sin escritura local obligatoria."),
		"outputDeliveryBase64Also":                t("Ademas se devolvera contenido firmado en base64."),
		"outputArtifactTitle":                     t("Artefacto esperado"),
		"outputArtifactUnknown":                   t("No se pudo inferir el artefacto firmado esperado."),
		"outputArtifactExpected":                  t("Sin ruta de salida, se generará un artefacto con extensión "),
		"outputArtifactMismatch":                  t("La ruta de salida no termina en "),
		"outputArtifactMatch":                     t("La ruta de salida coincide con la extensión esperada "),
		"proxySecretStoreTitle":                   t("Almacen seguro de proxy"),
		"proxySecretStoreReady":                   t("El backend seguro de proxy esta disponible para este firmador local."),
		"proxySecretStoreNotReady":                t("El backend seguro de proxy no esta listo; el proxy autenticado puede quedar limitado en este equipo."),
		"proxySecretStoreAvailableLabel":          t("Disponible"),
		"proxySecretStoreBackendLabel":            t("Backend"),
		"proxySecretStorePlatformLabel":           t("Plataforma"),
		"proxySecretStoreRuntimeModeLabel":        t("Modo de proxy en ejecución"),
		"proxySecretStoreReasonLabel":             t("Motivo"),
		"proxySecretStoreNoIssues":                t("Sin incidencias"),
		"proxyRuntimeModeNone":                    t("Sin proxy"),
		"proxyRuntimeModeSystem":                  t("Proxy del sistema"),
		"proxyRuntimeModeConfigured":              t("Proxy configurado"),
		"proxyRuntimeModeAuthenticated":           t("Proxy autenticado"),
		"yes":                                     t("si"),
		"no":                                      t("no"),
		"sealEnabled":                             t("activo"),
		"sealDisabled":                            t("inactivo"),
		"signNow":                                 t("Firmar ahora"),
		"sealCardTitle":                           t("Firma visible (PAdES)"),
		"sealHelp":                                t("Adoptado del firmador web anterior. Esta vista usa el mismo contrato de sello visible que el backend actual."),
		"enableVisibleSeal":                       t("Activar sello visible"),
		"position":                                t("Posición"),
		"bottomLeft":                              t("Inferior izquierda"),
		"bottomRight":                             t("Inferior derecha"),
		"topLeft":                                 t("Superior izquierda"),
		"topRight":                                t("Superior derecha"),
		"custom":                                  t("Personalizada"),
		"page":                                    t("Página"),
		"pageRange":                               t("Página(s)"),
		"pageRangePlaceholder":                    t("1 o 1,3-5"),
		"allPages":                                t("Todas las páginas"),
		"invalidPageSelection":                    t("Selección de páginas inválida. Usa 1, 1,3-5 o all."),
		"xAxis":                                   t("X (0..1)"),
		"yAxis":                                   t("Y (0..1)"),
		"width":                                   t("Ancho (0..1)"),
		"height":                                  t("Alto (0..1)"),
		"signatureImage":                          t("Imagen de firma (opcional)"),
		"selectImage":                             t("Seleccionar imagen"),
		"removeImage":                             t("Quitar imagen"),
		"keepText":                                t("Mantener texto visible del sello"),
		"imageNotPersisted":                       t("La imagen seleccionada solo vive en esta sesión web local y no se guarda como ajuste persistente."),
		"signatureQR":                             t("QR del sello"),
		"signatureQRHelp":                         t("Opcional. Si indicas un texto o URL, se incrusta un QR dentro del sello visible y puede combinarse con imagen personalizada."),
		"signatureQRPlaceholder":                  t("https://verifica.ejemplo/"),
		"metadataTitle":                           t("Metadatos de la firma"),
		"metadataHelp":                            t("Opcional. Estos datos se incrustan en la firma PAdES y no sustituyen al certificado ni al sello visible."),
		"metadataSummaryTitle":                    t("Resumen de metadatos"),
		"metadataSummaryEmpty":                    t("Sin metadatos adicionales configurados."),
		"metadataSummaryConfigured":               t("Campos configurados: "),
		"metadataApplicabilityTitle":              t("Aplicabilidad de metadatos"),
		"metadataApplicabilityEmpty":              t("Sin metadatos aplicables configurados."),
		"metadataApplicabilityPAdES":              t("Los metadatos configurados se aplicarán en PAdES."),
		"metadataApplicabilityOther":              t("Los metadatos configurados solo se aplican cuando el formato es PAdES."),
		"metadataDeliveryTitle":                   t("Entrega efectiva"),
		"metadataDeliveryEmpty":                   t("Sin metadatos o QR adicionales para incorporar."),
		"metadataDeliveryPAdESOnly":               t("Los metadatos y el QR solo se incorporarán cuando el resultado sea PAdES."),
		"metadataDeliveryTextReady":               t("Motivo, ubicación y contacto se incrustarán como metadatos PAdES."),
		"metadataDeliveryQRReady":                 t("El QR se incorporará dentro del sello visible."),
		"metadataDeliveryQRNeedsSeal":             t("El QR requiere sello visible activo sobre PDF."),
		"signatureReason":                         t("Motivo"),
		"signatureLocation":                       t("Ubicación"),
		"signatureContact":                        t("Contacto"),
		"signatureReasonPlaceholder":              t("Firma electrónica avanzada"),
		"signatureLocationPlaceholder":            t("Granada"),
		"signatureContactPlaceholder":             t("correo@ejemplo.es"),
		"facturaeAdvancedTitle":                   t("FacturaE avanzada"),
		"facturaeAdvancedHelp":                    t("Opcional. Estos campos solo se aplican cuando el formato efectivo es FacturaE."),
		"facturaePolicyVersion":                   t("Versión de política FacturaE"),
		"facturaeSignerRole":                      t("Rol FacturaE"),
		"facturaeSignerRolePlaceholder":           t("emisor"),
		"facturaePolicyIdentifier":                t("Identificador de política"),
		"facturaePolicyIdentifierPlaceholder":     t("urn:oid:..."),
		"facturaePolicyIdentifierHash":            t("Hash de política"),
		"facturaePolicyIdentifierHashPlaceholder": t("Base64 del digest de la política"),
		"facturaePolicyQualifier":                 t("Qualifier de política"),
		"facturaePolicyQualifierPlaceholder":      t("https://www.facturae.gob.es/politica.html"),
		"facturaeSignatureCity":                   t("Ciudad de firma"),
		"facturaeSignatureCityPlaceholder":        t("Granada"),
		"facturaeSignatureProvince":               t("Provincia de firma"),
		"facturaeSignatureProvincePlaceholder":    t("Granada"),
		"facturaeSignaturePostalCode":             t("Código postal de firma"),
		"facturaeSignaturePostalCodePlaceholder":  t("18001"),
		"facturaeSignatureCountry":                t("País de firma"),
		"facturaeSignatureCountryPlaceholder":     t("ES"),
		"preview":                                 t("Previsualización"),
		"previewLegend":                           t("Puedes arrastrar y redimensionar el sello. El backend recibirá coordenadas seguras y normalizadas."),
		"signaturePreviewTitle":                   t("FIRMA DIGITAL"),
		"signaturePreviewSubtitle":                t("Muestra de sello visible"),
		"results":                                 t("Resultado"),
		"quickVerifyTitle":                        t("Validación rápida"),
		"quickVerifyHelp":                         t("Comprueba una firma ya existente desde fichero cargado o ruta local usando el mismo backend de verificación avanzada."),
		"quickVerifyPickFile":                     t("Seleccionar artefacto firmado"),
		"quickVerifyPickOriginal":                 t("Añadir original de referencia"),
		"quickVerifyClear":                        t("Quitar verificación"),
		"quickVerifyPath":                         t("Ruta local a verificar"),
		"quickVerifyPathPlaceholder":              t("/ruta/al/documento_firmado.pdf"),
		"quickVerifyOriginalPath":                 t("Ruta local del original"),
		"quickVerifyOriginalPathPlaceholder":      t("/ruta/al/original.pdf"),
		"quickVerifySource":                       t("Artefacto a verificar"),
		"quickVerifyOriginal":                     t("Original de referencia"),
		"quickVerifyOriginalOptional":             t("Sin original de referencia. La validación se hará solo sobre el artefacto firmado."),
		"quickVerifyNeedArtifact":                 t("Selecciona primero un artefacto firmado o indica una ruta local."),
		"quickVerifyPlanTitle":                    t("Plan de verificación rápida"),
		"quickVerifyArtifactOnly":                 t("Se verificará el artefacto firmado con validación avanzada y sin original de referencia."),
		"quickVerifyPlanWithOriginal":             t("con original de referencia"),
		"quickVerifyPairTitle":                    t("Emparejamiento"),
		"quickVerifyPairReady":                    t("Artefacto firmado y original preparados para validación avanzada."),
		"quickVerifyPairArtifactOnly":             t("No hay original de referencia: se validará solo el artefacto firmado."),
		"quickVerifyDetectedKind":                 t("Tipo detectado:"),
		"quickVerifyResultTitle":                  t("Resultado rápido"),
		"quickVerifyResultPending":                t("Aún no se ha ejecutado una validación rápida en esta sesión."),
		"quickVerifyExportPending":                t("La exportación JSON de verificación estará disponible tras ejecutar una verificación rápida o una autoverificación posterior a la firma."),
		"quickVerifyExportReady":                  t("Descargar informe JSON de verificación"),
		"quickVerifyCopySummaryReady":             t("Copiar resumen de validación"),
		"quickVerifyCopySummaryDone":              t("Resumen de validación copiado al portapapeles."),
		"quickVerifyCopySummaryError":             t("No se pudo copiar el resumen de validación."),
		"quickVerifyCopyReady":                    t("Copiar JSON de verificación"),
		"quickVerifyCopyDone":                     t("JSON de verificación copiado al portapapeles."),
		"quickVerifyCopyError":                    t("No se pudo copiar el JSON de verificación."),
		"quickVerifyNow":                          t("Verificar ahora"),
		"supportSummaryTitle":                     t("Preparar incidencia"),
		"supportSummaryHelp":                      t("Resume el contexto actual para soporte sin obligarte a revisar el log técnico completo."),
		"diagnosticAppLocal":                      t("Aplicación en este equipo"),
		"diagnosticCertificateStore":              t("Certificado o dispositivo de firma"),
		"diagnosticNetworkProxy":                  t("Conexión, proxy o configuración del equipo"),
		"diagnosticLocalWebService":               t("Canal local entre navegador y aplicación"),
		"diagnosticRemoteService":                 t("Servicio externo"),
		"diagnosticGovernmentAFirma":              t("Plataforma pública @firma"),
		"diagnosticUnknown":                       t("Origen todavía no clasificado"),
		"supportExpertMode":                       t("Ir a experto"),
		"supportExpertHint":                       t("Si estás en modo experto, añade detalles técnicos útiles para soporte."),
		"supportPrepareIncident":                  t("Preparar incidencia"),
		"supportPrepareRunning":                   t("Preparando incidencia..."),
		"supportPrepareDone":                      t("Incidencia preparada para soporte."),
		"supportExportPending":                    t("La incidencia exportable se generará al preparar un informe para soporte."),
		"supportExportReady":                      t("Descarga o copia la incidencia preparada con el diagnóstico local y el estado visible actual."),
		"supportExportCopyDone":                   t("Incidencia preparada copiada al portapapeles."),
		"supportExportCopyError":                  t("No se pudo copiar la incidencia preparada."),
		"Copiar diagnóstico":                      t("Copiar diagnóstico"),
		"Descargar informe":                       t("Descargar informe"),
		"supportCopySummary":                      t("Copiar resumen para soporte"),
		"supportCopyDone":                         t("Resumen para soporte copiado al portapapeles."),
		"supportCopyError":                        t("No se pudo copiar el resumen para soporte."),
		"supportCopyUnavailable":                  t("El navegador no permite copiar el resumen automáticamente en este contexto."),
		"supportFilesLoaded":                      t("Ficheros cargados"),
		"supportSuggestedStep":                    t("Siguiente paso sugerido"),
		"supportVisibleResult":                    t("Resultado visible"),
		"quickVerifyRunning":                      t("Verificando artefacto firmado..."),
		"resultArtifactTitle":                     t("Último artefacto firmado"),
		"resultArtifactPending":                   t("Aún no se ha generado ningún artefacto firmado en esta sesión."),
		"resultArtifactSingle":                    t("Resultado individual"),
		"resultArtifactBatch":                     t("Resultado por lote"),
		"resultArtifactDelivery":                  t("Entrega efectiva"),
		"resultArtifactDownloadReady":             t("descarga disponible"),
		"resultArtifactMemoryOnly":                t("solo memoria"),
		"resultArtifactStored":                    t("guardado en disco"),
		"resultArtifactStoredAndDownload":         t("disco + descarga"),
		"resultArtifactBatchDownloads":            t("Elementos descargables"),
		"signing":                                 t("Procesando firma..."),
		"signSuccess":                             t("Firma completada correctamente."),
		"batchSignSuccess":                        t("Lote firmado correctamente."),
		"batchSignPartial":                        t("Lote completado con incidencias."),
		"batchResultFile":                         t("Fichero"),
		"batchResultStatus":                       t("Estado"),
		"batchResultTotal":                        t("Total"),
		"batchResultOk":                           t("OK"),
		"batchResultKo":                           t("KO"),
		"batchResultError":                        t("ERROR"),
		"memoryOutput":                            t("en memoria"),
		"downloadSigned":                          t("Descargar firmado"),
		"downloadSignedBatch":                     t("Descargar"),
		"verifyAfterSign":                         t("Verificación automática"),
		"verificationValid":                       t("Firma válida."),
		"verificationInvalid":                     t("Firma no válida."),
		"verificationReason":                      t("Razón"),
		"formatConfigFacturaEHash":                t("formatConfigFacturaEHash"),
		"formatConfigFacturaEIdentifier":          t("formatConfigFacturaEIdentifier"),
		"formatConfigFacturaENone":                t("formatConfigFacturaENone"),
		"formatConfigFacturaEPlace":               t("formatConfigFacturaEPlace"),
		"formatConfigFacturaEPolicy":              t("formatConfigFacturaEPolicy"),
		"formatConfigFacturaEQualifier":           t("qualifier"),
		"formatConfigFacturaERole":                t("formatConfigFacturaERole"),
		"formatConfigNone":                        t("formatConfigNone"),
		"formatConfigPadesSubFilter":              t("formatConfigPadesSubFilter"),
		"formatConfigProfile":                     t("formatConfigProfile"),
		"formatConfigProfileNone":                 t("formatConfigProfileNone"),
		"formatConfigTitle":                       t("formatConfigTitle"),
		"formatConfigTsa":                         t("formatConfigTsa"),
		"verificationSigners":                     t("Firmantes"),
		"verificationDetails":                     t("Detalles"),
		"verificationFormat":                      t("Formato"),
		"verificationAlgorithm":                   t("Algoritmo"),
		"verificationCoverage":                    t("Cobertura"),
		"verificationIntegrity":                   t("Integridad"),
		"verificationCertificate":                 t("Certificado"),
		"verificationTrust":                       t("Confianza"),
		"verificationWarnings":                    t("Advertencias"),
		"verificationErrors":                      t("Errores"),
		"verificationEvidence":                    t("Evidencias"),
		"verificationSignerSummary":               t("Firmantes resumidos"),
		"verificationType":                        t("Tipo"),
		"verificationStatus":                      t("Estado"),
		"verificationPosture":                     t("Resumen de seguridad"),
		"verificationPostureTrusted":              t("Firma íntegra y confiable."),
		"verificationPostureWarning":              t("Firma íntegra, pero con advertencias de confianza o certificado."),
		"verificationPostureInvalid":              t("Firma inválida o integridad no garantizada."),
		"verificationUnknown":                     t("Desconocido"),
		"verificationWarning":                     t("Con advertencias"),
		"verificationStatusValid":                 t("Válida"),
		"verificationStatusInvalid":               t("Inválida"),
		"verificationStatusWarning":               t("Con advertencias"),
		"verificationStatusUnknown":               t("Desconocido"),
		"notAvailable":                            t("No disponible"),
		"needFile":                                t("Selecciona primero uno o varios ficheros."),
		"needCertificate":                         t("Selecciona primero un certificado."),
		"sealOnlyForPDF":                          t("La firma visible solo se aplica a PAdES (PDF)."),
		"noCertificates":                          t("No hay certificados disponibles."),
		"loadingCertificates":                     t("Cargando certificados..."),
		"checking":                                t("Comprobando..."),
		"connected":                               t("Conectado"),
		"disconnected":                            t("Desconectado"),
		"selectedFile":                            t("Fichero(s) seleccionado(s)"),
		"selectedFilesCount":                      t("%d fichero(s)"),
		"selectedCertificate":                     t("Certificado seleccionado"),
		"certificateUnknown":                      t("Certificado"),
		"extensionDocumentLoaded":                 t("Documento detectado por la extensión y cargado en el firmador local."),
		"extensionDocumentError":                  t("No se pudo cargar automáticamente el documento detectado por la extensión."),
	}
}
