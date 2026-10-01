// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// formatPKCS1 es el formato "NONE"/"PKCS1" de AutoFirma Java: la firma
// PKCS#1 (o ECDSA) de los datos, sin contenedor CMS ni XML. Lo usan
// aplicaciones que montan su propio formato o validan la firma en servidor.
const formatPKCS1 = domain.SignatureFormat("PKCS1")

type PKCS1Signer struct{}

func NewPKCS1Signer() *PKCS1Signer { return &PKCS1Signer{} }

func (s *PKCS1Signer) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != formatPKCS1 {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", formatPKCS1)
	}
	if job.Action != domain.ActionSign {
		return domain.SignatureResult{}, errors.New("una firma PKCS#1 no admite cofirma ni contrafirma")
	}
	clave, err := requireLocalSigningKey(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	spec := cmsHashFromOptions(job.Options)
	firma, err := clave.Signer.Sign(rand.Reader, cmsDigest(spec.hash, job.Document.Content), spec.hash)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error en la firma PKCS#1: %w", err)
	}
	return domain.SignatureResult{Format: formatPKCS1, Data: firma, Algorithm: etiquetaAlgoritmoCMS(clave, spec)}, nil
}
