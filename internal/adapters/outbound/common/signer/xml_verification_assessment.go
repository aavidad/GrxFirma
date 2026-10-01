// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"grxfirma/internal/domain"
)

const xmlCompatibilityEvidence = "xml.canonicalization.compatibility"

// El contexto es el documento original, no un fragmento reserializado. El
// ordinal identifica el SignedInfo de esta firma incluso con varias firmas
// que heredan namespaces/xml:* diferentes o no tienen atributo Id.
type xmlSignatureContext struct {
	document          []byte
	signedInfoOrdinal int
	signatureStart    int64
	signatureEnd      int64
	namespaces        map[string]string
}

type contextualXMLSignature struct {
	fragment []byte
	context  xmlSignatureContext
	start    int64
}

func extractContextualXMLSignatures(document []byte) ([]contextualXMLSignature, error) {
	type frame struct {
		name       xml.Name
		start      int64
		ordinal    int
		namespaces map[string]string
	}
	decoder := xml.NewDecoder(bytes.NewReader(document))
	stack := []frame{}
	ordinal := 0
	var signatures []contextualXMLSignature
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			namespaces := map[string]string{}
			if len(stack) > 0 {
				namespaces = cloneStringMap(stack[len(stack)-1].namespaces)
			}
			for _, attribute := range node.Attr {
				if attribute.Name.Space == "xmlns" {
					namespaces[attribute.Name.Local] = attribute.Value
				} else if attribute.Name.Space == "" && attribute.Name.Local == "xmlns" {
					namespaces[""] = attribute.Value
				}
			}
			if node.Name.Space == nsXMLDSig && node.Name.Local == "SignedInfo" {
				if len(stack) > 0 && stack[len(stack)-1].name == (xml.Name{Space: nsXMLDSig, Local: "Signature"}) {
					if stack[len(stack)-1].ordinal >= 0 {
						return nil, errors.New("firma XML con múltiples SignedInfo")
					}
					stack[len(stack)-1].ordinal = ordinal
				}
				ordinal++
			}
			stack = append(stack, frame{name: node.Name, start: offset, ordinal: -1, namespaces: namespaces})
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("cierre XML sin apertura")
			}
			entry := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if entry.name == (xml.Name{Space: nsXMLDSig, Local: "Signature"}) {
				if entry.ordinal < 0 {
					return nil, errors.New("firma XML sin SignedInfo directo")
				}
				end := decoder.InputOffset()
				signatures = append(signatures, contextualXMLSignature{
					fragment: append([]byte(nil), document[entry.start:end]...),
					context:  xmlSignatureContext{document: document, signedInfoOrdinal: entry.ordinal, signatureStart: entry.start, signatureEnd: end, namespaces: entry.namespaces},
					start:    entry.start,
				})
			}
		}
	}
	sort.Slice(signatures, func(i, j int) bool { return signatures[i].start < signatures[j].start })
	return signatures, nil
}

// El envoltorio sólo sirve para interpretar los nombres de los campos de un
// fragmento. Nunca se usa para acreditar digests/canonicalización: éstos se
// calculan sobre el documento original y el ordinal/rango de su firma.
func contextualSignatureFragment(fragment []byte, context xmlSignatureContext) []byte {
	var wrapper strings.Builder
	wrapper.WriteString("<verification-context")
	prefixes := make([]string, 0, len(context.namespaces))
	for prefix := range context.namespaces {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		wrapper.WriteString(" xmlns")
		if prefix != "" {
			wrapper.WriteString(":" + prefix)
		}
		wrapper.WriteString(`="` + escapeXMLAttr(context.namespaces[prefix]) + `"`)
	}
	wrapper.WriteString(">")
	wrapper.Write(fragment)
	wrapper.WriteString("</verification-context>")
	return []byte(wrapper.String())
}

// Vincular los campos interpretados a los hijos directos de la misma
// Signature que se canonicaliza. Buscar un literal homónimo por separado
// permite autenticar un SignedInfo y evaluar las referencias de otro.
func extractContextualSignatureCore(fragment []byte, context xmlSignatureContext) ([]byte, string, []*x509.Certificate, error) {
	wrapped := contextualSignatureFragment(fragment, context)
	decoder := xml.NewDecoder(bytes.NewReader(wrapped))
	type fieldFrame struct {
		name  xml.Name
		start int64
	}
	var stack []fieldFrame
	fields := map[string][]byte{}
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", nil, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			stack = append(stack, fieldFrame{name: node.Name, start: offset})
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, "", nil, errors.New("cierre XML sin apertura")
			}
			field := stack[len(stack)-1]
			if len(stack) == 3 && stack[1].name == (xml.Name{Space: nsXMLDSig, Local: "Signature"}) && field.name.Space == nsXMLDSig {
				switch field.name.Local {
				case "SignedInfo", "SignatureValue", "KeyInfo":
					if _, exists := fields[field.name.Local]; exists {
						return nil, "", nil, fmt.Errorf("firma XML con múltiples %s directos", field.name.Local)
					}
					fields[field.name.Local] = append([]byte(nil), wrapped[field.start:decoder.InputOffset()]...)
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
	for _, field := range []string{"SignedInfo", "SignatureValue", "KeyInfo"} {
		if len(fields[field]) == 0 {
			return nil, "", nil, fmt.Errorf("%s directo ausente en la firma XML seleccionada", field)
		}
	}
	var value struct {
		Text     string     `xml:",chardata"`
		Children []struct{} `xml:",any"`
	}
	if err := xml.Unmarshal(fields["SignatureValue"], &value); err != nil {
		return nil, "", nil, err
	}
	if len(value.Children) != 0 {
		return nil, "", nil, errors.New("SignatureValue contiene elementos no soportados")
	}
	certificates, err := extractXMLCertificates(contextualSignatureFragment(fields["KeyInfo"], context))
	if err != nil {
		return nil, "", nil, err
	}
	return fields["SignedInfo"], value.Text, certificates, nil
}

func canonicalizeDeclaredSignedInfo(context xmlSignatureContext, signedInfoXML []byte) ([]byte, error) {
	var info signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &info); err != nil {
		return nil, err
	}
	algorithm := strings.TrimSpace(info.CanonicalizationMethod.Algorithm)
	if algorithm != algC14N && algorithm != algExcC14N {
		return nil, errors.New("canonicalización de SignedInfo no declarada o no soportada")
	}
	ordinal := 0
	return canonicalizeXMLSelection(context.document, algorithm, func(element c14nElement) bool {
		if element.namespaceURI != nsXMLDSig || element.local != "SignedInfo" {
			return false
		}
		selected := ordinal == context.signedInfoOrdinal
		ordinal++
		return selected
	})
}

func verifiesDeclaredSignedInfo(context xmlSignatureContext, signedInfoXML []byte, value string, certificate *x509.Certificate) bool {
	var info signedInfoForVerify
	if xml.Unmarshal(signedInfoXML, &info) != nil {
		return false
	}
	method := strings.TrimSpace(info.SignatureMethod.Algorithm)
	switch method {
	case algRSASHA1, algRSASHA256, algRSASHA384, algRSASHA512:
	default:
		return false
	}
	public, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return false
	}
	canonical, err := canonicalizeDeclaredSignedInfo(context, signedInfoXML)
	if err != nil {
		return false
	}
	hash, digest, err := digestForSignatureMethod(method, canonical)
	if err != nil {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(compactBase64(value))
	return err == nil && rsa.VerifyPKCS1v15(public, hash, digest, signature) == nil
}

func identifyXMLSignersWithContext(fragment []byte, context xmlSignatureContext, signedInfo []byte, value string, certificates []*x509.Certificate, requireSigningCertificate bool) ([]*x509.Certificate, []string, error) {
	var info signedInfoForVerify
	if err := xml.Unmarshal(signedInfo, &info); err != nil {
		return nil, nil, err
	}
	// Los parámetros no implementados no son otra receta histórica: no se
	// pueden ignorar y presentar como validado un procedimiento diferente.
	if strings.TrimSpace(info.CanonicalizationMethod.Parameters) != "" || strings.TrimSpace(info.SignatureMethod.Parameters) != "" {
		return nil, nil, errors.New("parámetros de canonicalización o firma XML no soportados")
	}
	var signers []*x509.Certificate
	declaredMatch := false
	var lastError error
	for _, certificate := range certificates {
		declared := verifiesDeclaredSignedInfo(context, signedInfo, value, certificate)
		if !declared {
			if err := verifyXAdESSignatureValue(fragment, signedInfo, value, certificate); err != nil {
				lastError = err
				continue
			}
		}
		if requireSigningCertificate {
			if err := verifySigningCertificate(contextualSignatureFragment(fragment, context), signedInfo, certificate); err != nil {
				lastError = err
				continue
			}
		}
		declaredMatch = declaredMatch || declared
		if !containsCertificate(signers, certificate) {
			signers = append(signers, certificate)
		}
	}
	if len(signers) == 0 {
		if lastError == nil {
			lastError = errors.New("no hay certificados X.509 utilizables")
		}
		return nil, nil, fmt.Errorf("ningún certificado embebido verifica la firma XML: %w", lastError)
	}
	if !declaredMatch {
		return signers, []string{"SignedInfo sólo coincide con una canonicalización histórica no declarada/contextual."}, nil
	}
	return signers, nil, nil
}

func declaredReferenceDigest(context xmlSignatureContext, ref referenceForVerify, target []byte) ([]byte, error) {
	document := context.document
	// Las referencias externas son octetos. No canonicalizar automáticamente
	// un payload XML si la referencia no anuncia esa transformación.
	external := strings.TrimSpace(ref.URI) != "" && !strings.HasPrefix(strings.TrimSpace(ref.URI), "#")
	if external && len(ref.Transforms) == 0 {
		return digestXML(ref.DigestMethod.Algorithm, target)
	}
	// La exclusión XPath de firmas de Java va tras la canonicalización; se
	// aplica aparte y no cuenta para el orden de la cadena.
	excluirFirmas := false
	sinXPath := make([]algorithmAttr, 0, len(ref.Transforms))
	for _, transform := range ref.Transforms {
		if strings.TrimSpace(transform.Algorithm) == algXPathFilter {
			if !esXPathExcluirFirmas(transform) {
				return nil, errors.New("transformación XPath no soportada")
			}
			excluirFirmas = true
			continue
		}
		sinXPath = append(sinXPath, transform)
	}
	ref.Transforms = sinXPath
	for index, transform := range ref.Transforms {
		algorithm := strings.TrimSpace(transform.Algorithm)
		// Con la exclusión de firmas el resultado es la canonicalización del
		// documento sin firmas sea cual sea el orden (Java usa C14N,
		// enveloped, XPath), así que el orden no se exige.
		if (algorithm == algExcC14N || algorithm == algC14N) && index != len(ref.Transforms)-1 && !excluirFirmas {
			return nil, errors.New("cadena de transformaciones no acreditada por el procedimiento contextual")
		}
	}
	if external {
		for _, transform := range ref.Transforms {
			if strings.TrimSpace(transform.Algorithm) == algEnveloped {
				return nil, errors.New("transformación enveloped fuera del documento de la firma")
			}
		}
		canonical, err := canonicalizeReference(ref, target)
		if err != nil {
			return nil, err
		}
		return digestXML(ref.DigestMethod.Algorithm, canonical)
	}
	if strings.HasPrefix(strings.TrimSpace(ref.URI), "#") {
		id := strings.TrimPrefix(strings.TrimSpace(ref.URI), "#")
		decoder := xml.NewDecoder(bytes.NewReader(document))
		matches := 0
		for {
			token, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if element, ok := token.(xml.StartElement); ok {
				for _, attribute := range element.Attr {
					if strings.EqualFold(attribute.Name.Local, "Id") && attribute.Value == id {
						matches++
					}
				}
			}
		}
		if matches != 1 {
			return nil, errors.New("referencia interna sin identidad única en el documento original")
		}
	}
	// Enveloped excluye ESTA firma, no la primera cadena con apariencia de
	// Signature. Los offsets proceden del parser del documento original y
	// preservan todas las demás firmas, independientemente de su prefijo/Id.
	filtered := ref
	filtered.Transforms = nil
	hasEnveloped := false
	for _, transform := range ref.Transforms {
		if strings.TrimSpace(transform.Algorithm) == algEnveloped {
			hasEnveloped = true
		} else {
			filtered.Transforms = append(filtered.Transforms, transform)
		}
	}
	if hasEnveloped {
		start, end := context.signatureStart, context.signatureEnd
		if start < 0 || end <= start || end > int64(len(document)) {
			return nil, errors.New("firma propietaria de enveloped no identificada")
		}
		working := make([]byte, 0, int64(len(document))-(end-start))
		working = append(working, document[:start]...)
		working = append(working, document[end:]...)
		document = working
	}
	if excluirFirmas {
		sinFirmas, err := quitarTodasLasFirmas(document)
		if err != nil {
			return nil, err
		}
		return digestExcluyendoFirmas(sinFirmas, filtered)
	}
	return computeContextualReferenceDigest(document, filtered)
}

// digestExcluyendoFirmas aplica la semántica XMLDSig de la cadena de Java
// [enveloped, C14N, XPath]: el conjunto de nodos resultante se convierte a
// octetos con C14N inclusiva.
func digestExcluyendoFirmas(document []byte, ref referenceForVerify) ([]byte, error) {
	algorithm := algC14N
	for _, transform := range ref.Transforms {
		if a := strings.TrimSpace(transform.Algorithm); a == algExcC14N || a == algC14N {
			algorithm = a
		}
	}
	uri := strings.TrimSpace(ref.URI)
	var (
		canonical []byte
		err       error
	)
	switch {
	case uri == "":
		canonical, err = canonicalizeXML(document, algorithm)
	case strings.HasPrefix(uri, "#"):
		canonical, err = canonicalizeElementByIDInDocument(document, strings.TrimPrefix(uri, "#"), algorithm)
	default:
		return nil, errors.New("exclusión de firmas sobre una referencia externa")
	}
	if err != nil {
		return nil, err
	}
	if algorithm != algC14N {
		if canonical, err = canonicalizeXML(canonical, algC14N); err != nil {
			return nil, err
		}
	}
	return digestXML(ref.DigestMethod.Algorithm, canonical)
}

func assessXMLReferences(fragment []byte, context xmlSignatureContext, signedInfoXML []byte, resolver func(string) ([]byte, error)) ([]string, error) {
	document := context.document
	var info signedInfoForVerify
	if err := xml.Unmarshal(signedInfoXML, &info); err != nil {
		return nil, err
	}
	if len(info.References) == 0 {
		return nil, errors.New("SignedInfo no contiene referencias")
	}
	var compatibility []string
	for index, ref := range info.References {
		if strings.TrimSpace(ref.DigestMethod.Parameters) != "" {
			return nil, errors.New("parámetros de digest XML no soportados")
		}
		for _, transform := range ref.Transforms {
			if strings.TrimSpace(transform.Parameters) != "" && !esXPathExcluirFirmas(transform) {
				return nil, errors.New("parámetros de transformación XML no soportados")
			}
			switch strings.TrimSpace(transform.Algorithm) {
			case algEnveloped, algExcC14N, algC14N, algBase64Transform, algXPathFilter:
			default:
				return nil, fmt.Errorf("transform no soportado: %s", transform.Algorithm)
			}
		}
		// La resolución histórica permanece acotada a la firma seleccionada;
		// referencias a elementos hermanos se resuelven en el documento real.
		resolutionDocument := fragment
		if strings.TrimSpace(ref.URI) == "" {
			resolutionDocument = document
		}
		target, err := resolveReferenceTarget(resolutionDocument, ref.URI, resolver)
		if err != nil && !bytes.Equal(fragment, document) {
			target, err = resolveReferenceTarget(document, ref.URI, resolver)
		}
		if err != nil {
			return nil, fmt.Errorf("referencia %d no resuelta: %w", index+1, err)
		}
		expected, err := base64.StdEncoding.DecodeString(compactBase64(ref.DigestValue))
		if err != nil {
			return nil, err
		}
		declared, declaredErr := declaredReferenceDigest(context, ref, target)
		if declaredErr == nil && equalBytes(declared, expected) {
			continue
		}
		legacy, err := computeReferenceDigests(ref, target)
		if err != nil {
			return nil, fmt.Errorf("digest referencia %d: %w", index+1, err)
		}
		if !containsDigest(legacy, expected) {
			return nil, fmt.Errorf("digest de referencia no coincide para referencia %d", index+1)
		}
		// No incluir URI ni contenido controlado por el documento en la marca.
		compatibility = append(compatibility, fmt.Sprintf("Referencia %d sólo coincide mediante canonicalización histórica; el procedimiento declarado/contextual no queda acreditado.", index+1))
	}
	return compatibility, nil
}

func markXMLCompatibility(result domain.VerificationResult, compatibility []string) domain.VerificationResult {
	if len(compatibility) == 0 {
		return result
	}
	const reason = "Compatibilidad histórica; validación XML estándar no acreditada."
	result.Valid = false
	result.Coverage = "unknown"
	result.Reason = reason
	result.Integrity.Status = domain.VerificationStatusWarning
	result.Integrity.Reason = reason
	withoutSuccessClaims := func(details []string) []string {
		var filtered []string
		for _, detail := range details {
			if detail != "referencias=ok" && detail != "signedproperties=ok" && detail != "referencias_zip=ok" {
				filtered = append(filtered, detail)
			}
		}
		return filtered
	}
	result.Details = withoutSuccessClaims(result.Details)
	result.Integrity.Details = append(withoutSuccessClaims(result.Integrity.Details), compatibility...)
	result.Warnings = appendUniqueString(result.Warnings, reason)
	for _, detail := range compatibility {
		result.Evidence = append(result.Evidence, domain.VerificationEvidence{Type: xmlCompatibilityEvidence, Summary: detail})
	}
	return result.Normalize()
}

func xmlCompatibilityDetails(result domain.VerificationResult) []string {
	var details []string
	for _, evidence := range result.Evidence {
		if evidence.Type == xmlCompatibilityEvidence {
			details = append(details, evidence.Summary)
		}
	}
	return details
}
