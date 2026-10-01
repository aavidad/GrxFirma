// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/ocsp"
)

const (
	revocationClockSkew         = 5 * time.Minute
	maxAgeWithoutNextUpdate     = 24 * time.Hour
	ocspResponderSigningUsageID = x509.ExtKeyUsageOCSPSigning
)

var oidOCSPBasicResponse = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 1}

// ValidationResult describe el estado contenido en una evidencia cuya firma,
// identidad y ventana temporal ya han sido verificadas.
type ValidationResult struct {
	Status    CertificateStatus
	RevokedAt time.Time
}

func validateCertificateIssuer(cert, issuer *x509.Certificate) error {
	if cert == nil || issuer == nil {
		return errors.New("certificado o emisor nulo")
	}
	if !bytes.Equal(cert.RawIssuer, issuer.RawSubject) {
		return errors.New("el nombre del emisor no corresponde al certificado")
	}
	if err := cert.CheckSignatureFrom(issuer); err != nil {
		return fmt.Errorf("el certificado no fue emitido por el emisor indicado: %w", err)
	}
	if len(cert.AuthorityKeyId) > 0 &&
		len(issuer.SubjectKeyId) > 0 &&
		!bytes.Equal(cert.AuthorityKeyId, issuer.SubjectKeyId) {
		return errors.New("el identificador de clave del emisor no corresponde al certificado")
	}
	return nil
}

// ValidateOCSPResponse autentica una respuesta OCSP completa y devuelve el
// BasicOCSPResponse listo para ser embebido como evidencia LT/LTA.
func ValidateOCSPResponse(
	body []byte,
	cert *x509.Certificate,
	issuer *x509.Certificate,
	at time.Time,
) (ValidationResult, []byte, error) {
	if err := validateCertificateIssuer(cert, issuer); err != nil {
		return ValidationResult{}, nil, err
	}
	if at.IsZero() {
		return ValidationResult{}, nil, errors.New("instante de validación OCSP nulo")
	}

	basicDER, err := extractBasicOCSPResponse(body)
	if err != nil {
		return ValidationResult{}, nil, err
	}
	response, err := ocsp.ParseResponseForCert(body, cert, issuer)
	if err != nil {
		return ValidationResult{}, nil, fmt.Errorf("respuesta OCSP no autenticada: %w", err)
	}
	if response.SerialNumber == nil || response.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		return ValidationResult{}, nil, errors.New("la respuesta OCSP no corresponde al serial consultado")
	}
	if err := validateOCSPCertID(basicDER, cert, issuer, response.IssuerHash); err != nil {
		return ValidationResult{}, nil, err
	}
	if err := validateOCSPResponder(response, issuer); err != nil {
		return ValidationResult{}, nil, err
	}
	if err := validateOCSPFreshness(response, at.UTC()); err != nil {
		return ValidationResult{}, nil, err
	}

	switch response.Status {
	case ocsp.Good:
		return ValidationResult{Status: CertificateStatusGood}, basicDER, nil
	case ocsp.Revoked:
		if response.RevokedAt.IsZero() {
			return ValidationResult{}, nil, errors.New("estado OCSP revocado sin RevokedAt")
		}
		if response.RevokedAt.After(at.Add(revocationClockSkew)) {
			return ValidationResult{}, nil, errors.New("RevokedAt OCSP está en el futuro")
		}
		return ValidationResult{
			Status:    CertificateStatusRevoked,
			RevokedAt: response.RevokedAt,
		}, basicDER, nil
	case ocsp.Unknown:
		return ValidationResult{Status: CertificateStatusUnknown}, basicDER, nil
	default:
		return ValidationResult{}, nil, fmt.Errorf("estado OCSP no soportado: %d", response.Status)
	}
}

// ValidateBasicOCSPResponse valida una evidencia BasicOCSPResponse ya embebida.
func ValidateBasicOCSPResponse(
	basicDER []byte,
	cert *x509.Certificate,
	issuer *x509.Certificate,
	at time.Time,
) (ValidationResult, error) {
	fullDER, err := asn1.Marshal(ocspEnvelope{
		ResponseStatus: 0,
		ResponseBytes: ocspResponseBytes{
			ResponseType: oidOCSPBasicResponse,
			Response:     basicDER,
		},
	})
	if err != nil {
		return ValidationResult{}, fmt.Errorf("envolviendo BasicOCSPResponse: %w", err)
	}
	result, _, err := ValidateOCSPResponse(fullDER, cert, issuer, at)
	return result, err
}

// ValidateCRLResponse autentica una CRL y comprueba que cubra el certificado.
func ValidateCRLResponse(
	body []byte,
	cert *x509.Certificate,
	issuer *x509.Certificate,
	at time.Time,
) (ValidationResult, error) {
	if err := validateCertificateIssuer(cert, issuer); err != nil {
		return ValidationResult{}, err
	}
	if at.IsZero() {
		return ValidationResult{}, errors.New("instante de validación CRL nulo")
	}
	crl, err := x509.ParseRevocationList(body)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("parseando CRL: %w", err)
	}
	if !bytes.Equal(crl.RawIssuer, issuer.RawSubject) {
		return ValidationResult{}, errors.New("el emisor de la CRL no corresponde al certificado")
	}
	if len(crl.AuthorityKeyId) > 0 &&
		len(issuer.SubjectKeyId) > 0 &&
		!bytes.Equal(crl.AuthorityKeyId, issuer.SubjectKeyId) {
		return ValidationResult{}, errors.New("el identificador de clave de la CRL no corresponde al emisor")
	}
	if err := crl.CheckSignatureFrom(issuer); err != nil {
		return ValidationResult{}, fmt.Errorf("firma de CRL no válida: %w", err)
	}
	if err := validateFreshness("CRL", crl.ThisUpdate, crl.NextUpdate, at.UTC()); err != nil {
		return ValidationResult{}, err
	}
	for _, entry := range crl.RevokedCertificateEntries {
		if entry.SerialNumber != nil && entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			if entry.RevocationTime.IsZero() {
				return ValidationResult{}, errors.New("entrada CRL revocada sin RevocationTime")
			}
			if entry.RevocationTime.After(at.Add(revocationClockSkew)) {
				return ValidationResult{}, errors.New("RevocationTime de CRL está en el futuro")
			}
			return ValidationResult{
				Status:    CertificateStatusRevoked,
				RevokedAt: entry.RevocationTime,
			}, nil
		}
	}
	return ValidationResult{Status: CertificateStatusGood}, nil
}

type ocspEnvelope struct {
	ResponseStatus asn1.Enumerated
	ResponseBytes  ocspResponseBytes `asn1:"explicit,tag:0,optional"`
}

type ocspResponseBytes struct {
	ResponseType asn1.ObjectIdentifier
	Response     []byte
}

func extractBasicOCSPResponse(body []byte) ([]byte, error) {
	var envelope ocspEnvelope
	rest, err := asn1.Unmarshal(body, &envelope)
	if err != nil {
		return nil, fmt.Errorf("parseando respuesta OCSP: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("respuesta OCSP con datos finales")
	}
	if envelope.ResponseStatus != 0 {
		return nil, fmt.Errorf("estado OCSP no exitoso: %d", envelope.ResponseStatus)
	}
	if len(envelope.ResponseBytes.ResponseType) == 0 {
		return nil, errors.New("respuesta OCSP sin ResponseBytes")
	}
	if !envelope.ResponseBytes.ResponseType.Equal(oidOCSPBasicResponse) {
		return nil, fmt.Errorf(
			"tipo de respuesta OCSP no soportado: %s",
			envelope.ResponseBytes.ResponseType.String(),
		)
	}
	return append([]byte(nil), envelope.ResponseBytes.Response...), nil
}

type basicOCSPMetadata struct {
	TBSResponseData    ocspResponseMetadata
	SignatureAlgorithm asn1.RawValue
	Signature          asn1.BitString
	Certificates       []asn1.RawValue `asn1:"explicit,tag:0,optional"`
}

type ocspResponseMetadata struct {
	Version        int `asn1:"optional,default:0,explicit,tag:0"`
	RawResponderID asn1.RawValue
	ProducedAt     time.Time
	Responses      []ocspSingleResponseMetadata
	Extensions     []pkix.Extension `asn1:"optional,explicit,tag:1"`
}

type ocspSingleResponseMetadata struct {
	CertID           ocspCertIDMetadata
	CertStatus       asn1.RawValue
	ThisUpdate       time.Time
	NextUpdate       time.Time        `asn1:"optional,explicit,tag:0"`
	SingleExtensions []pkix.Extension `asn1:"optional,explicit,tag:1"`
}

type ocspCertIDMetadata struct {
	HashAlgorithm  pkix.AlgorithmIdentifier
	IssuerNameHash []byte
	IssuerKeyHash  []byte
	SerialNumber   *big.Int
}

func validateOCSPCertID(
	basicDER []byte,
	cert *x509.Certificate,
	issuer *x509.Certificate,
	issuerHash crypto.Hash,
) error {
	if issuerHash == 0 || !issuerHash.Available() {
		return errors.New("algoritmo hash del CertID OCSP no disponible")
	}
	var basic basicOCSPMetadata
	rest, err := asn1.Unmarshal(basicDER, &basic)
	if err != nil {
		return fmt.Errorf("parseando CertID OCSP: %w", err)
	}
	if len(rest) != 0 {
		return errors.New("BasicOCSPResponse con datos finales")
	}

	var matching []ocspCertIDMetadata
	for _, single := range basic.TBSResponseData.Responses {
		if single.CertID.SerialNumber != nil &&
			single.CertID.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			matching = append(matching, single.CertID)
		}
	}
	if len(matching) != 1 {
		return fmt.Errorf("se esperaban una respuesta OCSP para el serial y se encontraron %d", len(matching))
	}
	certID := matching[0]

	nameHasher := issuerHash.New()
	_, _ = nameHasher.Write(issuer.RawSubject)
	expectedNameHash := nameHasher.Sum(nil)

	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	rest, err = asn1.Unmarshal(issuer.RawSubjectPublicKeyInfo, &spki)
	if err != nil || len(rest) != 0 {
		return fmt.Errorf("parseando clave pública del emisor OCSP: %w", err)
	}
	keyHasher := issuerHash.New()
	_, _ = keyHasher.Write(spki.PublicKey.RightAlign())
	expectedKeyHash := keyHasher.Sum(nil)

	if !bytes.Equal(certID.IssuerNameHash, expectedNameHash) ||
		!bytes.Equal(certID.IssuerKeyHash, expectedKeyHash) {
		return errors.New("el CertID OCSP no corresponde al emisor indicado")
	}
	return nil
}

func validateOCSPResponder(response *ocsp.Response, issuer *x509.Certificate) error {
	responder := issuer
	if response.Certificate != nil {
		responder = response.Certificate
		if !responder.Equal(issuer) {
			if !bytes.Equal(responder.RawIssuer, issuer.RawSubject) {
				return errors.New("el respondedor OCSP delegado tiene otro emisor")
			}
			roots := x509.NewCertPool()
			roots.AddCert(issuer)
			if _, err := responder.Verify(x509.VerifyOptions{
				Roots:       roots,
				CurrentTime: response.ProducedAt,
				KeyUsages:   []x509.ExtKeyUsage{ocspResponderSigningUsageID},
			}); err != nil {
				return fmt.Errorf("respondedor OCSP delegado no autorizado: %w", err)
			}
			if responder.KeyUsage != 0 && responder.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
				return errors.New("respondedor OCSP delegado sin uso de firma digital")
			}
		}
	}

	switch {
	case len(response.RawResponderName) > 0:
		if !bytes.Equal(response.RawResponderName, responder.RawSubject) {
			return errors.New("ResponderID OCSP no corresponde al firmante")
		}
	case len(response.ResponderKeyHash) > 0:
		keyHash, err := responderPublicKeyHash(responder)
		if err != nil {
			return err
		}
		if !bytes.Equal(response.ResponderKeyHash, keyHash) {
			return errors.New("ResponderID OCSP no corresponde a la clave firmante")
		}
	default:
		return errors.New("respuesta OCSP sin ResponderID")
	}
	return nil
}

func responderPublicKeyHash(cert *x509.Certificate) ([]byte, error) {
	if !crypto.SHA1.Available() {
		return nil, errors.New("SHA-1 de ResponderID OCSP no disponible")
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	rest, err := asn1.Unmarshal(cert.RawSubjectPublicKeyInfo, &spki)
	if err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("parseando clave del respondedor OCSP: %w", err)
	}
	hasher := crypto.SHA1.New() // Hash protocolario del ResponderID, no algoritmo de firma.
	_, _ = hasher.Write(spki.PublicKey.RightAlign())
	return hasher.Sum(nil), nil
}

func validateOCSPFreshness(response *ocsp.Response, at time.Time) error {
	if response.ProducedAt.IsZero() {
		return errors.New("respuesta OCSP sin ProducedAt")
	}
	if response.ProducedAt.After(at.Add(revocationClockSkew)) {
		return errors.New("ProducedAt OCSP está en el futuro")
	}
	if response.ProducedAt.Before(response.ThisUpdate.Add(-revocationClockSkew)) {
		return errors.New("ProducedAt OCSP es anterior a ThisUpdate")
	}
	return validateFreshness("OCSP", response.ThisUpdate, response.NextUpdate, at)
}

func validateFreshness(label string, thisUpdate, nextUpdate, at time.Time) error {
	if thisUpdate.IsZero() {
		return fmt.Errorf("%s sin ThisUpdate", label)
	}
	if thisUpdate.After(at.Add(revocationClockSkew)) {
		return fmt.Errorf("ThisUpdate de %s está en el futuro", label)
	}
	if nextUpdate.IsZero() {
		if at.After(thisUpdate.Add(maxAgeWithoutNextUpdate).Add(revocationClockSkew)) {
			return fmt.Errorf("%s caducada: sin NextUpdate y con más de %s", label, maxAgeWithoutNextUpdate)
		}
		return nil
	}
	if nextUpdate.Before(thisUpdate) {
		return fmt.Errorf("NextUpdate de %s es anterior a ThisUpdate", label)
	}
	if at.After(nextUpdate.Add(revocationClockSkew)) {
		return fmt.Errorf("%s caducada desde %s", label, nextUpdate.UTC().Format(time.RFC3339))
	}
	return nil
}
