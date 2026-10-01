// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"crypto/x509"
)

// LocalSigningKey es una implementación simple de ports.SigningKey respaldada por
// una clave privada en memoria y su certificado asociado.
type LocalSigningKey struct {
	ID          string
	Signer      crypto.Signer
	Certificate *x509.Certificate
	Chain       []*x509.Certificate
}

// KeyID devuelve el identificador opaco de la clave.
func (k *LocalSigningKey) KeyID() string {
	if k == nil {
		return ""
	}
	return k.ID
}

func (k *LocalSigningKey) CertificateChainDER() [][]byte {
	if k == nil || k.Certificate == nil {
		return nil
	}
	out := make([][]byte, 0, 1+len(k.Chain))
	out = append(out, k.Certificate.Raw)
	for _, c := range k.Chain {
		if c != nil {
			out = append(out, c.Raw)
		}
	}
	return out
}
