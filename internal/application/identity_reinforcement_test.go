// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"grxfirma/internal/domain"
)

func TestPreparadorRetoIdentidadCopiaYRegistraCanon(t *testing.T) {
	repositorio := &repositorioIdentidadFalso{}
	preparador, err := NuevoPreparadorRetoIdentidad(canonicalizadorIdentidadFalso{salida: []byte("canon")}, autorizadorIdentidadFalso{}, repositorio)
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	solicitud := solicitudIdentidadAplicacion()
	reto, err := preparador.Preparar(context.Background(), solicitud)
	if err != nil {
		t.Fatalf("preparar: %v", err)
	}
	solicitud.Nonce[0] = 9
	reto.ContenidoCanonico[0] = 'X'
	if repositorio.registrado.Solicitud.Nonce[0] != 0 || string(repositorio.registrado.ContenidoCanonico) != "canon" {
		t.Fatal("el repositorio comparte memoria mutable con la entrada o la salida")
	}
}

func TestConfirmadorReservaAntesDeVerificarYFinaliza(t *testing.T) {
	reto := domain.RetoIdentidad{Solicitud: solicitudIdentidadAplicacion(), ContenidoCanonico: []byte("canon")}
	repositorio := &repositorioIdentidadFalso{reservado: reto}
	verificador := &verificadorIdentidadFalso{repositorio: repositorio, resultado: resultadoAceptadoAplicacion(reto.Solicitud)}
	confirmador, err := NuevoConfirmadorIdentidad(repositorio, verificador, relojIdentidadFalso{instante: reto.Solicitud.EmitidoEn})
	if err != nil {
		t.Fatalf("construir: %v", err)
	}
	resultado, err := confirmador.Confirmar(context.Background(), pruebaIdentidadAplicacion())
	if err != nil || resultado.Resultado != domain.ResultadoIdentidadAceptada {
		t.Fatalf("confirmar: resultado=%+v error=%v", resultado, err)
	}
	if !repositorio.reservo || !repositorio.finalizo || !verificador.observoReserva {
		t.Fatal("no se respetó reserva → llamada externa → finalización")
	}
}

func TestConfirmadorConvierteFalloEnIndeterminadoPersistido(t *testing.T) {
	reto := domain.RetoIdentidad{Solicitud: solicitudIdentidadAplicacion(), ContenidoCanonico: []byte("canon")}
	repositorio := &repositorioIdentidadFalso{reservado: reto}
	confirmador, _ := NuevoConfirmadorIdentidad(repositorio, &verificadorIdentidadFalso{repositorio: repositorio, err: errors.New("ocsp no disponible")}, relojIdentidadFalso{})
	resultado, err := confirmador.Confirmar(context.Background(), pruebaIdentidadAplicacion())
	if !errors.Is(err, ErrDependenciaIdentidad) || resultado.Resultado != domain.ResultadoIdentidadIndeterminada {
		t.Fatalf("fallo no conservado como indeterminado: resultado=%+v error=%v", resultado, err)
	}
	if repositorio.finalizado.Resultado != domain.ResultadoIdentidadIndeterminada {
		t.Fatal("no se persistió el resultado indeterminado")
	}
}

func TestConfirmadorConservaReferenciaDurableEnIndeterminacion(t *testing.T) {
	reto := domain.RetoIdentidad{Solicitud: solicitudIdentidadAplicacion(), ContenidoCanonico: []byte("canon")}
	repositorio := &repositorioIdentidadFalso{reservado: reto}
	indeterminado := domain.ResultadoVerificacionIdentidad{Resultado: domain.ResultadoIdentidadIndeterminada,
		EvidenciaRef: "evidencia:durable:indeterminada", RetoID: reto.Solicitud.RetoID, Solicitud: reto.Solicitud}
	confirmador, _ := NuevoConfirmadorIdentidad(repositorio, &verificadorIdentidadFalso{repositorio: repositorio,
		resultado: indeterminado, err: errors.New("revocación no disponible")}, relojIdentidadFalso{})
	resultado, err := confirmador.Confirmar(context.Background(), pruebaIdentidadAplicacion())
	if !errors.Is(err, ErrDependenciaIdentidad) || resultado.EvidenciaRef != indeterminado.EvidenciaRef ||
		repositorio.finalizado.EvidenciaRef != indeterminado.EvidenciaRef {
		t.Fatalf("se perdió evidencia durable: resultado=%+v persistido=%+v error=%v", resultado, repositorio.finalizado, err)
	}
}

type canonicalizadorIdentidadFalso struct{ salida []byte }

func (c canonicalizadorIdentidadFalso) Canonicalizar(domain.SolicitudRetoIdentidad) ([]byte, error) {
	return c.salida, nil
}

type autorizadorIdentidadFalso struct{ err error }

func (a autorizadorIdentidadFalso) Autorizar(context.Context, domain.SolicitudRetoIdentidad) error {
	return a.err
}

type repositorioIdentidadFalso struct {
	registrado, reservado domain.RetoIdentidad
	finalizado            domain.ResultadoVerificacionIdentidad
	reservo, finalizo     bool
}

func (r *repositorioIdentidadFalso) Registrar(_ context.Context, reto domain.RetoIdentidad) error {
	r.registrado = copiarRetoIdentidad(reto)
	return nil
}

func (r *repositorioIdentidadFalso) Reservar(_ context.Context, _ string, _ time.Time) (domain.RetoIdentidad, error) {
	r.reservo = true
	return copiarRetoIdentidad(r.reservado), nil
}

func (r *repositorioIdentidadFalso) Finalizar(_ context.Context, resultado domain.ResultadoVerificacionIdentidad) error {
	r.finalizo = true
	r.finalizado = resultado
	return nil
}

type verificadorIdentidadFalso struct {
	repositorio    *repositorioIdentidadFalso
	resultado      domain.ResultadoVerificacionIdentidad
	err            error
	observoReserva bool
}

func (v *verificadorIdentidadFalso) Verificar(_ context.Context, _ domain.RetoIdentidad, _ domain.PruebaIdentidad) (domain.ResultadoVerificacionIdentidad, error) {
	v.observoReserva = v.repositorio.reservo && !v.repositorio.finalizo
	return v.resultado, v.err
}

type relojIdentidadFalso struct{ instante time.Time }

func (r relojIdentidadFalso) Now() time.Time { return r.instante }

func solicitudIdentidadAplicacion() domain.SolicitudRetoIdentidad {
	emitido := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return domain.SolicitudRetoIdentidad{
		Contrato: domain.VersionContratoIdentidadReforzada, RetoID: "reto:1",
		Audiencia: "urn:dipgra:identidad", ClienteRegistrado: "cliente-web",
		Finalidad: "Acreditar identidad", Operacion: "identidad.reforzar.v1",
		HuellaContextoTenant: "hmac:tenant", VinculoSesion: "hmac:sesion",
		Origen: "https://cliente.example", ConsentimientoID: "consentimiento",
		VersionConsentimiento: "1", PoliticaID: "politica", VersionPolitica: "1",
		Nonce: make([]byte, 32), EmitidoEn: emitido, ExpiraEn: emitido.Add(2 * time.Minute),
	}
}

func pruebaIdentidadAplicacion() domain.PruebaIdentidad {
	return domain.PruebaIdentidad{RetoID: "reto:1", Formato: "cades-detached",
		AlgoritmoFirma: "sha256-rsa-pkcs1v15", AlgoritmoHuella: "sha-256",
		Firma: []byte("firma"), Certificado: []byte("certificado")}
}

func resultadoAceptadoAplicacion(s domain.SolicitudRetoIdentidad) domain.ResultadoVerificacionIdentidad {
	d := domain.DictamenComprobacion{Estado: "conforme", Fuente: "servidor/v1", ComprobadoEn: s.EmitidoEn}
	return domain.ResultadoVerificacionIdentidad{Resultado: domain.ResultadoIdentidadAceptada,
		EvidenciaRef: "evidencia:1", RetoID: s.RetoID, Solicitud: s,
		NivelAseguramiento: "certificado", MetodoAutenticacion: "x509",
		IdentidadAcreditada: "identidad:opaca", HuellaCertificado: "sha256:abc",
		Integridad: d, Cadena: d, Vigencia: d, EKU: d, PoliticaCertificado: d, Revocacion: d}
}
