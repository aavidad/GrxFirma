// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestIdentityRoutesSoloSePublicanCompletasYAutenticadas(t *testing.T) {
	base := New(nil, nil, nil)
	peticion := httptest.NewRequest(http.MethodPost, "/identity/challenges", nil)
	respuesta := httptest.NewRecorder()
	base.Routes().ServeHTTP(respuesta, peticion)
	if respuesta.Code != http.StatusNotFound {
		t.Fatalf("ruta parcial publicada: %d", respuesta.Code)
	}

	adaptador := New(nil, nil, nil).WithIdentidadReforzada(&preparadorIdentidadRESTFalso{}, &confirmadorIdentidadRESTFalso{})
	respuesta = httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(respuesta, peticion)
	if respuesta.Code != http.StatusServiceUnavailable || !strings.Contains(respuesta.Body.String(), "identity.configuration_unavailable") {
		t.Fatalf("ruta sin autenticación admitida: %d %s", respuesta.Code, respuesta.Body.String())
	}
}

func TestIdentityChallengeRechazaJSONAmbiguoYRespetaOrigen(t *testing.T) {
	preparador := &preparadorIdentidadRESTFalso{}
	adaptador := adaptadorIdentidadREST(preparador, &confirmadorIdentidadRESTFalso{})
	valido := cuerpoRetoIdentidad()
	duplicado := strings.Replace(valido, `"contract":`, `"contract":"identidad-reforzada/v1","contract":`, 1)
	for nombre, cuerpo := range map[string]string{
		"duplicado":   duplicado,
		"desconocido": strings.Replace(valido, `"contract":`, `"extra":true,"contract":`, 1),
	} {
		t.Run(nombre, func(t *testing.T) {
			respuesta := ejecutarIdentidad(adaptador, "/identity/challenges", cuerpo, "https://integrador.example")
			if respuesta.Code != http.StatusBadRequest || preparador.llamadas != 0 {
				t.Fatalf("JSON ambiguo delegado: %d %s", respuesta.Code, respuesta.Body.String())
			}
		})
	}
	respuesta := ejecutarIdentidad(adaptador, "/identity/challenges", valido, "https://otro.example")
	if respuesta.Code != http.StatusBadRequest || preparador.llamadas != 0 {
		t.Fatalf("origen distinto delegado: %d %s", respuesta.Code, respuesta.Body.String())
	}
}

func TestIdentityChallengeYVerificationMantienenContrato(t *testing.T) {
	preparador := &preparadorIdentidadRESTFalso{}
	confirmador := &confirmadorIdentidadRESTFalso{}
	adaptador := adaptadorIdentidadREST(preparador, confirmador)
	respuesta := ejecutarIdentidad(adaptador, "/identity/challenges", cuerpoRetoIdentidad(), "https://integrador.example")
	if respuesta.Code != http.StatusCreated || !strings.Contains(respuesta.Body.String(), `"canonicalPayloadB64":"Y2Fub24="`) {
		t.Fatalf("respuesta de reto inesperada: %d %s", respuesta.Code, respuesta.Body.String())
	}
	prueba := `{"contract":"identidad-reforzada/v1","challengeId":"reto:1","signatureB64":"Zg==","certificateB64":"Yw==","chainB64":[],"format":"cades-detached","signatureAlgorithm":"sha256-ecdsa","digestAlgorithm":"sha-256"}`
	respuesta = ejecutarIdentidad(adaptador, "/identity/verifications", prueba, "")
	if respuesta.Code != http.StatusOK || !strings.Contains(respuesta.Body.String(), `"outcome":"indeterminate"`) || confirmador.llamadas != 1 {
		t.Fatalf("respuesta trivalente inesperada: %d %s", respuesta.Code, respuesta.Body.String())
	}
}

type preparadorIdentidadRESTFalso struct{ llamadas int }

func (p *preparadorIdentidadRESTFalso) Preparar(_ context.Context, solicitud domain.SolicitudRetoIdentidad) (domain.RetoIdentidad, error) {
	p.llamadas++
	return domain.RetoIdentidad{Solicitud: solicitud, ContenidoCanonico: []byte("canon")}, nil
}

type confirmadorIdentidadRESTFalso struct{ llamadas int }

func (c *confirmadorIdentidadRESTFalso) Confirmar(_ context.Context, prueba domain.PruebaIdentidad) (domain.ResultadoVerificacionIdentidad, error) {
	c.llamadas++
	solicitud, _ := solicitudRetoIdentidadJSONPrueba().dominio()
	return domain.ResultadoVerificacionIdentidad{Resultado: domain.ResultadoIdentidadIndeterminada,
		EvidenciaRef: "evidencia:1", RetoID: prueba.RetoID, Solicitud: solicitud}, nil
}

func adaptadorIdentidadREST(preparar PrepareIdentityUseCase, confirmar ConfirmIdentityUseCase) *Adaptador {
	adaptador := New(nil, nil, nil).WithIdentidadReforzada(preparar, confirmar)
	adaptador.BearerToken = "token-prueba"
	return adaptador
}

func ejecutarIdentidad(adaptador *Adaptador, ruta, cuerpo, origen string) *httptest.ResponseRecorder {
	peticion := httptest.NewRequest(http.MethodPost, ruta, bytes.NewBufferString(cuerpo))
	peticion.Header.Set("Authorization", "Bearer token-prueba")
	if origen != "" {
		peticion.Header.Set("Origin", origen)
	}
	respuesta := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(respuesta, peticion)
	return respuesta
}

func cuerpoRetoIdentidad() string {
	dto := solicitudRetoIdentidadJSONPrueba()
	contenido, _ := json.Marshal(dto)
	return string(contenido)
}

func solicitudRetoIdentidadJSONPrueba() solicitudRetoIdentidadJSON {
	emitido := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return solicitudRetoIdentidadJSON{Contract: domain.VersionContratoIdentidadReforzada,
		ChallengeID: "reto:1", Audience: "urn:dipgra:identidad", RegisteredClient: "cliente",
		Purpose: "Acreditar identidad", Operation: "identidad.reforzar.v1",
		TenantContextHash: "hmac:tenant", SessionBinding: "hmac:sesion",
		Origin: "https://integrador.example", ConsentID: "consentimiento", ConsentVersion: "1",
		PolicyID: "politica", PolicyVersion: "1",
		Nonce:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		IssuedAt: emitido.Format(time.RFC3339Nano), ExpiresAt: emitido.Add(time.Minute).Format(time.RFC3339Nano)}
}
