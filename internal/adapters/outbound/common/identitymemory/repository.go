// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package identitymemory conserva retos efímeros de identidad de un solo uso.
package identitymemory

import (
	"context"
	"errors"
	"sync"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	// ErrConfiguracionInvalida rechaza capacidades sin límites operativos.
	ErrConfiguracionInvalida = errors.New("configuración del repositorio de identidad inválida")
	// ErrRetoNoDisponible oculta duplicados, cruces, caducidad y consumo previo.
	ErrRetoNoDisponible = errors.New("reto de identidad no disponible")
	// ErrCapacidadAgotada impide crecer sin límite ante abuso local.
	ErrCapacidadAgotada = errors.New("capacidad de retos de identidad agotada")
)

type estadoReto uint8

const (
	estadoPendiente estadoReto = iota + 1
	estadoVerificando
	estadoFinalizado
)

type entrada struct {
	reto      domain.RetoIdentidad
	estado    estadoReto
	resultado domain.ResultadoVerificacionIdentidad
}

// Repositorio implementa una instantánea concurrente acotada en memoria.
type Repositorio struct {
	mu               sync.Mutex
	maximoPendientes int
	retencionFinal   time.Duration
	retos            map[string]entrada
}

// Nuevo valida límites y construye un almacén vacío.
func Nuevo(maximoPendientes int, retencionFinal time.Duration) (*Repositorio, error) {
	if maximoPendientes <= 0 || maximoPendientes > 4096 || retencionFinal <= 0 || retencionFinal > 24*time.Hour {
		return nil, ErrConfiguracionInvalida
	}
	return &Repositorio{maximoPendientes: maximoPendientes, retencionFinal: retencionFinal, retos: make(map[string]entrada)}, nil
}

// Registrar guarda una copia pendiente y rechaza identificadores repetidos.
func (r *Repositorio) Registrar(ctx context.Context, reto domain.RetoIdentidad) error {
	if err := validarContextoYReto(ctx, reto); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limpiar(reto.Solicitud.EmitidoEn)
	if _, existe := r.retos[reto.Solicitud.RetoID]; existe {
		return ErrRetoNoDisponible
	}
	if r.pendientes() >= r.maximoPendientes {
		return ErrCapacidadAgotada
	}
	r.retos[reto.Solicitud.RetoID] = entrada{reto: copiarReto(reto), estado: estadoPendiente}
	return nil
}

// Reservar consume atómicamente el único uso antes de una llamada externa.
func (r *Repositorio) Reservar(ctx context.Context, retoID string, ahora time.Time) (domain.RetoIdentidad, error) {
	if ctx == nil || ctx.Err() != nil || retoID == "" || ahora.IsZero() {
		return domain.RetoIdentidad{}, ErrRetoNoDisponible
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limpiar(ahora)
	actual, existe := r.retos[retoID]
	if !existe || actual.estado != estadoPendiente || !ahora.Before(actual.reto.Solicitud.ExpiraEn) {
		return domain.RetoIdentidad{}, ErrRetoNoDisponible
	}
	actual.estado = estadoVerificando
	r.retos[retoID] = actual
	return copiarReto(actual.reto), nil
}

// Finalizar fija una única salida terminal y conserva solo evidencia acotada.
func (r *Repositorio) Finalizar(ctx context.Context, resultado domain.ResultadoVerificacionIdentidad) error {
	if ctx == nil || ctx.Err() != nil || resultado.ValidarEstructura() != nil {
		return ErrRetoNoDisponible
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	actual, existe := r.retos[resultado.RetoID]
	if !existe || actual.estado != estadoVerificando || actual.reto.Solicitud.RetoID != resultado.Solicitud.RetoID {
		return ErrRetoNoDisponible
	}
	actual.estado = estadoFinalizado
	actual.resultado = copiarResultado(resultado)
	r.retos[resultado.RetoID] = actual
	return nil
}

func (r *Repositorio) pendientes() int {
	total := 0
	for _, actual := range r.retos {
		if actual.estado == estadoPendiente {
			total++
		}
	}
	return total
}

func (r *Repositorio) limpiar(ahora time.Time) {
	for id, actual := range r.retos {
		limite := actual.reto.Solicitud.ExpiraEn
		if actual.estado == estadoFinalizado {
			limite = limite.Add(r.retencionFinal)
		}
		if !ahora.Before(limite) {
			delete(r.retos, id)
		}
	}
}

func validarContextoYReto(ctx context.Context, reto domain.RetoIdentidad) error {
	if ctx == nil || ctx.Err() != nil || reto.Solicitud.Validar() != nil || len(reto.ContenidoCanonico) == 0 || len(reto.ContenidoCanonico) > 16*1024 {
		return ErrRetoNoDisponible
	}
	return nil
}

func copiarReto(reto domain.RetoIdentidad) domain.RetoIdentidad {
	reto.Solicitud.Nonce = append([]byte(nil), reto.Solicitud.Nonce...)
	reto.ContenidoCanonico = append([]byte(nil), reto.ContenidoCanonico...)
	return reto
}

func copiarResultado(resultado domain.ResultadoVerificacionIdentidad) domain.ResultadoVerificacionIdentidad {
	resultado.Solicitud.Nonce = append([]byte(nil), resultado.Solicitud.Nonce...)
	return resultado
}

var _ ports.RepositorioRetosIdentidad = (*Repositorio)(nil)
