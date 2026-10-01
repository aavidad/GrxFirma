// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// RevocationStatus representa el estado de revocación de un certificado.
type RevocationStatus int

const (
	RevocationUnknown RevocationStatus = iota
	RevocationGood
	RevocationRevoked
)

func (s RevocationStatus) String() string {
	switch s {
	case RevocationGood:
		return "bueno"
	case RevocationRevoked:
		return "revocado"
	default:
		return "desconocido"
	}
}

// RevocationResult contiene el resultado de la comprobación de revocación.
type RevocationResult struct {
	Status    RevocationStatus
	Method    string // "ocsp" o "crl"
	CheckedAt time.Time
	RevokedAt time.Time // solo si Status == RevocationRevoked
	Reason    string
}

func revocationSubjectLabel(cert *x509.Certificate, index int) string {
	if cert == nil {
		return fmt.Sprintf("cert[%d]", index)
	}
	if cn := strings.TrimSpace(cert.Subject.CommonName); cn != "" {
		return fmt.Sprintf("cert[%d] %s", index, cn)
	}
	if name := strings.TrimSpace(cert.Subject.String()); name != "" {
		return fmt.Sprintf("cert[%d] %s", index, name)
	}
	return fmt.Sprintf("cert[%d]", index)
}

func revocationDetailText(cert *x509.Certificate, index int, res RevocationResult) string {
	base := revocationSubjectLabel(cert, index) + ": " + res.Status.String()
	method := strings.TrimSpace(res.Method)
	reason := strings.TrimSpace(res.Reason)
	if method != "" {
		if reason != "" && res.Status == RevocationUnknown {
			return fmt.Sprintf("%s vía %s (%s)", base, method, reason)
		}
		return fmt.Sprintf("%s vía %s", base, method)
	}
	if reason != "" {
		return fmt.Sprintf("%s (%s)", base, reason)
	}
	return base
}

// RevocationChecker comprueba la revocación de certificados usando OCSP y CRL.
type RevocationChecker struct {
	client *revocationclient.Client
}

// NewRevocationChecker crea un verificador de revocación con timeout de 15s.
func NewRevocationChecker() *RevocationChecker {
	return &RevocationChecker{
		client: revocationclient.New(),
	}
}

// NewRevocationCheckerWithClient crea un verificador con un http.Client personalizado (para tests).
func NewRevocationCheckerWithClient(c *http.Client) *RevocationChecker {
	return &RevocationChecker{client: revocationclient.NewWithHTTPClient(c)}
}

// Check comprueba el estado de revocación del certificado dado.
// La descarga, la política SSRF y la autenticación criptográfica se centralizan
// en revocationclient para que la verificación no pueda divergir de la firma.
func (r *RevocationChecker) Check(ctx context.Context, cert, issuer *x509.Certificate) (RevocationResult, error) {
	if cert == nil || issuer == nil {
		return RevocationResult{Status: RevocationUnknown, Reason: "certificado o emisor nulo"}, nil
	}
	if r == nil || r.client == nil {
		return RevocationResult{Status: RevocationUnknown, Reason: "cliente de revocación no configurado"}, nil
	}

	checkedAt := time.Now()
	result, err := r.client.Check(ctx, cert, issuer)
	switch result.Status {
	case revocationclient.CertificateStatusGood:
		return RevocationResult{
			Status:    RevocationGood,
			Method:    result.Method,
			CheckedAt: checkedAt,
		}, nil
	case revocationclient.CertificateStatusRevoked:
		return RevocationResult{
			Status:    RevocationRevoked,
			Method:    result.Method,
			CheckedAt: checkedAt,
			RevokedAt: result.RevokedAt,
			Reason:    "certificado revocado según " + result.Method,
		}, nil
	case revocationclient.CertificateStatusUnknown:
		reason := result.Reason
		if reason == "" && err != nil {
			reason = err.Error()
		}
		if reason == "" {
			reason = "estado de revocación desconocido"
		}
		return RevocationResult{
			Status:    RevocationUnknown,
			Method:    result.Method,
			CheckedAt: checkedAt,
			Reason:    reason,
		}, nil
	default:
		return RevocationResult{
			Status:    RevocationUnknown,
			CheckedAt: checkedAt,
			Reason:    "sin información de revocación disponible",
		}, nil
	}
}

// CAdESVerifier verifica firmas CAdES comprobando cadena de certificados y revocación.
type CAdESVerifier struct {
	revChecker *RevocationChecker
}

// NewCAdESVerifier crea un verificador CAdES con comprobación OCSP/CRL real.
func NewCAdESVerifier() *CAdESVerifier {
	return &CAdESVerifier{revChecker: NewRevocationChecker()}
}

// NewCAdESVerifierWithChecker crea un verificador con un RevocationChecker personalizado.
func NewCAdESVerifierWithChecker(rc *RevocationChecker) *CAdESVerifier {
	return &CAdESVerifier{revChecker: rc}
}

type contentInfoRaw struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue
}

type signedDataRaw struct {
	Version          int
	DigestAlgorithms asn1.RawValue
	EncapContentInfo asn1.RawValue
	Certificates     asn1.RawValue `asn1:"optional"`
	SignerInfos      asn1.RawValue
}

type encapContentInfoRaw struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"optional,tag:0,explicit"`
}

type signerInfoRaw struct {
	Version            int
	SID                issuerAndSerialNumber
	DigestAlgorithm    algorithmIdentifier
	SignedAttributes   asn1.RawValue `asn1:"tag:0,optional"`
	SignatureAlgorithm algorithmIdentifier
	Signature          []byte
	UnsignedAttributes asn1.RawValue `asn1:"tag:1,optional"`
}

// Verify comprueba una firma CAdES básica: estructura CMS, digest del contenido,
// validez criptográfica de SignerInfo y extracción del certificado firmante.
// Si la cadena embebida lo permite, añade además el estado de revocación real.
func (v *CAdESVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return v.verifyCMS(ctx, signedDocument.Content, nil, anchors)
}

// VerifyDetached verifica un CMS detached con contenido externo ya resuelto
// por el adaptador llamante (por ejemplo, PAdES tras aplicar ByteRange).
func (v *CAdESVerifier) VerifyDetachedCMS(ctx context.Context, cmsDER, content []byte) (domain.VerificationResult, []domain.CertificateRef, error) {
	return v.verifyCMS(ctx, cmsDER, content, domain.CertificateChain{})
}

// VerifyDetachedCMSWithAnchors verifica un CMS detached y valida su cadena
// contra las anclas X.509 proporcionadas.
func (v *CAdESVerifier) VerifyDetachedCMSWithAnchors(ctx context.Context, cmsDER, content []byte, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return v.verifyCMS(ctx, cmsDER, content, anchors)
}

func (v *CAdESVerifier) VerifyDetached(ctx context.Context, signedDocument domain.Document, originalDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	return v.verifyCMS(ctx, signedDocument.Content, originalDocument.Content, anchors)
}

func (v *CAdESVerifier) verifyCMS(ctx context.Context, cmsDER, externalContent []byte, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	contentInfo, err := parseContentInfo(cmsDER)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	if !contentInfo.ContentType.Equal(oidSignedData) {
		return domain.VerificationResult{}, nil, fmt.Errorf("OID CMS no soportado: %v", contentInfo.ContentType)
	}

	signedData, err := parseSignedData(contentInfo.Content.Bytes)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}

	content, err := parseEncapsulatedContent(signedData.EncapContentInfo)
	if err != nil {
		if len(externalContent) == 0 {
			return domain.VerificationResult{}, nil, err
		}
		content = externalContent
	}

	certs, certRefs, err := parseCertificateSet(signedData.Certificates)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	if len(certs) == 0 {
		return domain.VerificationResult{}, nil, errors.New("la firma no contiene certificados embebidos")
	}

	signerInfos, err := parseSignerInfos(signedData.SignerInfos)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}

	result := domain.NewVerificationSuccess(string(domain.FormatCAdES), "firma CAdES válida", nil)
	signers := make([]domain.CertificateRef, 0, len(signerInfos))
	signerCerts := make([]*x509.Certificate, 0, len(signerInfos))
	signerDetails := make([][]string, 0, len(signerInfos))

	for _, signerInfo := range signerInfos {
		signerCert, err := findSignerCertificate(certs, signerInfo.SID)
		if err != nil {
			return domain.VerificationResult{}, certRefs, err
		}
		spec, err := validarAlgoritmosSignerInfo(signerCert, signerInfo)
		if err != nil {
			return domain.VerificationResult{}, certRefs, err
		}
		if err := verifyMessageDigest(signerInfo.SignedAttributes, content, spec.hash); err != nil {
			return domain.VerificationResult{}, certRefs, err
		}
		if err := verifyCMSignature(signerCert, signerInfo, spec.hash); err != nil {
			return domain.VerificationResult{}, certRefs, err
		}
		signerCerts = append(signerCerts, signerCert)
		signers = append(signers, certificateToRef(signerCert))
		signerDetails = append(signerDetails, []string{
			fmt.Sprintf("firmante=%s", signerCert.Subject.String()),
			fmt.Sprintf("algoritmo=%s", signerInfo.SignatureAlgorithm.Algorithm.String()),
		})
	}
	if len(signers) > 1 {
		reverseCertificateRefs(signers)
		reverseCertificates(signerCerts)
		reverseStringSlices(signerDetails)
	}
	for _, chunk := range signerDetails {
		result.Details = append(result.Details, chunk...)
		result.Integrity.Details = append(result.Integrity.Details, chunk...)
	}
	result = applySignerVerificationMetadata(result, signerCerts, certs, anchors)

	// La revocación se evalúa sobre la cadena real de cada firmante
	// (hoja → emisor → ...), construida por nombre y firma. Antes se usaba el
	// orden en que venían los certificados dentro del CMS: con un P12 que
	// guarda la cadena al revés se emparejaban certificados que no eran
	// emisor/sujeto, OCSP quedaba "desconocido" y una firma con un
	// certificado revocado se presentaba como válida.
	for _, signerCert := range signerCerts {
		chain := chainForRevocation(ctx, v.revChecker, signerCert, certs)
		if len(chain) < 2 {
			result = markRevocationNotChecked(result, signerCert)
			continue
		}
		revocation, err := v.verifyRevocationWithEmbedded(ctx, chain, signerInfos)
		if err == nil {
			result = mergeRevocationResult(result, revocation)
		}
	}

	return result.Normalize(), signers, nil
}

// validarAlgoritmosSignerInfo admite SHA-256, SHA-384 y SHA-512 (AutoFirma
// Java firma con SHA-512 en sedes como JCyL) y exige que el algoritmo de
// firma declarado sea coherente con el resumen y con la clave.
func validarAlgoritmosSignerInfo(cert *x509.Certificate, signerInfo signerInfoRaw) (cmsHashSpec, error) {
	if cert == nil {
		return cmsHashSpec{}, errors.New("certificado firmante nulo")
	}
	spec, ok := cmsHashForDigestOID(signerInfo.DigestAlgorithm.Algorithm)
	if !ok {
		return cmsHashSpec{}, errors.New("SignerInfo no declara SHA-256, SHA-384 ni SHA-512")
	}
	switch cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if !signerInfo.SignatureAlgorithm.Algorithm.Equal(spec.rsaOID) &&
			!signerInfo.SignatureAlgorithm.Algorithm.Equal(oidSignatureRSAEncryption) {
			return cmsHashSpec{}, fmt.Errorf("SignerInfo RSA no declara un algoritmo PKCS#1 compatible con %s", spec.hash)
		}
	case *ecdsa.PublicKey:
		if !signerInfo.SignatureAlgorithm.Algorithm.Equal(spec.ecdsaOID) {
			return cmsHashSpec{}, fmt.Errorf("SignerInfo ECDSA no declara ecdsa-with-%s", spec.hash)
		}
	default:
		return cmsHashSpec{}, fmt.Errorf("tipo de clave pública no soportado: %T", cert.PublicKey)
	}
	return spec, nil
}

func (v *CAdESVerifier) verifyRevocationWithEmbedded(ctx context.Context, chain []*x509.Certificate, signerInfos []signerInfoRaw) (domain.VerificationResult, error) {
	for _, signerInfo := range signerInfos {
		evidence, err := extractEmbeddedRevocationEvidence(signerInfo)
		if err != nil {
			continue
		}
		if len(evidence.OCSPResponses) == 0 && len(evidence.CRLs) == 0 {
			continue
		}
		return verifyEmbeddedRevocation(chain, evidence)
	}
	return v.VerifyRevocation(ctx, chain)
}

// VerifyRevocation comprueba el estado de revocación de todos los certificados de la cadena.
// Retorna un VerificationResult con los detalles de cada certificado.
func (v *CAdESVerifier) VerifyRevocation(ctx context.Context, chain []*x509.Certificate) (domain.VerificationResult, error) {
	if len(chain) == 0 {
		return domain.NewVerificationFailure(string(domain.FormatCAdES), "cadena de certificados vacía", nil), nil
	}

	result := domain.NewVerificationSuccess(string(domain.FormatCAdES), "", nil)
	result.Details = nil
	result.Integrity.Details = nil
	result.Coverage = "chain"
	result.Integrity.Status = domain.VerificationStatusUnknown
	result.Integrity.Reason = "revocación evaluada sobre la cadena"
	result.Certificate.Status = domain.VerificationStatusValid
	result.Certificate.Reason = "certificados no revocados"
	result.Trust = evaluateTrustFromCertificates(chain[:1], chain, domain.CertificateChain{})

	for i := 0; i < len(chain)-1; i++ {
		cert := chain[i]
		issuer := chain[i+1]

		res, err := v.revChecker.Check(ctx, cert, issuer)
		if err != nil {
			detail := fmt.Sprintf("cert[%d] %s: error comprobando revocación: %v", i, cert.Subject.CommonName, err)
			result = result.AddDetail(detail)
			result.Certificate.Status = domain.VerificationStatusWarning
			result.Certificate.Reason = "no se pudo completar la comprobación de revocación"
			result.Warnings = append(result.Warnings, detail)
			continue
		}

		detail := revocationDetailText(cert, i, res)
		result = result.AddDetail(detail)
		result.Evidence = append(result.Evidence, domain.VerificationEvidence{Type: "revocation", Summary: detail})

		if res.Status == RevocationRevoked {
			result.Valid = false
			result.Reason = fmt.Sprintf("certificado revocado: %s", cert.Subject.CommonName)
			result.Certificate.Status = domain.VerificationStatusInvalid
			result.Certificate.Reason = result.Reason
			result.Errors = append(result.Errors, result.Reason)
		} else if res.Status == RevocationUnknown {
			result.Certificate.Status = domain.VerificationStatusWarning
			result.Certificate.Reason = "revocación no concluyente"
			result.Warnings = append(result.Warnings, detail)
		}
	}

	// No se pisa una advertencia previa ("revocación no concluyente"): solo se
	// declara la cadena no revocada si todas las comprobaciones lo fueron.
	if result.Valid && result.Reason == "" && result.Certificate.Status == domain.VerificationStatusValid {
		result.Reason = "cadena válida, ningún certificado revocado"
		result.Certificate.Reason = result.Reason
	} else if result.Valid && result.Reason == "" {
		result.Reason = "no se pudo confirmar que los certificados no estén revocados"
	}

	return result.Normalize(), nil
}

// mergeRevocationResult incorpora al resultado de una firma el de su
// comprobación de revocación: un certificado revocado invalida la firma y una
// revocación no concluyente deja el certificado en advertencia.
func mergeRevocationResult(result, revocation domain.VerificationResult) domain.VerificationResult {
	result.Details = append(result.Details, revocation.Details...)
	result.Integrity.Details = append(result.Integrity.Details, revocation.Details...)
	result.Warnings = append(result.Warnings, revocation.Warnings...)
	result.Errors = append(result.Errors, revocation.Errors...)
	result.Evidence = append(result.Evidence, revocation.Evidence...)
	if revocation.Certificate.Status != domain.VerificationStatusUnknown {
		result.Certificate = mergeVerificationAspect(result.Certificate, revocation.Certificate)
	}
	if revocation.Trust.Status != domain.VerificationStatusUnknown {
		result.Trust = mergeVerificationAspect(result.Trust, revocation.Trust)
	}
	if !revocation.Valid {
		result.Valid = false
		result.Reason = revocation.Reason
	}
	return result
}

// applyXMLRevocation comprueba la revocación de los firmantes de una firma
// XML (XAdES, FacturaE, ASiC, ODF, OOXML). Antes estas firmas no se
// contrastaban con OCSP/CRL: una firma con un certificado revocado se
// presentaba como válida.
func applyXMLRevocation(ctx context.Context, result domain.VerificationResult, signers, embedded []*x509.Certificate) domain.VerificationResult {
	checker := NewCAdESVerifier()
	for _, signer := range signers {
		chain := chainForRevocation(ctx, checker.revChecker, signer, embedded)
		if len(chain) < 2 {
			result = markRevocationNotChecked(result, signer)
			continue
		}
		revocation, err := checker.VerifyRevocation(ctx, chain)
		if err == nil {
			result = mergeRevocationResult(result, revocation)
		}
	}
	return result.Normalize()
}

// chainForRevocation obtiene la cadena del firmante para comprobar su
// revocación. Si la firma no incluye el emisor (p. ej. XAdES con solo el
// certificado del firmante en KeyInfo), la completa con el almacén del
// sistema; en Windows, CryptoAPI puede descargar la CA intermedia por AIA.
func chainForRevocation(ctx context.Context, checker *RevocationChecker, leaf *x509.Certificate, embedded []*x509.Certificate) []*x509.Certificate {
	chain := buildIssuerChain(leaf, embedded)
	if len(chain) >= 2 || leaf == nil {
		return chain
	}
	intermediates := x509.NewCertPool()
	for _, cert := range embedded {
		if cert != nil {
			intermediates.AddCert(cert)
		}
	}
	chains, err := leaf.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err == nil && len(chains) > 0 && len(chains[0]) >= 2 {
		return chains[0]
	}
	// Último recurso: descargar el emisor por AIA (CA Issuers) con el
	// cliente endurecido. Solo completa la cadena; la confianza se evalúa
	// aparte contra las anclas del sistema.
	if checker == nil || checker.client == nil {
		return chain
	}
	current := chain[len(chain)-1]
	for i := 0; i < 4 && !bytes.Equal(current.RawIssuer, current.RawSubject); i++ {
		issuer, err := checker.client.FetchIssuer(ctx, current)
		if err != nil {
			break
		}
		chain = append(chain, issuer)
		current = issuer
	}
	return chain
}

// markRevocationNotChecked deja constancia de que no se pudo comprobar la
// revocación de un certificado emitido por un tercero: no se presenta como
// vigente sin más. Los certificados autofirmados no tienen revocación.
func markRevocationNotChecked(result domain.VerificationResult, cert *x509.Certificate) domain.VerificationResult {
	if cert == nil || bytes.Equal(cert.RawIssuer, cert.RawSubject) {
		return result
	}
	detail := fmt.Sprintf("revocación no comprobada para %s: no se dispone del certificado emisor", cert.Subject.CommonName)
	result.Details = append(result.Details, detail)
	result.Warnings = append(result.Warnings, detail)
	result.Certificate = mergeVerificationAspect(result.Certificate, domain.VerificationAspect{
		Status: domain.VerificationStatusWarning,
		Reason: "no se pudo comprobar la revocación del certificado",
	})
	return result
}

// buildIssuerChain ordena la cadena desde el certificado firmante buscando,
// entre los certificados disponibles, el emisor cuyo nombre coincide y cuya
// clave verifica la firma del certificado anterior.
func buildIssuerChain(leaf *x509.Certificate, pool []*x509.Certificate) []*x509.Certificate {
	if leaf == nil {
		return nil
	}
	chain := []*x509.Certificate{leaf}
	current := leaf
	for len(chain) <= len(pool) {
		if bytes.Equal(current.RawIssuer, current.RawSubject) {
			break
		}
		var issuer *x509.Certificate
		for _, candidate := range pool {
			if candidate == nil || candidate.Equal(current) || containsCertificate(chain, candidate) {
				continue
			}
			if bytes.Equal(candidate.RawSubject, current.RawIssuer) && current.CheckSignatureFrom(candidate) == nil {
				issuer = candidate
				break
			}
		}
		if issuer == nil {
			break
		}
		chain = append(chain, issuer)
		current = issuer
	}
	return chain
}

func parseContentInfo(der []byte) (contentInfoRaw, error) {
	var ci contentInfoRaw
	if len(der) == 0 {
		return ci, errors.New("firma CAdES vacía")
	}
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		return ci, fmt.Errorf("parseando ContentInfo CMS: %w", err)
	}
	return ci, nil
}

func parseSignedData(der []byte) (signedDataRaw, error) {
	var sd signedDataRaw
	if _, err := asn1.Unmarshal(der, &sd); err != nil {
		return sd, fmt.Errorf("parseando SignedData CMS: %w", err)
	}
	return sd, nil
}

func parseEncapsulatedContent(raw asn1.RawValue) ([]byte, error) {
	var eci encapContentInfoRaw
	if _, err := asn1.Unmarshal(raw.FullBytes, &eci); err != nil {
		return nil, fmt.Errorf("parseando EncapContentInfo: %w", err)
	}
	if !eci.ContentType.Equal(oidData) {
		return nil, fmt.Errorf("contenido encapsulado no soportado: %v", eci.ContentType)
	}
	if len(eci.Content.Bytes) == 0 && len(eci.Content.FullBytes) == 0 {
		return nil, errors.New("la firma CAdES no contiene contenido encapsulado")
	}
	var content []byte
	if _, err := asn1.Unmarshal(eci.Content.Bytes, &content); err != nil {
		if _, err2 := asn1.Unmarshal(eci.Content.FullBytes, &content); err2 != nil {
			return nil, fmt.Errorf("parseando contenido encapsulado: %w", err)
		}
	}
	return content, nil
}

func parseCertificateSet(raw asn1.RawValue) ([]*x509.Certificate, []domain.CertificateRef, error) {
	if len(raw.Bytes) == 0 {
		return nil, nil, nil
	}
	remaining := raw.Bytes
	certs := make([]*x509.Certificate, 0, 4)
	refs := make([]domain.CertificateRef, 0, 4)
	for idx := 0; len(remaining) > 0; idx++ {
		var value asn1.RawValue
		var err error
		remaining, err = asn1.Unmarshal(remaining, &value)
		if err != nil {
			return nil, nil, fmt.Errorf("parseando conjunto de certificados CMS: %w", err)
		}
		cert, err := x509.ParseCertificate(value.FullBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parseando certificado embebido %d: %w", idx, err)
		}
		certs = append(certs, cert)
		refs = append(refs, certificateToRef(cert))
	}
	return certs, refs, nil
}

func parseSignerInfos(raw asn1.RawValue) ([]signerInfoRaw, error) {
	if len(raw.Bytes) == 0 {
		return nil, errors.New("SignerInfos vacío")
	}
	remaining := raw.Bytes
	out := make([]signerInfoRaw, 0, 2)
	for i := 0; len(remaining) > 0; i++ {
		var rawSigner asn1.RawValue
		var err error
		remaining, err = asn1.Unmarshal(remaining, &rawSigner)
		if err != nil {
			return nil, fmt.Errorf("parseando SignerInfos CMS: %w", err)
		}
		var si signerInfoRaw
		if _, err := asn1.Unmarshal(rawSigner.FullBytes, &si); err != nil {
			return nil, fmt.Errorf("parseando SignerInfo %d: %w", i, err)
		}
		out = append(out, si)
	}
	if len(out) == 0 {
		return nil, errors.New("SignerInfos vacío")
	}
	return out, nil
}

// findSignerCertificate localiza el certificado del SignerInfo por emisor y
// número de serie (RFC 5652, IssuerAndSerialNumber). Buscar solo por serie
// confundía firmantes de CA distintas con la misma serie (p. ej. en una
// cofirma). Se mantiene la búsqueda por serie únicamente si es inequívoca,
// para firmas antiguas con el nombre del emisor re-codificado.
func findSignerCertificate(certs []*x509.Certificate, sid issuerAndSerialNumber) (*x509.Certificate, error) {
	serial := sid.SerialNumber
	if serial == nil {
		return nil, errors.New("SignerInfo sin número de serie")
	}
	var porSerie []*x509.Certificate
	for _, cert := range certs {
		if cert == nil || cert.SerialNumber.Cmp(serial) != 0 {
			continue
		}
		if len(sid.Issuer.FullBytes) > 0 && bytes.Equal(cert.RawIssuer, sid.Issuer.FullBytes) {
			return cert, nil
		}
		porSerie = append(porSerie, cert)
	}
	if len(porSerie) == 1 {
		return porSerie[0], nil
	}
	if len(porSerie) > 1 {
		return nil, fmt.Errorf("varios certificados con la serie %s y ninguno del emisor declarado", serial.String())
	}
	return nil, fmt.Errorf("no se encontró el certificado firmante para serie %s", serial.String())
}

func verifyMessageDigest(signedAttrs asn1.RawValue, content []byte, hash crypto.Hash) error {
	if len(content) == 0 {
		return errors.New("contenido firmado vacío")
	}
	attrsSET, err := asSetDER(signedAttrs)
	if err != nil {
		return err
	}
	_ = attrsSET
	messageDigest, err := extractMessageDigestAttribute(signedAttrs.Bytes)
	if err != nil {
		return err
	}
	if !equalBytes(messageDigest, cmsDigest(hash, content)) {
		return errors.New("el atributo messageDigest no coincide con el contenido firmado")
	}
	return nil
}

func verifyCMSignature(cert *x509.Certificate, signerInfo signerInfoRaw, hash crypto.Hash) error {
	if cert == nil {
		return errors.New("certificado firmante nulo")
	}
	attrsSET, err := asSetDER(signerInfo.SignedAttributes)
	if err != nil {
		return err
	}
	digest := cmsDigest(hash, attrsSET)

	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pub, hash, digest, signerInfo.Signature); err != nil {
			return fmt.Errorf("firma RSA inválida: %w", err)
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, digest, signerInfo.Signature) {
			return errors.New("firma ECDSA inválida")
		}
	default:
		return fmt.Errorf("tipo de clave pública no soportado: %T", cert.PublicKey)
	}
	return nil
}

func asSetDER(raw asn1.RawValue) ([]byte, error) {
	if len(raw.FullBytes) == 0 {
		return nil, errors.New("SignerInfo sin atributos firmados")
	}
	if raw.Class != asn1.ClassContextSpecific || raw.Tag != 0 {
		return nil, errors.New("los atributos firmados no usan el tag [0] esperado")
	}
	out := make([]byte, len(raw.FullBytes))
	copy(out, raw.FullBytes)
	out[0] = 0x31
	return out, nil
}

func extractEmbeddedRevocationEvidence(signerInfo signerInfoRaw) (ports.RevocationEvidence, error) {
	attrs, err := parseUnsignedAttributes(signerInfo.UnsignedAttributes)
	if err != nil {
		return ports.RevocationEvidence{}, err
	}
	for _, attr := range attrs {
		if !attr.Type.Equal(oidRevocationValues) {
			continue
		}
		if len(attr.Values) == 0 {
			return ports.RevocationEvidence{}, errors.New("revocationValues vacío")
		}
		var values revocationValues
		if _, err := asn1.Unmarshal(attr.Values[0].FullBytes, &values); err != nil {
			return ports.RevocationEvidence{}, fmt.Errorf("parseando revocationValues embebido: %w", err)
		}
		evidence := ports.RevocationEvidence{
			OCSPResponses: make([][]byte, 0, len(values.OCSPVals)),
			CRLs:          make([][]byte, 0, len(values.CRLVals)),
		}
		for _, ocsp := range values.OCSPVals {
			evidence.OCSPResponses = append(evidence.OCSPResponses, append([]byte(nil), ocsp.FullBytes...))
		}
		for _, crl := range values.CRLVals {
			evidence.CRLs = append(evidence.CRLs, append([]byte(nil), crl.FullBytes...))
		}
		return evidence, nil
	}
	return ports.RevocationEvidence{}, errors.New("sin revocationValues embebido")
}

func verifyEmbeddedRevocation(chain []*x509.Certificate, evidence ports.RevocationEvidence) (domain.VerificationResult, error) {
	if len(chain) == 0 {
		return domain.NewVerificationFailure(string(domain.FormatCAdES), "cadena de certificados vacía", nil), nil
	}
	result := domain.NewVerificationSuccess(string(domain.FormatCAdES), "", nil)
	result.Details = nil
	result.Integrity.Details = nil
	result.Coverage = "chain"
	result.Integrity.Status = domain.VerificationStatusUnknown
	result.Integrity.Reason = "revocación embebida evaluada"
	result.Certificate.Status = domain.VerificationStatusUnknown
	result.Certificate.Reason = "evidencia de revocación embebida pendiente de autenticar"
	result.Trust.Status = domain.VerificationStatusUnknown
	result.Trust.Reason = "evidencias embebidas pendientes de verificar"
	allAuthenticatedGood := true
	for i := 0; i < len(chain)-1; i++ {
		cert := chain[i]
		issuer := chain[i+1]
		res, err := evaluateEmbeddedRevocation(evidence, cert, issuer)
		if err != nil {
			allAuthenticatedGood = false
			detail := fmt.Sprintf("cert[%d] %s: sin evidencias embebidas utilizables", i, cert.Subject.CommonName)
			result = result.AddDetail(detail)
			result.Certificate.Status = domain.VerificationStatusWarning
			result.Certificate.Reason = "sin evidencias embebidas utilizables"
			result.Warnings = append(result.Warnings, detail)
			continue
		}
		detail := revocationDetailText(cert, i, res)
		result = result.AddDetail(detail)
		result.Evidence = append(result.Evidence, domain.VerificationEvidence{Type: "revocation", Summary: detail})
		if res.Status == RevocationRevoked {
			allAuthenticatedGood = false
			result.Valid = false
			result.Reason = fmt.Sprintf("certificado revocado: %s", cert.Subject.CommonName)
			result.Certificate.Status = domain.VerificationStatusInvalid
			result.Certificate.Reason = result.Reason
			result.Errors = append(result.Errors, result.Reason)
		} else if res.Status == RevocationUnknown {
			allAuthenticatedGood = false
			result.Certificate.Status = domain.VerificationStatusWarning
			result.Certificate.Reason = "revocación embebida no concluyente"
			result.Warnings = append(result.Warnings, detail)
		}
	}
	if result.Valid && result.Reason == "" {
		if allAuthenticatedGood {
			result.Reason = "cadena válida con revocación embebida autenticada"
			result.Certificate.Status = domain.VerificationStatusValid
			result.Certificate.Reason = result.Reason
			result.Trust.Status = domain.VerificationStatusValid
			result.Trust.Reason = "evidencias embebidas autenticadas y vigentes"
		} else {
			result.Reason = "cadena válida con revocación embebida no concluyente"
			if result.Certificate.Status == domain.VerificationStatusUnknown {
				result.Certificate.Status = domain.VerificationStatusWarning
				result.Certificate.Reason = "revocación embebida no concluyente"
			}
			result.Trust.Status = domain.VerificationStatusWarning
			result.Trust.Reason = "evidencias embebidas no autenticadas o incompletas"
		}
	}
	return result.Normalize(), nil
}

// evaluateEmbeddedRevocation autentica toda la evidencia aplicable usando
// time.Now como instante de validación. Revoked prevalece sobre cualquier
// Good/Unknown y Unknown impide concluir Good.
func evaluateEmbeddedRevocation(
	evidence ports.RevocationEvidence,
	cert *x509.Certificate,
	issuer *x509.Certificate,
) (RevocationResult, error) {
	var (
		goodResult    RevocationResult
		unknownResult RevocationResult
		usable        bool
	)
	for _, ocspDER := range evidence.OCSPResponses {
		result, err := parseBasicOCSPResponse(ocspDER, cert, issuer)
		if err != nil {
			continue
		}
		usable = true
		result.Method = "ocsp-embebido"
		switch result.Status {
		case RevocationRevoked:
			return result, nil
		case RevocationUnknown:
			unknownResult = result
		case RevocationGood:
			goodResult = result
		}
	}
	for _, crlDER := range evidence.CRLs {
		result, err := parseCRL(crlDER, cert, issuer)
		if err != nil {
			continue
		}
		usable = true
		result.Method = "crl-embebida"
		switch result.Status {
		case RevocationRevoked:
			return result, nil
		case RevocationUnknown:
			unknownResult = result
		case RevocationGood:
			if goodResult.Method == "" {
				goodResult = result
			}
		}
	}
	if unknownResult.Method != "" {
		return unknownResult, nil
	}
	if goodResult.Method != "" {
		return goodResult, nil
	}
	if usable {
		return RevocationResult{
			Status: RevocationUnknown,
			Reason: "evidencia de revocación autenticada no concluyente",
		}, nil
	}
	return RevocationResult{}, errors.New("sin evidencias embebidas autenticadas aplicables")
}

func parseBasicOCSPResponse(
	body []byte,
	cert *x509.Certificate,
	issuer *x509.Certificate,
) (RevocationResult, error) {
	result, err := revocationclient.ValidateBasicOCSPResponse(body, cert, issuer, time.Now())
	if err != nil {
		return RevocationResult{}, err
	}
	return mapValidatedRevocation(result, "ocsp"), nil
}

func parseCRL(
	body []byte,
	cert *x509.Certificate,
	issuer *x509.Certificate,
) (RevocationResult, error) {
	result, err := revocationclient.ValidateCRLResponse(body, cert, issuer, time.Now())
	if err != nil {
		return RevocationResult{}, err
	}
	return mapValidatedRevocation(result, "crl"), nil
}

func mapValidatedRevocation(
	result revocationclient.ValidationResult,
	method string,
) RevocationResult {
	mapped := RevocationResult{
		Method:    method,
		CheckedAt: time.Now(),
		RevokedAt: result.RevokedAt,
	}
	switch result.Status {
	case revocationclient.CertificateStatusGood:
		mapped.Status = RevocationGood
	case revocationclient.CertificateStatusRevoked:
		mapped.Status = RevocationRevoked
		mapped.Reason = "certificado revocado según " + method
	default:
		mapped.Status = RevocationUnknown
		mapped.Reason = "estado de revocación desconocido"
	}
	return mapped
}

func extractMessageDigestAttribute(attrsDER []byte) ([]byte, error) {
	remaining := attrsDER
	for len(remaining) > 0 {
		var rawAttr asn1.RawValue
		var err error
		remaining, err = asn1.Unmarshal(remaining, &rawAttr)
		if err != nil {
			return nil, fmt.Errorf("parseando atributos firmados: %w", err)
		}
		var attr attribute
		if _, err := asn1.Unmarshal(rawAttr.FullBytes, &attr); err != nil {
			return nil, fmt.Errorf("parseando atributo firmado: %w", err)
		}
		if !attr.Type.Equal(oidMessageDigest) {
			continue
		}
		if len(attr.Values) == 0 {
			return nil, errors.New("atributo messageDigest vacío")
		}
		var digest []byte
		if _, err := asn1.Unmarshal(attr.Values[0].FullBytes, &digest); err != nil {
			return nil, fmt.Errorf("parseando valor de messageDigest: %w", err)
		}
		return digest, nil
	}
	return nil, errors.New("atributo messageDigest ausente")
}

func certificateToRef(cert *x509.Certificate) domain.CertificateRef {
	fingerprint := sha256.Sum256(cert.Raw)
	return domain.CertificateRef{
		ID:          cert.SerialNumber.String(),
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		NotAfter:    cert.NotAfter,
		Fingerprint: hex.EncodeToString(fingerprint[:]),
	}
}

func reverseCertificateRefs(items []domain.CertificateRef) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func reverseCertificates(items []*x509.Certificate) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func reverseStringSlices(items [][]string) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func applySignerVerificationMetadata(result domain.VerificationResult, signerCerts, embeddedCerts []*x509.Certificate, anchors domain.CertificateChain) domain.VerificationResult {
	if result.Format == "" {
		result.Format = string(domain.FormatCAdES)
	}
	result.Certificate = evaluateCertificateAspect(signerCerts)
	result.Trust = evaluateTrustFromCertificates(signerCerts, embeddedCerts, anchors)
	for _, cert := range signerCerts {
		if cert == nil {
			continue
		}
		result.Evidence = append(result.Evidence,
			domain.VerificationEvidence{Type: "certificate.subject", Summary: cert.Subject.String()},
			domain.VerificationEvidence{Type: "certificate.issuer", Summary: cert.Issuer.String()},
		)
	}
	if result.Certificate.Status == domain.VerificationStatusInvalid {
		result.Valid = false
		result.Reason = result.Certificate.Reason
		result.Errors = appendUniqueString(result.Errors, result.Certificate.Reason)
	}
	switch result.Trust.Status {
	case domain.VerificationStatusInvalid:
		result.Valid = false
		result.Reason = result.Trust.Reason
		result.Errors = appendUniqueString(result.Errors, result.Trust.Reason)
	case domain.VerificationStatusUnknown, domain.VerificationStatusWarning:
		if result.Trust.Reason != "" {
			result.Warnings = appendUniqueString(result.Warnings, result.Trust.Reason)
		}
	}
	return result.Normalize()
}

func evaluateCertificateAspect(certs []*x509.Certificate) domain.VerificationAspect {
	now := time.Now()
	if len(certs) == 0 {
		return domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: "sin certificado firmante embebido"}
	}
	aspect := domain.VerificationAspect{
		Status: domain.VerificationStatusValid,
		Reason: "certificado firmante vigente",
	}
	evaluated := 0
	for _, cert := range certs {
		if cert == nil {
			continue
		}
		evaluated++
		aspect.Details = append(aspect.Details, fmt.Sprintf("subject=%s", cert.Subject.String()))
		switch {
		case now.Before(cert.NotBefore):
			aspect.Status = domain.VerificationStatusInvalid
			aspect.Reason = fmt.Sprintf("certificado aún no válido: %s", cert.Subject.CommonName)
			aspect.Details = append(aspect.Details, fmt.Sprintf("not_before=%s", cert.NotBefore.UTC().Format(time.RFC3339)))
		case now.After(cert.NotAfter):
			aspect.Status = domain.VerificationStatusInvalid
			aspect.Reason = fmt.Sprintf("certificado caducado: %s", cert.Subject.CommonName)
			aspect.Details = append(aspect.Details, fmt.Sprintf("not_after=%s", cert.NotAfter.UTC().Format(time.RFC3339)))
		case cert.KeyUsage != 0 && cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0:
			aspect.Status = domain.VerificationStatusInvalid
			aspect.Reason = fmt.Sprintf("certificado no autorizado para firma digital: %s", cert.Subject.CommonName)
			aspect.Details = append(aspect.Details, fmt.Sprintf("key_usage=%d", cert.KeyUsage))
		}
	}
	if evaluated == 0 {
		return domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: "sin certificado firmante embebido"}
	}
	return aspect
}

func evaluateTrustFromCertificates(signerCerts, embeddedCerts []*x509.Certificate, anchors domain.CertificateChain) domain.VerificationAspect {
	if len(signerCerts) == 0 {
		return domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: "sin certificado firmante para evaluar confianza"}
	}
	if anchors.IsEmpty() {
		return domain.VerificationAspect{Status: domain.VerificationStatusUnknown, Reason: "sin anclas de confianza disponibles"}
	}

	roots := x509.NewCertPool()
	rootFingerprints := make(map[string]struct{})
	details := make([]string, 0, len(anchors.DERCertificates)+len(anchors.Certificates))
	hasRoots := false
	systemRootsUnavailable := false

	if anchors.UseSystemRoots {
		systemRoots, err := x509.SystemCertPool()
		if err != nil {
			systemRootsUnavailable = true
			details = append(details, "system_roots_error="+err.Error())
		} else if systemRoots == nil {
			systemRootsUnavailable = true
			details = append(details, "system_roots=nil")
		} else {
			roots = systemRoots.Clone()
			hasRoots = true
			// CertPool.Subjects no representa necesariamente las raíces del
			// sistema (en Windows puede devolver una lista vacía aunque el pool
			// sea utilizable). SystemCertPool sin error es la señal portable.
			details = append(details, "system_roots=loaded")
		}
	}

	for i, der := range anchors.DERCertificates {
		anchor, err := x509.ParseCertificate(der)
		if err != nil {
			return domain.VerificationAspect{
				Status:  domain.VerificationStatusInvalid,
				Reason:  "ancla de confianza X.509 inválida",
				Details: []string{fmt.Sprintf("anchor_der[%d]: %v", i, err)},
			}
		}
		roots.AddCert(anchor)
		hasRoots = true
		fingerprint := certificateFingerprint(anchor)
		rootFingerprints[fingerprint] = struct{}{}
		details = append(details, fmt.Sprintf("anchor=%s", anchor.Subject.String()))
	}

	// Compatibilidad con proveedores antiguos: una referencia puede designar
	// un certificado embebido únicamente por su huella exacta. El nombre del
	// sujeto o emisor no constituye una prueba de identidad del ancla.
	for i, anchorRef := range anchors.Certificates {
		fingerprint := normalizeFingerprint(anchorRef.Fingerprint)
		if fingerprint == "" {
			details = append(details, fmt.Sprintf("anchor_ref[%d]=sin_huella", i))
			continue
		}
		for _, embedded := range embeddedCerts {
			if embedded == nil || certificateFingerprint(embedded) != fingerprint {
				continue
			}
			if _, exists := rootFingerprints[fingerprint]; !exists {
				roots.AddCert(embedded)
				hasRoots = true
				rootFingerprints[fingerprint] = struct{}{}
			}
			details = append(details, fmt.Sprintf("anchor=%s", embedded.Subject.String()))
			break
		}
	}

	if !hasRoots {
		if systemRootsUnavailable {
			return domain.VerificationAspect{
				Status:  domain.VerificationStatusUnknown,
				Reason:  "no se pudo consultar un almacén raíz del sistema utilizable",
				Details: details,
			}
		}
		return domain.VerificationAspect{
			Status:  domain.VerificationStatusInvalid,
			Reason:  "ninguna ancla de confianza configurada contiene material X.509 verificable",
			Details: details,
		}
	}

	intermediates := x509.NewCertPool()
	for _, cert := range embeddedCerts {
		if cert == nil || containsCertificate(signerCerts, cert) {
			continue
		}
		if _, isRoot := rootFingerprints[certificateFingerprint(cert)]; isRoot {
			continue
		}
		intermediates.AddCert(cert)
	}

	for i, signerCert := range signerCerts {
		if signerCert == nil {
			return domain.VerificationAspect{
				Status:  domain.VerificationStatusInvalid,
				Reason:  "certificado firmante nulo",
				Details: details,
			}
		}
		chains, err := signerCert.Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			CurrentTime:   time.Now(),
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		if err != nil {
			return domain.VerificationAspect{
				Status:  domain.VerificationStatusInvalid,
				Reason:  fmt.Sprintf("cadena de confianza inválida para %s", signerCert.Subject.CommonName),
				Details: append(details, fmt.Sprintf("signer[%d]: %v", i, err)),
			}
		}
		details = append(details, fmt.Sprintf("signer[%d].chain_length=%d", i, len(chains[0])))
	}

	return domain.VerificationAspect{
		Status:  domain.VerificationStatusValid,
		Reason:  "cadena X.509 verificada contra las anclas de confianza configuradas",
		Details: details,
	}
}

// EvaluateCertificateTrust comprueba una o varias cadenas de firmante contra
// anclas X.509 explícitas. Se expone para que otros contenedores firmados
// apliquen exactamente la misma política de raíces, intermedios y vigencia que
// los verificadores de firma, sin confundir integridad matemática con confianza.
func EvaluateCertificateTrust(signerCerts, embeddedCerts []*x509.Certificate, anchors domain.CertificateChain) domain.VerificationAspect {
	return evaluateTrustFromCertificates(signerCerts, embeddedCerts, anchors)
}

func containsCertificate(certs []*x509.Certificate, candidate *x509.Certificate) bool {
	if candidate == nil {
		return false
	}
	fingerprint := certificateFingerprint(candidate)
	for _, cert := range certs {
		if cert != nil && certificateFingerprint(cert) == fingerprint {
			return true
		}
	}
	return false
}

func certificateFingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

func normalizeFingerprint(raw string) string {
	normalized := strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(raw))
	return strings.ToLower(normalized)
}

func appendUniqueString(items []string, value string) []string {
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

// ContenidoCAdESImplicito devuelve el contenido firmado de una firma CAdES
// implícita (attached); ok es false si la firma es explícita (detached).
func ContenidoCAdESImplicito(der []byte) (contenido []byte, ok bool, err error) {
	ci, err := parseContentInfo(der)
	if err != nil {
		return nil, false, err
	}
	if !ci.ContentType.Equal(oidSignedData) {
		return nil, false, errors.New("el fichero no es una firma CMS SignedData")
	}
	sd, err := parseSignedData(ci.Content.Bytes)
	if err != nil {
		return nil, false, err
	}
	var eci encapContentInfoRaw
	if _, err := asn1.Unmarshal(sd.EncapContentInfo.FullBytes, &eci); err != nil {
		return nil, false, fmt.Errorf("parseando EncapContentInfo: %w", err)
	}
	if len(eci.Content.Bytes) == 0 && len(eci.Content.FullBytes) == 0 {
		return nil, false, nil
	}
	contenido, err = parseEncapsulatedContent(sd.EncapContentInfo)
	return contenido, err == nil, err
}
