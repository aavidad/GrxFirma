// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	oidRevocationValues = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 24}
	oidArchiveTimeStamp = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 27}
)

type revocationValues struct {
	CRLVals  []asn1.RawValue `asn1:"optional,explicit,tag:0"`
	OCSPVals []asn1.RawValue `asn1:"optional,explicit,tag:1"`
}

type SignerCAdESLT struct {
	base       *SignerCAdEST
	revocation ports.RevocationProvider
}

func NewSignerCAdESLT(base *SignerCAdEST, revocation ports.RevocationProvider) *SignerCAdESLT {
	return &SignerCAdESLT{base: base, revocation: revocation}
}

func (s *SignerCAdESLT) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if s.base == nil || s.revocation == nil {
		return domain.SignatureResult{}, errors.New("CAdES-LT requiere timestamp y proveedor de revocación")
	}
	localKey, err := requireLocalSigningKey(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	// Missing issuer metadata is deterministic: reject it before a token PIN,
	// private-key operation or TSA request. Fetch remains after the base signature.
	if _, err := revocationIssuer(localKey.Certificate, localKey.Chain); err != nil {
		return domain.SignatureResult{}, fmt.Errorf("no se pudo obtener revocación para CAdES-LT: %w", err)
	}
	result, err := s.base.Sign(ctx, job, key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	evidence, err := fetchRevocationEvidence(ctx, s.revocation, localKey.Certificate, localKey.Chain)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("no se pudo obtener revocación para CAdES-LT: %w", err)
	}
	withRevocation, err := addRevocationValues(result.Data, evidence)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	result.Data = withRevocation
	result.Algorithm = "CAdES-B-LT"
	return result, nil
}

type SignerCAdESLTA struct {
	base *SignerCAdESLT
	tsa  ports.TimestampAuthority
}

func NewSignerCAdESLTA(base *SignerCAdESLT, tsa ports.TimestampAuthority) *SignerCAdESLTA {
	return &SignerCAdESLTA{base: base, tsa: tsa}
}

func (s *SignerCAdESLTA) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if s.base == nil || s.tsa == nil {
		return domain.SignatureResult{}, errors.New("CAdES-LTA requiere CAdES-LT y TSA")
	}
	result, err := s.base.Sign(ctx, job, key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	sigHash := sha256.Sum256(result.Data)
	tst, err := s.tsa.RequestTimestamp(ctx, sigHash[:], crypto.SHA256)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error solicitando archive timestamp a TSA: %w", err)
	}
	result.Data, err = addUnsignedAttribute(result.Data, newRawAttribute(oidArchiveTimeStamp, tst))
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("no se pudo añadir archive timestamp a CAdES-LTA: %w", err)
	}
	result.Algorithm = "CAdES-B-LTA"
	return result, nil
}

func fetchRevocationEvidence(ctx context.Context, provider ports.RevocationProvider, cert *x509.Certificate, chain []*x509.Certificate) (ports.RevocationEvidence, error) {
	issuer, err := revocationIssuer(cert, chain)
	if err != nil {
		return ports.RevocationEvidence{}, err
	}
	return provider.Fetch(ctx, cert, issuer)
}

func revocationIssuer(cert *x509.Certificate, chain []*x509.Certificate) (*x509.Certificate, error) {
	if cert == nil {
		return nil, errors.New("certificado firmante nulo")
	}
	if len(chain) == 0 {
		return nil, errors.New("cadena de certificación vacía")
	}
	issuer := chain[0]
	if issuer == nil {
		return nil, errors.New("emisor nulo en cadena")
	}
	return issuer, nil
}

func addRevocationValues(cmsData []byte, evidence ports.RevocationEvidence) ([]byte, error) {
	if len(evidence.OCSPResponses) == 0 && len(evidence.CRLs) == 0 {
		return nil, errors.New("sin evidencias de revocación para embebido LT")
	}
	values := revocationValues{
		CRLVals:  make([]asn1.RawValue, 0, len(evidence.CRLs)),
		OCSPVals: make([]asn1.RawValue, 0, len(evidence.OCSPResponses)),
	}
	for _, crlDER := range evidence.CRLs {
		values.CRLVals = append(values.CRLVals, asn1.RawValue{FullBytes: append([]byte(nil), crlDER...)})
	}
	for _, ocspDER := range evidence.OCSPResponses {
		values.OCSPVals = append(values.OCSPVals, asn1.RawValue{FullBytes: append([]byte(nil), ocspDER...)})
	}
	attrDER, err := asn1.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("serializando RevocationValues: %w", err)
	}
	return addUnsignedAttribute(cmsData, newRawAttribute(oidRevocationValues, attrDER))
}

func addUnsignedAttribute(cmsData []byte, attr attribute) ([]byte, error) {
	ci, sd, err := parseCMSForUnsignedMutation(cmsData)
	if err != nil {
		return nil, err
	}
	if len(sd.SignerInfos) == 0 {
		return nil, errors.New("SignedData no contiene SignerInfos")
	}
	attrs, err := parseUnsignedAttributes(sd.SignerInfos[0].UnsignedAttributes)
	if err != nil {
		return nil, err
	}
	attrs = append(attrs, attr)
	unsignedRaw, err := marshalUnsignedAttributes(attrs)
	if err != nil {
		return nil, err
	}
	sd.SignerInfos[0].UnsignedAttributes = unsignedRaw
	return rebuildCMSWithUnsigned(ci, sd)
}

func parseCMSForUnsignedMutation(cmsData []byte) (contentInfo, signedDataConUnsigned, error) {
	var ci contentInfo
	if _, err := asn1.Unmarshal(cmsData, &ci); err != nil {
		return contentInfo{}, signedDataConUnsigned{}, fmt.Errorf("error parseando ContentInfo: %w", err)
	}
	var sd signedDataConUnsigned
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return contentInfo{}, signedDataConUnsigned{}, fmt.Errorf("error parseando SignedData: %w", err)
	}
	return ci, sd, nil
}

func rebuildCMSWithUnsigned(ci contentInfo, sd signedDataConUnsigned) ([]byte, error) {
	newSdDER, err := asn1.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("error serializando SignedData con atributos no firmados: %w", err)
	}
	ci.Content = asn1.RawValue{
		Class:      2,
		Tag:        0,
		IsCompound: true,
		Bytes:      newSdDER,
	}
	out, err := asn1.Marshal(ci)
	if err != nil {
		return nil, fmt.Errorf("error serializando ContentInfo: %w", err)
	}
	return out, nil
}

func parseUnsignedAttributes(raw asn1.RawValue) ([]attribute, error) {
	if len(raw.Bytes) == 0 {
		return nil, nil
	}
	setDER, err := asContextSpecificSetDER(raw, 1)
	if err != nil {
		return nil, err
	}
	var attrs []attribute
	if _, err := asn1.UnmarshalWithParams(setDER, &attrs, "set"); err != nil {
		return nil, fmt.Errorf("error parseando atributos no firmados: %w", err)
	}
	return attrs, nil
}

func marshalUnsignedAttributes(attrs []attribute) (asn1.RawValue, error) {
	if len(attrs) == 0 {
		return asn1.RawValue{}, nil
	}
	setDER, err := asn1.MarshalWithParams(attrs, "set")
	if err != nil {
		return asn1.RawValue{}, fmt.Errorf("error serializando atributos no firmados: %w", err)
	}
	var setRaw asn1.RawValue
	if _, err := asn1.Unmarshal(setDER, &setRaw); err != nil {
		return asn1.RawValue{}, fmt.Errorf("error parseando SET de atributos no firmados: %w", err)
	}
	return asn1.RawValue{
		Class:      2,
		Tag:        1,
		IsCompound: true,
		Bytes:      setRaw.Bytes,
	}, nil
}

func asContextSpecificSetDER(raw asn1.RawValue, tag int) ([]byte, error) {
	if len(raw.FullBytes) == 0 {
		return nil, errors.New("atributos ausentes")
	}
	if raw.Class != asn1.ClassContextSpecific || raw.Tag != tag {
		return nil, fmt.Errorf("los atributos no usan el tag [%d] esperado", tag)
	}
	out := make([]byte, len(raw.FullBytes))
	copy(out, raw.FullBytes)
	out[0] = 0x31
	return out, nil
}
