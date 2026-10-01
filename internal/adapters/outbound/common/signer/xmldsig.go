// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

const formatXMLDSig = domain.SignatureFormat("XMLdSig")

// XMLDSigDetached implementa una firma XMLDSig detached básica en Go nativo.
type XMLDSigDetached struct{}

func NewXMLDSigDetached() *XMLDSigDetached {
	return &XMLDSigDetached{}
}

func (e *XMLDSigDetached) Sign(ctx context.Context, job domain.SignatureJob, key ports.SigningKey) (domain.SignatureResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.SignatureResult{}, err
	}
	if key == nil {
		return domain.SignatureResult{}, errors.New("la clave de firma no puede ser nil")
	}
	if err := job.Validate(); err != nil {
		return domain.SignatureResult{}, err
	}
	if job.Format != formatXMLDSig {
		return domain.SignatureResult{}, fmt.Errorf("este motor solo soporta formato %s", formatXMLDSig)
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return domain.SignatureResult{}, err
	}
	// Variantes de AutoFirma Java (XMLDSig Enveloping, Enveloped, Externally
	// Detached) y cofirma: mismo constructor que XAdES, sin propiedades XAdES.
	if variante := xadesVariante(job.Options); variante != xadesVarianteDetached || job.Action == domain.ActionCoSign {
		pura := job
		pura.Options = map[string]string{opcionXMLDSigPura: "true"}
		for k, v := range job.Options {
			pura.Options[k] = v
		}
		var (
			firmado  []byte
			etiqueta string
		)
		if job.Action == domain.ActionCoSign {
			firmado, etiqueta, err = cosignXAdES(pura, clave)
		} else if job.Action == domain.ActionSign {
			firmado, etiqueta, err = buildXAdESEnvelop(pura, clave, variante)
		} else {
			err = errors.New("XMLDSig no admite contrafirma: requiere propiedades XAdES")
		}
		if err != nil {
			return domain.SignatureResult{}, err
		}
		return domain.SignatureResult{Format: formatXMLDSig, Data: firmado, Algorithm: "XMLDSig-" + etiqueta}, nil
	}
	if job.Action != domain.ActionSign {
		return domain.SignatureResult{}, errors.New("XMLDSig Detached no admite contrafirma")
	}
	xmlSig, err := buildXMLDSigDetached(job.Document.Name, job.Document.Content, job.Document.MIMEType, clave, time.Now().UTC())
	if err != nil {
		return domain.SignatureResult{}, err
	}
	return domain.SignatureResult{
		Format:    formatXMLDSig,
		Data:      xmlSig,
		Algorithm: "XMLDSig-RSA-SHA256",
	}, nil
}

func buildXMLDSigDetached(documentName string, data []byte, mimeType string, key *LocalSigningKey, now time.Time) ([]byte, error) {
	sigID := "Signature-1"
	keyInfoID := "KeyInfo-1"
	referenceID := "Reference-Document-1"
	contentXML := buildContentXML(documentName, normalizeMimeType(mimeType, data), data)

	contentC14N, err := exclusiveC14N(string(data))
	if err != nil {
		return nil, fmt.Errorf("error canonicalizando documento XML detached: %w", err)
	}
	docDigest := sha256.Sum256(contentC14N)
	docDigestB64 := base64.StdEncoding.EncodeToString(docDigest[:])

	certB64 := base64.StdEncoding.EncodeToString(key.Certificate.Raw)
	keyInfoXML := buildKeyInfoXML(keyInfoID, certB64)
	keyInfoC14N, err := exclusiveC14N(keyInfoXML)
	if err != nil {
		return nil, fmt.Errorf("error canonicalizando KeyInfo: %w", err)
	}
	keyInfoDigest := sha256.Sum256(keyInfoC14N)
	keyInfoDigestB64 := base64.StdEncoding.EncodeToString(keyInfoDigest[:])

	documentURI := documentName
	if documentURI == "" {
		documentURI = "documento.xml"
	}
	signedInfoXML := buildSignedInfoXMLDetached(documentURI, referenceID, docDigestB64, keyInfoID, keyInfoDigestB64)
	signedInfoC14N, err := exclusiveC14N(signedInfoXML)
	if err != nil {
		return nil, fmt.Errorf("error canonicalizando SignedInfo: %w", err)
	}

	hashSignedInfo := sha256.Sum256(signedInfoC14N)
	sigBytes, err := key.Signer.Sign(rand.Reader, hashSignedInfo[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("error firmando SignedInfo: %w", err)
	}
	sigValueB64 := base64.StdEncoding.EncodeToString(sigBytes)
	return []byte(buildFinalXMLDSig(contentXML, sigID, signedInfoXML, sigValueB64, keyInfoXML, now)), nil
}

func buildSignedInfoXMLDetached(documentURI, referenceID, docDigestB64, keyInfoID, keyInfoDigestB64 string) string {
	return fmt.Sprintf(`<ds:SignedInfo xmlns:ds="%s">`+
		`<ds:CanonicalizationMethod Algorithm="%s"/>`+
		`<ds:SignatureMethod Algorithm="%s"/>`+
		`<ds:Reference Id="%s" URI="%s">`+
		`<ds:Transforms>`+
		`<ds:Transform Algorithm="%s"/>`+
		`</ds:Transforms>`+
		`<ds:DigestMethod Algorithm="%s"/>`+
		`<ds:DigestValue>%s</ds:DigestValue>`+
		`</ds:Reference>`+
		`<ds:Reference URI="#%s">`+
		`<ds:DigestMethod Algorithm="%s"/>`+
		`<ds:DigestValue>%s</ds:DigestValue>`+
		`</ds:Reference>`+
		`</ds:SignedInfo>`,
		nsXMLDSig,
		algExcC14N,
		algRSASHA256,
		referenceID,
		documentURI,
		algExcC14N,
		algSHA256,
		docDigestB64,
		keyInfoID,
		algSHA256,
		keyInfoDigestB64,
	)
}

func buildFinalXMLDSig(contentXML, sigID, signedInfoXML, sigValueB64, keyInfoXML string, now time.Time) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<AFIRMA Version="XMLDSig-1.0" GeneratedAt="%s">`+
		`%s`+
		`<ds:Signature xmlns:ds="%s" Id="%s">`+
		`%s`+
		`<ds:SignatureValue>%s</ds:SignatureValue>`+
		`%s`+
		`</ds:Signature>`+
		`</AFIRMA>`,
		now.Format(time.RFC3339),
		contentXML,
		nsXMLDSig,
		sigID,
		signedInfoXML,
		sigValueB64,
		keyInfoXML,
	)
}

var _ ports.SignerEngine = (*XMLDSigDetached)(nil)
