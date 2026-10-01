// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package identitypolicy autoriza retos mediante un catálogo inmutable y exacto.
package identitypolicy

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	// ErrConfiguracionInvalida impide arrancar con políticas ambiguas.
	ErrConfiguracionInvalida = errors.New("configuración de política de identidad inválida")
	// ErrSolicitudNoAutorizada oculta qué atributo o política no coincide.
	ErrSolicitudNoAutorizada = errors.New("solicitud de identidad no autorizada")
)

// Registro fija una combinación autorizada sin opciones libres en cada petición.
type Registro struct {
	HuellaContextoTenant, ClienteRegistrado, Operacion, Audiencia, Finalidad string
	Origen, ConsentimientoID, VersionConsentimiento                          string
	PoliticaID, VersionPolitica                                              string
	VigenciaMaxima, DesfaseMaximo                                            time.Duration
}

type clave struct{ tenant, cliente, operacion string }

// Catalogo contiene una instantánea de configuración solo lectura.
type Catalogo struct {
	registros map[clave]Registro
	reloj     ports.Clock
}

// NuevoCatalogo copia y valida cada registro, sin fallback global.
func NuevoCatalogo(registros []Registro, reloj ports.Clock) (*Catalogo, error) {
	if len(registros) == 0 || reloj == nil {
		return nil, ErrConfiguracionInvalida
	}
	indice := make(map[clave]Registro, len(registros))
	for _, registro := range registros {
		if !registroValido(registro) {
			return nil, ErrConfiguracionInvalida
		}
		k := clave{registro.HuellaContextoTenant, registro.ClienteRegistrado, registro.Operacion}
		if _, existe := indice[k]; existe {
			return nil, ErrConfiguracionInvalida
		}
		indice[k] = registro
	}
	return &Catalogo{registros: indice, reloj: reloj}, nil
}

// Autorizar exige coincidencia exacta y vigencia temporal en el reloj del agente.
func (c *Catalogo) Autorizar(ctx context.Context, solicitud domain.SolicitudRetoIdentidad) error {
	if c == nil || ctx == nil || ctx.Err() != nil || solicitud.Validar() != nil {
		return ErrSolicitudNoAutorizada
	}
	registro, existe := c.registros[clave{solicitud.HuellaContextoTenant, solicitud.ClienteRegistrado, solicitud.Operacion}]
	if !existe || !coincide(registro, solicitud) {
		return ErrSolicitudNoAutorizada
	}
	vigencia := solicitud.ExpiraEn.Sub(solicitud.EmitidoEn)
	ahora := c.reloj.Now().UTC()
	if vigencia <= 0 || vigencia > registro.VigenciaMaxima || ahora.Before(solicitud.EmitidoEn.Add(-registro.DesfaseMaximo)) || !ahora.Before(solicitud.ExpiraEn) {
		return ErrSolicitudNoAutorizada
	}
	return nil
}

func coincide(r Registro, s domain.SolicitudRetoIdentidad) bool {
	return r.Audiencia == s.Audiencia && r.Finalidad == s.Finalidad && r.Origen == s.Origen &&
		r.ConsentimientoID == s.ConsentimientoID && r.VersionConsentimiento == s.VersionConsentimiento &&
		r.PoliticaID == s.PoliticaID && r.VersionPolitica == s.VersionPolitica
}

func registroValido(r Registro) bool {
	for _, texto := range []string{r.HuellaContextoTenant, r.ClienteRegistrado, r.Operacion,
		r.Audiencia, r.Finalidad, r.Origen, r.ConsentimientoID, r.VersionConsentimiento,
		r.PoliticaID, r.VersionPolitica} {
		if texto == "" || texto != strings.TrimSpace(texto) || !utf8.ValidString(texto) ||
			len(texto) > 512 || strings.IndexFunc(texto, unicode.IsControl) >= 0 {
			return false
		}
	}
	return r.VigenciaMaxima >= 15*time.Second && r.VigenciaMaxima <= 10*time.Minute &&
		r.DesfaseMaximo >= 0 && r.DesfaseMaximo <= time.Minute
}

var _ ports.AutorizadorSolicitudIdentidad = (*Catalogo)(nil)
