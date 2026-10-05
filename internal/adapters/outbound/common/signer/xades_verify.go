// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- verification must recognize existing legacy XAdES; new SHA-1 signatures are opt-in gated.
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"grxfirma/internal/adapters/outbound/common/officecontainer"
	"grxfirma/internal/domain"
)

type XAdESVerifier struct{}

func NewXAdESVerifier() *XAdESVerifier {
	return &XAdESVerifier{}
}

func (v *XAdESVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	verification, err := verifyXMLSignatureDocument(signedDocument.Content, true, nil, anchors, string(domain.FormatXAdES), "firma XAdES valida", []string{"referencias=ok", "signedproperties=ok"})
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	return applyXMLRevocation(ctx, verification.result, verification.signerCertificates, verification.embeddedCertificates), verification.signers, nil
}

type ODFVerifier struct{}

func NewODFVerifier() *ODFVerifier {
	return &ODFVerifier{}
}

func (v *ODFVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	xmlSig, err := officecontainer.ExtractODFSignatureXML(signedDocument.Content)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	resolver, err := zipEntryResolver(signedDocument.Content, false)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	signatures, err := extractContextualXMLSignatures(xmlSig)
	if err != nil {
		return domain.VerificationResult{}, nil, fmt.Errorf("extrayendo firmas ODF: %w", err)
	}
	if len(signatures) == 0 {
		return domain.VerificationResult{}, nil, errors.New("el envelope ODF no contiene firmas XML")
	}
	allSigners := make([]domain.CertificateRef, 0, len(signatures))
	allSignerCertificates := make([]*x509.Certificate, 0, len(signatures))
	allEmbeddedCertificates := make([]*x509.Certificate, 0, len(signatures))
	var compatibility []string
	for idx, signature := range signatures {
		verification, verifyErr := verifyXMLSignatureDocumentWithContext(signature.fragment, signature.context, false, resolver, anchors, "ODF", "firma ODF valida", nil)
		if verifyErr != nil {
			return domain.VerificationResult{}, nil, fmt.Errorf("firma ODF %d invalida: %w", idx+1, verifyErr)
		}
		allSigners = append(allSigners, verification.signers...)
		allSignerCertificates = append(allSignerCertificates, verification.signerCertificates...)
		allEmbeddedCertificates = append(allEmbeddedCertificates, verification.embeddedCertificates...)
		compatibility = append(compatibility, xmlCompatibilityDetails(verification.result)...)
	}
	result := domain.NewVerificationSuccess("ODF", "firma ODF valida", []string{
		fmt.Sprintf("firmas_xml=%d", len(signatures)),
		"contenedor=odf",
		"referencias_zip=ok",
	})
	result = applySignerVerificationMetadata(result, allSignerCertificates, allEmbeddedCertificates, anchors)
	for _, signature := range signatures {
		result = marcarSelloXMLNoEvaluable(result, signature.fragment)
	}
	result = markXMLCompatibility(result, compatibility)
	return applyXMLRevocation(ctx, result, allSignerCertificates, allEmbeddedCertificates), allSigners, nil
}

type OOXMLVerifier struct{}

func NewOOXMLVerifier() *OOXMLVerifier {
	return &OOXMLVerifier{}
}

func (v *OOXMLVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	signatures, err := officecontainer.ExtractOOXMLSignatureXMLs(signedDocument.Content)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	resolver, err := zipEntryResolver(signedDocument.Content, true)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}

	names := make([]string, 0, len(signatures))
	for name := range signatures {
		names = append(names, name)
	}
	sort.Strings(names)
	allSigners := make([]domain.CertificateRef, 0, len(signatures))
	allSignerCertificates := make([]*x509.Certificate, 0, len(signatures))
	allEmbeddedCertificates := make([]*x509.Certificate, 0, len(signatures))
	var compatibility []string
	verifiedSignatures := 0
	for _, name := range names {
		sigXML := signatures[name]
		parts, err := extractContextualXMLSignatures(sigXML)
		if err != nil || len(parts) == 0 {
			return domain.VerificationResult{}, nil, fmt.Errorf("entrada OOXML %s sin firmas XML utilizables", name)
		}
		for _, part := range parts {
			requireSigningCertificate := looksLikeXAdES(part.fragment)
			verification, verifyErr := verifyXMLSignatureDocumentWithContext(part.fragment, part.context, requireSigningCertificate, resolver, anchors, "OOXML", "firma OOXML valida", nil)
			if verifyErr != nil {
				return domain.VerificationResult{}, nil, fmt.Errorf("firma OOXML %s invalida: %w", name, verifyErr)
			}
			verifiedSignatures++
			allSigners = append(allSigners, verification.signers...)
			allSignerCertificates = append(allSignerCertificates, verification.signerCertificates...)
			allEmbeddedCertificates = append(allEmbeddedCertificates, verification.embeddedCertificates...)
			compatibility = append(compatibility, xmlCompatibilityDetails(verification.result)...)
		}
	}

	result := domain.NewVerificationSuccess("OOXML", "firma OOXML valida", []string{
		fmt.Sprintf("firmas_xml=%d", verifiedSignatures),
		"contenedor=ooxml",
		"referencias_zip=ok",
	})
	result = applySignerVerificationMetadata(result, allSignerCertificates, allEmbeddedCertificates, anchors)
	for _, name := range names {
		result = marcarSelloXMLNoEvaluable(result, signatures[name])
	}
	result = markXMLCompatibility(result, compatibility)
	return applyXMLRevocation(ctx, result, allSignerCertificates, allEmbeddedCertificates), allSigners, nil
}

type xmlSignatureVerification struct {
	result               domain.VerificationResult
	signers              []domain.CertificateRef
	signerCertificates   []*x509.Certificate
	embeddedCertificates []*x509.Certificate
}

func verifyXMLSignatureDocument(xmlData []byte, requireSigningCertificate bool, externalResolver func(string) ([]byte, error), anchors domain.CertificateChain, format string, reason string, details []string) (xmlSignatureVerification, error) {
	parts, err := extractContextualXMLSignatures(xmlData)
	if err != nil || len(parts) == 0 {
		return xmlSignatureVerification{}, errors.New("el documento no contiene firmas XML utilizables")
	}
	var aggregate xmlSignatureVerification
	var compatibility []string
	for _, part := range parts {
		verification, err := verifyXMLSignatureDocumentWithContext(part.fragment, part.context, requireSigningCertificate, externalResolver, anchors, format, reason, details)
		if err != nil {
			return xmlSignatureVerification{}, err
		}
		aggregate.signers = append(aggregate.signers, verification.signers...)
		aggregate.signerCertificates = append(aggregate.signerCertificates, verification.signerCertificates...)
		aggregate.embeddedCertificates = append(aggregate.embeddedCertificates, verification.embeddedCertificates...)
		compatibility = append(compatibility, xmlCompatibilityDetails(verification.result)...)
	}
	aggregate.result = applySignerVerificationMetadata(domain.NewVerificationSuccess(format, reason, details), aggregate.signerCertificates, aggregate.embeddedCertificates, anchors)
	for _, part := range parts {
		aggregate.result = marcarSelloXMLNoEvaluable(aggregate.result, part.fragment)
	}
	aggregate.result = markXMLCompatibility(aggregate.result, compatibility)
	return aggregate, nil
}

func verifyXMLSignatureDocumentWithContext(xmlData []byte, signatureContext xmlSignatureContext, requireSigningCertificate bool, externalResolver func(string) ([]byte, error), anchors domain.CertificateChain, format string, reason string, details []string) (xmlSignatureVerification, error) {
	signedInfoXML, signatureValue, certificates, err := extractContextualSignatureCore(xmlData, signatureContext)
	if err != nil {
		return xmlSignatureVerification{}, err
	}
	signerCertificates, compatibility, err := identifyXMLSignersWithContext(xmlData, signatureContext, signedInfoXML, signatureValue, certificates, requireSigningCertificate)
	if err != nil {
		return xmlSignatureVerification{}, err
	}
	referenceCompatibility, err := assessXMLReferences(xmlData, signatureContext, signedInfoXML, externalResolver)
	if err != nil {
		return xmlSignatureVerification{}, err
	}
	compatibility = append(compatibility, referenceCompatibility...)
	if usesSHA1(signedInfoXML) {
		// SHA-1 solo se acepta para leer firmas históricas: nunca como una
		// validación plena (CAdES/PAdES ya lo rechazan).
		compatibility = append(compatibility, "La firma usa SHA-1, un algoritmo obsoleto: solo tiene valor como evidencia histórica.")
	}

	signers := make([]domain.CertificateRef, 0, len(signerCertificates))
	for _, signerCertificate := range signerCertificates {
		signers = append(signers, certificateToRef(signerCertificate))
	}
	result := domain.NewVerificationSuccess(format, reason, details)
	result = applySignerVerificationMetadata(result, signerCertificates, certificates, anchors)
	result = marcarSelloXMLNoEvaluable(result, xmlData)
	result = markXMLCompatibility(result, compatibility)
	return xmlSignatureVerification{
		result:               result,
		signers:              signers,
		signerCertificates:   signerCertificates,
		embeddedCertificates: certificates,
	}, nil
}

func usesSHA1(signedInfoXML []byte) bool {
	var info signedInfoForVerify
	if xml.Unmarshal(signedInfoXML, &info) != nil {
		return false
	}
	if strings.HasSuffix(strings.TrimSpace(info.SignatureMethod.Algorithm), "-sha1") {
		return true
	}
	for _, ref := range info.References {
		if strings.HasSuffix(strings.TrimSpace(ref.DigestMethod.Algorithm), "#sha1") {
			return true
		}
	}
	return false
}

func extractXMLSignatureCore(xmlData []byte) ([]byte, string, []*x509.Certificate, error) {
	signedInfoXML, err := extractXMLLiteralElement(xmlData, "ds:SignedInfo")
	if err != nil {
		signedInfoXML, err = extractFirstElementByName(xmlData, nsXMLDSig, "SignedInfo")
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("signedinfo ausente: %w", err)
	}
	signatureValue, err := extractXMLLiteralText(xmlData, "ds:SignatureValue")
	if err != nil {
		signatureValue, err = extractFirstElementText(xmlData, nsXMLDSig, "SignatureValue")
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("signaturevalue ausente: %w", err)
	}
	certificates, err := extractXMLCertificates(xmlData)
	if err != nil {
		return nil, "", nil, err
	}
	return signedInfoXML, signatureValue, certificates, nil
}

func extractXMLCertificates(xmlData []byte) ([]*x509.Certificate, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	certificates := make([]*x509.Certificate, 0, 1)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("extrayendo certificados X.509: %w", err)
		}
		start, ok := token.(xml.StartElement)
		// Las firmas extraídas de ODF/OOXML pueden heredar xmlns:ds del
		// elemento raíz del contenedor. En ese fragmento aislado encoding/xml
		// conserva entonces el prefijo literal "ds" como espacio de nombres.
		if !ok || start.Name.Local != "X509Certificate" ||
			(start.Name.Space != nsXMLDSig && start.Name.Space != "ds") {
			continue
		}

		var certB64 string
		if err := decoder.DecodeElement(&certB64, &start); err != nil {
			return nil, fmt.Errorf("leyendo certificado x509 %d: %w", len(certificates)+1, err)
		}
		certDER, err := base64.StdEncoding.DecodeString(compactBase64(certB64))
		if err != nil {
			return nil, fmt.Errorf("certificado base64 invalido en posicion %d: %w", len(certificates)+1, err)
		}
		cert, err := parseCertificateForXML(certDER)
		if err != nil {
			return nil, fmt.Errorf("certificado x509 invalido en posicion %d: %w", len(certificates)+1, err)
		}
		certificates = append(certificates, cert)
	}
	if len(certificates) == 0 {
		return nil, errors.New("certificado x509 ausente")
	}
	return certificates, nil
}

func zipEntryResolver(container []byte, stripOOXMLQuery bool) (func(string) ([]byte, error), error) {
	reader, err := zip.NewReader(bytes.NewReader(container), int64(len(container)))
	if err != nil {
		return nil, err
	}
	entries := make(map[string]*zip.File, len(reader.File))
	for _, f := range reader.File {
		entries[normalizeReferenceURI(f.Name, false)] = f
	}
	var contentTypes ooxmlContentTypes
	if stripOOXMLQuery {
		contentTypesEntry, ok := entries[normalizeReferenceURI("[Content_Types].xml", false)]
		if !ok {
			return nil, errors.New("documento OOXML sin [Content_Types].xml")
		}
		contentTypesXML, err := readZipFile(contentTypesEntry)
		if err != nil {
			return nil, fmt.Errorf("leyendo [Content_Types].xml: %w", err)
		}
		contentTypes, err = parseOOXMLContentTypes(contentTypesXML)
		if err != nil {
			return nil, fmt.Errorf("parseando [Content_Types].xml: %w", err)
		}
	}
	return func(uri string) ([]byte, error) {
		name := normalizeReferenceURI(uri, stripOOXMLQuery)
		f, ok := entries[name]
		if !ok {
			return nil, fmt.Errorf("entrada ZIP no encontrada: %s", uri)
		}
		if stripOOXMLQuery {
			expectedContentType, hasContentType, err := ooxmlReferenceContentType(uri)
			if err != nil {
				return nil, err
			}
			if hasContentType {
				actualContentType := contentTypes.ContentTypeFor(f.Name)
				if !strings.EqualFold(actualContentType, expectedContentType) {
					return nil, fmt.Errorf(
						"content type no coincide para %s: firmado=%q actual=%q",
						f.Name,
						expectedContentType,
						actualContentType,
					)
				}
			}
		}
		return readZipFile(f)
	}, nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func ooxmlReferenceContentType(uri string) (string, bool, error) {
	raw := strings.TrimSpace(uri)
	queryIndex := strings.IndexByte(raw, '?')
	if queryIndex < 0 {
		return "", false, nil
	}
	query := raw[queryIndex+1:]
	const prefix = "ContentType="
	if !strings.HasPrefix(query, prefix) || strings.Contains(query, "&") {
		return "", false, fmt.Errorf("query OOXML no soportada en referencia: %s", uri)
	}
	contentType, err := url.PathUnescape(strings.TrimPrefix(query, prefix))
	if err != nil {
		return "", false, fmt.Errorf("content type OOXML invalido en referencia %s: %w", uri, err)
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return "", false, fmt.Errorf("content type OOXML vacio en referencia: %s", uri)
	}
	return contentType, true, nil
}

func normalizeReferenceURI(uri string, stripQuery bool) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(uri, "\\", "/"))
	if stripQuery {
		if q := strings.IndexByte(normalized, '?'); q >= 0 {
			normalized = normalized[:q]
		}
	}
	normalized = strings.TrimPrefix(normalized, "/")
	if unescaped, err := url.PathUnescape(normalized); err == nil {
		normalized = unescaped
	}
	return strings.ToLower(normalized)
}

func parseCertificateForXML(certDER []byte) (*x509.Certificate, error) {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parseando certificado: %w", err)
	}
	return cert, nil
}

func verifyXAdESSignatureValue(xmlData, signedInfoXML []byte, signatureValue string, cert *x509.Certificate) error {
	var signedInfo signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &signedInfo); err != nil {
		return fmt.Errorf("parseando signedinfo: %w", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("la firma XAdES actual solo soporta claves RSA")
	}
	// El método identifica también el tipo de firma, no solo el hash. Una
	// firma RSA no puede validar una declaración ECDSA ni un método ausente.
	method := strings.TrimSpace(signedInfo.SignatureMethod.Algorithm)
	switch method {
	case algRSASHA1, algRSASHA256, algRSASHA384, algRSASHA512:
		// SHA-1 se conserva exclusivamente para verificar firmas históricas.
	default:
		return errors.New("signaturemethod no soportado para una clave RSA")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(compactBase64(signatureValue))
	if err != nil {
		return fmt.Errorf("signaturevalue base64 invalido: %w", err)
	}
	candidates, err := canonicalizeSignedInfoCandidates(xmlData, signedInfoXML)
	if err != nil {
		return fmt.Errorf("canonicalizando signedinfo: %w", err)
	}
	var lastErr error
	for _, candidate := range candidates {
		hash, digest, digestErr := digestForSignatureMethod(method, candidate)
		if digestErr != nil {
			lastErr = digestErr
			continue
		}
		if verifyErr := rsa.VerifyPKCS1v15(pub, hash, digest, sigBytes); verifyErr == nil {
			return nil
		} else {
			lastErr = verifyErr
		}
	}
	return fmt.Errorf("firma XAdES invalida: %w", lastErr)
}

func resolveReferenceTarget(xmlData []byte, uri string, externalResolver func(string) ([]byte, error)) ([]byte, error) {
	if strings.TrimSpace(uri) == "" {
		return xmlData, nil
	}
	if strings.HasPrefix(uri, "#") {
		targetID := strings.TrimPrefix(uri, "#")
		if targetID == "" {
			return nil, fmt.Errorf("URI interna vacía")
		}
		return resolveReferencedElement(xmlData, targetID)
	}
	if externalResolver != nil {
		return externalResolver(uri)
	}
	return resolveDetachedContent(xmlData, uri)
}

func digestForSignatureMethod(algorithm string, data []byte) (crypto.Hash, []byte, error) {
	switch algorithm {
	case "", "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256":
		sum := sha256.Sum256(data)
		return crypto.SHA256, sum[:], nil
	case "http://www.w3.org/2000/09/xmldsig#rsa-sha1", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha1":
		sum := sha1.Sum(data) // #nosec G401 -- read-only verification of an existing legacy signature.
		return crypto.SHA1, sum[:], nil
	case "http://www.w3.org/2001/04/xmldsig-more#rsa-sha384", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384":
		sum := sha512.Sum384(data)
		return crypto.SHA384, sum[:], nil
	case "http://www.w3.org/2001/04/xmldsig-more#rsa-sha512", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512":
		sum := sha512.Sum512(data)
		return crypto.SHA512, sum[:], nil
	default:
		return 0, nil, fmt.Errorf("signaturemethod no soportado: %s", algorithm)
	}
}

func resolveReferencedElement(xmlData []byte, id string) ([]byte, error) {
	spans, err := scanXMLElements(xmlData)
	if err != nil {
		return nil, err
	}
	target, err := uniqueElementByID(spans, id)
	if err != nil {
		return nil, err
	}
	literalCandidates := []string{"xades:SignedProperties", "ds:KeyInfo", "CONTENT"}
	for _, tag := range literalCandidates {
		if !literalMatchesSpan(xmlData, tag, target) {
			continue
		}
		element, err := extractXMLLiteralElement(xmlData, tag)
		if err != nil {
			continue
		}
		return element, nil
	}
	return extractElementByID(xmlData, id)
}

func resolveDetachedContent(xmlData []byte, name string) ([]byte, error) {
	spans, err := scanXMLElements(xmlData)
	if err != nil {
		return nil, err
	}
	var contents []xmlElementSpan
	for _, span := range spans {
		if span.name.Local == "CONTENT" {
			contents = append(contents, span)
		}
	}
	if len(contents) != 1 || !literalMatchesSpan(xmlData, "CONTENT", contents[0]) {
		return nil, fmt.Errorf("contenido detached %s ambiguo o no encontrado", name)
	}
	contentXML, err := extractXMLLiteralElement(xmlData, "CONTENT")
	if err != nil {
		return nil, err
	}
	headEnd := strings.Index(string(contentXML), ">")
	if headEnd <= 0 || !strings.Contains(string(contentXML[:headEnd]), `Name="`+name+`"`) {
		return nil, fmt.Errorf("contenido detached %s no encontrado", name)
	}
	contentB64, err := extractXMLLiteralText(xmlData, "CONTENT")
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(compactBase64(contentB64))
	if err != nil {
		return nil, fmt.Errorf("contenido detached base64 inválido: %w", err)
	}
	return data, nil
}

func verifySigningCertificate(xmlData, signedInfoXML []byte, cert *x509.Certificate) error {
	signedPropsXML, err := referencedSignedProperties(xmlData, signedInfoXML)
	if err != nil {
		return fmt.Errorf("signedproperties ausente o no referenciado: %w", err)
	}
	var props signedPropertiesForVerify
	if err := xml.Unmarshal(signedPropsXML, &props); err != nil {
		return fmt.Errorf("parseando signedproperties: %w", err)
	}

	switch {
	case len(props.SigningCertificateV2.Certs) > 0 && props.SigningCertificateV2.Certs[0].CertDigest.DigestValue != "":
		got, err := digestXML(props.SigningCertificateV2.Certs[0].CertDigest.DigestMethod.Algorithm, cert.Raw)
		if err != nil {
			return err
		}
		expected, err := base64.StdEncoding.DecodeString(compactBase64(props.SigningCertificateV2.Certs[0].CertDigest.DigestValue))
		if err != nil {
			return err
		}
		if !equalBytes(got, expected) {
			return fmt.Errorf("signingcertificatev2 no coincide")
		}
		return nil
	case len(props.SigningCertificate.Certs) > 0 && props.SigningCertificate.Certs[0].CertDigest.DigestValue != "":
		got, err := digestXML(props.SigningCertificate.Certs[0].CertDigest.DigestMethod.Algorithm, cert.Raw)
		if err != nil {
			return err
		}
		expected, err := base64.StdEncoding.DecodeString(compactBase64(props.SigningCertificate.Certs[0].CertDigest.DigestValue))
		if err != nil {
			return err
		}
		if !equalBytes(got, expected) {
			return fmt.Errorf("signingcertificate no coincide")
		}
		return nil
	default:
		return fmt.Errorf("signedproperties no contiene signingcertificate")
	}
}

func canonicalizeSignedInfo(signedInfoXML []byte) ([]byte, error) {
	var signedInfo signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &signedInfo); err != nil {
		return nil, err
	}
	algorithm := strings.TrimSpace(signedInfo.CanonicalizationMethod.Algorithm)
	if algorithm == "" {
		algorithm = algExcC14N
	}
	return canonicalizeXML(signedInfoXML, algorithm)
}

func canonicalizeSignedInfoCandidates(xmlData, signedInfoXML []byte) ([][]byte, error) {
	var signedInfo signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &signedInfo); err != nil {
		return nil, err
	}
	algorithm := strings.TrimSpace(signedInfo.CanonicalizationMethod.Algorithm)
	if algorithm == "" {
		algorithm = algExcC14N
	}

	first, err := canonicalizeElementInDocument(xmlData, nsXMLDSig, "SignedInfo", algorithm)
	if err != nil {
		first, err = canonicalizeSignedInfo(signedInfoXML)
		if err != nil {
			return nil, err
		}
	}
	out := [][]byte{first}
	if fragment, fragmentErr := canonicalizeSignedInfo(signedInfoXML); fragmentErr == nil && !containsDigest(out, fragment) {
		out = append(out, fragment)
	}
	raw := bytes.TrimSpace(signedInfoXML)
	if len(raw) > 0 && !containsDigest(out, raw) {
		out = append(out, raw)
	}
	if c14n, err := exclusiveC14N(string(raw)); err == nil && !containsDigest(out, c14n) {
		out = append(out, c14n)
	}
	return out, nil
}

func canonicalizeReference(ref referenceForVerify, targetXML []byte) ([]byte, error) {
	working := append([]byte(nil), targetXML...)
	if len(ref.Transforms) == 0 {
		canonical, err := canonicalizeXML(working, algC14N)
		if err != nil {
			return working, nil
		}
		return canonical, nil
	}
	for _, transform := range ref.Transforms {
		algorithm := strings.TrimSpace(transform.Algorithm)
		switch algorithm {
		case algEnveloped:
			working = stripEnvelopedSignature(working)
		case algExcC14N, algC14N:
			return canonicalizeXML(working, algorithm)
		case algBase64Transform:
			// XMLDSig 6.6.2: el resumen cubre los octetos decodificados del
			// texto del nodo (firmas Java de datos binarios).
			return decodeBase64TransformInput(working)
		case algXPathFilter:
			if !esXPathExcluirFirmas(transform) {
				return nil, fmt.Errorf("transform XPath no soportado")
			}
			sinFirmas, err := quitarTodasLasFirmas(working)
			if err != nil {
				return nil, err
			}
			return canonicalizeXML(sinFirmas, algC14N)
		default:
			return nil, fmt.Errorf("transform no soportado: %s", transform.Algorithm)
		}
	}
	canonical, err := canonicalizeXML(working, algC14N)
	if err != nil {
		return working, nil
	}
	return canonical, nil
}

// computeContextualReferenceDigest canonicaliza las referencias internas sobre
// el árbol original. C14N inclusiva incorpora declaraciones xmlns y atributos
// xml:* de los ancestros, información que se pierde al serializar solo el
// elemento referenciado.
func computeContextualReferenceDigest(xmlData []byte, ref referenceForVerify) ([]byte, error) {
	uri := strings.TrimSpace(ref.URI)
	algorithm := algC14N
	hasEnvelopedTransform := false
	for _, transform := range ref.Transforms {
		switch strings.TrimSpace(transform.Algorithm) {
		case algEnveloped:
			hasEnvelopedTransform = true
		case algExcC14N, algC14N:
			algorithm = strings.TrimSpace(transform.Algorithm)
		case algBase64Transform:
			// Único caso admitido: Base64 como sola transformación sobre un
			// nodo interno (ds:Object o CONTENT de las firmas Java).
			if len(ref.Transforms) != 1 || !strings.HasPrefix(uri, "#") {
				return nil, errors.New("transformación Base64 combinada o sobre referencia no interna")
			}
			element, err := resolveReferencedElement(xmlData, strings.TrimPrefix(uri, "#"))
			if err != nil {
				return nil, err
			}
			decoded, err := decodeBase64TransformInput(element)
			if err != nil {
				return nil, err
			}
			return digestXML(ref.DigestMethod.Algorithm, decoded)
		}
	}

	var (
		canonical []byte
		err       error
	)
	switch {
	case uri == "":
		working := xmlData
		if hasEnvelopedTransform {
			working = stripEnvelopedSignature(working)
		}
		canonical, err = canonicalizeXML(working, algorithm)
	case strings.HasPrefix(uri, "#"):
		if hasEnvelopedTransform {
			return nil, errors.New("transformación enveloped sobre referencia interna no soportada en el candidato contextual")
		}
		canonical, err = canonicalizeElementByIDInDocument(xmlData, strings.TrimPrefix(uri, "#"), algorithm)
	default:
		return nil, errors.New("la referencia externa no pertenece al árbol XML")
	}
	if err != nil {
		return nil, err
	}
	return digestXML(ref.DigestMethod.Algorithm, canonical)
}

func computeReferenceDigests(ref referenceForVerify, targetXML []byte) ([][]byte, error) {
	c14n, err := canonicalizeReference(ref, targetXML)
	if err != nil {
		return nil, err
	}
	primary, err := digestXML(ref.DigestMethod.Algorithm, c14n)
	if err != nil {
		return nil, err
	}
	digests := [][]byte{primary}
	compatibilityInput := append([]byte(nil), targetXML...)
	hasExplicitCanonicalization := false
	for _, transform := range ref.Transforms {
		switch strings.TrimSpace(transform.Algorithm) {
		case algEnveloped:
			compatibilityInput = stripEnvelopedSignature(compatibilityInput)
		case algExcC14N, algC14N:
			hasExplicitCanonicalization = true
		}
	}
	if !hasExplicitCanonicalization {
		if exclusive, exclusiveErr := exclusiveC14N(string(compatibilityInput)); exclusiveErr == nil {
			if exclusiveDigest, digestErr := digestXML(ref.DigestMethod.Algorithm, exclusive); digestErr == nil && !containsDigest(digests, exclusiveDigest) {
				digests = append(digests, exclusiveDigest)
			}
		}
	}
	if legacy := legacyExclusiveC14N(string(compatibilityInput)); len(legacy) > 0 {
		if legacyDigest, legacyErr := digestXML(ref.DigestMethod.Algorithm, legacy); legacyErr == nil && !containsDigest(digests, legacyDigest) {
			digests = append(digests, legacyDigest)
		}
	}
	if len(ref.Transforms) == 0 {
		raw, err := digestXML(ref.DigestMethod.Algorithm, targetXML)
		if err == nil && !containsDigest(digests, raw) {
			digests = append(digests, raw)
		}
	}
	return digests, nil
}

func digestXML(algorithm string, data []byte) ([]byte, error) {
	switch strings.TrimSpace(algorithm) {
	case algSHA256:
		sum := sha256.Sum256(data)
		return sum[:], nil
	case algSHA384:
		sum := sha512.Sum384(data)
		return sum[:], nil
	case "http://www.w3.org/2001/04/xmlenc#sha512":
		sum := sha512.Sum512(data)
		return sum[:], nil
	case "http://www.w3.org/2000/09/xmldsig#sha1":
		sum := sha1.Sum(data) // #nosec G401 -- read-only verification of an existing legacy digest.
		return sum[:], nil
	default:
		return nil, fmt.Errorf("digestmethod no soportado: %s", algorithm)
	}
}

func containsDigest(candidates [][]byte, expected []byte) bool {
	for _, candidate := range candidates {
		if equalBytes(candidate, expected) {
			return true
		}
	}
	return false
}

func extractFirstElementByName(xmlData []byte, space, local string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Space == space && start.Name.Local == local {
			return captureCurrentElement(dec, start)
		}
	}
}

func extractRawElementsByName(xmlData []byte, space, local string) ([][]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	var elements [][]byte
	for {
		startOffset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return elements, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Space != space || start.Name.Local != local {
			continue
		}
		depth := 1
		for depth > 0 {
			token, err = decoder.Token()
			if err != nil {
				return nil, err
			}
			switch token.(type) {
			case xml.StartElement:
				depth++
			case xml.EndElement:
				depth--
			}
		}
		endOffset := decoder.InputOffset()
		if startOffset < 0 || endOffset < startOffset || endOffset > int64(len(xmlData)) {
			return nil, errors.New("offset XML fuera de rango al extraer firmas")
		}
		elements = append(elements, append([]byte(nil), xmlData[startOffset:endOffset]...))
	}
}

func extractFirstElementText(xmlData []byte, space, local string) (string, error) {
	element, err := extractFirstElementByName(xmlData, space, local)
	if err != nil {
		return "", err
	}
	var token struct {
		Text string `xml:",innerxml"`
	}
	if err := xml.Unmarshal(element, &token); err != nil {
		return "", err
	}
	return token.Text, nil
}

func extractElementByID(xmlData []byte, id string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range start.Attr {
			if strings.EqualFold(attr.Name.Local, "Id") && attr.Value == id {
				return captureCurrentElement(dec, start)
			}
		}
	}
}

func captureCurrentElement(dec *xml.Decoder, start xml.StartElement) ([]byte, error) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	depth := 1
	if err := enc.EncodeToken(start); err != nil {
		return nil, err
	}
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
		if err := enc.EncodeToken(tok); err != nil {
			return nil, err
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type signedInfoForVerify struct {
	XMLName                xml.Name             `xml:"SignedInfo"`
	CanonicalizationMethod algorithmAttr        `xml:"CanonicalizationMethod"`
	SignatureMethod        algorithmAttr        `xml:"SignatureMethod"`
	References             []referenceForVerify `xml:"Reference"`
}

type referenceForVerify struct {
	URI          string          `xml:"URI,attr"`
	DigestValue  string          `xml:"DigestValue"`
	DigestMethod algorithmAttr   `xml:"DigestMethod"`
	Transforms   []algorithmAttr `xml:"Transforms>Transform"`
}

type algorithmAttr struct {
	Algorithm  string `xml:"Algorithm,attr"`
	Parameters string `xml:",innerxml"`
}

type signedPropertiesForVerify struct {
	XMLName              xml.Name                    `xml:"SignedProperties"`
	SigningCertificateV2 signingCertificateV2Wrapper `xml:"SignedSignatureProperties>SigningCertificateV2"`
	SigningCertificate   signingCertificateWrapper   `xml:"SignedSignatureProperties>SigningCertificate"`
}

type signingCertificateV2Wrapper struct {
	Certs []signingCertificateEntry `xml:"Cert"`
}

type signingCertificateWrapper struct {
	Certs []signingCertificateEntry `xml:"Cert"`
}

type signingCertificateEntry struct {
	CertDigest digestEntry `xml:"CertDigest"`
}

type digestEntry struct {
	DigestMethod algorithmAttr `xml:"DigestMethod"`
	DigestValue  string        `xml:"DigestValue"`
}

func compactBase64(value string) string {
	replacer := strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "")
	return replacer.Replace(value)
}

func extractXMLLiteralElement(xmlData []byte, tag string) ([]byte, error) {
	source := string(xmlData)
	start := literalElementOffset(source, tag)
	if start < 0 {
		return nil, fmt.Errorf("tag %s no encontrada", tag)
	}
	endTag := "</" + tag + ">"
	end := strings.Index(source[start:], endTag)
	if end < 0 {
		return nil, fmt.Errorf("cierre %s no encontrado", tag)
	}
	end += start + len(endTag)
	element := source[start:end]
	headEnd := strings.Index(element, ">")
	if headEnd < 0 {
		return nil, fmt.Errorf("apertura invalida en %s", tag)
	}
	head := element[:headEnd+1]
	switch {
	case strings.HasPrefix(tag, "ds:") && !strings.Contains(head, `xmlns:ds=`):
		element = strings.Replace(element, "<"+tag, `<`+tag+` xmlns:ds="`+nsXMLDSig+`"`, 1)
	case strings.HasPrefix(tag, "xades:") && !strings.Contains(head, `xmlns:xades=`):
		element = strings.Replace(element, "<"+tag, `<`+tag+` xmlns:xades="`+nsXAdES+`"`, 1)
	}
	headEnd = strings.Index(element, ">")
	head = element[:headEnd+1]
	if strings.Contains(element, "ds:") && !strings.Contains(head, `xmlns:ds=`) {
		element = strings.Replace(element, "<"+tag, `<`+tag+` xmlns:ds="`+nsXMLDSig+`"`, 1)
	}
	if strings.Contains(element, "xades:") && !strings.Contains(head, `xmlns:xades=`) {
		element = strings.Replace(element, "<"+tag, `<`+tag+` xmlns:xades="`+nsXAdES+`"`, 1)
	}
	return []byte(element), nil
}

func extractXMLLiteralText(xmlData []byte, tag string) (string, error) {
	element, err := extractXMLLiteralElement(xmlData, tag)
	if err != nil {
		return "", err
	}
	source := string(element)
	start := strings.Index(source, ">")
	end := strings.LastIndex(source, "</")
	if start < 0 || end < 0 || end <= start {
		return "", fmt.Errorf("contenido invalido en %s", tag)
	}
	return source[start+1 : end], nil
}

func stripEnvelopedSignature(xmlData []byte) []byte {
	source := string(xmlData)
	startTags := []string{"<ds:Signature", "<Signature"}
	endTags := []string{"</ds:Signature>", "</Signature>"}
	for i, startTag := range startTags {
		start := strings.Index(source, startTag)
		if start < 0 {
			continue
		}
		end := strings.Index(source[start:], endTags[i])
		if end < 0 {
			continue
		}
		end += start + len(endTags[i])
		return []byte(source[:start] + source[end:])
	}
	return xmlData
}

// decodeBase64TransformInput extrae el texto del nodo referenciado y lo
// decodifica en Base64 (tolerando saltos de línea y espacios).
func decodeBase64TransformInput(nodeXML []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(nodeXML))
	var text strings.Builder
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("transformación Base64 sobre XML inválido: %w", err)
		}
		if cd, ok := tok.(xml.CharData); ok {
			text.Write(cd)
		}
	}
	out, err := base64.StdEncoding.DecodeString(compactBase64(text.String()))
	if err != nil {
		return nil, fmt.Errorf("transformación Base64: contenido no válido: %w", err)
	}
	return out, nil
}
