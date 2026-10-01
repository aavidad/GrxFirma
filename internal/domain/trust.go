// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "errors"

// TrustStatus representa el estado de una decision de confianza sobre un origen.
type TrustStatus string

const (
	TrustAllowed TrustStatus = "allowed"
	TrustDenied  TrustStatus = "denied"
	TrustPending TrustStatus = "pending"
)

// TrustDecision es el resultado de evaluar la confianza sobre un origen (dominio, host, emisor).
type TrustDecision struct {
	Origin string
	Status TrustStatus
	Reason string
}

func (t TrustDecision) Validate() error {
	if t.Origin == "" {
		return errors.New("la decision de confianza debe tener un origen")
	}
	switch t.Status {
	case TrustAllowed, TrustDenied, TrustPending:
		return nil
	default:
		return errors.New("estado de confianza no valido: " + string(t.Status))
	}
}

func (t TrustDecision) IsAllowed() bool {
	return t.Status == TrustAllowed
}
