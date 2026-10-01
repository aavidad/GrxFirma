// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package identityjcs

import (
	"errors"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestCanonicalizadorCoincideConVectorDelIntegrador(t *testing.T) {
	canonicalizador, err := Nuevo(ConfiguracionPredeterminada())
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	obtenido, err := canonicalizador.Canonicalizar(solicitudVector())
	if err != nil {
		t.Fatalf("canonicalizar: %v", err)
	}
	esperado := `{"audience":"urn:dipgra:identidad","challengeId":"018f47de-88c0-7d35-bd91-76a73848d111","consentId":"consentimiento-certificado","consentVersion":"2026-07","contract":"identidad-reforzada/v1","expiresAt":"2026-08-01T10:35:00.123456789Z","issuedAt":"2026-08-01T10:30:00.123456789Z","nonce":"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8","operation":"contabilidad.aprobar.v1","origin":"https://cliente.example","policyId":"politica-certificado","policyVersion":"7","purpose":"Aprobar asiento ñ","registeredClient":"cliente-web","sessionBinding":"hmac-sha256-lp-v1.k1.sesion","tenantContextHash":"hmac-sha256-lp-v1.k1.tenant"}`
	if string(obtenido) != esperado {
		t.Fatalf("vector distinto\nobtenido: %s\nesperado: %s", obtenido, esperado)
	}
}

func TestCanonicalizadorFallaCerrado(t *testing.T) {
	if obtenido, err := Nuevo(Configuracion{VersionFormato: "otra", MaximoBytes: 1024}); obtenido != nil || !errors.Is(err, ErrConfiguracionInvalida) {
		t.Fatalf("configuración no rechazada: adaptador=%v error=%v", obtenido, err)
	}
	canonicalizador, _ := Nuevo(Configuracion{VersionFormato: VersionFormato, MaximoBytes: 100})
	if canon, err := canonicalizador.Canonicalizar(solicitudVector()); canon != nil || !errors.Is(err, ErrCanonicalizacion) {
		t.Fatalf("límite no aplicado: canon=%q error=%v", canon, err)
	}
	solicitud := solicitudVector()
	solicitud.Contrato = "legacy"
	canonicalizador, _ = Nuevo(ConfiguracionPredeterminada())
	if canon, err := canonicalizador.Canonicalizar(solicitud); canon != nil || !errors.Is(err, ErrCanonicalizacion) {
		t.Fatalf("contrato no rechazado: canon=%q error=%v", canon, err)
	}
}

func solicitudVector() domain.SolicitudRetoIdentidad {
	instante := time.Date(2026, 8, 1, 10, 30, 0, 123456789, time.UTC)
	return domain.SolicitudRetoIdentidad{
		Contrato:          domain.VersionContratoIdentidadReforzada,
		RetoID:            "018f47de-88c0-7d35-bd91-76a73848d111",
		ClienteRegistrado: "cliente-web", Finalidad: "Aprobar asiento ñ",
		Operacion: "contabilidad.aprobar.v1", Audiencia: "urn:dipgra:identidad",
		HuellaContextoTenant: "hmac-sha256-lp-v1.k1.tenant",
		VinculoSesion:        "hmac-sha256-lp-v1.k1.sesion",
		Origen:               "https://cliente.example", ConsentimientoID: "consentimiento-certificado",
		VersionConsentimiento: "2026-07", PoliticaID: "politica-certificado",
		VersionPolitica: "7", Nonce: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
		EmitidoEn: instante, ExpiraEn: instante.Add(5 * time.Minute),
	}
}
