// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"grxfirma/internal/domain"
)

// ErrFirmaNoCorrespondeOriginal indica que la firma no cubre el documento
// aportado como original, o que no se ha podido comprobar. Antes de envolver
// una firma en un documento ENI se coteja sin red: un ENI que une un
// documento con una firma de otro aparentaría estar firmado sin estarlo.
var ErrFirmaNoCorrespondeOriginal = errors.New("la firma no corresponde al documento original")

// CotejarCAdESExplicita comprueba que el atributo messageDigest de cada
// firmante coincide con el resumen del original y que la firma de los
// atributos es válida. No consulta revocación ni confianza.
func CotejarCAdESExplicita(cmsDER, original []byte) error {
	if len(original) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	ci, err := parseContentInfo(cmsDER)
	if err != nil || !ci.ContentType.Equal(oidSignedData) {
		return ErrFirmaNoCorrespondeOriginal
	}
	sd, err := parseSignedData(ci.Content.Bytes)
	if err != nil {
		return ErrFirmaNoCorrespondeOriginal
	}
	certs, _, err := parseCertificateSet(sd.Certificates)
	if err != nil || len(certs) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	signerInfos, err := parseSignerInfos(sd.SignerInfos)
	if err != nil || len(signerInfos) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	for _, si := range signerInfos {
		cert, err := findSignerCertificate(certs, si.SID)
		if err != nil {
			return ErrFirmaNoCorrespondeOriginal
		}
		spec, err := validarAlgoritmosSignerInfo(cert, si)
		if err != nil {
			return ErrFirmaNoCorrespondeOriginal
		}
		if verifyMessageDigest(si.SignedAttributes, original, spec.hash) != nil ||
			verifyCMSignature(cert, si, spec.hash) != nil {
			return ErrFirmaNoCorrespondeOriginal
		}
	}
	return nil
}

// CotejarXAdESSeparada verifica las referencias y el valor de cada firma XML
// tomando el original para las referencias externas. Si la firma lleva el
// documento dentro de CONTENT (modo compatible con AutoFirma Java), exige que
// alguna referencia lo cubra y que su contenido sea el original.
func CotejarXAdESSeparada(firma, original []byte) error {
	if len(original) == 0 {
		return ErrFirmaNoCorrespondeOriginal
	}
	externa := false
	resolver := func(string) ([]byte, error) {
		externa = true
		return original, nil
	}
	if _, err := verifyXMLSignatureDocument(firma, false, resolver, domain.CertificateChain{}, string(domain.FormatXAdES), "", nil); err != nil {
		return ErrFirmaNoCorrespondeOriginal
	}
	if externa {
		return nil
	}
	if contenidoInternoCoincide(firma, original) {
		return nil
	}
	return ErrFirmaNoCorrespondeOriginal
}

// contenidoInternoCoincide busca un CONTENT con Id referenciado desde un
// ds:Reference y compara su contenido con el original.
func contenidoInternoCoincide(firma, original []byte) bool {
	type contenido struct {
		id, encoding string
		inicio       int64
		fin          int64
	}
	referencias := map[string]bool{}
	var contenidos []contenido
	d := xml.NewDecoder(bytes.NewReader(firma))
	var pila []*contenido
	for {
		inicio := d.InputOffset()
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "Reference" {
				for _, a := range t.Attr {
					if a.Name.Local == "URI" && strings.HasPrefix(a.Value, "#") {
						referencias[strings.TrimPrefix(a.Value, "#")] = true
					}
				}
			}
			if t.Name.Local == "CONTENT" && t.Name.Space == "" {
				c := &contenido{inicio: d.InputOffset()}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "Id":
						c.id = a.Value
					case "Encoding":
						c.encoding = a.Value
					}
				}
				pila = append(pila, c)
			} else {
				pila = append(pila, nil)
			}
		case xml.EndElement:
			if len(pila) == 0 {
				return false
			}
			if c := pila[len(pila)-1]; c != nil {
				c.fin = inicio
				contenidos = append(contenidos, *c)
			}
			pila = pila[:len(pila)-1]
		}
	}
	for _, c := range contenidos {
		if c.id == "" || !referencias[c.id] || c.fin < c.inicio {
			continue
		}
		dentro := firma[c.inicio:c.fin]
		if strings.EqualFold(c.encoding, "Base64") {
			datos, err := base64.StdEncoding.DecodeString(compactBase64(string(dentro)))
			if err == nil && bytes.Equal(datos, original) {
				return true
			}
			continue
		}
		if strings.TrimSpace(string(dentro)) == stripXMLDeclaration(original) {
			return true
		}
		a, errA := canonicalizeXML(bytes.TrimSpace(dentro), algC14N)
		b, errB := canonicalizeXML([]byte(stripXMLDeclaration(original)), algC14N)
		if errA == nil && errB == nil && bytes.Equal(a, b) {
			return true
		}
	}
	return false
}

// ComprobarIntegridadPAdES verifica sin red que cada firma del PDF cubre sus
// bytes. No evalúa confianza ni revocación.
func ComprobarIntegridadPAdES(ctx context.Context, pdf []byte) error {
	ctx = context.WithValue(ctx, offlineVerificationContextKey{}, true)
	verifier := NewPAdESVerifierWithCAdES(NewCAdESVerifierWithChecker(NewRevocationCheckerOffline()))
	doc, err := domain.NewDocument("documento.pdf", pdf, "application/pdf")
	if err != nil {
		return ErrFirmaNoCorrespondeOriginal
	}
	result, _, err := verifier.Verify(ctx, doc, domain.CertificateChain{})
	if err != nil || result.Integrity.Status == domain.VerificationStatusInvalid {
		return ErrFirmaNoCorrespondeOriginal
	}
	return nil
}
