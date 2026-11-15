// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/avisos"
	"grxfirma/internal/security/excepciones"
)

// Cofirma y contrafirma CAdES como las hace AutoFirma Java (AOCAdESSigner):
// la cofirma añade un SignerInfo al SignedData existente y la contrafirma
// firma el valor de firma de los firmantes hoja y la anexa como atributo no
// firmado id-countersignature (RFC 5652, apartado 11.4). Los SignerInfo y
// certificados existentes se conservan byte a byte: nunca se re-codifica lo
// que ya firmó otra persona.

var oidCounterSignature = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 6}

// maxProfundidadContrafirma limita la recursión sobre firmas manipuladas.
const maxProfundidadContrafirma = 16

type signedDataParts struct {
	version       []byte
	digestAlgs    [][]byte
	encapContent  asn1.RawValue
	certificates  [][]byte
	crls          []byte
	signerInfos   [][]byte
	eContentType  asn1.ObjectIdentifier
	eContentBytes []byte
}

// cosignCAdES añade un firmante. original solo llega, ya cotejado, en una
// CAdES explícita cuyo documento ha aportado quien firma.
func cosignCAdES(existing, original []byte, key *LocalSigningKey, spec cmsHashSpec, now time.Time) ([]byte, error) {
	sd, err := parseSignedDataParts(existing)
	if err != nil {
		return nil, err
	}
	digest, err := digestParaCofirma(sd, spec, original)
	if err != nil {
		return nil, err
	}
	attrsDER, attrsRaw, err := buildSignedAttributesWithContentType(key.Certificate, digest, now, sd.eContentType, true)
	if err != nil {
		return nil, err
	}
	nuevo, err := buildSignerInfoDER(key, spec, attrsDER, attrsRaw)
	if err != nil {
		return nil, err
	}
	sd.signerInfos = append(sd.signerInfos, nuevo)
	sd.addDigestAlgorithm(spec)
	if err := sd.addCertificates(key); err != nil {
		return nil, err
	}
	return sd.marshal()
}

// originalCofirmaCotejado decide qué hacer con el original que acompaña a
// una cofirma o contrafirma (KD-03). Solo cuenta en una CAdES explícita: si
// llega, se coteja con la firma recibida y, si no corresponde, no se firma.
// Una firma que corresponde pero usa SHA-1 sigue adelante con su aviso
// (lote 16c), salvo que la política de máquina lo prohíba. Devuelve el
// original que puede usar la cofirma, o nil si no lo hay o no hace falta.
func originalCofirmaCotejado(ctx context.Context, existing, original []byte) ([]byte, error) {
	if len(original) == 0 {
		if _, implicita, err := ContenidoCAdESImplicito(existing); err == nil && !implicita {
			if err := excepciones.Require(ctx, "cofirma_sin_original"); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if _, implicita, err := ContenidoCAdESImplicito(existing); err != nil || implicita {
		// La firma lleva el documento dentro (o no se puede leer y fallará
		// después con su propio error): el original no se usa.
		return nil, nil
	}
	aviso, err := AdmitirCotejoSHA1(CotejarCAdESExplicita(existing, original), "cofirma", ctx)
	if err != nil {
		slog.Warn("cofirma_original_no_corresponde", "error", err)
		return nil, err
	}
	if aviso {
		domain.AnotarAviso(ctx, excepciones.Warning("sha1_conservar"))
	}
	return original, nil
}

// AvisoCofirmaSinOriginal es la clave de catálogo del aviso que acompaña al
// resultado de una cofirma o contrafirma de una CAdES explícita sin el
// documento original (KD-03).
const AvisoCofirmaSinOriginal = domain.AvisoCofirmaSinOriginal

// avisarCofirmaSinDocumento deja aviso visible y en el registro cuando se
// cofirma o contrafirma una CAdES explícita: el motor firma el resumen que
// declara la firma recibida y nadie ha visto el documento. Se mantiene por
// compatibilidad con AutoFirma; el móvil exige además el original (KD-03).
// El aviso va también al resultado de la operación (domain.AnotarAviso), que
// la CLI escribe en su salida de error.
func avisarCofirmaSinDocumento(ctx context.Context, existing []byte) {
	if _, implicita, err := ContenidoCAdESImplicito(existing); err != nil || implicita {
		return
	}
	slog.Warn("cofirma_cades_sin_documento")
	t := localizador.Detectar()
	avisos.Registrar(t.T("firma.cofirma_sin_original.aviso_titulo"), t.T(excepciones.Warning("cofirma_sin_original")))
}

func countersignCAdES(existing []byte, key *LocalSigningKey, spec cmsHashSpec, now time.Time) ([]byte, error) {
	sd, err := parseSignedDataParts(existing)
	if err != nil {
		return nil, err
	}
	for i, si := range sd.signerInfos {
		actualizado, err := contrafirmarHojas(si, key, spec, now, 0)
		if err != nil {
			return nil, err
		}
		sd.signerInfos[i] = actualizado
	}
	sd.addDigestAlgorithm(spec)
	if err := sd.addCertificates(key); err != nil {
		return nil, err
	}
	return sd.marshal()
}

// contrafirmarHojas contrafirma el SignerInfo si no tiene contrafirmas; si
// las tiene, desciende a ellas (objetivo LEAFS de AutoFirma Java).
func contrafirmarHojas(signerInfoDER []byte, key *LocalSigningKey, spec cmsHashSpec, now time.Time, profundidad int) ([]byte, error) {
	if profundidad > maxProfundidadContrafirma {
		return nil, errors.New("contrafirma: anidamiento de contrafirmas excesivo")
	}
	elems, err := splitSequence(signerInfoDER)
	if err != nil {
		return nil, fmt.Errorf("contrafirma: SignerInfo inválido: %w", err)
	}
	signature, unsignedIdx, err := signerInfoSignatureAndUnsigned(elems)
	if err != nil {
		return nil, err
	}

	var attrs [][]byte
	if unsignedIdx >= 0 {
		attrs, err = splitContents(elems[unsignedIdx])
		if err != nil {
			return nil, fmt.Errorf("contrafirma: atributos no firmados inválidos: %w", err)
		}
	}
	tieneContrafirmas := false
	for ai, attrDER := range attrs {
		var attr attribute
		if _, err := asn1.Unmarshal(attrDER, &attr); err != nil {
			return nil, fmt.Errorf("contrafirma: atributo no firmado inválido: %w", err)
		}
		if !attr.Type.Equal(oidCounterSignature) {
			continue
		}
		tieneContrafirmas = true
		for vi, value := range attr.Values {
			actualizado, err := contrafirmarHojas(value.FullBytes, key, spec, now, profundidad+1)
			if err != nil {
				return nil, err
			}
			attr.Values[vi] = asn1.RawValue{FullBytes: actualizado}
		}
		reescrito, err := asn1.Marshal(attr)
		if err != nil {
			return nil, err
		}
		attrs[ai] = reescrito
	}

	if !tieneContrafirmas {
		contrafirma, err := buildCounterSignerInfo(signature, key, spec, now)
		if err != nil {
			return nil, err
		}
		attrDER, err := asn1.Marshal(attribute{Type: oidCounterSignature, Values: []asn1.RawValue{{FullBytes: contrafirma}}})
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, attrDER)
	}

	unsigned, err := encodeTagged(asn1.ClassContextSpecific, 1, sortedSetContents(attrs))
	if err != nil {
		return nil, err
	}
	if unsignedIdx >= 0 {
		elems[unsignedIdx] = unsigned
	} else {
		elems = append(elems, unsigned)
	}
	return encodeTagged(asn1.ClassUniversal, asn1.TagSequence, bytes.Join(elems, nil))
}

func buildCounterSignerInfo(targetSignature []byte, key *LocalSigningKey, spec cmsHashSpec, now time.Time) ([]byte, error) {
	digest := cmsDigest(spec.hash, targetSignature)
	attrsDER, attrsRaw, err := buildSignedAttributesWithContentType(key.Certificate, digest, now, nil, false)
	if err != nil {
		return nil, err
	}
	return buildSignerInfoDER(key, spec, attrsDER, attrsRaw)
}

func buildSignerInfoDER(key *LocalSigningKey, spec cmsHashSpec, attrsDER []byte, attrsRaw asn1.RawValue) ([]byte, error) {
	signature, signatureAlgorithm, _, err := signAttributes(cmsDigest(spec.hash, attrsDER), key.Signer, spec)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(signerInfo{
		Version: 1,
		SID: issuerAndSerialNumber{
			Issuer:       asn1.RawValue{FullBytes: key.Certificate.RawIssuer},
			SerialNumber: key.Certificate.SerialNumber,
		},
		DigestAlgorithm:    algorithmIdentifier{Algorithm: spec.digestOID},
		SignedAttributes:   attrsRaw,
		SignatureAlgorithm: signatureAlgorithm,
		Signature:          signature,
	})
}

// buildSignedAttributesWithContentType genera los atributos firmados. Una
// contrafirma no debe llevar content-type (RFC 5652, apartado 11.4).
func buildSignedAttributesWithContentType(cert *x509.Certificate, digest []byte, now time.Time, contentType asn1.ObjectIdentifier, conContentType bool) ([]byte, asn1.RawValue, error) {
	certHash := sha256.Sum256(cert.Raw)
	signingCertificateV2DER, err := asn1.Marshal(signingCertificateV2{Certs: []essCertIDv2{{CertHash: certHash[:]}}})
	if err != nil {
		return nil, asn1.RawValue{}, err
	}
	var attrs []attribute
	if conContentType {
		if len(contentType) == 0 {
			contentType = oidData
		}
		a, err := newAttribute(oidContentType, contentType)
		if err != nil {
			return nil, asn1.RawValue{}, err
		}
		attrs = append(attrs, a)
	}
	md, err := newAttribute(oidMessageDigest, digest)
	if err != nil {
		return nil, asn1.RawValue{}, err
	}
	st, err := newAttribute(oidSigningTime, now.UTC())
	if err != nil {
		return nil, asn1.RawValue{}, err
	}
	attrs = append(attrs, md, st, newRawAttribute(oidSigningCertificateV2, signingCertificateV2DER))
	setDER, err := asn1.MarshalWithParams(attrs, "set")
	if err != nil {
		return nil, asn1.RawValue{}, err
	}
	var setRaw asn1.RawValue
	if _, err := asn1.Unmarshal(setDER, &setRaw); err != nil {
		return nil, asn1.RawValue{}, err
	}
	return setDER, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: setRaw.Bytes}, nil
}

// digestParaCofirma obtiene el resumen del contenido: del contenido
// encapsulado si existe; en firmas explícitas, del original cotejado si se
// ha aportado o, si no, del messageDigest de un firmante con el mismo
// algoritmo, como hace AutoFirma Java.
func digestParaCofirma(sd signedDataParts, spec cmsHashSpec, original []byte) ([]byte, error) {
	if len(sd.eContentBytes) > 0 {
		return cmsDigest(spec.hash, sd.eContentBytes), nil
	}
	if len(original) > 0 {
		return cmsDigest(spec.hash, original), nil
	}
	for _, siDER := range sd.signerInfos {
		var si signerInfoRaw
		if _, err := asn1.Unmarshal(siDER, &si); err != nil {
			continue
		}
		if !si.DigestAlgorithm.Algorithm.Equal(spec.digestOID) || len(si.SignedAttributes.Bytes) == 0 {
			continue
		}
		if md, err := extractMessageDigestAttribute(si.SignedAttributes.Bytes); err == nil && len(md) > 0 {
			return md, nil
		}
	}
	return nil, fmt.Errorf("cofirma: la firma no contiene los datos y ningún firmante usa %s; hacen falta los datos originales", spec.hash)
}

func parseSignedDataParts(der []byte) (signedDataParts, error) {
	var out signedDataParts
	ci, err := parseContentInfo(der)
	if err != nil {
		return out, err
	}
	if !ci.ContentType.Equal(oidSignedData) {
		return out, errors.New("los datos no son una firma CAdES/CMS (SignedData)")
	}
	elems, err := splitSequence(ci.Content.Bytes)
	if err != nil {
		return out, fmt.Errorf("SignedData inválido: %w", err)
	}
	if len(elems) < 4 {
		return out, errors.New("SignedData incompleto")
	}
	out.version = elems[0]
	if out.digestAlgs, err = splitContents(elems[1]); err != nil {
		return out, fmt.Errorf("digestAlgorithms inválido: %w", err)
	}
	if _, err := asn1.Unmarshal(elems[2], &out.encapContent); err != nil {
		return out, err
	}
	var eci encapContentInfoRaw
	if _, err := asn1.Unmarshal(elems[2], &eci); err != nil {
		return out, fmt.Errorf("EncapContentInfo inválido: %w", err)
	}
	out.eContentType = eci.ContentType
	if len(eci.Content.Bytes) > 0 {
		var content []byte
		if _, err := asn1.Unmarshal(eci.Content.Bytes, &content); err == nil {
			out.eContentBytes = content
		}
	}
	for _, elem := range elems[3 : len(elems)-1] {
		var raw asn1.RawValue
		if _, err := asn1.Unmarshal(elem, &raw); err != nil {
			return out, err
		}
		switch {
		case raw.Class == asn1.ClassContextSpecific && raw.Tag == 0:
			if out.certificates, err = splitContents(elem); err != nil {
				return out, fmt.Errorf("certificados inválidos: %w", err)
			}
		case raw.Class == asn1.ClassContextSpecific && raw.Tag == 1:
			out.crls = elem
		default:
			return out, errors.New("SignedData con campos inesperados")
		}
	}
	if out.signerInfos, err = splitContents(elems[len(elems)-1]); err != nil {
		return out, fmt.Errorf("SignerInfos inválido: %w", err)
	}
	if len(out.signerInfos) == 0 {
		return out, errors.New("la firma no contiene firmantes")
	}
	return out, nil
}

func (sd *signedDataParts) addDigestAlgorithm(spec cmsHashSpec) {
	for _, raw := range sd.digestAlgs {
		var alg algorithmIdentifier
		if _, err := asn1.Unmarshal(raw, &alg); err == nil && alg.Algorithm.Equal(spec.digestOID) {
			return
		}
	}
	if der, err := asn1.Marshal(algorithmIdentifier{Algorithm: spec.digestOID}); err == nil {
		sd.digestAlgs = append(sd.digestAlgs, der)
	}
}

func (sd *signedDataParts) addCertificates(key *LocalSigningKey) error {
	candidatos := [][]byte{key.Certificate.Raw}
	for _, c := range key.Chain {
		if c != nil {
			candidatos = append(candidatos, c.Raw)
		}
	}
	for _, c := range candidatos {
		presente := false
		for _, existente := range sd.certificates {
			if bytes.Equal(existente, c) {
				presente = true
				break
			}
		}
		if !presente {
			sd.certificates = append(sd.certificates, c)
		}
	}
	return nil
}

func (sd signedDataParts) marshal() ([]byte, error) {
	digestSet, err := encodeTagged(asn1.ClassUniversal, asn1.TagSet, sortedSetContents(sd.digestAlgs))
	if err != nil {
		return nil, err
	}
	parts := [][]byte{sd.version, digestSet, sd.encapContent.FullBytes}
	if len(sd.certificates) > 0 {
		certs, err := encodeTagged(asn1.ClassContextSpecific, 0, sortedSetContents(sd.certificates))
		if err != nil {
			return nil, err
		}
		parts = append(parts, certs)
	}
	if len(sd.crls) > 0 {
		parts = append(parts, sd.crls)
	}
	signers, err := encodeTagged(asn1.ClassUniversal, asn1.TagSet, sortedSetContents(sd.signerInfos))
	if err != nil {
		return nil, err
	}
	parts = append(parts, signers)
	body, err := encodeTagged(asn1.ClassUniversal, asn1.TagSequence, bytes.Join(parts, nil))
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(contentInfo{
		ContentType: oidSignedData,
		Content:     asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: body},
	})
}

// signerInfoSignatureAndUnsigned localiza el valor de firma (OCTET STRING) y
// el índice de los atributos no firmados [1] de un SignerInfo desglosado.
func signerInfoSignatureAndUnsigned(elems [][]byte) ([]byte, int, error) {
	unsignedIdx := -1
	var signature []byte
	for i, elem := range elems {
		var raw asn1.RawValue
		if _, err := asn1.Unmarshal(elem, &raw); err != nil {
			return nil, -1, err
		}
		switch {
		case raw.Class == asn1.ClassUniversal && raw.Tag == asn1.TagOctetString && i >= 4:
			signature = raw.Bytes
		case raw.Class == asn1.ClassContextSpecific && raw.Tag == 1:
			unsignedIdx = i
		}
	}
	if len(signature) == 0 {
		return nil, -1, errors.New("contrafirma: SignerInfo sin valor de firma")
	}
	return signature, unsignedIdx, nil
}

// splitSequence devuelve los elementos DER completos de una SEQUENCE.
func splitSequence(der []byte) ([][]byte, error) {
	var raw asn1.RawValue
	rest, err := asn1.Unmarshal(der, &raw)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("datos sobrantes tras la estructura ASN.1")
	}
	return splitElements(raw.Bytes)
}

// splitContents devuelve los elementos DER contenidos en un SET, SEQUENCE o
// etiqueta implícita construida.
func splitContents(der []byte) ([][]byte, error) {
	var raw asn1.RawValue
	if _, err := asn1.Unmarshal(der, &raw); err != nil {
		return nil, err
	}
	return splitElements(raw.Bytes)
}

func splitElements(content []byte) ([][]byte, error) {
	var out [][]byte
	for len(content) > 0 {
		var raw asn1.RawValue
		rest, err := asn1.Unmarshal(content, &raw)
		if err != nil {
			return nil, err
		}
		out = append(out, append([]byte(nil), raw.FullBytes...))
		content = rest
	}
	return out, nil
}

// sortedSetContents concatena los elementos en el orden DER de SET OF.
func sortedSetContents(elems [][]byte) []byte {
	ordenados := append([][]byte(nil), elems...)
	for i := 1; i < len(ordenados); i++ {
		for j := i; j > 0 && bytes.Compare(ordenados[j-1], ordenados[j]) > 0; j-- {
			ordenados[j-1], ordenados[j] = ordenados[j], ordenados[j-1]
		}
	}
	return bytes.Join(ordenados, nil)
}

func encodeTagged(class, tag int, content []byte) ([]byte, error) {
	return asn1.Marshal(asn1.RawValue{Class: class, Tag: tag, IsCompound: true, Bytes: content})
}
