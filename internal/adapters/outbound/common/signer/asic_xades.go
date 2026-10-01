// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/asiccontainer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const formatASiCXAdES = domain.SignatureFormat("ASiC-XAdES")

type ASiCXAdESSigner struct{}
type ASiCXAdESVerifier struct{}

func NewASiCXAdESSigner() *ASiCXAdESSigner     { return &ASiCXAdESSigner{} }
func NewASiCXAdESVerifier() *ASiCXAdESVerifier { return &ASiCXAdESVerifier{} }

func (s *ASiCXAdESSigner) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != formatASiCXAdES {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", formatASiCXAdES)
	}
	if job.Action != domain.ActionSign {
		return domain.SignatureResult{}, errors.New("ASiC-XAdES solo soporta la accion sign")
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	payloadName := asiccontainer.EnsureDataFilename(valorOpcion(job.Options, "asicsFilename"))
	if strings.TrimSpace(payloadName) == "" || payloadName == "dataobject.bin" {
		payloadName = asiccontainer.EnsureDataFilename(job.Document.Name)
	}

	signatureXML, err := buildASiCXAdESSignatureXML(clave, payloadName, job.Document.Content, normalizeMimeType(job.Document.MIMEType, job.Document.Content), time.Now().UTC())
	if err != nil {
		return domain.SignatureResult{}, err
	}
	container, err := asiccontainer.CreateXAdESContainer([]byte(signatureXML), job.Document.Content, payloadName)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	return domain.SignatureResult{
		Format:    formatASiCXAdES,
		Data:      container,
		Algorithm: "ASiC-XAdES-RSA-SHA256",
	}, nil
}

func (v *ASiCXAdESVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	signatureXML, err := asiccontainer.ExtractXAdESSignature(signedDocument.Content)
	if err != nil {
		// CAdES-ASiC-S: el mismo contenedor con META-INF/signature.p7s.
		if cms, cmsErr := asiccontainer.ExtractCAdESSignature(signedDocument.Content); cmsErr == nil {
			return verifyASiCCAdES(ctx, cms, signedDocument.Content, anchors)
		}
		return domain.VerificationResult{}, nil, err
	}
	payload, payloadName, err := asiccontainer.ExtractData(signedDocument.Content)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	resolver := func(uri string) ([]byte, error) {
		trimmed := strings.TrimSpace(strings.TrimPrefix(uri, "/"))
		if trimmed == "" || trimmed == "." || strings.EqualFold(trimmed, payloadName) {
			return payload, nil
		}
		return nil, fmt.Errorf("recurso ASiC no encontrado: %s", uri)
	}
	verification, err := verifyXMLSignatureDocument(signatureXML, true, resolver, anchors, string(formatASiCXAdES), "firma ASiC-XAdES valida", []string{"contenedor=asics", fmt.Sprintf("payload=%s", payloadName)})
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	return applyXMLRevocation(ctx, verification.result, verification.signerCertificates, verification.embeddedCertificates), verification.signers, nil
}

func buildASiCXAdESSignatureXML(key *LocalSigningKey, payloadName string, payload []byte, mimeType string, now time.Time) (string, error) {
	sigID := fmt.Sprintf("Signature-%d", now.UnixNano())
	keyInfoID := fmt.Sprintf("KeyInfo-%d", now.UnixNano())
	signedPropsID := fmt.Sprintf("SignedProperties-%d", now.UnixNano())
	referenceID := fmt.Sprintf("Reference-%d", now.UnixNano())
	buildOpts := resolveXAdESBuildOptions(nil, xadesAlgorithmOptions{
		Label:           "SHA256withRSA",
		Hash:            crypto.SHA256,
		SignatureMethod: algRSASHA256,
		DigestMethod:    algSHA256,
	})

	documentDigestInput := payload
	if canonical, canonicalErr := canonicalizeXML(payload, algExcC14N); canonicalErr == nil {
		// La referencia anuncia C14N solo cuando el payload es XML bien
		// formado y el digest se calcula exactamente sobre esa salida.
		documentDigestInput = canonical
		buildOpts.DocumentTransform = algExcC14N
	} else {
		// ASiC-S también admite datos binarios. En ese caso la referencia
		// externa se firma como octetos y no debe anunciar una transformación
		// XML que el verificador no podría aplicar.
		buildOpts.OmitDocumentTransform = true
	}
	docDigest := sha256.Sum256(documentDigestInput)
	docDigestB64 := base64.StdEncoding.EncodeToString(docDigest[:])

	certDER := key.Certificate.Raw
	certDigest := sha256.Sum256(certDER)
	certDigestB64 := base64.StdEncoding.EncodeToString(certDigest[:])
	issuerSerialV2B64, err := buildIssuerSerialV2(key.Certificate)
	if err != nil {
		return "", err
	}

	signedPropsXML := buildSignedPropertiesXML(signedPropsID, sigID, now.Format("2006-01-02T15:04:05Z"), certDigestB64, issuerSerialV2B64, referenceID, mimeType, buildOpts)
	signedPropsDigest := sha256.Sum256(mustExclusiveC14N([]byte(signedPropsXML)))
	signedPropsDigestB64 := base64.StdEncoding.EncodeToString(signedPropsDigest[:])

	keyInfoXML := buildKeyInfoXML(keyInfoID, base64.StdEncoding.EncodeToString(certDER))
	keyInfoDigest := sha256.Sum256(mustExclusiveC14N([]byte(keyInfoXML)))
	keyInfoDigestB64 := base64.StdEncoding.EncodeToString(keyInfoDigest[:])

	signedInfoXML := buildSignedInfoXML(payloadName, referenceID, docDigestB64, keyInfoID, keyInfoDigestB64, signedPropsDigestB64, signedPropsID, buildOpts)
	signedInfoC14N, err := exclusiveC14N(signedInfoXML)
	if err != nil {
		return "", err
	}
	hashSignedInfo := sha256.Sum256(signedInfoC14N)
	sigBytes, err := key.Signer.Sign(rand.Reader, hashSignedInfo[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><asic:XAdESSignatures xmlns:asic="http://uri.etsi.org/02918/v1.2.1#" xmlns:ds="%s" xmlns:xades="%s"><ds:Signature Id="%s">%s<ds:SignatureValue>%s</ds:SignatureValue>%s<ds:Object><xades:QualifyingProperties Target="#%s">%s</xades:QualifyingProperties></ds:Object></ds:Signature></asic:XAdESSignatures>`,
		nsXMLDSig,
		nsXAdES,
		sigID,
		signedInfoXML,
		base64.StdEncoding.EncodeToString(sigBytes),
		keyInfoXML,
		sigID,
		signedPropsXML,
	), nil
}

var _ ports.SignerEngine = (*ASiCXAdESSigner)(nil)
var _ ports.VerifierEngine = (*ASiCXAdESVerifier)(nil)
