// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build !linux

package servicemanager

import (
	"context"

	"grxfirma/internal/ports"
)

type gestorNoSoportado struct{}

// New construye un gestor de servicio no soportado para plataformas sin
// implementación específica.
func New(_ string) ports.GestorServicio {
	return gestorNoSoportado{}
}

func (gestorNoSoportado) Estado(context.Context) (ports.EstadoServicio, error) {
	return ports.EstadoServicio{
		Instalado:  false,
		Activo:     false,
		Plataforma: "unsupported",
		Metodo:     "unsupported",
	}, ErrNoSoportado
}

func (gestorNoSoportado) Instalar(context.Context, string) error { return ErrNoSoportado }
func (gestorNoSoportado) Desinstalar(context.Context) error      { return ErrNoSoportado }
func (gestorNoSoportado) Iniciar(context.Context) error          { return ErrNoSoportado }
func (gestorNoSoportado) Detener(context.Context) error          { return ErrNoSoportado }
