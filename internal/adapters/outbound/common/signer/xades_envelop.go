// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
)

// Variantes XAdES de AutoFirma Java (propiedad "format"):
//   - XAdES Detached: datos en un nodo CONTENT hermano de la firma (AFIRMA).
//   - XAdES Enveloping: datos dentro de un ds:Object de la propia firma. Es
//     el valor por defecto de Java cuando el portal no indica formato.
//   - XAdES Enveloped: la firma se inserta dentro del XML firmado. Solo es
//     posible sobre datos XML, igual que en Java.
const (
	xadesVarianteDetached   = "detached"
	xadesVarianteEnveloping = "enveloping"
	xadesVarianteEnveloped  = "enveloped"
	xadesVarianteExterna    = "externally-detached"

	algBase64Transform   = "http://www.w3.org/2000/09/xmldsig#base64"
	typeXMLDSigObjectRef = "http://www.w3.org/2000/09/xmldsig#Object"
)

func xadesVariante(options map[string]string) string {
	formato := strings.ToLower(strings.TrimSpace(valorOpcion(options, "format")))
	switch {
	case strings.Contains(formato, "externally detached") ||
		strings.EqualFold(strings.TrimSpace(valorOpcion(options, "useManifest")), "true"):
		return xadesVarianteExterna
	case strings.Contains(formato, "enveloping"):
		return xadesVarianteEnveloping
	case strings.Contains(formato, "enveloped"):
		return xadesVarianteEnveloped
	default:
		return xadesVarianteDetached
	}
}

func buildXAdESEnvelop(job domain.SignatureJob, key *LocalSigningKey, variante string) ([]byte, string, error) {
	return buildXAdESEnvelopConIDs(job, key, variante, idsXAdESNuevos())
}

// buildXAdESEnvelopConIDs usa identificadores únicos: en una cofirma conviven
// varias firmas y sus Id no pueden repetirse.
func buildXAdESEnvelopConIDs(job domain.SignatureJob, key *LocalSigningKey, variante string, ids xadesIDs) ([]byte, string, error) {
	data := job.Document.Content
	algOpts, err := resolveXAdESAlgorithmOptions(job.Options)
	if err != nil {
		return nil, "", err
	}
	buildOpts := resolveXAdESBuildOptions(job.Options, algOpts)
	mimeType := normalizeMimeType(job.Document.MIMEType, data)
	esXML := isXMLPayload(data, "application/xml")
	if !esXML && strings.Contains(strings.ToLower(mimeType), "xml") {
		mimeType = detectarMimeNoXML(data)
	}

	var (
		documentURI string
		docDigest   []byte
		objectXML   string
	)
	switch variante {
	case xadesVarianteExterna:
		return buildXAdESExterna(job, key, ids, algOpts, buildOpts, mimeType)
	case xadesVarianteEnveloped:
		if !esXML {
			return nil, "", errors.New("las firmas XAdES Enveloped solo pueden realizarse sobre datos XML")
		}
		// Cadena de Java: enveloped, C14N y XPath que excluye todas las
		// firmas; así una cofirma posterior no invalida esta firma.
		sinFirmas, err := quitarTodasLasFirmas([]byte(stripXMLDeclaration(data)))
		if err != nil {
			return nil, "", fmt.Errorf("XML a firmar inválido: %w", err)
		}
		var canonical []byte
		if nodo := strings.TrimSpace(valorOpcion(job.Options, "nodeToSign")); nodo != "" {
			// Firma de un nodo concreto (nodeToSign de Java), por su Id único.
			if _, err := uniqueElementByIDInXML(sinFirmas, nodo); err != nil {
				return nil, "", fmt.Errorf("nodeToSign %q: %w", nodo, err)
			}
			canonical, err = canonicalizeElementByIDInDocument(sinFirmas, nodo, algC14N)
			documentURI = "#" + nodo
		} else {
			canonical, err = canonicalizeXML(sinFirmas, algC14N)
		}
		if err != nil {
			return nil, "", fmt.Errorf("error canonicalizando el XML a firmar: %w", err)
		}
		if docDigest, err = digestBytes(algOpts.Hash, canonical); err != nil {
			return nil, "", err
		}
		buildOpts.DocumentTransforms = []string{algEnveloped, algC14N, algXPathFilter}
	case xadesVarianteEnveloping:
		objectID := ids.sig + "-Object"
		if nodo := strings.TrimSpace(valorOpcion(job.Options, "nodeToSign")); nodo != "" && !esXML {
			return nil, "", errors.New("nodeToSign solo puede usarse con datos XML")
		}
		if esXML {
			objectXML = fmt.Sprintf(`<ds:Object xmlns:ds="%s" Id="%s" MimeType="%s">%s</ds:Object>`,
				nsXMLDSig, objectID, escapeXMLAttr(mimeType), stripXMLDeclaration(data))
			canonical, err := exclusiveC14N(objectXML)
			if err != nil {
				return nil, "", fmt.Errorf("error canonicalizando el objeto firmado: %w", err)
			}
			if docDigest, err = digestBytes(algOpts.Hash, canonical); err != nil {
				return nil, "", err
			}
			buildOpts.DocumentTransforms = []string{algExcC14N}
			if nodo := strings.TrimSpace(valorOpcion(job.Options, "nodeToSign")); nodo != "" {
				if _, err := uniqueElementByIDInXML([]byte(objectXML), nodo); err != nil {
					return nil, "", fmt.Errorf("nodeToSign %q: %w", nodo, err)
				}
				canonical, err := canonicalizeElementByIDInDocument([]byte(objectXML), nodo, algC14N)
				if err != nil {
					return nil, "", err
				}
				if docDigest, err = digestBytes(algOpts.Hash, canonical); err != nil {
					return nil, "", err
				}
				buildOpts.DocumentTransforms = []string{algC14N}
				documentURI = "#" + nodo
			}
		} else {
			// Como Java: datos binarios en Base64 con la transformación Base64,
			// de modo que el resumen cubre exactamente los bytes originales.
			objectXML = fmt.Sprintf(`<ds:Object xmlns:ds="%s" Id="%s" MimeType="%s" Encoding="%s">%s</ds:Object>`,
				nsXMLDSig, objectID, escapeXMLAttr(mimeType), algBase64Transform, base64.StdEncoding.EncodeToString(data))
			if docDigest, err = digestBytes(algOpts.Hash, data); err != nil {
				return nil, "", err
			}
			buildOpts.DocumentTransforms = []string{algBase64Transform}
		}
		if documentURI == "" {
			documentURI = "#" + objectID
			buildOpts.DocumentReferenceType = typeXMLDSigObjectRef
		}
	default:
		return nil, "", fmt.Errorf("variante XAdES no soportada: %s", variante)
	}

	signedInfoXML, sigValueB64, keyInfoXML, signedPropsXML, err := firmarPartesXAdES(
		key, algOpts, buildOpts, ids, documentURI, base64.StdEncoding.EncodeToString(docDigest), mimeType)
	if err != nil {
		return nil, "", err
	}
	signatureXML := ensamblarFirmaXAdES(ids, buildOpts.Namespace, signedInfoXML, sigValueB64, keyInfoXML, objectXML, signedPropsXML)

	if variante == xadesVarianteEnveloping {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>` + signatureXML), buildOpts.Algorithm.Label, nil
	}
	firmado, err := insertarFirmaEnRaiz(data, signatureXML)
	if err != nil {
		return nil, "", err
	}
	return firmado, buildOpts.Algorithm.Label, nil
}

// insertarFirmaEnRaiz coloca la firma como último hijo del elemento raíz,
// conservando byte a byte el resto del documento.
func insertarFirmaEnRaiz(data []byte, signatureXML string) ([]byte, error) {
	spans, err := scanXMLElements(data)
	if err != nil || len(spans) == 0 {
		return nil, fmt.Errorf("XML a firmar inválido: %v", err)
	}
	raiz := spans[0]
	for _, span := range spans[1:] {
		if span.start < raiz.start {
			raiz = span
		}
	}
	end := int(raiz.end)
	if end <= 0 || end > len(data) {
		return nil, errors.New("no se localiza el cierre del elemento raíz")
	}
	literal := string(data[raiz.start:end])
	if strings.HasSuffix(literal, "/>") {
		// Raíz vacía (<raiz/>): se abre para alojar la firma. La
		// canonicalización ya la trataba como <raiz></raiz>.
		nombre := strings.TrimPrefix(literal, "<")
		if i := strings.IndexAny(nombre, " \t\r\n/>"); i > 0 {
			nombre = nombre[:i]
		}
		out := make([]byte, 0, len(data)+len(signatureXML)+len(nombre)+3)
		out = append(out, data[:end-2]...)
		out = append(out, '>')
		out = append(out, signatureXML...)
		out = append(out, "</"+nombre+">"...)
		out = append(out, data[end:]...)
		return out, nil
	}
	cierre := strings.LastIndex(string(data[:end]), "</")
	if cierre < int(raiz.start) {
		return nil, errors.New("no se localiza el cierre del elemento raíz")
	}
	out := make([]byte, 0, len(data)+len(signatureXML))
	out = append(out, data[:cierre]...)
	out = append(out, signatureXML...)
	out = append(out, data[cierre:]...)
	return out, nil
}

func uniqueElementByIDInXML(data []byte, id string) (xmlElementSpan, error) {
	spans, err := scanXMLElements(data)
	if err != nil {
		return xmlElementSpan{}, err
	}
	return uniqueElementByID(spans, id)
}
