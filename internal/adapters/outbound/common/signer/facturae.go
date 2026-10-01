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
	"crypto/sha1" // #nosec G505 -- FacturaE/XAdES-EPES mandates SHA-1 certificate/policy digest fields, not the document signature.
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
)

const (
	formatFacturaE = domain.SignatureFormat("FacturaE")

	facturaeNamespace31       = "http://www.facturae.es/Facturae/2007/v3.1/Facturae"
	facturaeNamespace32       = "http://www.facturae.es/Facturae/2009/v3.2/Facturae"
	facturaeNamespace321      = "http://www.facturae.es/Facturae/2014/v3.2.1/Facturae"
	facturaeNamespace322      = "http://www.facturae.gob.es/formato/Versiones/Facturaev3_2_2.xml"
	facturaeNamespace32Legacy = "http://www.facturae.gob.es/formato/Versiones/Facturae/Facturaev3.2.xml"
	facturaePolicy31ID        = "http://www.facturae.es/politica_de_firma_formato_facturae/politica_de_firma_formato_facturae_v3_1.pdf"
	facturaePolicy31Hash      = "Ohixl6upD6av8N7pEvDABhEL6hM="
	facturaePolicy30ID        = "http://www.facturae.es/politica de firma formato facturae/politica de firma formato facturae v3_0.pdf"
	facturaePolicy30Hash      = "xmfh8D/Ec/hHeE1IB4zPd61zHIY="

	algRSASHA1   = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	algSHA1      = "http://www.w3.org/2000/09/xmldsig#sha1"
	algEnveloped = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
)

type FacturaESigner struct{}
type FacturaEVerifier struct{}

func NewFacturaESigner() *FacturaESigner     { return &FacturaESigner{} }
func NewFacturaEVerifier() *FacturaEVerifier { return &FacturaEVerifier{} }

func (s *FacturaESigner) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != formatFacturaE {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", formatFacturaE)
	}
	if job.Action != domain.ActionSign {
		return domain.SignatureResult{}, errors.New("FacturaE solo soporta la accion sign")
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	invoiceXML := sanitizeXMLDocument(job.Document.Content)
	if err := validateFacturaEXML(invoiceXML); err != nil {
		return domain.SignatureResult{}, err
	}
	if looksLikeSignedXML(invoiceXML) {
		return domain.SignatureResult{}, errors.New("la factura ya contiene una firma y no se soporta cofirma FacturaE")
	}
	// Revisión del contenido: se informa al usuario sin impedir la firma.
	if incidencias, err := RevisarFacturaE(invoiceXML); err == nil && len(incidencias) > 0 {
		avisos.Registrar("Revisión de la factura", ResumenIncidenciasFactura(incidencias))
	}

	now := time.Now().UTC()
	policyID, policyHash := resolveFacturaEPolicy(job.Options)
	role, err := resolveFacturaERole(job.Options)
	if err != nil {
		return domain.SignatureResult{}, err
	}

	docCanonical, err := canonicalizeXML(invoiceXML, algExcC14N)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("canonicalizando documento FacturaE: %w", err)
	}
	docDigest := sha256.Sum256(docCanonical)
	docDigestB64 := base64.StdEncoding.EncodeToString(docDigest[:])

	keyInfoID := "Certificate1"
	signedPropsID := "Signature-SignedProperties"
	sigID := "Signature"

	keyInfoXML := buildFacturaEKeyInfoXML(keyInfoID, clave)
	keyInfoCanonical, err := canonicalizeXML([]byte(keyInfoXML), algExcC14N)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("canonicalizando KeyInfo FacturaE: %w", err)
	}
	keyInfoDigest := sha256.Sum256(keyInfoCanonical)
	keyInfoDigestB64 := base64.StdEncoding.EncodeToString(keyInfoDigest[:])

	signedPropsXML, err := buildFacturaESignedPropertiesXML(sigID, signedPropsID, now, clave.Certificate, policyID, policyHash, role, job.Options)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	signedPropsCanonical, err := canonicalizeXML([]byte(signedPropsXML), algExcC14N)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("canonicalizando SignedProperties FacturaE: %w", err)
	}
	signedPropsDigest := sha256.Sum256(signedPropsCanonical)
	signedPropsDigestB64 := base64.StdEncoding.EncodeToString(signedPropsDigest[:])

	signedInfoXML := buildFacturaESignedInfoXML(signedPropsID, signedPropsDigestB64, docDigestB64, keyInfoID, keyInfoDigestB64)
	signedInfoC14N, err := exclusiveC14N(signedInfoXML)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error canonicalizando SignedInfo FacturaE: %w", err)
	}
	sigHash := sha256.Sum256(signedInfoC14N)
	sigBytes, err := clave.Signer.Sign(rand.Reader, sigHash[:], crypto.SHA256)
	if err != nil {
		return domain.SignatureResult{}, fmt.Errorf("error firmando FacturaE: %w", err)
	}
	signatureXML := buildFacturaESignatureXML(sigID, signedInfoXML, base64.StdEncoding.EncodeToString(sigBytes), keyInfoXML, signedPropsXML)
	signedInvoice, err := injectSignatureIntoRoot(invoiceXML, []byte(signatureXML))
	if err != nil {
		return domain.SignatureResult{}, err
	}
	return domain.SignatureResult{
		Format:    formatFacturaE,
		Data:      signedInvoice,
		Algorithm: "FacturaE-XAdES-Enveloped-RSA-SHA256",
	}, nil
}

func (v *FacturaEVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if err := ctx.Err(); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	xmlData := sanitizeXMLDocument(signedDocument.Content)
	if err := validateFacturaEXML(xmlData); err != nil {
		return domain.VerificationResult{}, nil, err
	}
	if !looksLikeSignedXML(xmlData) {
		return domain.VerificationResult{}, nil, errors.New("la factura no contiene una firma XML")
	}
	verification, err := verifyXMLSignatureDocument(xmlData, true, nil, anchors, string(formatFacturaE), "firma FacturaE valida", []string{"perfil=facturae", "xades=enveloped"})
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	result := applyXMLRevocation(ctx, verification.result, verification.signerCertificates, verification.embeddedCertificates)
	result.Details = append(result.Details, detectFacturaEPolicyDetail(xmlData))
	return result, verification.signers, nil
}

func validateFacturaEXML(data []byte) error {
	if !isFacturaEXML(data) {
		return errors.New("el XML no parece una FacturaE valida")
	}
	s := strings.TrimSpace(string(data))
	required := []string{"<FileHeader", "<Parties", "<Invoices"}
	for _, fragment := range required {
		if !strings.Contains(s, fragment) {
			return errors.New("el XML no contiene la estructura minima de FacturaE")
		}
	}
	return nil
}

func isFacturaEXML(data []byte) bool {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return false
	}
	if !strings.Contains(s, "<Facturae") {
		return false
	}
	for _, namespace := range []string{
		facturaeNamespace31,
		facturaeNamespace32,
		facturaeNamespace321,
		facturaeNamespace322,
		facturaeNamespace32Legacy,
	} {
		if strings.Contains(s, namespace) {
			return true
		}
	}
	return false
}

func sanitizeXMLDocument(data []byte) []byte {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	return append([]byte(nil), trimmed...)
}

func mustExclusiveC14N(data []byte) []byte {
	c14n, err := exclusiveC14N(string(data))
	if err == nil {
		return c14n
	}
	return data
}

func resolveFacturaEPolicy(options map[string]string) (string, string) {
	version := strings.TrimSpace(valorOpcion(options, "facturaePolicyVersion"))
	policyID := strings.TrimSpace(valorOpcion(options, "policyIdentifier"))
	policyHash := strings.TrimSpace(valorOpcion(options, "policyIdentifierHash"))
	switch {
	case policyID != "":
		if policyHash == "" {
			if strings.EqualFold(policyID, facturaePolicy30ID) {
				policyHash = facturaePolicy30Hash
			} else {
				policyHash = facturaePolicy31Hash
			}
		}
		return policyID, policyHash
	case version == "3.0":
		return facturaePolicy30ID, facturaePolicy30Hash
	default:
		return facturaePolicy31ID, facturaePolicy31Hash
	}
}

func valorOpcion(options map[string]string, claves ...string) string {
	for _, clave := range claves {
		for k, v := range options {
			if strings.EqualFold(strings.TrimSpace(k), clave) {
				return v
			}
		}
	}
	return ""
}

func resolveFacturaERole(options map[string]string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(valorOpcion(options, "signerClaimedRole")))
	if role == "" {
		role = "emisor"
	}
	switch role {
	case "emisor", "receptor", "tercero", "supplier", "customer", "third party":
		return role, nil
	default:
		return "", fmt.Errorf("el papel %q no es valido para FacturaE", role)
	}
}

func buildFacturaEKeyInfoXML(keyInfoID string, key *LocalSigningKey) string {
	certB64 := base64.StdEncoding.EncodeToString(key.Certificate.Raw)
	rsaPub, _ := key.Certificate.PublicKey.(*rsa.PublicKey)
	keyValueXML := ""
	if rsaPub != nil {
		keyValueXML = fmt.Sprintf(`<ds:KeyValue><ds:RSAKeyValue><ds:Modulus>%s</ds:Modulus><ds:Exponent>%s</ds:Exponent></ds:RSAKeyValue></ds:KeyValue>`,
			base64.StdEncoding.EncodeToString(rsaPub.N.Bytes()),
			base64.StdEncoding.EncodeToString(big.NewInt(int64(rsaPub.E)).Bytes()),
		)
	}
	return fmt.Sprintf(`<ds:KeyInfo xmlns:ds="%s" Id="%s"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data>%s</ds:KeyInfo>`,
		nsXMLDSig,
		keyInfoID,
		certB64,
		keyValueXML,
	)
}

func buildFacturaESignedPropertiesXML(sigID, signedPropsID string, now time.Time, cert *x509.Certificate, policyID, policyHash, role string, options map[string]string) (string, error) {
	certDigest := sha1.Sum(cert.Raw) // #nosec G401 -- mandated certificate identifier field; the document signature uses RSA-SHA256.
	certDigestB64 := base64.StdEncoding.EncodeToString(certDigest[:])
	issuerName := escapeXMLText(cert.Issuer.String())
	serial := cert.SerialNumber.String()
	signingTime := now.Format(time.RFC3339)
	placeXML := buildFacturaESignaturePlaceXML(options)
	policyQualifierXML := buildFacturaEPolicyQualifierXML(options)
	return fmt.Sprintf(
		`<xades:SignedProperties xmlns:xades="%s" xmlns:ds="%s" Id="%s">`+
			`<xades:SignedSignatureProperties>`+
			`<xades:SigningTime>%s</xades:SigningTime>`+
			`<xades:SigningCertificate>`+
			`<xades:Cert>`+
			`<xades:CertDigest><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></xades:CertDigest>`+
			`<xades:IssuerSerial><ds:X509IssuerName>%s</ds:X509IssuerName><ds:X509SerialNumber>%s</ds:X509SerialNumber></xades:IssuerSerial>`+
			`</xades:Cert>`+
			`</xades:SigningCertificate>`+
			`<xades:SignaturePolicyIdentifier><xades:SignaturePolicyId><xades:SigPolicyId><xades:Identifier>%s</xades:Identifier><xades:Description>%s</xades:Description></xades:SigPolicyId><xades:SigPolicyHash><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></xades:SigPolicyHash>%s</xades:SignaturePolicyId></xades:SignaturePolicyIdentifier>`+
			`<xades:SignerRole><xades:ClaimedRoles><xades:ClaimedRole>%s</xades:ClaimedRole></xades:ClaimedRoles></xades:SignerRole>`+
			`%s`+
			`</xades:SignedSignatureProperties>`+
			`</xades:SignedProperties>`,
		nsXAdES,
		nsXMLDSig,
		signedPropsID,
		signingTime,
		algSHA1,
		certDigestB64,
		issuerName,
		serial,
		escapeXMLText(policyID),
		escapeXMLText(facturaePolicyDescription(policyID)),
		algSHA1,
		escapeXMLText(policyHash),
		policyQualifierXML,
		escapeXMLText(role),
		placeXML,
	), nil
}

func facturaePolicyDescription(policyID string) string {
	if strings.EqualFold(strings.TrimSpace(policyID), facturaePolicy30ID) {
		return "facturae30"
	}
	return "facturae31"
}

func buildFacturaEPolicyQualifierXML(options map[string]string) string {
	qualifier := strings.TrimSpace(valorOpcion(options, "policyQualifier"))
	if qualifier == "" {
		return ""
	}
	return `<xades:SigPolicyQualifiers><xades:SigPolicyQualifier><xades:SPURI>` +
		escapeXMLText(qualifier) +
		`</xades:SPURI></xades:SigPolicyQualifier></xades:SigPolicyQualifiers>`
}

func buildFacturaESignaturePlaceXML(options map[string]string) string {
	city := strings.TrimSpace(valorOpcion(options, "signatureProductionCity"))
	province := strings.TrimSpace(valorOpcion(options, "signatureProductionProvince"))
	postal := strings.TrimSpace(valorOpcion(options, "signatureProductionPostalCode"))
	country := strings.TrimSpace(valorOpcion(options, "signatureProductionCountry"))
	if city == "" && province == "" && postal == "" && country == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<xades:SignatureProductionPlace>`)
	if city != "" {
		b.WriteString(`<xades:City>` + escapeXMLText(city) + `</xades:City>`)
	}
	if province != "" {
		b.WriteString(`<xades:StateOrProvince>` + escapeXMLText(province) + `</xades:StateOrProvince>`)
	}
	if postal != "" {
		b.WriteString(`<xades:PostalCode>` + escapeXMLText(postal) + `</xades:PostalCode>`)
	}
	if country != "" {
		b.WriteString(`<xades:CountryName>` + escapeXMLText(country) + `</xades:CountryName>`)
	}
	b.WriteString(`</xades:SignatureProductionPlace>`)
	return b.String()
}

func buildFacturaESignedInfoXML(signedPropsID, signedPropsDigestB64, docDigestB64, keyInfoID, keyInfoDigestB64 string) string {
	return fmt.Sprintf(
		`<ds:SignedInfo xmlns:ds="%s" Id="Signature-SignedInfo">`+
			`<ds:CanonicalizationMethod Algorithm="%s"/>`+
			`<ds:SignatureMethod Algorithm="%s"/>`+
			`<ds:Reference Id="SignedPropertiesID" Type="%s" URI="#%s">`+
			`<ds:Transforms><ds:Transform Algorithm="%s"/></ds:Transforms>`+
			`<ds:DigestMethod Algorithm="%s"/>`+
			`<ds:DigestValue>%s</ds:DigestValue>`+
			`</ds:Reference>`+
			`<ds:Reference URI="">`+
			`<ds:Transforms><ds:Transform Algorithm="%s"/><ds:Transform Algorithm="%s"/></ds:Transforms>`+
			`<ds:DigestMethod Algorithm="%s"/>`+
			`<ds:DigestValue>%s</ds:DigestValue>`+
			`</ds:Reference>`+
			`<ds:Reference URI="#%s">`+
			`<ds:Transforms><ds:Transform Algorithm="%s"/></ds:Transforms>`+
			`<ds:DigestMethod Algorithm="%s"/>`+
			`<ds:DigestValue>%s</ds:DigestValue>`+
			`</ds:Reference>`+
			`</ds:SignedInfo>`,
		nsXMLDSig,
		algExcC14N,
		algRSASHA256,
		typeSignedProps,
		signedPropsID,
		algExcC14N,
		algSHA256,
		signedPropsDigestB64,
		algEnveloped,
		algExcC14N,
		algSHA256,
		docDigestB64,
		keyInfoID,
		algExcC14N,
		algSHA256,
		keyInfoDigestB64,
	)
}

func buildFacturaESignatureXML(sigID, signedInfoXML, signatureValueB64, keyInfoXML, signedPropsXML string) string {
	return fmt.Sprintf(
		`<ds:Signature xmlns:ds="%s" xmlns:xades="%s" Id="%s">`+
			`%s`+
			`<ds:SignatureValue Id="SignatureValue">%s</ds:SignatureValue>`+
			`%s`+
			`<ds:Object Id="Signature-Object"><xades:QualifyingProperties Target="#%s">%s</xades:QualifyingProperties></ds:Object>`+
			`</ds:Signature>`,
		nsXMLDSig,
		nsXAdES,
		sigID,
		signedInfoXML,
		signatureValueB64,
		keyInfoXML,
		sigID,
		signedPropsXML,
	)
}

func injectSignatureIntoRoot(xmlData, signatureXML []byte) ([]byte, error) {
	source := string(xmlData)
	root := detectRootLocalName(source)
	if root == "" {
		root = "Facturae"
	}
	closing := "</" + root + ">"
	idx := strings.LastIndex(source, closing)
	if idx < 0 {
		return nil, errors.New("no se ha encontrado el cierre del nodo raiz FacturaE")
	}
	out := source[:idx] + string(signatureXML) + source[idx:]
	return []byte(out), nil
}

func detectRootLocalName(xmlSource string) string {
	s := strings.TrimSpace(xmlSource)
	if strings.HasPrefix(s, "<?xml") {
		if idx := strings.Index(s, "?>"); idx >= 0 {
			s = strings.TrimSpace(s[idx+2:])
		}
	}
	if !strings.HasPrefix(s, "<") {
		return ""
	}
	s = s[1:]
	for i, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n', '>':
			name := s[:i]
			if colon := strings.IndexByte(name, ':'); colon >= 0 {
				return name[colon+1:]
			}
			return name
		}
	}
	return ""
}

func detectFacturaEPolicyDetail(xmlData []byte) string {
	s := string(xmlData)
	switch {
	case strings.Contains(s, facturaePolicy30ID):
		return "policy=facturae3.0"
	case strings.Contains(s, facturaePolicy31ID):
		return "policy=facturae3.1"
	default:
		return "policy=desconocida"
	}
}

var _ ports.SignerEngine = (*FacturaESigner)(nil)
var _ ports.VerifierEngine = (*FacturaEVerifier)(nil)
