// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

var (
	oidData                   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData             = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidSigningCertificateV2   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	oidDigestSHA256           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSignatureRSAWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidSignatureRSAEncryption = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidSignatureECDSAWith256  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
)

// CAdESBESDetached implementa ports.SignerEngine para el caso mínimo de CAdES-BES detached.
type CAdESBESDetached struct{}

// NewCAdESBESDetached crea el motor de firma CAdES-BES detached en Go nativo.
func NewCAdESBESDetached() *CAdESBESDetached {
	return &CAdESBESDetached{}
}

// Sign genera una firma CAdES-BES detached usando una clave privada local en memoria.
func (e *CAdESBESDetached) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != domain.FormatCAdES {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", domain.FormatCAdES)
	}
	clave, err := requireLocalSigningKey(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}

	spec := cmsHashFromOptions(job.Options)
	var (
		firma     []byte
		algoritmo string
	)
	switch job.Action {
	case domain.ActionSign:
		firma, algoritmo, err = signCAdESBES(job.Document.Content, clave, spec, modoImplicitoCAdES(job.Options))
	case domain.ActionCoSign:
		firma, err = cosignCAdES(job.Document.Content, clave, spec, time.Now())
		algoritmo = etiquetaAlgoritmoCMS(clave, spec)
	case domain.ActionCounterSign:
		firma, err = countersignCAdES(job.Document.Content, clave, spec, time.Now())
		algoritmo = etiquetaAlgoritmoCMS(clave, spec)
	default:
		err = fmt.Errorf("acción de firma CAdES no soportada: %s", job.Action)
	}
	if err != nil {
		return domain.SignatureResult{}, err
	}

	return domain.SignatureResult{
		Format:    domain.FormatCAdES,
		Data:      firma,
		Algorithm: algoritmo,
	}, nil
}

func etiquetaAlgoritmoCMS(key *LocalSigningKey, spec cmsHashSpec) string {
	if _, ok := key.Signer.Public().(*ecdsa.PublicKey); ok {
		return spec.ecdsaLabel
	}
	return spec.rsaLabel
}

func requireLocalSigningKey(key ports.SigningKey) (*LocalSigningKey, error) {
	clave, ok := key.(*LocalSigningKey)
	if !ok || clave == nil {
		return nil, errors.New("la clave de firma no es compatible con el motor CAdES local")
	}
	if clave.Signer == nil {
		return nil, errors.New("la clave de firma local no contiene signer")
	}
	if clave.Certificate == nil {
		return nil, errors.New("la clave de firma local no contiene certificado")
	}
	return clave, nil
}

// modoImplicitoCAdES aplica mode=implicit de AutoFirma Java: la firma incluye
// los datos (CAdES attached). Por defecto, como en Java, es explícita.
func modoImplicitoCAdES(options map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(valorOpcion(options, "mode")), "implicit")
}

func signDetachedCAdESBES(data []byte, key *LocalSigningKey, spec cmsHashSpec) ([]byte, string, error) {
	return signCAdESBES(data, key, spec, false)
}

func signCAdESBES(data []byte, key *LocalSigningKey, spec cmsHashSpec, implicito bool) ([]byte, string, error) {
	if len(data) == 0 {
		return nil, "", errors.New("no se pueden firmar datos vacios")
	}

	digest := cmsDigest(spec.hash, data)
	signedAttrsDER, signedAttrsRaw, err := buildSignedAttributes(key.Certificate, digest, time.Now().UTC())
	if err != nil {
		return nil, "", err
	}

	signature, signatureAlgorithm, algorithmLabel, err := signAttributes(cmsDigest(spec.hash, signedAttrsDER), key.Signer, spec)
	if err != nil {
		return nil, "", err
	}

	certificatesRaw, err := buildCertificateSet(key.Certificate, key.Chain)
	if err != nil {
		return nil, "", err
	}

	issuer := asn1.RawValue{FullBytes: key.Certificate.RawIssuer}
	sd := signedData{
		Version:          1,
		DigestAlgorithms: []algorithmIdentifier{{Algorithm: spec.digestOID}},
		EncapContentInfo: encapContentInfo{
			EContentType: oidData,
		},
		Certificates: certificatesRaw,
		SignerInfos: []signerInfo{
			{
				Version: 1,
				SID: issuerAndSerialNumber{
					Issuer:       issuer,
					SerialNumber: key.Certificate.SerialNumber,
				},
				DigestAlgorithm:    algorithmIdentifier{Algorithm: spec.digestOID},
				SignedAttributes:   signedAttrsRaw,
				SignatureAlgorithm: signatureAlgorithm,
				Signature:          signature,
			},
		},
	}

	if implicito {
		contenido, err := asn1.Marshal(data)
		if err != nil {
			return nil, "", fmt.Errorf("error serializando contenido implícito: %w", err)
		}
		// [0] EXPLICIT se construye a mano: asn1 escribe tal cual un
		// RawValue con FullBytes e ignoraría la etiqueta del campo.
		sd.EncapContentInfo.EContent = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: contenido}
	}

	signedDataDER, err := asn1.Marshal(sd)
	if err != nil {
		return nil, "", fmt.Errorf("error serializando SignedData: %w", err)
	}

	content := contentInfo{
		ContentType: oidSignedData,
		Content: asn1.RawValue{
			Class:      2,
			Tag:        0,
			IsCompound: true,
			Bytes:      signedDataDER,
		},
	}
	out, err := asn1.Marshal(content)
	if err != nil {
		return nil, "", fmt.Errorf("error serializando ContentInfo: %w", err)
	}

	return out, algorithmLabel, nil
}

func signAttributes(hashed []byte, signer crypto.Signer, spec cmsHashSpec) ([]byte, algorithmIdentifier, string, error) {
	switch signer.Public().(type) {
	case *rsa.PublicKey:
		sig, err := signer.Sign(rand.Reader, hashed, spec.hash)
		if err != nil {
			return nil, algorithmIdentifier{}, "", fmt.Errorf("error firmando atributos con RSA: %w", err)
		}
		return sig, signatureAlgorithmRSA(spec), spec.rsaLabel, nil
	case *ecdsa.PublicKey:
		sig, err := signer.Sign(rand.Reader, hashed, spec.hash)
		if err != nil {
			return nil, algorithmIdentifier{}, "", fmt.Errorf("error firmando atributos con ECDSA: %w", err)
		}
		return sig, algorithmIdentifier{Algorithm: spec.ecdsaOID}, spec.ecdsaLabel, nil
	default:
		return nil, algorithmIdentifier{}, "", errors.New("tipo de clave no soportado por el motor CAdES")
	}
}

func buildSignedAttributes(cert *x509.Certificate, digest []byte, signingTime time.Time) ([]byte, asn1.RawValue, error) {
	if cert == nil {
		return nil, asn1.RawValue{}, errors.New("certificado nulo en atributos firmados")
	}
	certHash := sha256.Sum256(cert.Raw)

	signingCertificateV2DER, err := asn1.Marshal(signingCertificateV2{
		Certs: []essCertIDv2{
			{CertHash: certHash[:]},
		},
	})
	if err != nil {
		return nil, asn1.RawValue{}, fmt.Errorf("error serializando SigningCertificateV2: %w", err)
	}

	contentTypeAttr, err := newAttribute(oidContentType, oidData)
	if err != nil {
		return nil, asn1.RawValue{}, fmt.Errorf("error creando atributo contentType: %w", err)
	}
	messageDigestAttr, err := newAttribute(oidMessageDigest, digest)
	if err != nil {
		return nil, asn1.RawValue{}, fmt.Errorf("error creando atributo messageDigest: %w", err)
	}
	signingTimeAttr, err := newAttribute(oidSigningTime, signingTime)
	if err != nil {
		return nil, asn1.RawValue{}, fmt.Errorf("error creando atributo signingTime: %w", err)
	}

	attrs := []attribute{
		contentTypeAttr,
		messageDigestAttr,
		signingTimeAttr,
		newRawAttribute(oidSigningCertificateV2, signingCertificateV2DER),
	}

	setDER, err := asn1.MarshalWithParams(attrs, "set")
	if err != nil {
		return nil, asn1.RawValue{}, fmt.Errorf("error serializando atributos firmados: %w", err)
	}

	var setRaw asn1.RawValue
	if _, err := asn1.Unmarshal(setDER, &setRaw); err != nil {
		return nil, asn1.RawValue{}, fmt.Errorf("error parseando atributos firmados: %w", err)
	}

	return setDER, asn1.RawValue{
		Class:      2,
		Tag:        0,
		IsCompound: true,
		Bytes:      setRaw.Bytes,
	}, nil
}

func newAttribute(oid asn1.ObjectIdentifier, value interface{}) (attribute, error) {
	encoded, err := asn1.Marshal(value)
	if err != nil {
		return attribute{}, err
	}
	return attribute{
		Type: oid,
		Values: []asn1.RawValue{
			{FullBytes: encoded},
		},
	}, nil
}

func newRawAttribute(oid asn1.ObjectIdentifier, valueDER []byte) attribute {
	return attribute{
		Type: oid,
		Values: []asn1.RawValue{
			{FullBytes: valueDER},
		},
	}
}

func buildCertificateSet(leaf *x509.Certificate, chain []*x509.Certificate) (asn1.RawValue, error) {
	if leaf == nil {
		return asn1.RawValue{}, errors.New("certificado del firmante ausente")
	}
	values := []asn1.RawValue{
		{FullBytes: leaf.Raw},
	}
	for _, cert := range chain {
		if cert == nil {
			continue
		}
		values = append(values, asn1.RawValue{FullBytes: cert.Raw})
	}

	setDER, err := asn1.MarshalWithParams(values, "set")
	if err != nil {
		return asn1.RawValue{}, fmt.Errorf("error serializando conjunto de certificados: %w", err)
	}

	var setRaw asn1.RawValue
	if _, err := asn1.Unmarshal(setDER, &setRaw); err != nil {
		return asn1.RawValue{}, fmt.Errorf("error parseando conjunto de certificados: %w", err)
	}

	return asn1.RawValue{
		Class:      2,
		Tag:        0,
		IsCompound: true,
		Bytes:      setRaw.Bytes,
	}, nil
}

func signatureAlgorithmRSA(spec cmsHashSpec) algorithmIdentifier {
	return algorithmIdentifier{
		Algorithm: spec.rsaOID,
		Parameters: asn1.RawValue{
			FullBytes: []byte{0x05, 0x00},
		},
	}
}

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
}

type signedData struct {
	Version          int
	DigestAlgorithms []algorithmIdentifier `asn1:"set"`
	EncapContentInfo encapContentInfo
	Certificates     asn1.RawValue `asn1:"tag:0,optional"`
	SignerInfos      []signerInfo  `asn1:"set"`
}

type encapContentInfo struct {
	EContentType asn1.ObjectIdentifier
	EContent     asn1.RawValue `asn1:"optional"`
}

type algorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type issuerAndSerialNumber struct {
	Issuer       asn1.RawValue
	SerialNumber *big.Int
}

type signerInfo struct {
	Version            int
	SID                issuerAndSerialNumber
	DigestAlgorithm    algorithmIdentifier
	SignedAttributes   asn1.RawValue `asn1:"tag:0,optional"`
	SignatureAlgorithm algorithmIdentifier
	Signature          []byte
}

type attribute struct {
	Type   asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

type signingCertificateV2 struct {
	Certs []essCertIDv2
}

type essCertIDv2 struct {
	CertHash []byte
}
