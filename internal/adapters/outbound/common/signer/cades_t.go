// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"fmt"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	oidSignatureTimeStamp = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
)

// SignerCAdEST firma con CAdES-BES y añade un timestamp RFC 3161.
type SignerCAdEST struct {
	base *CAdESBESDetached
	tsa  ports.TimestampAuthority
}

// NewSignerCAdEST crea un firmador CAdES-T que envuelve un CAdES-BES existente.
func NewSignerCAdEST(base *CAdESBESDetached, tsa ports.TimestampAuthority) *SignerCAdEST {
	return &SignerCAdEST{
		base: base,
		tsa:  tsa,
	}
}

// Sign firma el documento con CAdES-BES y añade el atributo no-firmado signatureTimeStamp.
func (s *SignerCAdEST) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if s == nil || s.base == nil || s.tsa == nil {
		return domain.SignatureResult{}, errors.New("CAdES-T requiere firmador base y TSA configurados")
	}

	// Paso 1: firmar con CAdES-BES
	result, err := s.base.Sign(ctx, job, key)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error en firma CAdES-BES base: %w", err)
	}

	// Paso 2: añadir timestamp al SignerInfo
	timestampedData, err := s.addTimestamp(ctx, result.Data)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error añadiendo timestamp CAdES-T: %w", err)
	}

	return domain.SignatureResult{
		Format:    result.Format,
		Data:      timestampedData,
		Algorithm: result.Algorithm,
	}, nil
}

// addTimestamp añade el atributo no-firmado signatureTimeStamp al primer SignerInfo del CMS.
func (s *SignerCAdEST) addTimestamp(ctx context.Context, cmsData []byte) ([]byte, error) {
	// Parsear ContentInfo
	var ci contentInfo
	if _, err := asn1.Unmarshal(cmsData, &ci); err != nil {
		return nil, fmt.Errorf("error parseando ContentInfo: %w", err)
	}

	// Parsear SignedData
	var sd signedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return nil, fmt.Errorf("error parseando SignedData: %w", err)
	}

	if len(sd.SignerInfos) == 0 {
		return nil, errors.New("SignedData no contiene SignerInfos")
	}

	// Calcular SHA-256 del valor de firma del primer SignerInfo
	sigBytes := sd.SignerInfos[0].Signature
	sigHash := sha256.Sum256(sigBytes)

	// Solicitar timestamp a la TSA
	tst, err := s.tsa.RequestTimestamp(ctx, sigHash[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("error solicitando timestamp a TSA: %w", err)
	}

	// Construir atributo no-firmado signatureTimeStamp
	tstAttr := attribute{
		Type: oidSignatureTimeStamp,
		Values: []asn1.RawValue{
			{FullBytes: tst},
		},
	}

	// Serializar la lista de atributos no firmados como SET para obtener los bytes internos
	unsignedAttrsSetDER, err := asn1.MarshalWithParams([]attribute{tstAttr}, "set")
	if err != nil {
		return nil, fmt.Errorf("error serializando atributos no firmados: %w", err)
	}

	// Parsear el SET para obtener sus bytes internos (sin el tag/length externo)
	var unsignedAttrsSetRaw asn1.RawValue
	if _, err := asn1.Unmarshal(unsignedAttrsSetDER, &unsignedAttrsSetRaw); err != nil {
		return nil, fmt.Errorf("error parseando SET de atributos no firmados: %w", err)
	}

	// Construir signerInfo con atributos no firmados: [1] IMPLICIT SET OF Attribute
	siConTimestamp := signerInfoConUnsigned{
		Version:            sd.SignerInfos[0].Version,
		SID:                sd.SignerInfos[0].SID,
		DigestAlgorithm:    sd.SignerInfos[0].DigestAlgorithm,
		SignedAttributes:   sd.SignerInfos[0].SignedAttributes,
		SignatureAlgorithm: sd.SignerInfos[0].SignatureAlgorithm,
		Signature:          sd.SignerInfos[0].Signature,
		UnsignedAttributes: asn1.RawValue{
			Class:      2,
			Tag:        1,
			IsCompound: true,
			Bytes:      unsignedAttrsSetRaw.Bytes,
		},
	}

	// Reconstruir SignedData con el nuevo SignerInfo
	sdConTimestamp := signedDataConUnsigned{
		Version:          sd.Version,
		DigestAlgorithms: sd.DigestAlgorithms,
		EncapContentInfo: sd.EncapContentInfo,
		Certificates:     sd.Certificates,
		SignerInfos:      []signerInfoConUnsigned{siConTimestamp},
	}

	newSdDER, err := asn1.Marshal(sdConTimestamp)
	if err != nil {
		return nil, fmt.Errorf("error serializando SignedData con timestamp: %w", err)
	}

	// Reconstruir ContentInfo
	newCI := contentInfo{
		ContentType: ci.ContentType,
		Content: asn1.RawValue{
			Class:      2,
			Tag:        0,
			IsCompound: true,
			Bytes:      newSdDER,
		},
	}

	out, err := asn1.Marshal(newCI)
	if err != nil {
		return nil, fmt.Errorf("error serializando ContentInfo con timestamp: %w", err)
	}

	return out, nil
}

// signerInfoConUnsigned es como signerInfo pero incluye atributos no firmados.
type signerInfoConUnsigned struct {
	Version            int
	SID                issuerAndSerialNumber
	DigestAlgorithm    algorithmIdentifier
	SignedAttributes   asn1.RawValue `asn1:"tag:0,optional"`
	SignatureAlgorithm algorithmIdentifier
	Signature          []byte
	UnsignedAttributes asn1.RawValue `asn1:"tag:1,optional"`
}

// signedDataConUnsigned es como signedData pero usa signerInfoConUnsigned.
type signedDataConUnsigned struct {
	Version          int
	DigestAlgorithms []algorithmIdentifier `asn1:"set"`
	EncapContentInfo encapContentInfo
	Certificates     asn1.RawValue           `asn1:"tag:0,optional"`
	SignerInfos      []signerInfoConUnsigned `asn1:"set"`
}
