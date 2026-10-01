// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"errors"
	"testing"
	"time"
)

func TestSolicitudRetoIdentidadValidaContratoMinimizado(t *testing.T) {
	solicitud := solicitudIdentidadPrueba()
	if err := solicitud.Validar(); err != nil {
		t.Fatalf("validar solicitud: %v", err)
	}
	solicitud.VinculoSesion = " sesión"
	if !errors.Is(solicitud.Validar(), ErrSolicitudIdentidadInvalida) {
		t.Fatal("se admitió un vínculo ambiguo")
	}
}

func TestAceptacionExigeTodosLosDictamenes(t *testing.T) {
	instante := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	dictamen := DictamenComprobacion{Estado: "conforme", Fuente: "validador-servidor/v1", ComprobadoEn: instante}
	resultado := ResultadoVerificacionIdentidad{
		Resultado: ResultadoIdentidadAceptada, EvidenciaRef: "evidencia:1", RetoID: "reto:1",
		Solicitud: solicitudIdentidadPrueba(), NivelAseguramiento: "certificado",
		MetodoAutenticacion: "certificado-x509", IdentidadAcreditada: "id-opaca",
		HuellaCertificado: "sha256:abc", Integridad: dictamen, Cadena: dictamen,
		Vigencia: dictamen, EKU: dictamen, PoliticaCertificado: dictamen, Revocacion: dictamen,
	}
	if err := resultado.ValidarEstructura(); err != nil {
		t.Fatalf("validar aceptación: %v", err)
	}
	resultado.Revocacion.Estado = "indeterminado"
	if !errors.Is(resultado.ValidarEstructura(), ErrResultadoIdentidadInvalido) {
		t.Fatal("se admitió aceptación con revocación indeterminada")
	}
	resultado.Resultado = ResultadoIdentidadIndeterminada
	if err := resultado.ValidarEstructura(); err != nil {
		t.Fatalf("se perdió el resultado trivalente: %v", err)
	}
}

func TestPruebaIdentidadSoloAdmitePerfilRegistrado(t *testing.T) {
	prueba := PruebaIdentidad{RetoID: "reto:1", Formato: "cades-detached",
		AlgoritmoFirma: "sha256-ecdsa", AlgoritmoHuella: "sha-256",
		Firma: []byte("firma"), Certificado: []byte("certificado")}
	if err := prueba.Validar(); err != nil {
		t.Fatalf("validar prueba: %v", err)
	}
	prueba.Formato = "pades"
	if !errors.Is(prueba.Validar(), ErrPruebaIdentidadInvalida) {
		t.Fatal("se admitió un formato no registrado para el contrato v1")
	}
}

func solicitudIdentidadPrueba() SolicitudRetoIdentidad {
	emitido := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return SolicitudRetoIdentidad{
		Contrato: VersionContratoIdentidadReforzada, RetoID: "reto:1",
		Audiencia:         "urn:dipgra:identidad",
		ClienteRegistrado: "cliente-web", Finalidad: "Acreditar identidad",
		Operacion: "identidad.reforzar.v1", HuellaContextoTenant: "hmac:tenant",
		VinculoSesion: "hmac:sesion", Origen: "https://cliente.example",
		ConsentimientoID: "identidad-certificado", VersionConsentimiento: "2026-08",
		PoliticaID: "politica-certificado", VersionPolitica: "1",
		Nonce: make([]byte, 32), EmitidoEn: emitido, ExpiraEn: emitido.Add(2 * time.Minute),
	}
}
