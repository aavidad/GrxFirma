// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"regexp"
	"strings"
)

type GuidedDiagnosticCategory string

const (
	GuidedDiagnosticAppLocal         GuidedDiagnosticCategory = "app_local"
	GuidedDiagnosticCertificateStore GuidedDiagnosticCategory = "certificate_store"
	GuidedDiagnosticNetworkProxy     GuidedDiagnosticCategory = "network_proxy"
	GuidedDiagnosticLocalWebService  GuidedDiagnosticCategory = "local_web_service"
	GuidedDiagnosticRemoteService    GuidedDiagnosticCategory = "remote_service"
	GuidedDiagnosticGovernmentAFirma GuidedDiagnosticCategory = "government_afirma"
	GuidedDiagnosticUnknown          GuidedDiagnosticCategory = "unknown"
)

type GuidedDiagnostic struct {
	Category               GuidedDiagnosticCategory `json:"category"`
	FailureCode            string                   `json:"failureCode,omitempty"`
	UserMessage            string                   `json:"userMessage"`
	ExpertMessage          string                   `json:"expertMessage"`
	LikelyOwner            string                   `json:"likelyOwner"`
	ResponsibilityMessage  string                   `json:"responsibilityMessage"`
	SuggestedAction        string                   `json:"suggestedAction"`
	UserCanResolveDirectly bool                     `json:"userCanResolveDirectly"`
}

type guidedFailureCatalogEntry struct {
	category       GuidedDiagnosticCategory
	userMessage    string
	expertHint     string
	likelyOwner    string
	responsibility string
	suggested      string
	userCanResolve bool
}

var guidedFailureCatalog = map[string]guidedFailureCatalogEntry{
	"SAF_03": {
		category:       GuidedDiagnosticAppLocal,
		userMessage:    "La solicitud enviada por la web o por la aplicación local no se ha podido interpretar correctamente.",
		expertHint:     "SAF_03 suele indicar parámetros inválidos o petición incompleta en el borde legacy.",
		likelyOwner:    "app_local",
		responsibility: "Parece un problema de compatibilidad entre la solicitud recibida y la app local. Debemos revisarlo nosotros.",
		suggested:      "Reintenta la operación. Si se repite, exporta la incidencia para soporte.",
		userCanResolve: false,
	},
	"SAF_08": {
		category:       GuidedDiagnosticCertificateStore,
		userMessage:    "No se ha podido acceder correctamente al almacén de certificados.",
		expertHint:     "SAF_08 apunta al catálogo, almacén o dispositivo criptográfico.",
		likelyOwner:    "certificate_or_device",
		responsibility: "Parece un problema del certificado, del almacén o del dispositivo criptográfico de este equipo.",
		suggested:      "Comprueba que el certificado exista, que la tarjeta o lector esté conectado y que el almacén esté accesible.",
		userCanResolve: true,
	},
	"SAF_09": {
		category:       GuidedDiagnosticAppLocal,
		userMessage:    "La web ha pedido una operación de firma que la aplicación no ha podido completar.",
		expertHint:     "SAF_09 es el contenedor legacy de errores de firma. Hay que mirar la causa exacta subyacente.",
		likelyOwner:    "app_local",
		responsibility: "Puede ser un problema de compatibilidad del formato pedido, del certificado o del motor local. Debemos revisar la causa concreta.",
		suggested:      "Repite la operación y revisa el detalle técnico o exporta la incidencia para soporte.",
		userCanResolve: false,
	},
	"SAF_20": {
		category:       GuidedDiagnosticAppLocal,
		userMessage:    "La operación de firma por lotes ha fallado en el proceso local.",
		expertHint:     "SAF_20 apunta al procesamiento local del lote, no al servicio remoto.",
		likelyOwner:    "app_local",
		responsibility: "Parece un problema del procesamiento local del lote. Debemos revisarlo nosotros.",
		suggested:      "Reintenta el lote. Si persiste, exporta la incidencia para soporte.",
		userCanResolve: false,
	},
	"SAF_25": {
		category:       GuidedDiagnosticAppLocal,
		userMessage:    "No se ha podido cargar el fichero solicitado.",
		expertHint:     "SAF_25 suele venir del selector o de la lectura del fichero pedido por la web.",
		likelyOwner:    "app_local",
		responsibility: "Puede ser un problema de acceso al fichero o del flujo local de selección/carga.",
		suggested:      "Vuelve a seleccionar el fichero y comprueba permisos y ruta.",
		userCanResolve: true,
	},
	"SAF_26": {
		category:       GuidedDiagnosticRemoteService,
		userMessage:    "Ha fallado la comunicación con el servicio remoto de firma.",
		expertHint:     "SAF_26 apunta a comunicación con servicio remoto o lote remoto.",
		likelyOwner:    "remote_service",
		responsibility: "Parece un problema del servicio remoto o del portal externo. No podemos corregirlo desde la app local.",
		suggested:      "Reintenta más tarde o contacta con la sede o portal responsable del trámite.",
		userCanResolve: false,
	},
	"SAF_27": {
		category:       GuidedDiagnosticRemoteService,
		userMessage:    "La firma por lotes ha fallado en el servicio remoto.",
		expertHint:     "SAF_27 apunta a error remoto en el proceso de firma por lotes.",
		likelyOwner:    "remote_service",
		responsibility: "Parece un problema del servicio remoto de firma por lotes.",
		suggested:      "Reintenta más tarde o contacta con el portal responsable del trámite.",
		userCanResolve: false,
	},
	"SAF_44": {
		category:       GuidedDiagnosticAppLocal,
		userMessage:    "No se ha podido guardar el resultado de la operación.",
		expertHint:     "SAF_44 suele indicar fallo observable de guardado final.",
		likelyOwner:    "app_local",
		responsibility: "Parece un problema local de guardado o de acceso al destino.",
		suggested:      "Comprueba permisos de escritura y espacio disponible, y vuelve a intentarlo.",
		userCanResolve: true,
	},
}

func BuildGuidedDiagnostic(action, message string) GuidedDiagnostic {
	raw := strings.TrimSpace(message)
	if isObsoleteTLSFailure(raw) {
		return GuidedDiagnostic{
			Category:    GuidedDiagnosticRemoteService,
			FailureCode: "TLS_LEGACY_UNSUPPORTED",
			UserMessage: "El portal o servicio remoto intenta usar TLS 1.0 o TLS 1.1, protocolos obsoletos e inseguros. GrxFirma ha rechazado la conexión.",
			ExpertMessage: "El servidor remoto no ha podido negociar TLS 1.2 " +
				"o superior.",
			LikelyOwner:            "remote_service",
			ResponsibilityMessage:  "La entidad responsable del portal debe actualizar la configuración TLS de su servidor. No se puede corregir desde la aplicación local.",
			SuggestedAction:        "Contacta con la sede responsable e indica que su servicio de firma debe admitir TLS 1.2 o superior.",
			UserCanResolveDirectly: false,
		}
	}
	if isLegacyDESPolicyFailure(raw) {
		return GuidedDiagnostic{
			Category:    GuidedDiagnosticAppLocal,
			FailureCode: "LEGACY_DES_DISABLED",
			UserMessage: "El portal usa el modo heredado de servidor intermedio de AutoFirma 1.x, " +
				"que cifra el intercambio con DES. Esta instalación lo tiene desactivado por seguridad.",
			ExpertMessage: "El intercambio por servidor intermedio exige DES (clave de 8 dígitos) y la " +
				"política de máquina permitir_des_legacy no está activa.",
			LikelyOwner: "administrator",
			ResponsibilityMessage: "No es un fallo del portal: es una decisión de seguridad de este equipo. " +
				"Solo un administrador puede habilitar la compatibilidad.",
			SuggestedAction: "Reintenta desde un navegador que use la conexión directa (WebSocket) o pide al " +
				"administrador que valore activar la política permitir_des_legacy.",
			UserCanResolveDirectly: false,
		}
	}
	code := extractGuidedDiagnosticFailureCode(raw)
	if entry, ok := guidedFailureCatalog[code]; ok {
		expert := entry.expertHint
		if code == "SAF_09" && containsAny(strings.ToLower(raw), "cms/pkcs#7", "pkcs#7", "pkcs7", "formato no soportado") {
			return GuidedDiagnostic{
				Category:               GuidedDiagnosticAppLocal,
				FailureCode:            code,
				UserMessage:            "La sede ha pedido un formato de firma que esta versión de la app no estaba interpretando correctamente.",
				ExpertMessage:          "SAF_09 por formato legacy no mapeado correctamente.",
				LikelyOwner:            "app_local",
				ResponsibilityMessage:  "Parece un problema de compatibilidad de formato entre la sede y la app local. Debemos revisarlo nosotros.",
				SuggestedAction:        "Actualiza o vuelve a intentar con la corrección del formato legacy. Si persiste, exporta la incidencia.",
				UserCanResolveDirectly: false,
			}
		}
		if entry.category == GuidedDiagnosticRemoteService &&
			containsGovernmentAFirmaSignal(strings.ToLower(raw)) {
			owner, responsibility, suggested, userCanResolve :=
				guidedDiagnosticGuidance(GuidedDiagnosticGovernmentAFirma)
			return GuidedDiagnostic{
				Category:               GuidedDiagnosticGovernmentAFirma,
				FailureCode:            code,
				UserMessage:            guidedDiagnosticUserMessage(GuidedDiagnosticGovernmentAFirma),
				ExpertMessage:          expert,
				LikelyOwner:            owner,
				ResponsibilityMessage:  responsibility,
				SuggestedAction:        suggested,
				UserCanResolveDirectly: userCanResolve,
			}
		}
		return GuidedDiagnostic{
			Category:               entry.category,
			FailureCode:            code,
			UserMessage:            entry.userMessage,
			ExpertMessage:          expert,
			LikelyOwner:            entry.likelyOwner,
			ResponsibilityMessage:  entry.responsibility,
			SuggestedAction:        entry.suggested,
			UserCanResolveDirectly: entry.userCanResolve,
		}
	}
	category := classifyGuidedDiagnostic(action, raw)
	owner, responsibility, suggested, userCanResolve := guidedDiagnosticGuidance(category)
	return GuidedDiagnostic{
		Category:               category,
		FailureCode:            code,
		UserMessage:            guidedDiagnosticUserMessage(category),
		ExpertMessage:          guidedDiagnosticExpertMessage(category, raw),
		LikelyOwner:            owner,
		ResponsibilityMessage:  responsibility,
		SuggestedAction:        suggested,
		UserCanResolveDirectly: userCanResolve,
	}
}

// isObsoleteTLSFailure reconoce únicamente errores de negociación que aparecen
// cuando el extremo remoto no admite el mínimo seguro TLS 1.2. No basta con
// encontrar la palabra "TLS": un timeout, un certificado no confiable o un
// fallo genérico de handshake tienen causas y responsables distintos.
func isObsoleteTLSFailure(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return containsAny(
		lower,
		"server selected unsupported protocol version",
		"protocol version not supported",
		"tls version 1.0",
		"tls version 1.1",
		"tls 1.0",
		"tls 1.1",
		"tls1.0",
		"tls1.1",
		"tlsv1.0",
		"tlsv1.1",
	)
}

var guidedDiagnosticFailureCodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bSAF[_-]?\d{2,3}\b`),
	regexp.MustCompile(`(?i)\bERR[-_ ]?\d{2,3}\b`),
	regexp.MustCompile(`(?i)\bHTTP[ _-]?(4\d{2}|5\d{2})\b`),
	regexp.MustCompile(`(?i)\b(4\d{2}|5\d{2})\b`),
}

func extractGuidedDiagnosticFailureCode(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	for i, re := range guidedDiagnosticFailureCodePatterns {
		match := re.FindString(text)
		if match == "" {
			continue
		}
		normalized := strings.ToUpper(strings.TrimSpace(match))
		switch i {
		case 0:
			normalized = strings.ReplaceAll(normalized, "-", "_")
		case 1:
			normalized = strings.ReplaceAll(normalized, "_", "-")
			normalized = strings.ReplaceAll(normalized, " ", "-")
		case 2:
			normalized = strings.ReplaceAll(normalized, " ", "_")
			normalized = strings.ReplaceAll(normalized, "-", "_")
		case 3:
			if len(normalized) == 3 && (strings.HasPrefix(normalized, "4") || strings.HasPrefix(normalized, "5")) {
				normalized = "HTTP_" + normalized
			}
		}
		return normalized
	}
	return ""
}

func classifyGuidedDiagnostic(action, message string) GuidedDiagnosticCategory {
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	haystack := strings.ToLower(strings.TrimSpace(normalizedAction + " " + message))
	switch {
	case normalizedAction == "websocket" || containsAny(
		haystack,
		"local network access",
		"loopback-network",
		"local-network",
		"loopback",
		"localhost",
		"127.0.0.1",
		"::1",
		"wss://",
		"websocket",
		"native messaging",
		"native host",
		"nativehost",
		"canal local",
		"servicio web local",
		"permiso de red local",
		"acceso a aplicaciones del mismo equipo",
	):
		// Estas señales proceden del control por sitio del navegador. Deben
		// evaluarse antes que palabras genéricas como "Firefox", "portal" o
		// "Autofirma", que de otro modo atribuirían el fallo al almacén de
		// certificados o al servicio remoto.
		return GuidedDiagnosticLocalWebService
	case containsGovernmentAFirmaSignal(haystack):
		// No se debe usar el token genérico "afirma": también aparece en
		// GrxFirma y convertiría fallos locales en falsos fallos remotos.
		return GuidedDiagnosticGovernmentAFirma
	case containsAny(haystack, "saf_26", "saf_27", "trif", "triphase", "rtservlet", "stservlet", "sede", "portal", "servicio remoto", "firma por lotes"):
		return GuidedDiagnosticRemoteService
	case containsAny(haystack, "saf_08", "saf_09", "cert", "pkcs", "pkcs11", "almac", "keystore", "alias", "clave", "p12", "pfx", "dnie", "smartcard", "tarjeta", "revoc", "ocsp", "crl", "nss", "certutil", "pk12util"):
		return GuidedDiagnosticCertificateStore
	case containsAny(haystack, "proxy", "tls", "ssl", "http", "https", "network", "red", "timeout", "connection refused", "lookup ", "dial tcp", "x509", "unknown authority", "refused", "sin conexión"):
		return GuidedDiagnosticNetworkProxy
	case containsAny(haystack, "ipc", "socket", "pipe", "backend desktop", "motor local", "json", "archivo", "fichero", "ruta", "permiso", "acceso denegado", "accion no soportada", "accion_no_soportada", "operacion no soportada", "verificador_no_configurado", "firmar_no_configurado", "catalogo_no_configurado", "servicio_no_configurado", "error.", "saf_03", "saf_05", "saf_20", "saf_25", "saf_44"):
		return GuidedDiagnosticAppLocal
	default:
		return GuidedDiagnosticUnknown
	}
}

func containsGovernmentAFirmaSignal(haystack string) bool {
	return containsAny(haystack, "@firma", "plataforma afirma", "government_afirma")
}

func guidedDiagnosticUserMessage(category GuidedDiagnosticCategory) string {
	switch category {
	case GuidedDiagnosticAppLocal:
		return "Posible problema en la aplicación local o en el servicio de escritorio."
	case GuidedDiagnosticCertificateStore:
		return "Posible problema con el certificado, el almacén o el dispositivo criptográfico."
	case GuidedDiagnosticNetworkProxy:
		return "Posible problema de red, proxy o conexión con el servicio local."
	case GuidedDiagnosticLocalWebService:
		return "Posible problema entre el navegador y el servicio local de firma."
	case GuidedDiagnosticRemoteService:
		return "Posible problema con el servicio remoto o el flujo externo de firma."
	case GuidedDiagnosticGovernmentAFirma:
		return "Posible problema con la plataforma pública @firma."
	default:
		return "No se ha podido clasificar automáticamente el origen del problema."
	}
}

func guidedDiagnosticExpertMessage(
	category GuidedDiagnosticCategory,
	_ string,
) string {
	label := "desconocido"
	switch category {
	case GuidedDiagnosticAppLocal:
		label = "app_local"
	case GuidedDiagnosticCertificateStore:
		label = "certificate_store"
	case GuidedDiagnosticNetworkProxy:
		label = "network_proxy"
	case GuidedDiagnosticLocalWebService:
		label = "local_web_service"
	case GuidedDiagnosticRemoteService:
		label = "remote_service"
	case GuidedDiagnosticGovernmentAFirma:
		label = "government_afirma"
	}
	// El detalle de transporte puede incluir rutas locales, nombres de
	// documentos o respuestas remotas. La respuesta IPC conserva únicamente
	// la clasificación y el código estable extraído; el texto crudo no forma
	// parte del diagnóstico visible/exportable.
	return "Clasificación automática: " + label
}

func guidedDiagnosticGuidance(category GuidedDiagnosticCategory) (owner, responsibility, suggested string, userCanResolve bool) {
	switch category {
	case GuidedDiagnosticAppLocal:
		return "app_local",
			"Parece un problema de la aplicación local o del servicio de escritorio. Debemos revisarlo nosotros.",
			"Reintenta la operación y, si persiste, exporta el diagnóstico para soporte técnico.",
			false
	case GuidedDiagnosticCertificateStore:
		return "certificate_or_device",
			"Parece un problema del certificado, del almacén o del dispositivo criptográfico de este equipo.",
			"Comprueba que el certificado siga vigente, que el lector o tarjeta esté conectado y que la clave pueda usarse para firmar.",
			true
	case GuidedDiagnosticNetworkProxy:
		return "environment",
			"Parece un problema de red, proxy o confianza TLS del equipo. Esta clasificación no demuestra un fallo del reloj.",
			"Revisa la conexión, la configuración del proxy y la confianza TLS. Comprueba la hora solo mediante la fase específica de reloj.",
			true
	case GuidedDiagnosticLocalWebService:
		return "local_web_service",
			"Parece un problema del canal local entre el navegador y la aplicación de firma de este equipo.",
			"Comprueba que la app esté abierta, permite el acceso local para esa sede y reinicia el navegador antes de reintentar.",
			true
	case GuidedDiagnosticRemoteService:
		return "remote_service",
			"Parece un problema del servicio remoto de firma o del portal externo. No podemos corregirlo desde la app local.",
			"Reintenta más tarde o contacta con la sede o portal remoto responsable del trámite.",
			false
	case GuidedDiagnosticGovernmentAFirma:
		return "government_afirma",
			"Parece un problema de la plataforma pública @firma o de su integración con la sede. No podemos corregirlo desde la app local.",
			"Reintenta más tarde o contacta con la sede responsable del trámite indicando que el fallo procede de @firma.",
			false
	default:
		return "unknown",
			"No se puede determinar automáticamente si el problema es local o remoto.",
			"Repite la operación y exporta el diagnóstico si vuelve a fallar.",
			true
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// isLegacyDESPolicyFailure reconoce el bloqueo local de DES para no culpar al
// portal remoto de una decisión de la política de máquina.
func isLegacyDESPolicyFailure(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "fallback des") || strings.Contains(lower, "permitir_des_legacy")
}
