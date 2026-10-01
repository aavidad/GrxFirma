// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package keyringstore implementa ports.SecureStorage sobre el almacén de
// secretos nativo de cada plataforma (T057):
//
//   - Linux:   Secret Service por D-Bus (GNOME Keyring, KWallet…)
//   - macOS:   Keychain
//   - Windows: Credential Manager (respaldado por DPAPI)
//
// Usa github.com/zalando/go-keyring, que habla D-Bus en Go puro en Linux.
// El material se codifica en base64 porque los almacenes nativos trabajan
// con cadenas de texto.
package keyringstore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"grxfirma/internal/ports"
)

// ServicioPorDefecto es el nombre bajo el que se agrupan los secretos de la
// aplicación en el almacén nativo.
const ServicioPorDefecto = "GrxFirma"

// ErrNoEncontrado se retorna cuando la clave pedida no existe en el almacén.
var ErrNoEncontrado = errors.New("keyringstore: secreto no encontrado")

// Almacen implementa ports.SecureStorage sobre el almacén nativo.
type Almacen struct {
	servicio string
}

// New crea un Almacen con el nombre de servicio por defecto.
func New() *Almacen {
	return &Almacen{servicio: ServicioPorDefecto}
}

// NewConServicio permite aislar los secretos bajo otro nombre de servicio
// (p. ej. tests o instalaciones multi-perfil).
func NewConServicio(servicio string) *Almacen {
	if servicio == "" {
		servicio = ServicioPorDefecto
	}
	return &Almacen{servicio: servicio}
}

// Store guarda value bajo key. Sobrescribe si ya existía.
func (a *Almacen) Store(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" {
		return errors.New("keyringstore: la clave no puede estar vacía")
	}
	if err := keyring.Set(a.servicio, key, base64.StdEncoding.EncodeToString(value)); err != nil {
		return fmt.Errorf("keyringstore: guardando %q: %w", key, err)
	}
	return nil
}

// Load recupera el valor guardado bajo key. Si no existe retorna
// ErrNoEncontrado (comprobable con errors.Is).
func (a *Almacen) Load(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	codificado, err := keyring.Get(a.servicio, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("%w: %q", ErrNoEncontrado, key)
		}
		return nil, fmt.Errorf("keyringstore: leyendo %q: %w", key, err)
	}
	value, err := base64.StdEncoding.DecodeString(codificado)
	if err != nil {
		return nil, fmt.Errorf("keyringstore: el secreto %q no tiene el formato esperado: %w", key, err)
	}
	return value, nil
}

// Delete elimina el secreto guardado bajo key. Borrar una clave inexistente
// no es un error.
func (a *Almacen) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := keyring.Delete(a.servicio, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keyringstore: borrando %q: %w", key, err)
	}
	return nil
}

var _ ports.SecureStorage = (*Almacen)(nil)
