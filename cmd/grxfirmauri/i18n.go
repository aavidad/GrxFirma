// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"fmt"
	"grxfirma/internal/appdirs"
	"os"
	"strings"
	"sync"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
)

var protocolI18n struct {
	mu  sync.RWMutex
	loc *localizador.Localizador
}

const legacyBrowserLocalAccessHelpID = "Si usas Firefox y la sede no conecta, revisa en los permisos del sitio el acceso a aplicaciones del mismo equipo (loopback) y, solo si la sede lo solicita, a dispositivos de la red local. Si acabas de instalar o renovar la confianza TLS local, cierra por completo el navegador y vuelve a abrirlo. No aceptes excepciones de certificado ni desactives la protección de red local."

var protocolFallbacks = map[string]map[string]string{
	"en": {
		"GRXFIRMA WEB": "GRXFIRMA WEB",
		"Abriendo el selector de fichero solicitado por la web":  "Opening the file picker requested by the website",
		"Arrancando GrxFirma...":                                 "Starting GrxFirma...",
		"GrxFirma está esperando la solicitud del portal.":       "GrxFirma is waiting for the portal request.",
		"GrxFirma está lista y esperando la conexión del portal": "GrxFirma is ready and waiting for the portal connection",
		"GrxFirma — WebSocket activo":                            "GrxFirma — Active WebSocket",
		"GrxFirma — Firma web":                                   "GrxFirma — Web signing",
		"GrxFirma — Solicitud web":                               "GrxFirma — Web request",
		"GrxFirma — error en solicitud web":                      "GrxFirma — Web request error",
		"Autorización de firma":                                  "Signing authorisation",
		"Aún no hay eventos en el log.":                          "There are no log events yet.",
		"Aceptar":                                                "Accept",
		"Activación del WebSocket local":                         "Local WebSocket activation",
		"GrxFirma":                                               "GrxFirma",
		"GrxFirma — incidencia protocolaria":                     "GrxFirma — protocol incident",
		"Cancelar solicitud web":                                 "Cancel web request",
		"Cargando certificados...":                               "Loading certificates...",
		"Cargando configuración...":                              "Loading configuration...",
		"Cargando el fichero solicitado por la web":              "Loading the file requested by the website",
		"Canal local: ":                                          "Local channel: ",
		"Cerrar":                                                 "Close",
		"Certificado enviado":                                    "Certificate sent",
		"El certificado se ha entregado a la web. El resultado final lo confirma el portal.":      "The certificate has been sent to the website. The portal confirms the final result.",
		"La firma se ha generado y entregado a la web. El resultado final lo confirma el portal.": "The signature has been generated and sent to the website. The portal confirms the final result.",
		"Firma completada":                                                 "Signing complete",
		"La web ha enviado una nueva operación.":                           "The website has sent a new operation.",
		"Consulte el portal en el navegador.":                              "Check the portal in your browser.",
		"Firma entregada al portal: consulte el resultado en la web":       "Signature delivered to the portal: check the result on the website",
		"Certificado entregado al portal: consulte el resultado en la web": "Certificate delivered to the portal: check the result on the website",
		"Tras entregar la firma o el certificado, GrxFirma se ocultará y podrá llegar otra operación del portal.": "After delivery, GrxFirma will hide and the portal may send another operation.",
		"Guardar firma":                                 "Save signature",
		"error: ":                                       "error: ",
		"grxfirma-afirmauri %s":                         "grxfirma-afirmauri %s",
		"falta la URI afirma:// como argumento":         "missing afirma:// URI argument",
		"falta valor para %s":                           "missing value for %s",
		"modo de servidor no soportado: %s":             "unsupported server mode: %s",
		"guardado legacy cancelado":                     "legacy save cancelled",
		"carga legacy cancelada":                        "legacy load cancelled",
		"Certificado preparado":                         "Certificate ready",
		"Conectado con la web":                          "Connected to the portal",
		"Confirmación de firma":                         "Signing confirmation",
		"Componente: ":                                  "Component: ",
		"Diagnóstico experto: ":                         "Expert diagnosis: ",
		"Diagnóstico usuario: ":                         "User diagnosis: ",
		"Detener servidor":                              "Stop server",
		"Error original: ":                              "Original error: ",
		"Fase actual: ":                                 "Current phase: ",
		"Fecha: ":                                       "Date: ",
		"Configurando el canal de compatibilidad":       "Configuring the compatibility channel",
		"Configurando el canal seguro con el navegador": "Configuring the secure channel with the browser",
		"Cuando llegue la petición, se mostrará el selector de documento o de certificado como en V1.": "When the request arrives, the document or certificate picker will be shown as in V1.",
		"Ejecutando operación...":                                            "Executing operation...",
		"El navegador ya está comunicándose con GrxFirma":                    "The browser is already communicating with GrxFirma",
		"Elige el certificado con el que quieres continuar para firmar":      "Choose the certificate you want to use to continue signing",
		"Error cargando certificados":                                        "Error loading certificates",
		"Esperando a la web...":                                              "Waiting for the portal...",
		"Generando el resultado para devolverlo a la web":                    "Generating the result to send back to the website",
		"El servicio permanecerá activo mientras esta ventana siga abierta.": "The service will remain active while this window stays open.",
		"Estado:":                                        "Status:",
		"Ficheros permitidos":                            "Allowed files",
		"Host origen: ":                                  "Origin host: ",
		"Incidencia persistida en ":                      "Incident saved at ",
		"Mostrar ventana":                                "Show window",
		"Modo: ":                                         "Mode: ",
		"Canal %s preparado":                             "Channel %s ready",
		"Canal legacy de servicio preparado":             "Legacy service channel ready",
		"Canal legacy WebSocket preparado":               "Legacy WebSocket channel ready",
		"Ocultar ventana":                                "Hide window",
		"No se ha capturado ninguna traza adicional.":    "No additional trace has been captured.",
		"Origen conectado: ":                             "Connected origin: ",
		"Operación recibida desde la web: ":              "Operation received from website: ",
		"Inicializando el flujo de firma web":            "Initialising the web signing flow",
		"Interpretando la petición recibida":             "Interpreting the received request",
		"La operación ha terminado correctamente":        "The operation finished successfully",
		"La selección de certificado se ha completado":   "Certificate selection has been completed",
		"La solicitud de firma ya está en curso":         "The signing request is already in progress",
		"La solicitud ya está preparada para procesarse": "The request is ready to be processed",
		"La ventana se cerrará automáticamente al terminar la sesión del portal.": "The window will close automatically when the portal session finishes.",
		"Mantener esta sesión web en segundo plano al cerrar":                     "Keep this web session running in the background when closing",
		"Mantener este servicio en segundo plano al cerrar":                       "Keep this service running in the background when closing",
		"No se han encontrado certificados compatibles":                           "No compatible certificates were found",
		"No se pudo leer el log en %s\n\n%s":                                      "Could not read the log at %s\n\n%s",
		"No se pudo preparar el catálogo de certificados":                         "The certificate catalog could not be prepared",
		"No se seleccionó ningún certificado":                                     "No certificate was selected",
		"Ocultar a bandeja":                                                       "Hide to tray",
		"Operación completada":                                                    "Operation completed",
		"Preparando canal local...":                                               "Preparing local channel...",
		"Preparando descarga...":                                                  "Preparing download...",
		"Preparando documento...":                                                 "Preparing document...",
		"Preparando el selector de certificados":                                  "Preparing the certificate picker",
		"Preparando preferencias y entorno local":                                 "Preparing local preferences and environment",
		"Preparando servicio local...":                                            "Preparing local service...",
		"Procesando firma...":                                                     "Processing signature...",
		"Procesando solicitud...":                                                 "Processing request...",
		"Responsabilidad probable: ":                                              "Likely responsibility: ",
		"Protocolo: %s":                                                           "Protocol: %s",
		"Siguiente acción sugerida: ":                                             "Suggested next action: ",
		"SO: ":                                                                    "OS: ",
		"Seleccionar fichero":                                                     "Select file",
		"Tiempo total hasta el fallo: %d ms":                                      "Total time until failure: %d ms",
		"Arranque hasta handshake: %d ms":                                         "Startup to handshake: %d ms",
		"Handshake hasta primer mensaje: %d ms":                                   "Handshake to first message: %d ms",
		"Carga de catálogo: %d ms":                                                "Catalog load: %d ms",
		"Selección de documento: %d ms":                                           "Document selection: %d ms",
		"Selección de certificado: %d ms":                                         "Certificate selection: %d ms",
		"SOLICITUD FALLIDA\n\nLa operación terminó con código %d.":                "REQUEST FAILED\n\nThe operation ended with code %d.",
		"Selecciona un certificado":                                               "Select a certificate",
		"Selección cancelada":                                                     "Selection cancelled",
		"Solicitud web de firma en curso":                                         "Web signing request in progress",
		"selector legacy de carga no disponible sin interfaz gráfica":             "legacy load picker not available without graphical interface",
		"SERVIDOR WEBSOCKET ACTIVO":                                               "ACTIVE WEBSOCKET SERVER",
		"Sesión %s activa en %s":                                                  "Session %s active on %s",
		"sin id":                                                                  "no id",
		"Operación: ":                                                             "Operation: ",
		"Origen: ":                                                                "Origin: ",
		"Tipo de origen: ":                                                        "Origin type: ",
		"Última fase completada: ":                                                "Last completed phase: ",
		"Últimos eventos:":                                                        "Latest events:",
		"Versión: ":                                                               "Version: ",
		"Código estable: ":                                                        "Stable code: ",
		"afirma:// service":                                                       "afirma:// service",
		"afirma:// directo":                                                       "afirma:// direct",
		"afirma:// websocket":                                                     "afirma:// websocket",
		"Dirección:":                                                              "Address:",
		"Si activas el modo residente, al cerrar la ventana se ocultará a la bandeja y la sesión seguirá viva.": "If you enable resident mode, closing the window will hide it to the tray and keep the session alive.",
		legacyBrowserLocalAccessHelpID:                          "If you use Firefox and the portal does not connect, check the site's permission to access applications on this device (loopback) and, only if requested by the portal, devices on the local network. If local TLS trust has just been installed or renewed, fully close and reopen the browser. Do not accept certificate exceptions or disable local-network protection.",
		"Sin certificados disponibles":                          "No certificates available",
		"Contraseña del archivo P12/PFX":                        "P12/PFX file password",
		"La contraseña es demasiado larga (máximo 4096 bytes).": "The password is too long (maximum 4096 bytes).",
		"El certificado se usará durante esta sesión de firma. No se instalará ni se guardará la contraseña.": "The certificate will be used during this signing session. It will not be installed and the password will not be saved.",
		"Continuar":                     "Continue",
		"TRAZA DE LA OPERACIÓN":         "OPERATION TRACE",
		"TRAZA DEL SISTEMA (redactada)": "SYSTEM TRACE (redacted)",
		"Cierra esta ventana o pulsa `Detener servidor` para desactivarlo.": "Close this window or click `Stop server` to deactivate it.",

		graphicsFailureTitleID:       "GrxFirma — graphical interface unavailable",
		graphicsUnavailableMessageID: "The GrxFirma graphical interface cannot be opened in this session.",
		graphicsRemediationMessageID: "Check that the graphical session is active. If the diagnosis mentions OpenGL, install or enable the manufacturer's graphics driver, or use a session with 3D acceleration. The operation was not signed.",
		graphicsDiagnosticMessageID:  "Diagnosis: %s",
	},
}

func setProtocolLocalizer(configDir string) {
	loc := resolveProtocolLocalizer(configDir)
	protocolI18n.mu.Lock()
	defer protocolI18n.mu.Unlock()
	protocolI18n.loc = loc
}

func resolveProtocolLocalizer(configDir string) *localizador.Localizador {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = appdirs.Config(home)
	}
	doc, err := usersettings.CargarDocumentoCompat(context.Background(), configDir)
	if err == nil && doc.General.Idioma != nil && strings.TrimSpace(*doc.General.Idioma) != "" {
		return localizador.Para(*doc.General.Idioma)
	}
	return localizador.Detectar()
}

func tl(id string, args ...any) string {
	protocolI18n.mu.RLock()
	loc := protocolI18n.loc
	protocolI18n.mu.RUnlock()
	if loc == nil {
		loc = localizador.Detectar()
	}
	msg := loc.T(id, args...)
	if msg == id {
		if fallback := protocolFallbackText(loc.Locale(), id); fallback != "" {
			if len(args) == 0 {
				return fallback
			}
			return fmt.Sprintf(fallback, args...)
		}
		// Sin traducción, el identificador es el propio texto en español:
		// hay que rellenar igualmente sus marcadores.
		if len(args) > 0 {
			return fmt.Sprintf(id, args...)
		}
	}
	return msg
}

func protocolFallbackText(locale, id string) string {
	base := strings.TrimSpace(strings.ToLower(locale))
	if base == "" {
		base = "es"
	}
	if items, ok := protocolFallbacks[base]; ok {
		return items[id]
	}
	return ""
}
