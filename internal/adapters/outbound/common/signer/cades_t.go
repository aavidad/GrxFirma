// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
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

// addTimestamp conserva todos los firmantes y sus atributos existentes. El
// sello se añade a las hojas, incluidas las contrafirmas, sin recodificar los
// campos firmados. Se limita el trabajo sobre CMS suministrados por terceros.
func (s *SignerCAdEST) addTimestamp(ctx context.Context, cmsData []byte) ([]byte, error) {
	sd, err := parseSignedDataParts(cmsData)
	if err != nil {
		return nil, err
	}
	remaining := 64
	for i, si := range sd.signerInfos {
		updated, err := s.timestampLeaves(ctx, si, 0, &remaining)
		if err != nil {
			return nil, err
		}
		sd.signerInfos[i] = updated
	}
	return sd.marshal()
}

func (s *SignerCAdEST) timestampLeaves(ctx context.Context, signerDER []byte, depth int, remaining *int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if depth > maxProfundidadContrafirma || *remaining <= 0 {
		return nil, errors.New("demasiados firmantes para sello CAdES-T")
	}
	*remaining--
	elems, err := splitSequence(signerDER)
	if err != nil {
		return nil, err
	}
	signature, unsignedIndex, err := signerInfoSignatureAndUnsigned(elems)
	if err != nil {
		return nil, err
	}
	var attrs [][]byte
	if unsignedIndex >= 0 {
		attrs, err = splitContents(elems[unsignedIndex])
		if err != nil {
			return nil, err
		}
	}
	hasCounters := false
	for i, der := range attrs {
		var attr attribute
		rest, err := asn1.Unmarshal(der, &attr)
		if err != nil || len(rest) != 0 {
			return nil, errors.New("atributo no firmado invalido")
		}
		if !attr.Type.Equal(oidCounterSignature) {
			continue
		}
		if len(attr.Values) == 0 {
			return nil, errors.New("contrafirma sin firmantes")
		}
		hasCounters = true
		for j, value := range attr.Values {
			updated, err := s.timestampLeaves(ctx, value.FullBytes, depth+1, remaining)
			if err != nil {
				return nil, err
			}
			attr.Values[j] = asn1.RawValue{FullBytes: updated}
		}
		attrs[i], err = asn1.Marshal(attr)
		if err != nil {
			return nil, err
		}
	}
	if !hasCounters {
		digest := sha256.Sum256(signature)
		token, err := s.tsa.RequestTimestamp(ctx, digest[:], crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("error solicitando timestamp a TSA: %w", err)
		}
		der, err := asn1.Marshal(attribute{Type: oidSignatureTimeStamp, Values: []asn1.RawValue{{FullBytes: token}}})
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, der)
	}
	unsigned, err := encodeTagged(asn1.ClassContextSpecific, 1, sortedSetContents(attrs))
	if err != nil {
		return nil, err
	}
	if unsignedIndex >= 0 {
		elems[unsignedIndex] = unsigned
	} else {
		elems = append(elems, unsigned)
	}
	return encodeTagged(asn1.ClassUniversal, asn1.TagSequence, bytes.Join(elems, nil))
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
