// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package identitypolicy

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestCatalogoAutorizaSoloCombinacionExacta(t *testing.T) {
	solicitud, registro, reloj := datosPoliticaPrueba()
	catalogo, err := NuevoCatalogo([]Registro{registro}, reloj)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	if err := catalogo.Autorizar(context.Background(), solicitud); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	for nombre, mutar := range map[string]func(*domain.SolicitudRetoIdentidad){
		"tenant":    func(s *domain.SolicitudRetoIdentidad) { s.HuellaContextoTenant = "hmac:otro" },
		"origen":    func(s *domain.SolicitudRetoIdentidad) { s.Origen = "https://malicioso.example" },
		"audiencia": func(s *domain.SolicitudRetoIdentidad) { s.Audiencia = "otra" },
		"política":  func(s *domain.SolicitudRetoIdentidad) { s.VersionPolitica = "2" },
	} {
		t.Run(nombre, func(t *testing.T) {
			alterada := solicitud
			mutar(&alterada)
			if !errors.Is(catalogo.Autorizar(context.Background(), alterada), ErrSolicitudNoAutorizada) {
				t.Fatal("se autorizó una combinación no registrada")
			}
		})
	}
}

func TestCatalogoRechazaTTLDesfaseYDuplicados(t *testing.T) {
	solicitud, registro, reloj := datosPoliticaPrueba()
	if catalogo, err := NuevoCatalogo([]Registro{registro, registro}, reloj); catalogo != nil || !errors.Is(err, ErrConfiguracionInvalida) {
		t.Fatalf("duplicado admitido: catálogo=%v error=%v", catalogo, err)
	}
	catalogo, _ := NuevoCatalogo([]Registro{registro}, reloj)
	solicitud.ExpiraEn = solicitud.EmitidoEn.Add(3 * time.Minute)
	if !errors.Is(catalogo.Autorizar(context.Background(), solicitud), ErrSolicitudNoAutorizada) {
		t.Fatal("TTL excesivo admitido")
	}
	solicitud, _, _ = datosPoliticaPrueba()
	solicitud.EmitidoEn = reloj.instante.Add(31 * time.Second)
	solicitud.ExpiraEn = solicitud.EmitidoEn.Add(time.Minute)
	if !errors.Is(catalogo.Autorizar(context.Background(), solicitud), ErrSolicitudNoAutorizada) {
		t.Fatal("emisión futura fuera de desfase admitida")
	}
}

type relojFalso struct{ instante time.Time }

func (r relojFalso) Now() time.Time { return r.instante }

func datosPoliticaPrueba() (domain.SolicitudRetoIdentidad, Registro, relojFalso) {
	emitido := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	solicitud := domain.SolicitudRetoIdentidad{Contrato: domain.VersionContratoIdentidadReforzada,
		RetoID: "reto:1", Audiencia: "urn:dipgra:identidad", ClienteRegistrado: "cliente",
		Finalidad: "Acreditar identidad", Operacion: "identidad.reforzar.v1",
		HuellaContextoTenant: "hmac:tenant", VinculoSesion: "hmac:sesion",
		Origen: "https://integrador.example", ConsentimientoID: "consentimiento",
		VersionConsentimiento: "1", PoliticaID: "politica", VersionPolitica: "1",
		Nonce: make([]byte, 32), EmitidoEn: emitido, ExpiraEn: emitido.Add(time.Minute)}
	registro := Registro{HuellaContextoTenant: solicitud.HuellaContextoTenant,
		ClienteRegistrado: solicitud.ClienteRegistrado, Operacion: solicitud.Operacion,
		Audiencia: solicitud.Audiencia, Finalidad: solicitud.Finalidad, Origen: solicitud.Origen,
		ConsentimientoID: solicitud.ConsentimientoID, VersionConsentimiento: solicitud.VersionConsentimiento,
		PoliticaID: solicitud.PoliticaID, VersionPolitica: solicitud.VersionPolitica,
		VigenciaMaxima: 2 * time.Minute, DesfaseMaximo: 30 * time.Second}
	return solicitud, registro, relojFalso{instante: emitido}
}
