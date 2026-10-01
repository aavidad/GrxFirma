// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package systemtrust conecta la verificación de firmas con el almacén raíz
// nativo de la plataforma sin convertir certificados de usuario en anclas.
package systemtrust

import (
	"context"

	"grxfirma/internal/domain"
)

// Provider solicita al verificador el pool raíz del sistema operativo.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Anchors(ctx context.Context) (domain.CertificateChain, error) {
	if err := ctx.Err(); err != nil {
		return domain.CertificateChain{}, err
	}
	return domain.CertificateChain{UseSystemRoots: true}, nil
}
