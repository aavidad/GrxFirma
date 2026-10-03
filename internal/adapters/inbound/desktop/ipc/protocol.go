// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"errors"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"

	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

const (
	desktopIPCProtocolV1 = "desktop-ipc-v1"

	ipcPhaseAdmission = "admission"
	ipcPhaseProtocol  = "protocol"
	ipcPhaseOperation = "operation"

	ipcOutcomeSuccess = "success"
	ipcOutcomeFailure = "failure"
	ipcOutcomePartial = "partial"
)

var (
	errInvalidDesktopIPCRequest      = errors.New("peticion IPC versionada incompleta")
	errUnsupportedDesktopIPCProtocol = errors.New("protocolo IPC no soportado")
)

type resultadoIPCHello struct {
	Protocol           string         `json:"protocol"`
	Version            string         `json:"version"`
	Build              buildIPCHello  `json:"build"`
	Capabilities       []string       `json:"capabilities"`
	Actions            []string       `json:"actions"`
	Limits             limitsIPCHello `json:"limits"`
	IdleTimeoutSeconds int64          `json:"idleTimeoutSeconds"`
}

type buildIPCHello struct {
	Version string `json:"version"`
	Target  string `json:"target"`
}

type limitsIPCHello struct {
	MaxRequestBytes             int   `json:"maxRequestBytes"`
	MaxConcurrentConnections    int   `json:"maxConcurrentConnections"`
	DefaultTimeoutSeconds       int64 `json:"defaultTimeoutSeconds"`
	LongOperationTimeoutSeconds int64 `json:"longOperationTimeoutSeconds"`
}

func validateDesktopIPCProtocol(p peticion) error {
	if !p.protocolPresent {
		return nil
	}
	if strings.TrimSpace(p.Protocol) == "" {
		return errInvalidDesktopIPCRequest
	}
	if p.Protocol != desktopIPCProtocolV1 {
		return errUnsupportedDesktopIPCProtocol
	}
	if !p.requestIDPresent || !p.traceIDPresent || !p.paramsPresent ||
		strings.TrimSpace(p.RequestID) == "" ||
		strings.TrimSpace(p.TraceID) == "" ||
		p.paramsNull {
		return errInvalidDesktopIPCRequest
	}
	return nil
}

func desktopIPCHello() resultadoIPCHello {
	return desktopIPCHelloWithManagedTrust(
		localtlstrust.ManagedTrustLifecycleSupported(),
	)
}

func desktopIPCHelloWithManagedTrust(managedTrustSupported bool) resultadoIPCHello {
	actions := []string{
		"certificate_access_options",
		"certificate_export_public",
		"certificates",
		"check_certificates",
		"check_updates",
		"clear_temporary_certificates",
		"clock_diagnostics",
		"diagnose_token_settings",
		"export_diagnostic",
		"facturae_create",
		"validate_invoice",
		"generate_eni_document",
		"generate_eni_file",
		"get_settings",
		"get_token_settings",
		"getcertificates",
		"hash_check",
		"hash_create",
		"hello",
		"import_certificate",
		"import_certificate_to_store",
		"local_tls_startup_status",
		"open_certificate_manager",
		"pdf_preview",
		"ping",
		"protect",
		"protect_sign",
		"protection_recipient_import",
		"protection_recipient_remove",
		"protection_recipients",
		"proxy_secret_delete",
		"proxy_secret_store",
		"proxy_secret_store_status",
		"remove_temporary_certificate",
		"save_settings",
		"save_token_settings",
		"seal_preview",
		"sign",
		"sign_batch",
		"sign_multicosign",
		"tls_diagnostics",
		"unprotect",
		"use_temporary_certificate",
		"validate_certificate_online",
		"verify",
	}
	if managedTrustSupported {
		actions = append(actions,
			"clear_tls_trust",
			"install_public_roots",
		)
	}
	if smartcardAvailable {
		actions = append(actions, "smartcard_status")
	}
	if runtime.GOOS == "linux" {
		actions = append(actions,
			"service_install",
			"service_start",
			"service_status",
			"service_stop",
			"service_uninstall",
		)
	}
	sort.Strings(actions)

	return resultadoIPCHello{
		Protocol: desktopIPCProtocolV1,
		Version:  "1.0",
		Build: buildIPCHello{
			Version: desktopIPCBuildVersion(),
			Target:  runtime.GOOS + "/" + runtime.GOARCH,
		},
		Capabilities: []string{
			"certificate-id-selection",
			"correlation",
			"guided-diagnostic-steps",
			"stable-result-metadata",
		},
		Actions: actions,
		Limits: limitsIPCHello{
			MaxRequestBytes:             maxBytesLinea,
			MaxConcurrentConnections:    maxConexionesSimultaneas,
			DefaultTimeoutSeconds:       int64(defaultIPCOperationTimeout.Seconds()),
			LongOperationTimeoutSeconds: int64(longIPCOperationTimeout.Seconds()),
		},
		IdleTimeoutSeconds: int64(timeoutInactividad.Seconds()),
	}
}

func desktopIPCBuildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	version := strings.TrimSpace(info.Main.Version)
	if version == "" || version == "(devel)" {
		return "devel"
	}
	return version
}

func normalizeIPCResponse(resp respuesta) respuesta {
	if strings.TrimSpace(resp.Phase) == "" {
		resp.Phase = ipcPhaseOperation
	}
	if strings.TrimSpace(resp.Outcome) == "" {
		switch {
		case !resp.OK:
			resp.Outcome = ipcOutcomeFailure
		case batchResponseHasFailures(resp.Data):
			resp.Outcome = ipcOutcomePartial
		default:
			resp.Outcome = ipcOutcomeSuccess
		}
	}
	if resp.Outcome == ipcOutcomePartial && strings.TrimSpace(resp.ErrorCode) == "" {
		resp.ErrorCode = "partial_failure"
	}
	if !resp.OK && strings.TrimSpace(resp.ErrorCode) == "" {
		resp.ErrorCode = "operation_failed"
	}

	resp = enrichIPCErrorDiagnostic(resp)
	if resp.Outcome == ipcOutcomePartial && resp.Diagnostic == nil {
		if message := firstBatchFailureMessage(resp.Data); message != "" {
			resp.Diagnostic = guidedDiagnosticResult(resp.Action, message)
		}
	}
	if !resp.OK && resp.ErrorCode == "operation_failed" &&
		resp.Diagnostic != nil && strings.TrimSpace(resp.Diagnostic.FailureCode) != "" {
		resp.ErrorCode = resp.Diagnostic.FailureCode
	}
	if resp.Diagnostic != nil && len(resp.Diagnostic.Steps) == 0 {
		resp.Diagnostic.Steps = observedDiagnosticSteps(resp)
	}
	return resp
}

func batchResponseHasFailures(data any) bool {
	switch value := data.(type) {
	case resultadoFirmaLote:
		return value.FailCount > 0
	case *resultadoFirmaLote:
		return value != nil && value.FailCount > 0
	default:
		return false
	}
}

func firstBatchFailureMessage(data any) string {
	var result resultadoFirmaLote
	switch value := data.(type) {
	case resultadoFirmaLote:
		result = value
	case *resultadoFirmaLote:
		if value == nil {
			return ""
		}
		result = *value
	default:
		return ""
	}
	for _, item := range result.Results {
		if message := strings.TrimSpace(item.Error); message != "" {
			return message
		}
	}
	return ""
}

func observedDiagnosticSteps(resp respuesta) []pasoDiagnosticoGuiado {
	if resp.Diagnostic == nil {
		return nil
	}
	finalStatus := "unknown"
	switch resp.Outcome {
	case ipcOutcomeFailure, ipcOutcomePartial:
		finalStatus = "failure"
	case ipcOutcomeSuccess:
		finalStatus = "success"
	}
	code := strings.TrimSpace(resp.ErrorCode)
	if code == "" {
		code = "observed_result"
	}
	phase := strings.TrimSpace(resp.Phase)
	if phase == "" {
		phase = ipcPhaseOperation
	}

	finalStep := pasoDiagnosticoGuiado{
		Code:            code,
		Label:           diagnosticPhaseLabel(phase),
		Status:          finalStatus,
		Owner:           resp.Diagnostic.LikelyOwner,
		UserMessage:     resp.Diagnostic.UserMessage,
		SuggestedAction: resp.Diagnostic.SuggestedAction,
	}
	switch phase {
	case ipcPhaseAdmission:
		finalStep.EvidenceRef = "phase:" + ipcPhaseAdmission
		return []pasoDiagnosticoGuiado{finalStep}
	case ipcPhaseProtocol:
		finalStep.EvidenceRef = "phase:" + ipcPhaseProtocol
		return []pasoDiagnosticoGuiado{
			observedCompletedDiagnosticStep(ipcPhaseAdmission),
			finalStep,
		}
	case ipcPhaseOperation:
		finalStep.EvidenceRef = "phase:" + ipcPhaseOperation
		return []pasoDiagnosticoGuiado{
			observedCompletedDiagnosticStep(ipcPhaseAdmission),
			observedCompletedDiagnosticStep(ipcPhaseProtocol),
			finalStep,
		}
	default:
		// Una fase desconocida no permite deducir qué fases anteriores se
		// alcanzaron. Se conserva únicamente el resultado realmente recibido.
		return []pasoDiagnosticoGuiado{finalStep}
	}
}

func observedCompletedDiagnosticStep(phase string) pasoDiagnosticoGuiado {
	code := "observed_success"
	message := "La fase se completó antes de alcanzar la fase siguiente."
	switch phase {
	case ipcPhaseAdmission:
		code = "admission_observed"
		message = "La petición fue admitida antes de validar el protocolo."
	case ipcPhaseProtocol:
		code = "protocol_observed"
		message = "El protocolo fue validado antes de ejecutar la operación."
	}
	return pasoDiagnosticoGuiado{
		Code:        code,
		Label:       diagnosticPhaseLabel(phase),
		Status:      "success",
		UserMessage: message,
		EvidenceRef: "phase:" + phase,
	}
}

func diagnosticPhaseLabel(phase string) string {
	switch phase {
	case ipcPhaseAdmission:
		return "Admisión de la petición"
	case ipcPhaseProtocol:
		return "Validación del protocolo"
	case ipcPhaseOperation:
		return "Ejecución de la operación"
	default:
		return "Resultado observado"
	}
}

func ipcErrorResponse(action, errorCode, phase, message string, retryable bool) respuesta {
	return normalizeIPCResponse(respuesta{
		OK:        false,
		Action:    action,
		ErrorCode: errorCode,
		Phase:     phase,
		Retryable: retryable,
		Outcome:   ipcOutcomeFailure,
		Error:     message,
	})
}
