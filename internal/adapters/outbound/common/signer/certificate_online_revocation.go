// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
)

type CertificateOnlineRevocationStatus string

var errOnlineIssuerAIA = errors.New("no se pudo obtener el certificado emisor por AIA")

const (
	CertificateOnlineRevocationValid        CertificateOnlineRevocationStatus = "valid"
	CertificateOnlineRevocationRevoked      CertificateOnlineRevocationStatus = "revoked"
	CertificateOnlineRevocationInconclusive CertificateOnlineRevocationStatus = "inconclusive"
	CertificateOnlineRevocationUnavailable  CertificateOnlineRevocationStatus = "unavailable"
)

type CertificateOnlineRevocationResult struct {
	Status      CertificateOnlineRevocationStatus
	UserMessage string
	Reason      string
	Method      string
	CheckedAt   time.Time
	RevokedAt   time.Time
	OCSPURL     string
	CRLURL      string
}

func CheckCertificateOnlineRevocation(ctx context.Context, chainDER [][]byte) (CertificateOnlineRevocationResult, error) {
	leaf, issuer, err := resolveOnlineCertificateChain(ctx, chainDER, revocationclient.New().FetchIssuer)
	if err != nil {
		return unavailableOnlineCertificate(err), nil
	}

	checker := NewRevocationChecker()
	revocation, err := checker.Check(ctx, leaf, issuer)
	if err != nil {
		return CertificateOnlineRevocationResult{
			Status:      CertificateOnlineRevocationUnavailable,
			UserMessage: "No se pudo comprobar online el estado del certificado.",
			Reason:      err.Error(),
			OCSPURL:     firstCertificateURL(leaf.OCSPServer),
			CRLURL:      firstCertificateURL(leaf.CRLDistributionPoints),
		}, nil
	}

	result := CertificateOnlineRevocationResult{
		Method:    strings.ToUpper(strings.TrimSpace(revocation.Method)),
		CheckedAt: revocation.CheckedAt,
		RevokedAt: revocation.RevokedAt,
		Reason:    strings.TrimSpace(revocation.Reason),
		OCSPURL:   firstCertificateURL(leaf.OCSPServer),
		CRLURL:    firstCertificateURL(leaf.CRLDistributionPoints),
	}

	switch revocation.Status {
	case RevocationGood:
		result.Status = CertificateOnlineRevocationValid
		result.UserMessage = "No consta revocación del certificado en la comprobación online."
	case RevocationRevoked:
		result.Status = CertificateOnlineRevocationRevoked
		result.UserMessage = "El certificado figura como revocado en la comprobación online."
	default:
		if result.OCSPURL == "" && result.CRLURL == "" {
			result.Status = CertificateOnlineRevocationInconclusive
			result.UserMessage = "El certificado no publica un servicio de revocación online utilizable."
			if result.Reason == "" {
				result.Reason = "sin OCSP ni CRL disponibles"
			}
		} else {
			result.Status = CertificateOnlineRevocationInconclusive
			result.UserMessage = "La comprobación online no ha podido confirmar si el certificado está revocado."
			if result.Reason == "" {
				result.Reason = "sin respuesta concluyente del servicio de revocación"
			}
		}
	}

	return result, nil
}

func resolveOnlineCertificateChain(ctx context.Context, chainDER [][]byte, fetchIssuer func(context.Context, *x509.Certificate) (*x509.Certificate, error)) (*x509.Certificate, *x509.Certificate, error) {
	leaf, issuer, err := parseCertificateOnlineRevocationChain(chainDER)
	if err != nil || issuer != nil {
		return leaf, issuer, err
	}
	issuer, err = fetchIssuer(ctx, leaf)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errOnlineIssuerAIA, err)
	}
	return leaf, issuer, nil
}

func unavailableOnlineCertificate(err error) CertificateOnlineRevocationResult {
	message := "No se pudo leer la cadena del certificado seleccionado."
	if errors.Is(err, errOnlineIssuerAIA) {
		message = "No se pudo obtener el certificado emisor indicado por el certificado. Revise la conexión de red o consulte al emisor."
	}
	return CertificateOnlineRevocationResult{
		Status:      CertificateOnlineRevocationUnavailable,
		UserMessage: message,
		Reason:      err.Error(),
	}
}

func parseCertificateOnlineRevocationChain(chainDER [][]byte) (*x509.Certificate, *x509.Certificate, error) {
	if len(chainDER) == 0 {
		return nil, nil, fmt.Errorf("cadena de certificados no disponible")
	}
	leaf, err := x509.ParseCertificate(chainDER[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parseando certificado titular: %w", err)
	}
	if len(chainDER) > 1 {
		issuer, err := x509.ParseCertificate(chainDER[1])
		if err != nil {
			return nil, nil, fmt.Errorf("parseando certificado emisor: %w", err)
		}
		if leaf.CheckSignatureFrom(issuer) != nil {
			return nil, nil, fmt.Errorf("el certificado de la cadena no firma al titular")
		}
		return leaf, issuer, nil
	}
	if leaf.CheckSignatureFrom(leaf) == nil {
		return leaf, leaf, nil
	}
	return leaf, nil, nil
}

func firstCertificateURL(values []string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
