// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const maximoContenidoCanonicoIdentidad = 16 * 1024

var (
	// ErrDependenciaIdentidad evita filtrar errores internos por el borde.
	ErrDependenciaIdentidad = errors.New("dependencia de identidad reforzada no disponible")
	// ErrRetoIdentidadNoDisponible oculta existencia, caducidad y consumo previo.
	ErrRetoIdentidadNoDisponible = errors.New("reto de identidad reforzada no disponible")
)

// PreparadorRetoIdentidad valida, canonicaliza y registra un reto genérico.
type PreparadorRetoIdentidad struct {
	canonicalizador ports.CanonicalizadorIdentidad
	autorizador     ports.AutorizadorSolicitudIdentidad
	repositorio     ports.RepositorioRetosIdentidad
}

// NuevoPreparadorRetoIdentidad exige conectores completos antes de publicar el caso de uso.
func NuevoPreparadorRetoIdentidad(c ports.CanonicalizadorIdentidad, a ports.AutorizadorSolicitudIdentidad, r ports.RepositorioRetosIdentidad) (*PreparadorRetoIdentidad, error) {
	if c == nil || a == nil || r == nil {
		return nil, ErrDependenciaIdentidad
	}
	return &PreparadorRetoIdentidad{canonicalizador: c, autorizador: a, repositorio: r}, nil
}

// Preparar devuelve una copia de los bytes que se deben firmar exactamente.
func (p *PreparadorRetoIdentidad) Preparar(ctx context.Context, solicitud domain.SolicitudRetoIdentidad) (domain.RetoIdentidad, error) {
	if ctx == nil || solicitud.Validar() != nil {
		return domain.RetoIdentidad{}, domain.ErrSolicitudIdentidadInvalida
	}
	if err := p.autorizador.Autorizar(ctx, copiarSolicitudIdentidad(solicitud)); err != nil {
		return domain.RetoIdentidad{}, domain.ErrSolicitudIdentidadInvalida
	}
	canon, err := p.canonicalizador.Canonicalizar(solicitud)
	if err != nil || len(canon) == 0 || len(canon) > maximoContenidoCanonicoIdentidad {
		return domain.RetoIdentidad{}, ErrDependenciaIdentidad
	}
	reto := domain.RetoIdentidad{Solicitud: copiarSolicitudIdentidad(solicitud), ContenidoCanonico: append([]byte(nil), canon...)}
	if err := p.repositorio.Registrar(ctx, reto); err != nil {
		return domain.RetoIdentidad{}, ErrDependenciaIdentidad
	}
	return copiarRetoIdentidad(reto), nil
}

// ConfirmadorIdentidad reserva antes de verificar y finaliza después de la llamada externa.
type ConfirmadorIdentidad struct {
	repositorio ports.RepositorioRetosIdentidad
	verificador ports.VerificadorPruebaIdentidad
	reloj       ports.Clock
}

// NuevoConfirmadorIdentidad construye el caso de uso únicamente con puertos.
func NuevoConfirmadorIdentidad(r ports.RepositorioRetosIdentidad, v ports.VerificadorPruebaIdentidad, reloj ports.Clock) (*ConfirmadorIdentidad, error) {
	if r == nil || v == nil || reloj == nil {
		return nil, ErrDependenciaIdentidad
	}
	return &ConfirmadorIdentidad{repositorio: r, verificador: v, reloj: reloj}, nil
}

// Confirmar consume el reto y conserva rechazo e indeterminación sin elevarlos.
func (c *ConfirmadorIdentidad) Confirmar(ctx context.Context, prueba domain.PruebaIdentidad) (domain.ResultadoVerificacionIdentidad, error) {
	if ctx == nil || prueba.Validar() != nil {
		return domain.ResultadoVerificacionIdentidad{}, domain.ErrPruebaIdentidadInvalida
	}
	reto, err := c.repositorio.Reservar(ctx, prueba.RetoID, c.reloj.Now().UTC())
	if err != nil || reto.Solicitud.RetoID != prueba.RetoID {
		return domain.ResultadoVerificacionIdentidad{}, ErrRetoIdentidadNoDisponible
	}
	resultado, causa := c.verificador.Verificar(ctx, copiarRetoIdentidad(reto), copiarPruebaIdentidad(prueba))
	resultadoValido := resultado.ValidarEstructura() == nil && resultado.Solicitud.RetoID == reto.Solicitud.RetoID
	if causa != nil && (!resultadoValido || resultado.Resultado != domain.ResultadoIdentidadIndeterminada) {
		resultado = resultadoIndeterminado(reto)
	} else if causa == nil && !resultadoValido {
		resultado = resultadoIndeterminado(reto)
	}
	if err := c.repositorio.Finalizar(ctx, resultado); err != nil {
		return domain.ResultadoVerificacionIdentidad{}, ErrDependenciaIdentidad
	}
	if causa != nil {
		return resultado, ErrDependenciaIdentidad
	}
	return resultado, nil
}

func resultadoIndeterminado(reto domain.RetoIdentidad) domain.ResultadoVerificacionIdentidad {
	return domain.ResultadoVerificacionIdentidad{Resultado: domain.ResultadoIdentidadIndeterminada, EvidenciaRef: reto.Solicitud.RetoID, RetoID: reto.Solicitud.RetoID, Solicitud: copiarSolicitudIdentidad(reto.Solicitud)}
}

func copiarSolicitudIdentidad(s domain.SolicitudRetoIdentidad) domain.SolicitudRetoIdentidad {
	s.Nonce = append([]byte(nil), s.Nonce...)
	s.EmitidoEn = s.EmitidoEn.UTC()
	s.ExpiraEn = s.ExpiraEn.UTC()
	return s
}

func copiarRetoIdentidad(r domain.RetoIdentidad) domain.RetoIdentidad {
	r.Solicitud = copiarSolicitudIdentidad(r.Solicitud)
	r.ContenidoCanonico = append([]byte(nil), r.ContenidoCanonico...)
	return r
}

func copiarPruebaIdentidad(p domain.PruebaIdentidad) domain.PruebaIdentidad {
	p.Firma = append([]byte(nil), p.Firma...)
	p.Certificado = append([]byte(nil), p.Certificado...)
	p.Cadena = copiarMatrizBytes(p.Cadena)
	return p
}

func copiarMatrizBytes(origen [][]byte) [][]byte {
	destino := make([][]byte, len(origen))
	for i := range origen {
		destino[i] = append([]byte(nil), origen[i]...)
	}
	return destino
}
