// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"strings"
	"testing"
)

func TestBuildGuidedDiagnostic_ClassificaAppLocal(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("sign", "SAF_03: Parametros incorrectos")
	if got.Category != GuidedDiagnosticAppLocal {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticAppLocal)
	}
	if got.FailureCode != "SAF_03" {
		t.Fatalf("failureCode=%q, want %q", got.FailureCode, "SAF_03")
	}
	if got.LikelyOwner != "app_local" || got.UserCanResolveDirectly {
		t.Fatalf("guided app_local inesperado: %#v", got)
	}
}

func TestBuildGuidedDiagnostic_ClassificaCertificado(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("verify", "error accediendo al almacén PKCS#11")
	if got.Category != GuidedDiagnosticCertificateStore {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticCertificateStore)
	}
	if got.LikelyOwner != "certificate_or_device" || !got.UserCanResolveDirectly {
		t.Fatalf("guided certificate_store inesperado: %#v", got)
	}
}

func TestBuildGuidedDiagnostic_ClassificaRedProxy(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("service_status", "proxy TLS timeout")
	if got.Category != GuidedDiagnosticNetworkProxy {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticNetworkProxy)
	}
	if got.LikelyOwner != "environment" || !got.UserCanResolveDirectly {
		t.Fatalf("guided network_proxy inesperado: %#v", got)
	}
}

func TestBuildGuidedDiagnostic_ExplicaTLSObsoletoComoFalloRemoto(t *testing.T) {
	t.Parallel()

	for _, message := range []string{
		"prefirma remota: tls: server selected unsupported protocol version 301",
		"SAF_26: remote error: tls: protocol version not supported",
		"el servicio externo solo admite TLS 1.0",
	} {
		got := BuildGuidedDiagnostic("sign_batch", message)
		if got.FailureCode != "TLS_LEGACY_UNSUPPORTED" {
			t.Fatalf("message=%q: failureCode=%q", message, got.FailureCode)
		}
		if got.Category != GuidedDiagnosticRemoteService ||
			got.LikelyOwner != "remote_service" ||
			got.UserCanResolveDirectly {
			t.Fatalf("message=%q: diagnóstico inesperado: %#v", message, got)
		}
		visible := strings.ToLower(strings.Join([]string{
			got.UserMessage,
			got.ResponsibilityMessage,
			got.SuggestedAction,
		}, " "))
		for _, fragment := range []string{
			"tls 1.0",
			"tls 1.1",
			"obsolet",
			"tls 1.2",
			"portal",
		} {
			if !strings.Contains(visible, fragment) {
				t.Fatalf("message=%q: falta %q en %#v", message, fragment, got)
			}
		}
	}
}

func TestBuildGuidedDiagnostic_NoConfundeOtrosFallosTLSConVersionObsoleta(t *testing.T) {
	t.Parallel()

	for _, message := range []string{
		"proxy TLS timeout",
		"x509: certificate signed by unknown authority",
		"tls: handshake failure",
	} {
		got := BuildGuidedDiagnostic("service_status", message)
		if got.FailureCode == "TLS_LEGACY_UNSUPPORTED" {
			t.Fatalf("message=%q se clasificó como versión TLS obsoleta", message)
		}
	}
}

func TestBuildGuidedDiagnostic_ClassificaCanalWebLocal(t *testing.T) {
	t.Parallel()

	for _, message := range []string{
		"Firefox Local Network Access denied for loopback-network",
		"GrxFirma: permiso de red local bloqueado por la sede",
		"fallo al conectar con wss://127.0.0.1:63119",
		"Native Messaging host no disponible",
	} {
		got := BuildGuidedDiagnostic("websocket", message)
		if got.Category != GuidedDiagnosticLocalWebService {
			t.Fatalf("message=%q: category=%q, want %q", message, got.Category, GuidedDiagnosticLocalWebService)
		}
		if got.LikelyOwner != "local_web_service" || !got.UserCanResolveDirectly {
			t.Fatalf("message=%q: guided local-network inesperado: %#v", message, got)
		}
	}
}

func TestBuildGuidedDiagnostic_ClassificaGovernmentAFirmaSinConfundirGrxFirma(t *testing.T) {
	t.Parallel()

	got := BuildGuidedDiagnostic("sign", "La plataforma @firma no está disponible")
	if got.Category != GuidedDiagnosticGovernmentAFirma {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticGovernmentAFirma)
	}
	if got.LikelyOwner != "government_afirma" || got.UserCanResolveDirectly {
		t.Fatalf("guided government_afirma inesperado: %#v", got)
	}

	got = BuildGuidedDiagnostic("sign", "GrxFirma devolvió un fallo no catalogado")
	if got.Category != GuidedDiagnosticUnknown {
		t.Fatalf("GrxFirma no debe implicar servicio remoto: category=%q", got.Category)
	}
}

func TestBuildGuidedDiagnostic_ClassificaServicioRemoto(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("sign_batch", "SAF_26: Error en la comunicación con el servicio de firma de lotes")
	if got.Category != GuidedDiagnosticRemoteService {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticRemoteService)
	}
	if got.LikelyOwner != "remote_service" || got.UserCanResolveDirectly {
		t.Fatalf("guided remote_service inesperado: %#v", got)
	}
}

func TestBuildGuidedDiagnostic_SeparaAFirmaDeCodigoRemotoGenerico(t *testing.T) {
	t.Parallel()

	got := BuildGuidedDiagnostic("sign_batch", "SAF_26: la plataforma @firma no responde")
	if got.Category != GuidedDiagnosticGovernmentAFirma {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticGovernmentAFirma)
	}
	if got.FailureCode != "SAF_26" || got.LikelyOwner != "government_afirma" {
		t.Fatalf("diagnóstico @firma inesperado: %#v", got)
	}
}

func TestBuildGuidedDiagnostic_Unknown(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("verify", "situacion no catalogada")
	if got.Category != GuidedDiagnosticUnknown {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticUnknown)
	}
	if got.LikelyOwner != "unknown" || !got.UserCanResolveDirectly {
		t.Fatalf("guided unknown inesperado: %#v", got)
	}
}

func TestBuildGuidedDiagnostic_ExtraeERRLegacy(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("verify", "ERR-06: El identificador para los datos es inválido")
	if got.FailureCode != "ERR-06" {
		t.Fatalf("failureCode=%q, want %q", got.FailureCode, "ERR-06")
	}
}

func TestBuildGuidedDiagnostic_ExtraeHTTPStatus(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("verify", "HTTP 503 del servicio remoto")
	if got.FailureCode != "HTTP_503" {
		t.Fatalf("failureCode=%q, want %q", got.FailureCode, "HTTP_503")
	}
}

func TestBuildGuidedDiagnostic_SAF09FormatoLegacy(t *testing.T) {
	t.Parallel()
	got := BuildGuidedDiagnostic("sign", "SAF_09: error durante la operacion de firma: formato no soportado: CMS/PKCS#7")
	if got.FailureCode != "SAF_09" {
		t.Fatalf("failureCode=%q, want SAF_09", got.FailureCode)
	}
	if got.Category != GuidedDiagnosticAppLocal {
		t.Fatalf("category=%q, want %q", got.Category, GuidedDiagnosticAppLocal)
	}
	if !containsAny(strings.ToLower(got.UserMessage), "formato", "sede") {
		t.Fatalf("userMessage inesperado: %q", got.UserMessage)
	}
}

func TestBuildGuidedDiagnostic_NoReflejaDetalleCrudoEnDiagnostico(
	t *testing.T,
) {
	t.Parallel()

	sensitivePath := `/home/persona/expedientes/nomina-julio.pdf`
	got := BuildGuidedDiagnostic(
		"sign_batch",
		"SAF_44: no se pudo guardar "+sensitivePath,
	)
	visible := strings.Join(
		[]string{
			got.UserMessage,
			got.ExpertMessage,
			got.ResponsibilityMessage,
			got.SuggestedAction,
		},
		" ",
	)
	if strings.Contains(visible, sensitivePath) ||
		strings.Contains(visible, "nomina-julio.pdf") ||
		strings.Contains(visible, "Mensaje original") {
		t.Fatalf("el diagnóstico refleja detalle sensible: %#v", got)
	}
	if got.FailureCode != "SAF_44" {
		t.Fatalf("failureCode=%q, want SAF_44", got.FailureCode)
	}
}

func TestBuildGuidedDiagnostic_DESBloqueadoNoCulpaAlPortal(t *testing.T) {
	msg := "SAF_26: el fallback DES del protocolo heredado esta desactivado; si el portal aun exige el formato de sesion de V1.9, un administrador debe habilitar la politica de maquina permitir_des_legacy"
	d := BuildGuidedDiagnostic("sign", msg)
	if d.FailureCode != "LEGACY_DES_DISABLED" || d.LikelyOwner != "administrator" {
		t.Fatalf("diagnóstico = %+v", d)
	}
	if strings.Contains(d.ResponsibilityMessage, "servicio remoto") {
		t.Fatalf("no debe atribuirse al servicio remoto: %q", d.ResponsibilityMessage)
	}
}
