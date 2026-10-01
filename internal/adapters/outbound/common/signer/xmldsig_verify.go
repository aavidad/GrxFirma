// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"

	"grxfirma/internal/domain"
)

type XMLDSigVerifier struct{}

func NewXMLDSigVerifier() *XMLDSigVerifier {
	return &XMLDSigVerifier{}
}

func (v *XMLDSigVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	verification, err := verifyXMLSignatureDocument(signedDocument.Content, false, nil, anchors, "XMLdSig", "firma XMLdSig valida", []string{"referencias=ok", "perfil=xmldsig-basic"})
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	return applyXMLRevocation(ctx, verification.result, verification.signerCertificates, verification.embeddedCertificates), verification.signers, nil
}
