// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// SignerXAdEST firma con XAdES-BES y añade SignatureTimeStamp en las propiedades no firmadas.
type SignerXAdEST struct {
	base *XAdESBESDetached
	tsa  ports.TimestampAuthority
}

// NewSignerXAdEST crea un firmador XAdES-T que envuelve un firmador XAdES-BES.
func NewSignerXAdEST(base *XAdESBESDetached, tsa ports.TimestampAuthority) *SignerXAdEST {
	return &SignerXAdEST{
		base: base,
		tsa:  tsa,
	}
}

// Sign genera primero XAdES-BES y después añade SignatureTimeStamp.
func (s *SignerXAdEST) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if s == nil || s.base == nil {
		return domain.SignatureResult{}, fmt.Errorf("firmador XAdES-T sin base XAdES-BES")
	}
	if s.tsa == nil {
		return domain.SignatureResult{}, fmt.Errorf("firmador XAdES-T sin TSA configurada")
	}

	result, err := s.base.Sign(ctx, job, key)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error en firma XAdES-BES base: %w", err)
	}

	withTimestamp, err := s.addTimestamp(ctx, result.Data)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error añadiendo SignatureTimeStamp: %w", err)
	}

	result.Data = withTimestamp
	return result, nil
}

func (s *SignerXAdEST) addTimestamp(ctx context.Context, xmlData []byte) ([]byte, error) {
	signatureValueXML, err := extractXMLLiteralElement(xmlData, "ds:SignatureValue")
	if err != nil {
		signatureValueXML, err = extractFirstElementByName(xmlData, nsXMLDSig, "SignatureValue")
	}
	if err != nil {
		return nil, fmt.Errorf("signaturevalue ausente: %w", err)
	}

	c14n, err := exclusiveC14N(string(signatureValueXML))
	if err != nil {
		return nil, fmt.Errorf("error canonicalizando SignatureValue: %w", err)
	}
	digest := sha256.Sum256(c14n)

	tst, err := s.tsa.RequestTimestamp(ctx, digest[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("error solicitando timestamp a TSA: %w", err)
	}

	unsignedXML := buildUnsignedPropertiesXML(base64.StdEncoding.EncodeToString(tst))
	if strings.Contains(string(xmlData), "<xades:UnsignedProperties>") {
		return nil, fmt.Errorf("la firma XAdES ya contiene UnsignedProperties")
	}

	final := strings.Replace(
		string(xmlData),
		"</xades:QualifyingProperties>",
		unsignedXML+"</xades:QualifyingProperties>",
		1,
	)
	if final == string(xmlData) {
		return nil, fmt.Errorf("no se encontró xades:QualifyingProperties para insertar SignatureTimeStamp")
	}
	return []byte(final), nil
}

func buildUnsignedPropertiesXML(tstB64 string) string {
	return fmt.Sprintf(
		`<xades:UnsignedProperties>`+
			`<xades:UnsignedSignatureProperties>`+
			`<xades:SignatureTimeStamp>`+
			`<ds:CanonicalizationMethod xmlns:ds="%s" Algorithm="%s"/>`+
			`<xades:EncapsulatedTimeStamp>%s</xades:EncapsulatedTimeStamp>`+
			`</xades:SignatureTimeStamp>`+
			`</xades:UnsignedSignatureProperties>`+
			`</xades:UnsignedProperties>`,
		nsXMLDSig,
		algExcC14N,
		tstB64,
	)
}
