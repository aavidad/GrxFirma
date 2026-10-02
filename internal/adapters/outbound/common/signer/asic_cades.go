// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/adapters/outbound/common/asiccontainer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// formatASiCCAdES es el CAdES-ASiC-S de AutoFirma Java: contenedor ASiC-S
// con el dato y su firma CAdES explícita en META-INF/signature.p7s.
const formatASiCCAdES = domain.SignatureFormat("ASiC-CAdES")

type ASiCCAdESSigner struct{}

func NewASiCCAdESSigner() *ASiCCAdESSigner { return &ASiCCAdESSigner{} }

func (s *ASiCCAdESSigner) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != formatASiCCAdES {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", formatASiCCAdES)
	}
	if job.Action != domain.ActionSign {
		return domain.SignatureResult{}, errors.New("CAdES-ASiC-S solo admite firma; para cofirmar, firme de nuevo el dato")
	}
	clave, err := requireLocalSigningKey(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	payloadName := asiccontainer.EnsureDataFilename(valorOpcion(job.Options, "asicsFilename"))
	if strings.TrimSpace(payloadName) == "" || payloadName == "dataobject.bin" {
		payloadName = asiccontainer.EnsureDataFilename(job.Document.Name)
	}
	cms, algoritmo, err := signCAdESBES(job.Document.Content, clave, cmsHashFromOptions(job.Options), false)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	container, err := asiccontainer.CreateCAdESContainer(cms, job.Document.Content, payloadName)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	return domain.SignatureResult{Format: formatASiCCAdES, Data: container, Algorithm: "ASiC-CAdES-" + algoritmo}, nil
}

func verifyASiCCAdES(ctx context.Context, cms, container []byte, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	payload, payloadName, err := asiccontainer.ExtractData(container)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	verifier := NewCAdESVerifier()
	if offlineVerification(ctx) {
		verifier = NewCAdESVerifierWithChecker(NewRevocationCheckerOffline())
	}
	result, signers, err := verifier.VerifyDetachedCMSWithAnchors(ctx, cms, payload, anchors)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	result.Format = string(formatASiCCAdES)
	result.Details = append([]string{"contenedor=asics", "firma=cades", fmt.Sprintf("payload=%s", payloadName)}, result.Details...)
	return result, signers, nil
}
