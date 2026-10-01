// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package nativehost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type generadorPruebaIdentidadNativaFalso struct {
	prueba   domain.PruebaIdentidad
	err      error
	reto     domain.RetoIdentidad
	llamadas int
}

func (g *generadorPruebaIdentidadNativaFalso) Generar(
	_ context.Context,
	reto domain.RetoIdentidad,
) (domain.PruebaIdentidad, error) {
	g.llamadas++
	g.reto = reto
	return g.prueba, g.err
}

func TestProcessPruebaIdentidadNativaMinimizaYFragmenta(t *testing.T) {
	firma := bytes.Repeat([]byte{0xa5}, 70*1024)
	generador := &generadorPruebaIdentidadNativaFalso{prueba: domain.PruebaIdentidad{
		RetoID: "018f47de-88c0-4d35-bd91-76a73848d111", Formato: "cades-detached",
		AlgoritmoFirma: "sha256-rsa-pkcs1v15", AlgoritmoHuella: "sha-256",
		Firma: firma, Certificado: []byte("certificado-hoja"),
		Cadena: [][]byte{[]byte("certificado-emisor")},
	}}
	adaptador := New(nil, nil, nil, nil).WithGeneradorPruebaIdentidad(generador)
	respuestas, err := adaptador.Process(context.Background(), "extension-oficial",
		solicitudPruebaIdentidadNativa(t, contenidoCanonicoIdentidadNativa()))
	if err != nil {
		t.Fatalf("procesar: %v", err)
	}
	if len(respuestas) != 2 || generador.llamadas != 1 {
		t.Fatalf("fragmentos=%d llamadas=%d", len(respuestas), generador.llamadas)
	}
	var firmaCodificada stringsBuilder
	for indice, contenido := range respuestas {
		var respuesta response
		if err := json.Unmarshal(contenido, &respuesta); err != nil {
			t.Fatalf("decodificar fragmento %d: %v", indice, err)
		}
		if !respuesta.Success || respuesta.Chunk != indice || respuesta.TotalChunks != 2 ||
			respuesta.IdentityProof == nil || respuesta.IdentityProof.ChallengeID != generador.prueba.RetoID ||
			respuesta.IdentityProof.CertificateB64 != base64.StdEncoding.EncodeToString([]byte("certificado-hoja")) {
			t.Fatalf("fragmento inseguro: %+v", respuesta)
		}
		firmaCodificada.WriteString(respuesta.Signature)
	}
	if firmaCodificada.String() != base64.StdEncoding.EncodeToString(firma) ||
		generador.reto.Solicitud.Origen != "https://cliente.example" ||
		!bytes.Equal(generador.reto.ContenidoCanonico, contenidoCanonicoIdentidadNativa()) {
		t.Fatal("el canon o la firma no se conservaron exactamente")
	}
}

func TestProcessPruebaIdentidadNativaRechazaCamposYOrigenFalseables(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{
			nombre: "certificado impuesto por portal",
			cuerpo: `{"requestId":"identidad-1","action":"proveIdentity",` +
				`"canonicalPayloadB64":"%s","requesterOrigin":"https://cliente.example",` +
				`"certificateId":"certificado-atacante"}`,
		},
		{
			nombre: "origen con ruta",
			cuerpo: `{"requestId":"identidad-1","action":"proveIdentity",` +
				`"canonicalPayloadB64":"%s","requesterOrigin":"https://cliente.example/ruta"}`,
		},
		{
			nombre: "origen distinto del canon",
			cuerpo: `{"requestId":"identidad-1","action":"proveIdentity",` +
				`"canonicalPayloadB64":"%s","requesterOrigin":"https://otra.dipgra.es"}`,
		},
	}
	canon := base64.StdEncoding.EncodeToString(contenidoCanonicoIdentidadNativa())
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			generador := &generadorPruebaIdentidadNativaFalso{}
			adaptador := New(nil, nil, nil, nil).WithGeneradorPruebaIdentidad(generador)
			respuestas, err := adaptador.Process(context.Background(), "extension-oficial",
				[]byte(fmt.Sprintf(caso.cuerpo, canon)))
			if err != nil {
				t.Fatalf("procesar: %v", err)
			}
			var respuesta response
			_ = json.Unmarshal(respuestas[0], &respuesta)
			if respuesta.Success || respuesta.Code != "identity.invalid_request" ||
				generador.llamadas != 0 {
				t.Fatalf("entrada no bloqueada: %+v llamadas=%d", respuesta, generador.llamadas)
			}
		})
	}
}

func TestProcessPruebaIdentidadNativaRechazaCanonDuplicadoYFalloInterno(t *testing.T) {
	duplicado := bytes.Replace(contenidoCanonicoIdentidadNativa(),
		[]byte(`"origin":"https://cliente.example"`),
		[]byte(`"origin":"https://cliente.example","origin":"https://cliente.example"`), 1)
	adaptador := New(nil, nil, nil, nil).WithGeneradorPruebaIdentidad(
		&generadorPruebaIdentidadNativaFalso{},
	)
	respuestas, _ := adaptador.Process(context.Background(), "extension-oficial",
		solicitudPruebaIdentidadNativa(t, duplicado))
	var respuesta response
	_ = json.Unmarshal(respuestas[0], &respuesta)
	if respuesta.Code != "identity.invalid_request" {
		t.Fatalf("canon duplicado admitido: %+v", respuesta)
	}

	generador := &generadorPruebaIdentidadNativaFalso{err: application.ErrPruebaIdentidadLocalNoDisponible}
	adaptador = New(nil, nil, nil, nil).WithGeneradorPruebaIdentidad(generador)
	respuestas, _ = adaptador.Process(context.Background(), "extension-oficial",
		solicitudPruebaIdentidadNativa(t, contenidoCanonicoIdentidadNativa()))
	_ = json.Unmarshal(respuestas[0], &respuesta)
	if respuesta.Code != "identity.unavailable" ||
		!errors.Is(generador.err, application.ErrPruebaIdentidadLocalNoDisponible) ||
		respuesta.Error != respuesta.Code || respuesta.Error == generador.err.Error() {
		t.Fatalf("fallo interno filtrado: %+v", respuesta)
	}
}

func solicitudPruebaIdentidadNativa(t *testing.T, canon []byte) []byte {
	t.Helper()
	contenido, err := json.Marshal(solicitudPruebaIdentidadNativaJSON{
		RequestID: "identidad-1", Action: "proveIdentity",
		CanonicalPayloadB64: base64.StdEncoding.EncodeToString(canon),
		RequesterOrigin:     "https://cliente.example",
	})
	if err != nil {
		t.Fatalf("serializar solicitud: %v", err)
	}
	return contenido
}

func contenidoCanonicoIdentidadNativa() []byte {
	return []byte(`{"audience":"urn:dipgra:identidad","challengeId":"018f47de-88c0-4d35-bd91-76a73848d111","consentId":"consentimiento-certificado","consentVersion":"2026-07","contract":"identidad-reforzada/v1","expiresAt":"2026-08-01T17:05:00Z","issuedAt":"2026-08-01T17:00:00Z","nonce":"AAECAwQFBgcICQoLDA0ODw","operation":"identidad.reforzar.v1","origin":"https://cliente.example","policyId":"politica-certificado","policyVersion":"7","purpose":"Acreditar identidad","registeredClient":"cliente-web","sessionBinding":"hmac-sha256-lp-v1.k1.sesion","tenantContextHash":"hmac-sha256-lp-v1.k1.tenant"}`)
}

type stringsBuilder struct{ bytes.Buffer }
