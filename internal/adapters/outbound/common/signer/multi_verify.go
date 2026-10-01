// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"grxfirma/internal/adapters/outbound/common/asiccontainer"
	"grxfirma/internal/adapters/outbound/common/officecontainer"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// MultiVerifier selecciona el verificador apropiado según MIME, nombre o firma mágica.
type MultiVerifier struct {
	cades     ports.VerifierEngine
	xades     ports.VerifierEngine
	xmldsig   ports.VerifierEngine
	pades     ports.VerifierEngine
	odf       ports.VerifierEngine
	ooxml     ports.VerifierEngine
	facturae  ports.VerifierEngine
	asicxades ports.VerifierEngine
}

func NewMultiVerifier() *MultiVerifier {
	return &MultiVerifier{
		cades:     NewCAdESVerifier(),
		xades:     NewXAdESVerifier(),
		xmldsig:   NewXMLDSigVerifier(),
		pades:     NewPAdESVerifier(),
		odf:       NewODFVerifier(),
		ooxml:     NewOOXMLVerifier(),
		facturae:  NewFacturaEVerifier(),
		asicxades: NewASiCXAdESVerifier(),
	}
}

// NewMultiVerifierWithHTTPClient aplica el mismo transporte HTTP seguro a las
// consultas OCSP/CRL de CAdES y PAdES. El resto de verificadores no realiza
// conexiones de red.
func NewMultiVerifierWithHTTPClient(client *http.Client) *MultiVerifier {
	cades := NewCAdESVerifierWithChecker(NewRevocationCheckerWithClient(client))
	return &MultiVerifier{
		cades:     cades,
		xades:     NewXAdESVerifier(),
		xmldsig:   NewXMLDSigVerifier(),
		pades:     NewPAdESVerifierWithCAdES(cades),
		odf:       NewODFVerifier(),
		ooxml:     NewOOXMLVerifier(),
		facturae:  NewFacturaEVerifier(),
		asicxades: NewASiCXAdESVerifier(),
	}
}

func NewMultiVerifierWithEngines(cades, xades, xmldsig, pades, odf, ooxml, facturae, asicxades ports.VerifierEngine) *MultiVerifier {
	if cades == nil {
		cades = NewCAdESVerifier()
	}
	if xades == nil {
		xades = NewXAdESVerifier()
	}
	if xmldsig == nil {
		xmldsig = NewXMLDSigVerifier()
	}
	if pades == nil {
		pades = NewPAdESVerifier()
	}
	if odf == nil {
		odf = NewODFVerifier()
	}
	if ooxml == nil {
		ooxml = NewOOXMLVerifier()
	}
	if facturae == nil {
		facturae = NewFacturaEVerifier()
	}
	if asicxades == nil {
		asicxades = NewASiCXAdESVerifier()
	}
	return &MultiVerifier{cades: cades, xades: xades, xmldsig: xmldsig, pades: pades, odf: odf, ooxml: ooxml, facturae: facturae, asicxades: asicxades}
}

func (v *MultiVerifier) Verify(ctx context.Context, signedDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	engine, format, err := v.pickEngine(signedDocument)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	result, signers, err := engine.Verify(ctx, signedDocument, anchors)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	result.Details = append([]string{fmt.Sprintf("formato_detectado=%s", format)}, result.Details...)
	return result, signers, nil
}

func (v *MultiVerifier) VerifyDetached(ctx context.Context, signedDocument domain.Document, originalDocument domain.Document, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	engine, format, err := v.pickEngine(signedDocument)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	if format == "PAdES" {
		result, signers, err := engine.Verify(ctx, signedDocument, anchors)
		if err != nil {
			return domain.VerificationResult{}, nil, err
		}
		result.Details = append([]string{
			fmt.Sprintf("formato_detectado=%s", format),
			"modo=embedded",
			"original_aportado_ignorado_en_pades",
		}, result.Details...)
		return result, signers, nil
	}
	detached, ok := engine.(interface {
		VerifyDetached(context.Context, domain.Document, domain.Document, domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error)
	})
	if !ok {
		return domain.VerificationResult{}, nil, fmt.Errorf("el formato %s no soporta verificación detached", format)
	}
	result, signers, err := detached.VerifyDetached(ctx, signedDocument, originalDocument, anchors)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	result.Details = append([]string{fmt.Sprintf("formato_detectado=%s", format), "modo=detached"}, result.Details...)
	return result, signers, nil
}

func (v *MultiVerifier) pickEngine(doc domain.Document) (ports.VerifierEngine, string, error) {
	mime := strings.ToLower(strings.TrimSpace(doc.MIMEType))
	name := strings.ToLower(strings.TrimSpace(doc.Name))
	content := doc.Content

	switch {
	case strings.Contains(mime, "pdf"), strings.HasSuffix(name, ".pdf"), hasPrefix(content, "%PDF-"):
		return v.pades, "PAdES", nil
	case asiccontainer.SignedDetect(content), strings.HasSuffix(name, ".asics"), strings.Contains(mime, asiccontainer.MIMETypeASiCS):
		if _, err := asiccontainer.ExtractXAdESSignature(content); err != nil {
			if _, err := asiccontainer.ExtractCAdESSignature(content); err == nil {
				return v.asicxades, "ASiC-CAdES", nil
			}
		}
		return v.asicxades, "ASiC-XAdES", nil
	case officecontainer.SignedDetect(content) == officecontainer.KindODF:
		return v.odf, "ODF", nil
	case officecontainer.SignedDetect(content) == officecontainer.KindOOXML:
		return v.ooxml, "OOXML", nil
	case looksLikeSignedXML(content) && isFacturaEXML(content):
		return v.facturae, "FacturaE", nil
	case strings.HasSuffix(name, ".xsig"), looksLikeXAdES(content):
		return v.xades, "XAdES", nil
	case strings.HasSuffix(name, ".dsig"), strings.HasSuffix(name, ".xmlsig"), looksLikeXMLDSig(content):
		return v.xmldsig, "XMLdSig", nil
	case strings.Contains(mime, "xml"), strings.HasSuffix(name, ".xml"), looksLikeSignedXML(content):
		if looksLikeXAdES(content) {
			return v.xades, "XAdES", nil
		}
		return v.xmldsig, "XMLdSig", nil
	case strings.Contains(mime, "pkcs7"), strings.Contains(mime, "cms"), strings.HasSuffix(name, ".csig"), strings.HasSuffix(name, ".p7s"):
		return v.cades, "CAdES", nil
	case len(content) > 0 && content[0] == 0x30:
		return v.cades, "CAdES", nil
	default:
		return nil, "", fmt.Errorf("no se pudo detectar el formato de firma para %q", doc.Name)
	}
}

func hasPrefix(data []byte, prefix string) bool {
	if len(data) < len(prefix) {
		return false
	}
	return string(data[:len(prefix)]) == prefix
}

func looksLikeSignedXML(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return strings.Contains(trimmed, "<ds:Signature") || strings.Contains(trimmed, "<Signature") || strings.Contains(trimmed, "<AFIRMA")
}

func looksLikeXAdES(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return strings.Contains(trimmed, "xades:QualifyingProperties") ||
		strings.Contains(trimmed, "xades:SignedProperties") ||
		strings.Contains(trimmed, "http://uri.etsi.org/01903/")
}

func looksLikeXMLDSig(data []byte) bool {
	return looksLikeSignedXML(data) && !looksLikeXAdES(data)
}

var _ ports.VerifierEngine = (*MultiVerifier)(nil)
