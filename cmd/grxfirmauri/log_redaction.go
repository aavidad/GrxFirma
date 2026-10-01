// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"regexp"
	"strings"
)

var (
	reShellLogSessionID       = regexp.MustCompile(`(?i)((?:idsession|session_id|expected_session|got_session)=)([^&\s"']+)`)
	reShellLogRequestID       = regexp.MustCompile(`(?i)((?:request_id|certificate_id|sticky_id)=)([^\s"']+)`)
	reShellLogSensitiveIDJSON = regexp.MustCompile(`(?i)(["']?(?:idsession|session_id|expected_session|got_session|request_id|certificate_id|sticky_id)["']?\s*:\s*["']?)([^"'\s,}\\]+)`)
	reShellLogSensitiveParam  = regexp.MustCompile(`(?i)((?:^|[?&\s])(?:dat|data|payload|token|authorization|password|passphrase|certificate|signature|file|filename|filepath|key|api[_-]?key|cookie)=)([^&\s"']+)`)
	reShellLogSensitiveJSON   = regexp.MustCompile(`(?i)(["']?(?:dat|data|payload|token|authorization|password|passphrase|certificate|signature|file|filename|filepath|key|api[_-]?key|cookie)["']?\s*:\s*["']?)([^"'\s,}\\]+)`)
	reShellLogPrefix          = regexp.MustCompile(`(?i)((?:message|payload|uri|response|raw)_prefix=)"[^"]*"`)
	reShellAfirmaURI          = regexp.MustCompile(`(?i)afirma://[^\s"']+`)
	reShellHTTPURL            = regexp.MustCompile(`(?i)(?:https?|wss?)://[^\s"']+`)
	reShellPEMBlock           = regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]+-----.*?-----END [A-Z0-9 ]+-----`)
	reShellLongEncodedData    = regexp.MustCompile(`\b[A-Za-z0-9+/_-]{64,}={0,2}\b`)
	reShellWindowsPath        = regexp.MustCompile(`(?i)(^|[\s"'=])(?:[A-Z]:[\\/]|\\\\)[^\s"'<>:,;)}\]]+`)
	reShellUnixPath           = regexp.MustCompile(`(^|[\s"'=:])/(?:[^/\s"'<>]+/)*[^ \t\r\n"'<>:,;)}\]]+`)
	reShellDocumentName       = regexp.MustCompile(`(?i)\b[^\s"'<>/\\]+\.(?:pdf|xml|json|txt|csv|odt|ods|odp|docx?|xlsx?|pptx?|p7s|csig|xsig|zip|bin)\b`)
	reShellEmail              = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	reShellSpanishPersonalID  = regexp.MustCompile(`(?i)\b(?:[XYZ]\d{7}[A-Z]|\d{8}[A-Z])\b`)
)

func redactProtocolLogText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tl("Aún no hay eventos en el log.")
	}
	raw = reShellLogSessionID.ReplaceAllString(raw, `${1}[REDACTED_ID]`)
	raw = reShellLogRequestID.ReplaceAllString(raw, `${1}[REDACTED_ID]`)
	raw = reShellLogSensitiveIDJSON.ReplaceAllString(raw, `${1}[REDACTED_ID]`)
	raw = reShellPEMBlock.ReplaceAllString(raw, "[PEM_REDACTED]")
	raw = reShellLogSensitiveParam.ReplaceAllString(raw, `${1}[REDACTED]`)
	raw = reShellLogSensitiveJSON.ReplaceAllString(raw, `${1}[REDACTED]`)
	raw = reShellLogPrefix.ReplaceAllString(raw, `${1}"[REDACTED_PREFIX]"`)
	raw = reShellAfirmaURI.ReplaceAllStringFunc(raw, func(value string) string {
		return "[AFIRMA_REQUEST " + protocolRawURISummary(value) + "]"
	})
	raw = reShellHTTPURL.ReplaceAllStringFunc(raw, func(value string) string {
		endpoint := sanitizeEndpointForLog(value)
		if endpoint == "" {
			return "[ENDPOINT_REDACTED]"
		}
		return endpoint
	})
	raw = reShellWindowsPath.ReplaceAllString(raw, `${1}[LOCAL_PATH_REDACTED]`)
	raw = reShellUnixPath.ReplaceAllString(raw, `${1}[LOCAL_PATH_REDACTED]`)
	raw = reShellDocumentName.ReplaceAllString(raw, "[DOCUMENT_REDACTED]")
	raw = reShellLongEncodedData.ReplaceAllString(raw, "[ENCODED_DATA_REDACTED]")
	raw = reShellEmail.ReplaceAllString(raw, "[EMAIL_REDACTED]")
	raw = reShellSpanishPersonalID.ReplaceAllString(raw, "[PERSONAL_ID_REDACTED]")
	return protocolRedactPathPrefixes(raw)
}
