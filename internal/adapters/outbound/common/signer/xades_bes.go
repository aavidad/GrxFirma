// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/cryptopolicy"
)

const (
	nsXMLDSig = "http://www.w3.org/2000/09/xmldsig#"
	nsXAdES   = "http://uri.etsi.org/01903/v1.3.2#"
	nsExcC14N = "http://www.w3.org/2001/10/xml-exc-c14n#"

	algRSASHA256        = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	algRSASHA384        = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha384"
	algRSASHA512        = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha512"
	algSHA256           = "http://www.w3.org/2001/04/xmlenc#sha256"
	algSHA384           = "http://www.w3.org/2001/04/xmldsig-more#sha384"
	algSHA512           = "http://www.w3.org/2001/04/xmlenc#sha512"
	algExcC14N          = "http://www.w3.org/2001/10/xml-exc-c14n#"
	algC14N             = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	typeSignedProps     = "http://uri.etsi.org/01903#SignedProperties"
	typeSignedPropsV122 = "http://uri.etsi.org/01903/v1.2.2#SignedProperties"
	typeXMLDSigObject   = "http://www.w3.org/2000/09/xmldsig#Object"
	xadesNamespaceV132  = "http://uri.etsi.org/01903/v1.3.2#"
	xadesNamespaceV122  = "http://uri.etsi.org/01903/v1.2.2#"
)

// XAdESBESDetached implementa ports.SignerEngine para XAdES-BES detached conforme a ETSI EN 319 132-1.
type XAdESBESDetached struct{}

// NewXAdESBESDetached crea el motor de firma XAdES-BES detached en Go nativo.
func NewXAdESBESDetached() *XAdESBESDetached {
	return &XAdESBESDetached{}
}

// Sign firma un documento con XAdES-BES detached.
// Retorna el XML de la firma como bytes en domain.SignatureResult.Data.
func (e *XAdESBESDetached) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if key == nil {
		return domain.SignatureResult{}, errors.New("la clave de firma no puede ser nil")
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != domain.FormatXAdES {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", domain.FormatXAdES)
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	avisarIncidenciasFacturaB2B(job)

	var (
		xmlSig         []byte
		algorithmLabel string
	)
	switch variante := xadesVariante(job.Options); {
	case job.Action == domain.ActionCoSign:
		xmlSig, algorithmLabel, err = cosignXAdES(job, clave)
	case job.Action == domain.ActionCounterSign:
		xmlSig, algorithmLabel, err = countersignXAdES(job, clave)
	case job.Action != domain.ActionSign:
		err = fmt.Errorf("acción de firma XAdES no soportada: %s", job.Action)
	case variante != xadesVarianteDetached:
		xmlSig, algorithmLabel, err = buildXAdESEnvelop(job, clave, variante)
	default:
		xmlSig, algorithmLabel, err = buildXAdESBES(job, clave)
	}
	if err != nil {
		return domain.SignatureResult{}, err
	}

	return domain.SignatureResult{
		Format:    domain.FormatXAdES,
		Data:      xmlSig,
		Algorithm: algorithmLabel,
	}, nil
}

func requireLocalSigningKeyXAdES(key ports.SigningKey) (*LocalSigningKey, error) {
	if key == nil {
		return nil, errors.New("la clave de firma no puede ser nil")
	}
	clave, ok := key.(*LocalSigningKey)
	if !ok || clave == nil {
		return nil, errors.New("la clave de firma no es compatible con el motor XAdES local")
	}
	if clave.Signer == nil {
		return nil, errors.New("la clave de firma local no contiene signer")
	}
	if clave.Certificate == nil {
		return nil, errors.New("la clave de firma local no contiene certificado")
	}
	// XAdES-BES con este motor solo soporta RSA
	if _, ok := clave.Signer.Public().(*rsa.PublicKey); !ok {
		return nil, errors.New("el motor XAdES-BES solo soporta claves RSA en esta implementacion")
	}
	return clave, nil
}

type xadesPolicyOptions struct {
	Identifier string
	Hash       string
	HashAlgURI string
	Qualifier  string
}

type xadesAlgorithmOptions struct {
	Label           string
	Hash            crypto.Hash
	SignatureMethod string
	DigestMethod    string
}

type xadesBuildOptions struct {
	Algorithm             xadesAlgorithmOptions
	Policy                xadesPolicyOptions
	Namespace             string
	SignedPropertiesURI   string
	UseSigningCertV2      bool
	CanonicalizationAlg   string
	DocumentTransform     string
	OmitDocumentTransform bool
	DocumentReferenceType string
	IncludeDataEncoding   bool
	// DocumentTransforms, si no está vacío, sustituye a DocumentTransform por
	// una cadena de transformaciones (enveloped + C14N, Base64...).
	DocumentTransforms []string
	// SinXAdES genera XMLDSig pura: sin SignedProperties ni
	// QualifyingProperties.
	SinXAdES bool
}

// opcionXMLDSigPura marca internamente un trabajo XMLDSig que reutiliza el
// constructor XAdES.
const opcionXMLDSigPura = "grxfirma.xmldsigPura"

// buildXAdESBES construye el XML de firma XAdES-BES detached.
func buildXAdESBES(job domain.SignatureJob, key *LocalSigningKey) ([]byte, string, error) {
	documentName := job.Document.Name
	data := job.Document.Content
	mimeType := job.Document.MIMEType
	algOpts, err := resolveXAdESAlgorithmOptions(job.Options)
	if err != nil {
		return nil, "", err
	}
	buildOpts := resolveXAdESBuildOptions(job.Options, algOpts)

	sigID := "Signature-1"
	keyInfoID := "KeyInfo-1"
	signedPropsID := "SignedProperties-1"
	qualifyingPropsID := "QualifyingProperties-1"
	referenceID := "Reference-Document-1"
	contentID := "CONTENT-1"
	normalizedMimeType := normalizeMimeType(mimeType, data)
	// Los datos que no son XML (texto, binarios) no se pueden canonicalizar:
	// se firman como hace AutoFirma Java, en Base64 dentro de CONTENT.
	datosNoXML := !isXMLPayload(data, normalizedMimeType)
	if datosNoXML && strings.Contains(strings.ToLower(normalizedMimeType), "xml") {
		normalizedMimeType = detectarMimeNoXML(data)
	}
	javaDetachedCompat := useJavaDetachedCompat(job.Options) || datosNoXML
	contentXML := buildContentXML(documentName, normalizedMimeType, data)
	contentForDigest := string(data)
	documentURI := documentName
	if strings.TrimSpace(documentURI) == "" {
		documentURI = "documento.xml"
	}
	if javaDetachedCompat {
		buildOpts.CanonicalizationAlg = algC14N
		buildOpts.DocumentTransform = algC14N
		buildOpts.DocumentReferenceType = typeXMLDSigObject
		buildOpts.IncludeDataEncoding = true
		contentXML = buildXAdESDetachedContentXML(contentID, normalizedMimeType, data)
		contentForDigest = contentXML
		documentURI = "#" + contentID
	}

	// La compatibilidad AGE declara inclusiva para CONTENT; el modo normal
	// declara exclusiva. El digest debe seguir ese contrato, incluidos xmlns
	// que no se usan visiblemente en el documento.
	contentC14N, err := canonicalizeXML([]byte(contentForDigest), buildOpts.DocumentTransform)
	if err != nil {
		return nil, "", fmt.Errorf("error canonicalizando documento XML detached: %w", err)
	}
	docDigest, err := digestBytes(algOpts.Hash, contentC14N)
	if err != nil {
		return nil, "", err
	}
	docDigestB64 := base64.StdEncoding.EncodeToString(docDigest)

	signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML, err := firmarPartesXAdES(key, algOpts, buildOpts, xadesIDs{
		sig: sigID, keyInfo: keyInfoID, signedProps: signedPropsID, reference: referenceID,
	}, documentURI, docDigestB64, normalizedMimeType)
	if err != nil {
		return nil, "", err
	}

	xmlOut := buildFinalXML(contentXML, sigID, signedInfoXML, sigValueB64, keyInfoXML, qualifyingPropsID, signedPropsXML, buildOpts)
	return []byte(xmlOut), buildOpts.Algorithm.Label, nil
}

// buildSignedInfoXML construye el elemento ds:SignedInfo como string XML.
// xadesIDs agrupa los identificadores internos de una firma XAdES.
type xadesIDs struct {
	sig, keyInfo, signedProps, reference string
}

// firmarPartesXAdES genera las propiedades firmadas, el KeyInfo, el
// SignedInfo con la referencia al documento ya resumida y el valor de firma.
// Es común a las variantes Detached, Enveloping y Enveloped.
func firmarPartesXAdES(key *LocalSigningKey, algOpts xadesAlgorithmOptions, buildOpts xadesBuildOptions, ids xadesIDs, documentURI, docDigestB64, mimeType string) (signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML string, err error) {
	certDER := key.Certificate.Raw
	certDigest, err := digestBytes(algOpts.Hash, certDER)
	if err != nil {
		return "", "", "", "", err
	}
	certDigestB64 := base64.StdEncoding.EncodeToString(certDigest)

	issuerSerial, err := buildIssuerSerialXML(key.Certificate)
	if err != nil {
		return "", "", "", "", fmt.Errorf("error construyendo IssuerSerial: %w", err)
	}

	signingTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	signedPropsDigestB64 := ""
	if !buildOpts.SinXAdES {
		signedPropsXML = buildSignedPropertiesXML(ids.signedProps, ids.sig, signingTime, certDigestB64, issuerSerial, ids.reference, mimeType, buildOpts)
		signedPropsC14N, err := exclusiveC14N(signedPropsXML)
		if err != nil {
			return "", "", "", "", fmt.Errorf("error canonicalizando SignedProperties: %w", err)
		}
		signedPropsDigest, err := digestBytes(algOpts.Hash, signedPropsC14N)
		if err != nil {
			return "", "", "", "", err
		}
		signedPropsDigestB64 = base64.StdEncoding.EncodeToString(signedPropsDigest)
	}

	certB64 := base64.StdEncoding.EncodeToString(certDER)
	keyInfoXML = buildKeyInfoXML(ids.keyInfo, certB64)
	keyInfoC14N, err := exclusiveC14N(keyInfoXML)
	if err != nil {
		return "", "", "", "", fmt.Errorf("error canonicalizando KeyInfo: %w", err)
	}
	keyInfoDigest, err := digestBytes(algOpts.Hash, keyInfoC14N)
	if err != nil {
		return "", "", "", "", err
	}
	keyInfoDigestB64 := base64.StdEncoding.EncodeToString(keyInfoDigest)

	signedInfoXML = buildSignedInfoXML(documentURI, ids.reference, docDigestB64, ids.keyInfo, keyInfoDigestB64, signedPropsDigestB64, ids.signedProps, buildOpts)
	signedInfoC14N, err := exclusiveC14N(signedInfoXML)
	if err != nil {
		return "", "", "", "", fmt.Errorf("error canonicalizando SignedInfo: %w", err)
	}

	hashSignedInfo, err := digestBytes(algOpts.Hash, signedInfoC14N)
	if err != nil {
		return "", "", "", "", err
	}
	sigBytes, err := key.Signer.Sign(rand.Reader, hashSignedInfo, algOpts.Hash)
	if err != nil {
		return "", "", "", "", fmt.Errorf("error firmando SignedInfo: %w", err)
	}
	sigValueB64 = base64.StdEncoding.EncodeToString(sigBytes)
	return signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML, nil
}

func buildSignedInfoXML(documentURI, referenceID, docDigestB64, keyInfoID, keyInfoDigestB64, signedPropsDigestB64, signedPropsID string, opts xadesBuildOptions) string {
	canonicalizationAlg := opts.CanonicalizationAlg
	if canonicalizationAlg == "" {
		canonicalizationAlg = algExcC14N
	}
	documentTransform := opts.DocumentTransform
	if documentTransform == "" && !opts.OmitDocumentTransform {
		documentTransform = algExcC14N
	}
	documentTransformsXML := ""
	if len(opts.DocumentTransforms) > 0 {
		var b strings.Builder
		b.WriteString(`<ds:Transforms>`)
		for _, t := range opts.DocumentTransforms {
			if t == algXPathFilter {
				fmt.Fprintf(&b, `<ds:Transform Algorithm="%s">%s</ds:Transform>`, algXPathFilter, xpathTransformFirmasXML)
				continue
			}
			fmt.Fprintf(&b, `<ds:Transform Algorithm="%s"/>`, escapeXMLAttr(t))
		}
		b.WriteString(`</ds:Transforms>`)
		documentTransformsXML = b.String()
	} else if !opts.OmitDocumentTransform {
		documentTransformsXML = fmt.Sprintf(
			`<ds:Transforms><ds:Transform Algorithm="%s"/></ds:Transforms>`,
			escapeXMLAttr(documentTransform),
		)
	}
	referenceTypeAttr := ""
	if strings.TrimSpace(opts.DocumentReferenceType) != "" {
		referenceTypeAttr = fmt.Sprintf(` Type="%s"`, escapeXMLAttr(opts.DocumentReferenceType))
	}
	return fmt.Sprintf(`<ds:SignedInfo xmlns:ds="%s">`+
		`<ds:CanonicalizationMethod Algorithm="%s"/>`+
		`<ds:SignatureMethod Algorithm="%s"/>`+
		`<ds:Reference Id="%s"%s URI="%s">`+
		`%s`+
		`<ds:DigestMethod Algorithm="%s"/>`+
		`<ds:DigestValue>%s</ds:DigestValue>`+
		`</ds:Reference>`+
		`<ds:Reference URI="#%s">`+
		`<ds:Transforms><ds:Transform Algorithm="%s"/></ds:Transforms>`+
		`<ds:DigestMethod Algorithm="%s"/>`+
		`<ds:DigestValue>%s</ds:DigestValue>`+
		`</ds:Reference>`+
		`%s`+
		`</ds:SignedInfo>`,
		nsXMLDSig,
		canonicalizationAlg,
		opts.Algorithm.SignatureMethod,
		referenceID,
		referenceTypeAttr,
		escapeXMLAttr(documentURI),
		documentTransformsXML,
		opts.Algorithm.DigestMethod,
		docDigestB64,
		keyInfoID,
		algExcC14N,
		opts.Algorithm.DigestMethod,
		keyInfoDigestB64,
		referenciaSignedProperties(signedPropsID, signedPropsDigestB64, opts),
	)
}

func referenciaSignedProperties(signedPropsID, signedPropsDigestB64 string, opts xadesBuildOptions) string {
	if opts.SinXAdES {
		return ""
	}
	return fmt.Sprintf(`<ds:Reference URI="#%s" Type="%s">`+
		`<ds:Transforms><ds:Transform Algorithm="%s"/></ds:Transforms>`+
		`<ds:DigestMethod Algorithm="%s"/>`+
		`<ds:DigestValue>%s</ds:DigestValue>`+
		`</ds:Reference>`,
		signedPropsID, opts.SignedPropertiesURI, algExcC14N, opts.Algorithm.DigestMethod, signedPropsDigestB64)
}

// buildSignedPropertiesXML construye el elemento xades:SignedProperties como string XML.
func buildSignedPropertiesXML(signedPropsID, sigID, signingTime, certDigestB64, issuerSerialXML, objectReferenceID, mimeType string, opts xadesBuildOptions) string {
	policyXML := ""
	policy := opts.Policy
	if strings.TrimSpace(policy.Identifier) != "" && strings.TrimSpace(policy.Hash) != "" {
		qualifierXML := ""
		if strings.TrimSpace(policy.Qualifier) != "" {
			qualifierXML = fmt.Sprintf(`<xades:SigPolicyQualifiers><xades:SigPolicyQualifier><xades:SPURI>%s</xades:SPURI></xades:SigPolicyQualifier></xades:SigPolicyQualifiers>`, escapeXMLText(policy.Qualifier))
		}
		policyXML = fmt.Sprintf(
			`<xades:SignaturePolicyIdentifier><xades:SignaturePolicyId><xades:SigPolicyId><xades:Identifier>%s</xades:Identifier></xades:SigPolicyId><xades:SigPolicyHash><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></xades:SigPolicyHash>%s</xades:SignaturePolicyId></xades:SignaturePolicyIdentifier>`,
			escapeXMLText(policy.Identifier),
			policy.HashAlgURI,
			policy.Hash,
			qualifierXML,
		)
	}
	signingCertOpen := "<xades:SigningCertificate>"
	signingCertClose := "</xades:SigningCertificate>"
	issuerSerialNode := fmt.Sprintf(`<xades:IssuerSerial>%s</xades:IssuerSerial>`, issuerSerialXML)
	if opts.UseSigningCertV2 {
		signingCertOpen = "<xades:SigningCertificateV2>"
		signingCertClose = "</xades:SigningCertificateV2>"
		issuerSerialNode = fmt.Sprintf(`<xades:IssuerSerialV2>%s</xades:IssuerSerialV2>`, issuerSerialXML)
	}
	return fmt.Sprintf(`<xades:SignedProperties xmlns:xades="%s" xmlns:ds="%s" Id="%s">`+
		`<xades:SignedSignatureProperties>`+
		`<xades:SigningTime>%s</xades:SigningTime>`+
		`%s`+
		`<xades:Cert>`+
		`<xades:CertDigest>`+
		`<ds:DigestMethod Algorithm="%s"/>`+
		`<ds:DigestValue>%s</ds:DigestValue>`+
		`</xades:CertDigest>`+
		`%s`+
		`</xades:Cert>`+
		`%s`+
		`%s`+
		`</xades:SignedSignatureProperties>`+
		`<xades:SignedDataObjectProperties>`+
		`<xades:DataObjectFormat ObjectReference="#%s">`+
		`%s`+
		`<xades:MimeType>%s</xades:MimeType>`+
		`%s`+
		`</xades:DataObjectFormat>`+
		`</xades:SignedDataObjectProperties>`+
		`</xades:SignedProperties>`,
		opts.Namespace,
		nsXMLDSig,
		signedPropsID,
		signingTime,
		signingCertOpen,
		opts.Algorithm.DigestMethod,
		certDigestB64,
		issuerSerialNode,
		signingCertClose,
		policyXML,
		objectReferenceID,
		optionalXAdESDataObjectFormatElement("Description", opts.IncludeDataEncoding),
		mimeType,
		optionalXAdESDataObjectFormatElement("Encoding", opts.IncludeDataEncoding),
	)
}

func optionalXAdESDataObjectFormatElement(name string, enabled bool) string {
	if !enabled {
		return ""
	}
	return fmt.Sprintf(`<xades:%s/>`, name)
}

func useJavaDetachedCompat(options map[string]string) bool {
	switch strings.ToLower(strings.TrimSpace(valorOpcion(options, "expPolicy"))) {
	case "firmaage", "firmaage18", "firmaage19":
		return true
	default:
		return false
	}
}

func resolveXAdESAlgorithmOptions(options map[string]string) (xadesAlgorithmOptions, error) {
	raw := strings.ToUpper(strings.TrimSpace(valorOpcion(options, "algorithm")))
	switch raw {
	case "", "SHA256WITHRSA":
		return xadesAlgorithmOptions{Label: "SHA256withRSA", Hash: crypto.SHA256, SignatureMethod: algRSASHA256, DigestMethod: algSHA256}, nil
	case "SHA384WITHRSA":
		return xadesAlgorithmOptions{Label: "SHA384withRSA", Hash: crypto.SHA384, SignatureMethod: algRSASHA384, DigestMethod: algSHA384}, nil
	case "SHA512WITHRSA":
		return xadesAlgorithmOptions{Label: "SHA512withRSA", Hash: crypto.SHA512, SignatureMethod: algRSASHA512, DigestMethod: algSHA512}, nil
	case "SHA1WITHRSA":
		if err := cryptopolicy.RequireLegacySHA1(); err != nil {
			return xadesAlgorithmOptions{}, err
		}
		return xadesAlgorithmOptions{Label: "SHA1withRSA", Hash: crypto.SHA1, SignatureMethod: algRSASHA1, DigestMethod: algSHA1}, nil
	default:
		return xadesAlgorithmOptions{}, fmt.Errorf("algoritmo XAdES no soportado: %s", raw)
	}
}

func resolveXAdESBuildOptions(options map[string]string, alg xadesAlgorithmOptions) xadesBuildOptions {
	namespace := strings.TrimSpace(valorOpcion(options, "xadesNamespace"))
	signedPropsURI := strings.TrimSpace(valorOpcion(options, "signedPropertiesTypeUrl"))
	profile := strings.ToLower(strings.TrimSpace(valorOpcion(options, "profile")))
	if namespace == "" {
		namespace = xadesNamespaceV132
	}
	if signedPropsURI == "" {
		signedPropsURI = typeSignedProps
	}

	useV2 := profile == "" || strings.Contains(profile, "baseline")

	if exp := strings.ToLower(strings.TrimSpace(valorOpcion(options, "expPolicy"))); exp != "" {
		if exp == "firmaage" || exp == "firmaage18" || exp == "firmaage19" {
			if strings.TrimSpace(valorOpcion(options, "xadesNamespace")) == "" {
				if exp == "firmaage18" {
					namespace = xadesNamespaceV122
				}
			}
			if strings.TrimSpace(valorOpcion(options, "signedPropertiesTypeUrl")) == "" {
				if exp == "firmaage18" {
					signedPropsURI = typeSignedPropsV122
				}
			}
			useV2 = false
		}
	}

	return xadesBuildOptions{
		SinXAdES:            valorOpcion(options, opcionXMLDSigPura) == "true",
		Algorithm:           alg,
		Policy:              resolveXAdESPolicyOptions(options),
		Namespace:           namespace,
		SignedPropertiesURI: signedPropsURI,
		UseSigningCertV2:    useV2,
		CanonicalizationAlg: algExcC14N,
		DocumentTransform:   algExcC14N,
	}
}

func resolveXAdESPolicyOptions(options map[string]string) xadesPolicyOptions {
	policyID := strings.TrimSpace(valorOpcion(options, "policyIdentifier", "xadesPolicyIdentifier"))
	policyHash := strings.TrimSpace(valorOpcion(options, "policyIdentifierHash", "xadesPolicyIdentifierHash"))
	policyHashAlg := strings.TrimSpace(valorOpcion(options, "policyIdentifierHashAlgorithm", "xadesPolicyIdentifierHashAlgorithm"))
	policyQualifier := strings.TrimSpace(valorOpcion(options, "policyQualifier", "xadesPolicyQualifier"))

	if exp := strings.ToLower(strings.TrimSpace(valorOpcion(options, "expPolicy"))); exp != "" && policyID == "" {
		switch exp {
		case "firmaage", "firmaage19":
			policyID = "urn:oid:2.16.724.1.3.1.1.2.1.9"
			policyQualifier = "https://sede.administracion.gob.es/politica_de_firma_anexo_1.pdf"
			policyHashAlg = algSHA1
			policyHash = "G7roucf600+f03r/o0bAOQ6WAs0="
		case "firmaage18":
			policyID = "urn:oid:2.16.724.1.3.1.1.2.1.8"
			policyQualifier = "https://sede.administracion.gob.es/PAG_Sede/dam/jcr:b0de3f91-5171-48e2-81f3-5c2407d9c091/politica_firma_AGE_v1_8.pdf"
			policyHashAlg = algSHA1
			policyHash = "V8lVVNGDCPen6VELRD1Ja8HARFk="
		}
	}
	if policyHashAlg == "" && policyHash != "" {
		policyHashAlg = algSHA1
	}
	return xadesPolicyOptions{
		Identifier: policyID,
		Hash:       policyHash,
		HashAlgURI: policyHashAlg,
		Qualifier:  policyQualifier,
	}
}

func digestBytes(hash crypto.Hash, data []byte) ([]byte, error) {
	if !hash.Available() {
		return nil, fmt.Errorf("hash no disponible: %v", hash)
	}
	h := hash.New()
	if _, err := h.Write(data); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func buildIssuerSerialXML(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", errors.New("certificado ausente")
	}
	return fmt.Sprintf(
		`<ds:X509IssuerName>%s</ds:X509IssuerName><ds:X509SerialNumber>%s</ds:X509SerialNumber>`,
		escapeXMLText(cert.Issuer.String()),
		escapeXMLText(cert.SerialNumber.String()),
	), nil
}

// buildFinalXML ensambla el documento XML de firma completo.
func buildFinalXML(contentXML, sigID, signedInfoXML, sigValueB64, keyInfoXML, qualifyingPropsID, signedPropsXML string, opts xadesBuildOptions) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<AFIRMA>`+
		`%s`+
		`<ds:Signature xmlns:ds="%s" Id="%s">`+
		`%s`+
		`<ds:SignatureValue>%s</ds:SignatureValue>`+
		`%s`+
		`<ds:Object Id="%s">`+
		`<xades:QualifyingProperties xmlns:xades="%s" Target="#%s">`+
		`%s`+
		`</xades:QualifyingProperties>`+
		`</ds:Object>`+
		`</ds:Signature>`+
		`</AFIRMA>`,
		contentXML,
		nsXMLDSig,
		sigID,
		signedInfoXML,
		sigValueB64,
		keyInfoXML,
		qualifyingPropsID,
		opts.Namespace,
		sigID,
		signedPropsXML,
	)
}

func buildContentXML(documentName, mimeType string, data []byte) string {
	name := escapeXMLAttr(documentName)
	if strings.TrimSpace(name) == "" {
		name = "documento.xml"
	}
	return fmt.Sprintf(
		`<CONTENT Name="%s" MimeType="%s">%s</CONTENT>`,
		name,
		escapeXMLAttr(mimeType),
		base64.StdEncoding.EncodeToString(data),
	)
}

func buildXAdESDetachedContentXML(contentID, mimeType string, data []byte) string {
	if strings.TrimSpace(contentID) == "" {
		contentID = "CONTENT-1"
	}
	if isXMLPayload(data, mimeType) {
		return fmt.Sprintf(
			`<CONTENT Id="%s">%s</CONTENT>`,
			escapeXMLAttr(contentID),
			stripXMLDeclaration(data),
		)
	}
	return fmt.Sprintf(
		`<CONTENT Id="%s" Encoding="Base64" MimeType="%s">%s</CONTENT>`,
		escapeXMLAttr(contentID),
		escapeXMLAttr(mimeType),
		base64.StdEncoding.EncodeToString(data),
	)
}

// detectarMimeNoXML corrige el tipo cuando el portal declara XML pero los
// datos no lo son, para no publicar un MimeType engañoso en la firma.
func detectarMimeNoXML(data []byte) string {
	detected := http.DetectContentType(data)
	if base, _, ok := strings.Cut(detected, ";"); ok {
		detected = base
	}
	detected = strings.TrimSpace(detected)
	if detected == "" || strings.Contains(detected, "xml") {
		return "application/octet-stream"
	}
	return detected
}

func isXMLPayload(data []byte, mimeType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	if !strings.Contains(normalized, "xml") {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))), "<")
}

func stripXMLDeclaration(data []byte) string {
	content := strings.TrimSpace(string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	if strings.HasPrefix(content, "<?xml") {
		if end := strings.Index(content, "?>"); end >= 0 {
			content = strings.TrimSpace(content[end+2:])
		}
	}
	return content
}

func buildKeyInfoXML(keyInfoID, certB64 string) string {
	return fmt.Sprintf(`<ds:KeyInfo xmlns:ds="%s" Id="%s"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`,
		nsXMLDSig,
		keyInfoID,
		certB64,
	)
}

func normalizeMimeType(mimeType string, data []byte) string {
	if strings.TrimSpace(mimeType) != "" {
		return mimeType
	}
	contenido := strings.TrimSpace(string(data))
	if strings.HasPrefix(contenido, "<") {
		return "application/xml"
	}
	return "application/octet-stream"
}

// buildIssuerSerialV2 construye el valor IssuerSerialV2 conforme a RFC 5035 / ETSI.
// Codifica IssuerAndSerialNumber como DER y lo devuelve en base64.
func buildIssuerSerialV2(cert *x509.Certificate) (string, error) {
	type issuerSerial struct {
		Issuer       asn1.RawValue
		SerialNumber *big.Int
	}
	is := issuerSerial{
		Issuer:       asn1.RawValue{FullBytes: cert.RawIssuer},
		SerialNumber: cert.SerialNumber,
	}
	der, err := asn1.Marshal(is)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// exclusiveC14N canonicaliza un documento o fragmento con Exclusive XML
// Canonicalization 1.0 sin comentarios.
func exclusiveC14N(xmlStr string) ([]byte, error) {
	return canonicalizeXML([]byte(xmlStr), algExcC14N)
}

// escapeXMLAttr escapa caracteres especiales en valores de atributos XML.
func escapeXMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "\t", "&#9;")
	s = strings.ReplaceAll(s, "\n", "&#10;")
	s = strings.ReplaceAll(s, "\r", "&#13;")
	return s
}

// escapeXMLText escapa caracteres especiales en contenido de texto XML.
func escapeXMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\r", "&#13;")
	return s
}
