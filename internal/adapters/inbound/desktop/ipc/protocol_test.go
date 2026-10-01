// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestPeticionProtocol_CompatibilidadLegacyYPresenciaEstricta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantPresent bool
		wantValue   string
		wantError   bool
	}{
		{
			name:        "legacy omitido",
			raw:         `{"action":"ping","params":{}}`,
			wantPresent: false,
		},
		{
			name:        "v1",
			raw:         `{"protocol":"desktop-ipc-v1","requestId":"req-1","traceId":"trace-1","action":"ping","params":{}}`,
			wantPresent: true,
			wantValue:   desktopIPCProtocolV1,
		},
		{
			name:        "vacío",
			raw:         `{"protocol":"","action":"ping","params":{}}`,
			wantPresent: true,
			wantError:   true,
		},
		{
			name:        "null",
			raw:         `{"protocol":null,"action":"ping","params":{}}`,
			wantPresent: true,
			wantError:   true,
		},
		{
			name:        "futuro",
			raw:         `{"protocol":"desktop-ipc-v2","action":"ping","params":{}}`,
			wantPresent: true,
			wantValue:   "desktop-ipc-v2",
			wantError:   true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var req peticion
			if err := json.Unmarshal([]byte(test.raw), &req); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if req.protocolPresent != test.wantPresent || req.Protocol != test.wantValue {
				t.Fatalf("presencia/valor = %t/%q; want %t/%q", req.protocolPresent, req.Protocol, test.wantPresent, test.wantValue)
			}
			if got := validateDesktopIPCProtocol(req); (got != nil) != test.wantError {
				t.Fatalf("validateDesktopIPCProtocol() error = %v; wantError=%t", got, test.wantError)
			}
		})
	}
}

func TestServidorServirConexion_HelloV1PublicaContrato(t *testing.T) {
	t.Parallel()

	srv := New(&Manejador{})
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	if _, err := clientConn.Write([]byte(`{"protocol":"desktop-ipc-v1","requestId":"req-hello","traceId":"trace-hello","action":"hello","params":{}}` + "\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var response respuesta
	if err := json.NewDecoder(bufio.NewReader(clientConn)).Decode(&response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !response.OK || response.Protocol != desktopIPCProtocolV1 ||
		response.ErrorCode != "" || response.Phase != ipcPhaseOperation ||
		response.Retryable || response.Outcome != ipcOutcomeSuccess {
		t.Fatalf("respuesta hello inesperada: %#v", response)
	}
	hello, ok := response.Data.(map[string]any)
	if !ok {
		t.Fatalf("data hello = %T; want map[string]any", response.Data)
	}
	if hello["protocol"] != desktopIPCProtocolV1 || hello["version"] != "1.0" {
		t.Fatalf("versión hello inesperada: %#v", hello)
	}
	actions, ok := hello["actions"].([]any)
	if !ok || len(actions) == 0 {
		t.Fatalf("actions hello inesperadas: %#v", hello["actions"])
	}
	if !sliceAnyStringContains(actions, "hello") || !sliceAnyStringContains(actions, "protect_sign") {
		t.Fatalf("actions hello incompletas: %#v", actions)
	}
	if !sliceAnyStringContains(actions, "check_updates") {
		t.Fatalf("hello no anuncia check_updates: %#v", actions)
	}
	limits, ok := hello["limits"].(map[string]any)
	if !ok || limits["maxRequestBytes"] != float64(maxBytesLinea) ||
		hello["idleTimeoutSeconds"] != timeoutInactividad.Seconds() {
		t.Fatalf("límites hello inesperados: limits=%#v idle=%#v", limits, hello["idleTimeoutSeconds"])
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_ClienteLegacyConservaEnvelope(t *testing.T) {
	t.Parallel()

	srv := New(&Manejador{})
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	if _, err := clientConn.Write([]byte(`{"action":"ping","params":{}}` + "\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var wire map[string]any
	if err := json.NewDecoder(bufio.NewReader(clientConn)).Decode(&wire); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if _, present := wire["protocol"]; present {
		t.Fatalf("la respuesta legacy no debe negociar protocol: %#v", wire)
	}
	for _, field := range []string{"errorCode", "phase", "retryable", "outcome"} {
		if _, present := wire[field]; !present {
			t.Fatalf("falta el campo estable %q: %#v", field, wire)
		}
	}
	if wire["ok"] != true || wire["outcome"] != ipcOutcomeSuccess {
		t.Fatalf("respuesta legacy inesperada: %#v", wire)
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_RechazaProtocolosInvalidosAntesDelDispatch(t *testing.T) {
	t.Parallel()

	catalog := &correlationCapturingCatalog{}
	srv := New(&Manejador{Catalogo: catalog})
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(clientConn)
	for _, test := range []struct {
		rawProtocol string
		errorCode   string
	}{
		{rawProtocol: `""`, errorCode: "invalid_request"},
		{rawProtocol: `null`, errorCode: "invalid_request"},
		{rawProtocol: `"desktop-ipc-v2"`, errorCode: "unsupported_protocol"},
	} {
		request := `{"protocol":` + test.rawProtocol + `,"action":"certificates","params":{}}` + "\n"
		if _, err := clientConn.Write([]byte(request)); err != nil {
			t.Fatalf("Write(%s) error = %v", test.rawProtocol, err)
		}
		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var response respuesta
		if err := json.NewDecoder(reader).Decode(&response); err != nil {
			t.Fatalf("Decode(%s) error = %v", test.rawProtocol, err)
		}
		if response.OK || response.Protocol != "" ||
			response.ErrorCode != test.errorCode ||
			response.Phase != ipcPhaseProtocol ||
			response.Retryable || response.Outcome != ipcOutcomeFailure {
			t.Fatalf("respuesta para %s inesperada: %#v", test.rawProtocol, response)
		}
		if response.Diagnostic == nil || len(response.Diagnostic.Steps) != 2 ||
			response.Diagnostic.Steps[0].Status != "success" ||
			response.Diagnostic.Steps[0].EvidenceRef != "phase:admission" ||
			response.Diagnostic.Steps[1].Status != "failure" ||
			response.Diagnostic.Steps[1].EvidenceRef != "phase:protocol" {
			t.Fatalf("pasos diagnósticos para %s inesperados: %#v", test.rawProtocol, response.Diagnostic)
		}
	}
	if got := catalog.calls.Load(); got != 0 {
		t.Fatalf("dispatches con protocolo inválido = %d; want 0", got)
	}

	_ = clientConn.Close()
	<-done
}

func TestServidorServirConexion_V1ExigeCorrelacionYParams(t *testing.T) {
	t.Parallel()

	catalog := &correlationCapturingCatalog{}
	srv := New(&Manejador{Catalogo: catalog})
	serverConn, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.servirConexion(context.Background(), serverConn)
	}()

	reader := bufio.NewReader(clientConn)
	for _, request := range []string{
		`{"protocol":"desktop-ipc-v1","traceId":"trace-1","action":"certificates","params":{}}`,
		`{"protocol":"desktop-ipc-v1","requestId":"req-1","action":"certificates","params":{}}`,
		`{"protocol":"desktop-ipc-v1","requestId":"req-1","traceId":"trace-1","action":"certificates"}`,
		`{"protocol":"desktop-ipc-v1","requestId":"req-1","traceId":"trace-1","action":"certificates","params":null}`,
	} {
		if _, err := clientConn.Write([]byte(request + "\n")); err != nil {
			t.Fatalf("Write(%s): %v", request, err)
		}
		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var response respuesta
		if err := json.NewDecoder(reader).Decode(&response); err != nil {
			t.Fatalf("Decode(%s): %v", request, err)
		}
		if response.OK || response.Protocol != desktopIPCProtocolV1 ||
			response.RequestID != "" || response.TraceID != "" ||
			response.ErrorCode != "invalid_request" ||
			response.Phase != ipcPhaseProtocol ||
			response.Outcome != ipcOutcomeFailure {
			t.Fatalf("respuesta v1 incompleta inesperada: %#v", response)
		}
	}
	if got := catalog.calls.Load(); got != 0 {
		t.Fatalf("dispatches de peticiones v1 incompletas=%d", got)
	}

	_ = clientConn.Close()
	<-done
}

func TestNormalizeIPCResponse_MetadatosYStepperObservado(t *testing.T) {
	t.Parallel()

	response := normalizeIPCResponse(respuesta{
		OK:     false,
		Action: "sign",
		Error:  "SAF_09: formato no soportado",
	})
	if response.ErrorCode != "SAF_09" || response.Phase != ipcPhaseOperation ||
		response.Outcome != ipcOutcomeFailure || response.Retryable {
		t.Fatalf("metadatos inesperados: %#v", response)
	}
	if response.Diagnostic == nil || len(response.Diagnostic.Steps) != 3 {
		t.Fatalf("stepper inesperado: %#v", response.Diagnostic)
	}
	step := response.Diagnostic.Steps[2]
	if step.Code != response.ErrorCode || step.Status != "failure" ||
		step.Owner == "" || step.EvidenceRef != "phase:operation" {
		t.Fatalf("paso inesperado: %#v", step)
	}

	partial := normalizeIPCResponse(respuesta{
		OK:     true,
		Action: "sign_batch",
		Data: resultadoFirmaLote{
			FailCount: 1,
			Results: []resultadoFirmaLoteItem{{
				Error: "fallo observado",
			}},
		},
	})
	if partial.Outcome != ipcOutcomePartial || partial.ErrorCode != "partial_failure" ||
		len(partial.Diagnostic.Steps) != 3 ||
		partial.Diagnostic.Steps[2].Status != "failure" {
		t.Fatalf("resultado parcial inesperado: %#v", partial)
	}
}

func TestObservedDiagnosticSteps_CronologiaHonestaPorFaseYResultado(t *testing.T) {
	t.Parallel()

	phases := []struct {
		name         string
		phase        string
		wantLabels   []string
		wantEvidence []string
	}{
		{
			name:         "admission",
			phase:        ipcPhaseAdmission,
			wantLabels:   []string{"Admisión de la petición"},
			wantEvidence: []string{"phase:admission"},
		},
		{
			name:  "protocol",
			phase: ipcPhaseProtocol,
			wantLabels: []string{
				"Admisión de la petición",
				"Validación del protocolo",
			},
			wantEvidence: []string{
				"phase:admission",
				"phase:protocol",
			},
		},
		{
			name:  "operation",
			phase: ipcPhaseOperation,
			wantLabels: []string{
				"Admisión de la petición",
				"Validación del protocolo",
				"Ejecución de la operación",
			},
			wantEvidence: []string{
				"phase:admission",
				"phase:protocol",
				"phase:operation",
			},
		},
		{
			name:  "empty defaults to operation",
			phase: "",
			wantLabels: []string{
				"Admisión de la petición",
				"Validación del protocolo",
				"Ejecución de la operación",
			},
			wantEvidence: []string{
				"phase:admission",
				"phase:protocol",
				"phase:operation",
			},
		},
		{
			name:         "unknown is conservative",
			phase:        "future-phase",
			wantLabels:   []string{"Resultado observado"},
			wantEvidence: []string{""},
		},
	}
	outcomes := []struct {
		outcome     string
		finalStatus string
	}{
		{outcome: ipcOutcomeFailure, finalStatus: "failure"},
		{outcome: ipcOutcomePartial, finalStatus: "failure"},
		{outcome: ipcOutcomeSuccess, finalStatus: "success"},
	}

	for _, phase := range phases {
		phase := phase
		for _, outcome := range outcomes {
			outcome := outcome
			t.Run(phase.name+"/"+outcome.outcome, func(t *testing.T) {
				t.Parallel()
				response := respuesta{
					ErrorCode: "SAF_26",
					Phase:     phase.phase,
					Outcome:   outcome.outcome,
					Diagnostic: &resultadoDiagnosticoGuiado{
						UserMessage:     "Fallo observado.",
						LikelyOwner:     "remote_service",
						SuggestedAction: "Reintente más tarde.",
					},
				}

				steps := observedDiagnosticSteps(response)
				if len(steps) != len(phase.wantLabels) {
					t.Fatalf("len(steps)=%d, want %d: %#v",
						len(steps), len(phase.wantLabels), steps)
				}
				for index, got := range steps {
					wantStatus := "success"
					if index == len(steps)-1 {
						wantStatus = outcome.finalStatus
					}
					if got.Label != phase.wantLabels[index] ||
						got.Status != wantStatus ||
						got.EvidenceRef != phase.wantEvidence[index] {
						t.Fatalf("steps[%d]=%#v, want label=%q status=%q evidence=%q",
							index, got, phase.wantLabels[index], wantStatus,
							phase.wantEvidence[index])
					}
					if index < len(steps)-1 {
						if got.Owner != "" || got.SuggestedAction != "" ||
							got.Code == response.ErrorCode {
							t.Fatalf("paso previo contiene atribución final: %#v", got)
						}
						continue
					}
					if got.Code != response.ErrorCode ||
						got.Owner != response.Diagnostic.LikelyOwner ||
						got.UserMessage != response.Diagnostic.UserMessage ||
						got.SuggestedAction != response.Diagnostic.SuggestedAction {
						t.Fatalf("el paso final no conserva el diagnóstico: %#v", got)
					}
				}
			})
		}
	}
}

func TestObservedDiagnosticSteps_SinDiagnosticoNoInventaCronologia(t *testing.T) {
	t.Parallel()

	if steps := observedDiagnosticSteps(respuesta{
		Phase:   ipcPhaseOperation,
		Outcome: ipcOutcomeFailure,
	}); steps != nil {
		t.Fatalf("steps=%#v, want nil", steps)
	}
}

func TestIPCErrorResponse_CronologiaAlcanzaSoloLaFaseFallida(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		phase        string
		wantEvidence []string
	}{
		{
			name:         "admission",
			phase:        ipcPhaseAdmission,
			wantEvidence: []string{"phase:admission"},
		},
		{
			name:         "protocol",
			phase:        ipcPhaseProtocol,
			wantEvidence: []string{"phase:admission", "phase:protocol"},
		},
		{
			name:  "operation",
			phase: ipcPhaseOperation,
			wantEvidence: []string{
				"phase:admission",
				"phase:protocol",
				"phase:operation",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := ipcErrorResponse(
				"sign",
				"operation_failed",
				test.phase,
				"fallo observado",
				false,
			)
			if response.Diagnostic == nil {
				t.Fatal("ipcErrorResponse() no incorporó diagnóstico")
			}
			steps := response.Diagnostic.Steps
			if len(steps) != len(test.wantEvidence) {
				t.Fatalf("len(steps)=%d, want %d: %#v",
					len(steps), len(test.wantEvidence), steps)
			}
			for index, evidence := range test.wantEvidence {
				wantStatus := "success"
				if index == len(steps)-1 {
					wantStatus = "failure"
				}
				if steps[index].EvidenceRef != evidence ||
					steps[index].Status != wantStatus {
					t.Fatalf("steps[%d]=%#v, want evidence=%q status=%q",
						index, steps[index], evidence, wantStatus)
				}
			}
		})
	}
}

func TestCacheCertificados_AccesoConcurrenteSeguro(t *testing.T) {
	t.Parallel()

	handler := &Manejador{}
	const iterations = 500
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		for i := 0; i < iterations; i++ {
			handler.setUltimosCerts([]domain.CertificateRef{{ID: "cert-a"}, {ID: "cert-b"}})
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < iterations; i++ {
			_ = handler.resolverCertID(i % 2)
		}
	}()
	go func() {
		defer wait.Done()
		for i := 0; i < iterations; i++ {
			_, _, _ = handler.resolverCertRefPorID(context.Background(), "cert-b")
		}
	}()
	wait.Wait()
}

func TestDesktopIPCHello_ActionsOrdenadasYCapacidadesCerradas(t *testing.T) {
	t.Parallel()

	hello := desktopIPCHello()
	if !sortIsStrictlyAscending(hello.Actions) {
		t.Fatalf("actions no están ordenadas o contienen duplicados: %#v", hello.Actions)
	}
	if !slices.Contains(hello.Capabilities, "certificate-id-selection") ||
		!slices.Contains(hello.Capabilities, "guided-diagnostic-steps") {
		t.Fatalf("capabilities incompletas: %#v", hello.Capabilities)
	}
	if strings.Contains(strings.Join(hello.Capabilities, ","), "cancel") {
		t.Fatalf("hello anuncia una capacidad no implementada: %#v", hello.Capabilities)
	}
	hasServiceStatus := slices.Contains(hello.Actions, "service_status")
	if runtime.GOOS == "linux" && !hasServiceStatus {
		t.Fatalf("hello no anuncia el gestor systemd disponible: %#v", hello.Actions)
	}
	if runtime.GOOS != "linux" && hasServiceStatus {
		t.Fatalf("hello anuncia una gestión de servicio no implementada: %#v", hello.Actions)
	}
	hasInstallTrust := slices.Contains(hello.Actions, "install_public_roots")
	hasClearTrust := slices.Contains(hello.Actions, "clear_tls_trust")
	if hasInstallTrust != hasClearTrust {
		t.Fatalf("hello anuncia un ciclo TLS incompleto: %#v", hello.Actions)
	}
	if (runtime.GOOS == "windows" || runtime.GOOS == "linux") && !hasInstallTrust {
		t.Fatalf("hello no anuncia el ciclo TLS gestionado: %#v", hello.Actions)
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && hasInstallTrust {
		t.Fatalf("hello anuncia un ciclo TLS no reversible: %#v", hello.Actions)
	}
}

func TestDesktopIPCHello_CicloTLSSeAnunciaSoloSiEsReversible(t *testing.T) {
	t.Parallel()

	unsupported := desktopIPCHelloWithManagedTrust(false)
	if slices.Contains(unsupported.Actions, "install_public_roots") ||
		slices.Contains(unsupported.Actions, "clear_tls_trust") {
		t.Fatalf("hello sin ciclo gestionado anuncia TLS: %#v", unsupported.Actions)
	}

	supported := desktopIPCHelloWithManagedTrust(true)
	if !slices.Contains(supported.Actions, "install_public_roots") ||
		!slices.Contains(supported.Actions, "clear_tls_trust") {
		t.Fatalf("hello con ciclo gestionado no anuncia ambas acciones: %#v", supported.Actions)
	}
}

func TestDesktopIPCV1_FixturesSonDecodificables(t *testing.T) {
	t.Parallel()

	requestData, err := os.ReadFile(filepath.Join("testdata", "desktop-ipc-v1-hello-request.json"))
	if err != nil {
		t.Fatalf("os.ReadFile(request) error = %v", err)
	}
	var request peticion
	if err := json.Unmarshal(requestData, &request); err != nil {
		t.Fatalf("json.Unmarshal(request) error = %v", err)
	}
	if err := validateDesktopIPCProtocol(request); err != nil ||
		request.Action != "hello" || !request.protocolPresent {
		t.Fatalf("fixture request inesperada: request=%#v error=%v", request, err)
	}

	responseData, err := os.ReadFile(filepath.Join("testdata", "desktop-ipc-v1-hello-response.json"))
	if err != nil {
		t.Fatalf("os.ReadFile(response) error = %v", err)
	}
	var response respuesta
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if !response.OK || response.Protocol != desktopIPCProtocolV1 ||
		response.Phase != ipcPhaseOperation || response.Outcome != ipcOutcomeSuccess {
		t.Fatalf("fixture response inesperada: %#v", response)
	}
}

func TestDesktopIPCV1_EsquemaPublicoSincronizaAcciones(t *testing.T) {
	t.Parallel()

	schemaData, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "..",
		"docs", "schemas", "desktop-ipc-v1.schema.json",
	))
	if err != nil {
		t.Fatalf("os.ReadFile(schema) error = %v", err)
	}
	var schema struct {
		Definitions struct {
			CorrelationID struct {
				Pattern string `json:"pattern"`
			} `json:"correlationId"`
			Action struct {
				Enum []string `json:"enum"`
			} `json:"action"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatalf("json.Unmarshal(schema) error = %v", err)
	}
	if schema.Definitions.CorrelationID.Pattern != `^[A-Za-z0-9._:-]+$` {
		t.Fatalf(
			"pattern de correlación diverge del validador fail-closed: %q",
			schema.Definitions.CorrelationID.Pattern,
		)
	}
	for _, action := range desktopIPCHello().Actions {
		if !slices.Contains(schema.Definitions.Action.Enum, action) {
			t.Fatalf(
				"hello anuncia una acción ausente del schema: %q",
				action,
			)
		}
	}
}

func sliceAnyStringContains(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortIsStrictlyAscending(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] >= values[i] {
			return false
		}
	}
	return true
}
