// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"errors"

	"grxfirma/internal/ports"
)

// AutorizadorLote lo implementa una clave remota que puede autorizar de una
// vez las firmas de varios documentos (firma CSC con multisign).
type AutorizadorLote interface {
	// CapacidadLote dice cuántos documentos de un lote de total se
	// autorizan juntos; 1 significa uno a uno.
	CapacidadLote(total int) (int, error)
	// IniciarLote prepara la autorización conjunta de n documentos.
	IniciarLote(ctx context.Context, n int) (FirmantesLote, error)
}

// FirmantesLote da un crypto.Signer por documento del grupo.
type FirmantesLote interface {
	Firmante(i int) crypto.Signer
	Terminar(i int)
	Cerrar()
}

// ConAutorizadorLote asocia a la clave la autorización conjunta de lotes.
func (c *ClaveLocal) ConAutorizadorLote(a AutorizadorLote) *ClaveLocal {
	c.lote = a
	return c
}

// BatchCapacity implementa ports.BatchSigningKey. Una clave sin autorizador
// de lotes firma uno a uno.
func (c *ClaveLocal) BatchCapacity(total int) (int, error) {
	if c == nil || c.lote == nil {
		return 1, nil
	}
	return c.lote.CapacidadLote(total)
}

// BeginBatch implementa ports.BatchSigningKey. Cada documento recibe una
// ClaveLocal con el mismo certificado y cadena y su propio firmante.
func (c *ClaveLocal) BeginBatch(ctx context.Context, n int) (ports.SigningBatch, error) {
	if c == nil || c.lote == nil {
		return nil, errors.New("la clave no admite autorizar lotes")
	}
	firmantes, err := c.lote.IniciarLote(ctx, n)
	if err != nil {
		return nil, err
	}
	return &loteClaves{base: c, firmantes: firmantes}, nil
}

type loteClaves struct {
	base      *ClaveLocal
	firmantes FirmantesLote
}

func (l *loteClaves) Key(i int) ports.SigningKey {
	return NuevaClaveLocalConCadena(l.firmantes.Firmante(i), l.base.cert, l.base.chain)
}

func (l *loteClaves) Done(i int) { l.firmantes.Terminar(i) }

func (l *loteClaves) Close() { l.firmantes.Cerrar() }

var _ ports.BatchSigningKey = (*ClaveLocal)(nil)
