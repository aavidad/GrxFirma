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
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/asiccontainer"
	"grxfirma/internal/domain"
)

// Las claves de estas pruebas sólo existen en memoria. Los helpers firman los
// octetos seleccionados explícitamente, no utilizan el evaluador bajo prueba.
func assessmentSignature(t *testing.T, key crypto.Signer, cert *x509.Certificate, prefix, suffix, id string, payload []byte, legacyReference, legacySignedInfo bool) []byte {
	t.Helper()
	keyInfo := fmt.Sprintf(`<ds:KeyInfo xmlns:ds="%s" Id="key-%s"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`, nsXMLDSig, id, base64.StdEncoding.EncodeToString(cert.Raw))
	canonical, err := exclusiveC14N(keyInfo)
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := sha256.Sum256(canonical)
	dataDigest := sha256.Sum256(payload)
	transform := `<ds:Transforms><ds:Transform Algorithm="` + algExcC14N + `"/></ds:Transforms>`
	if legacyReference {
		transform = ""
	}
	info := fmt.Sprintf(`<ds:SignedInfo xmlns:ds="%s"><ds:CanonicalizationMethod Algorithm="%s"/><ds:SignatureMethod Algorithm="%s"/><ds:Reference URI="payload.xml"><ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></ds:Reference><ds:Reference URI="#key-%s">%s<ds:DigestMethod Algorithm="%s"/><ds:DigestValue>%s</ds:DigestValue></ds:Reference></ds:SignedInfo>`, nsXMLDSig, algC14N, algRSASHA256, algSHA256, base64.StdEncoding.EncodeToString(dataDigest[:]), id, transform, algSHA256, base64.StdEncoding.EncodeToString(keyDigest[:]))
	document := []byte(prefix + `<ds:Signature xmlns:ds="` + nsXMLDSig + `">` + info + `<ds:SignatureValue>PLACEHOLDER</ds:SignatureValue>` + keyInfo + `</ds:Signature>` + suffix)
	canonical, err = canonicalizeElementInDocument(document, nsXMLDSig, "SignedInfo", algC14N)
	if err != nil {
		t.Fatal(err)
	}
	if legacySignedInfo {
		canonical, err = exclusiveC14N(info)
		if err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256(canonical)
	signature, err := key.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Replace(document, []byte("PLACEHOLDER"), []byte(base64.StdEncoding.EncodeToString(signature)), 1)
}

func assessDocument(t *testing.T, document, payload []byte) (domain.VerificationResult, error) {
	t.Helper()
	verification, err := verifyXMLSignatureDocument(document, false, func(uri string) ([]byte, error) {
		if uri != "payload.xml" {
			return nil, fmt.Errorf("URI inesperada")
		}
		return payload, nil
	}, domain.CertificateChain{}, "XMLDSig", "firma válida", []string{"referencias=ok"})
	return verification.result, err
}

func requireCompatibility(t *testing.T, result domain.VerificationResult) {
	t.Helper()
	if result.Valid || result.Integrity.Status != domain.VerificationStatusWarning || len(xmlCompatibilityDetails(result)) == 0 {
		t.Fatalf("compatibilidad no propagada: %+v", result)
	}
	if result.Coverage == "full" || strings.Contains(strings.Join(result.Details, ";"), "referencias=ok") {
		t.Fatalf("éxito contradictorio en compatibilidad: %+v", result)
	}
	if result.Certificate.Status != domain.VerificationStatusValid &&
		!(result.Certificate.Status == domain.VerificationStatusWarning && strings.Contains(result.Certificate.Reason, "revocación")) {
		t.Fatalf("se perdieron datos de certificado: %+v", result.Certificate)
	}
}

func TestXMLAssessment_DeclaredRawAndHistoricalCompatibility(t *testing.T) {
	key, cert := certForTest(t, "XML assessment synthetic")
	// XML sin canonicalizar: el raw normativo difiere de C14N (autocierre y
	// namespace no usado). No debe degradarse por ser el último candidato viejo.
	payload := []byte(`<payload xmlns:unused="urn:unused"><item/></payload>`)
	for _, tc := range []struct {
		name                              string
		legacyReference, legacySignedInfo bool
	}{
		{name: "declared_raw_external"}, {name: "legacy_reference", legacyReference: true}, {name: "legacy_signedinfo", legacySignedInfo: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := assessmentSignature(t, key, cert, `<root xmlns="urn:parent" xmlns:unused="urn:ancestor" xml:lang="es">`, `</root>`, "one", payload, tc.legacyReference, tc.legacySignedInfo)
			result, err := assessDocument(t, document, payload)
			if err != nil {
				t.Fatal(err)
			}
			if tc.legacyReference || tc.legacySignedInfo {
				requireCompatibility(t, result)
			} else if !result.Valid || result.Integrity.Status != domain.VerificationStatusValid || len(xmlCompatibilityDetails(result)) != 0 {
				t.Fatalf("firma declarada degradada: %+v", result)
			}
			if _, err := assessDocument(t, document, []byte(`<payload>manipulado</payload>`)); err == nil {
				t.Fatal("payload manipulado aceptado")
			}
		})
	}
}

func TestXMLAssessment_HistoricalQAFixtureUsesLegacyDocumentDigest(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "test", "regression", "fixtures", "v1", "samples", "2_xml_signed.xsig")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	infoXML, err := extractXMLLiteralElement(document, "ds:SignedInfo")
	if err != nil {
		t.Fatal(err)
	}
	var info signedInfoForVerify
	if err := xml.Unmarshal(infoXML, &info); err != nil {
		t.Fatal(err)
	}
	ref := info.References[0]
	if len(ref.Transforms) != 1 || ref.Transforms[0].Algorithm != algExcC14N {
		t.Fatalf("cambió receta de muestra: %+v", ref)
	}
	payload, err := resolveReferenceTarget(document, ref.URI, nil)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := exclusiveC14N(string(payload))
	if err != nil {
		t.Fatal(err)
	}
	declared := sha256.Sum256(canonical)
	legacy := sha256.Sum256(legacyExclusiveC14N(string(payload)))
	expected, err := base64.StdEncoding.DecodeString(ref.DigestValue)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(declared[:], expected) || !bytes.Equal(legacy[:], expected) {
		t.Fatal("la muestra ya no demuestra diferencia entre receta declarada y canonicalizador histórico")
	}
}

func TestXMLAssessment_IdenticalDeclaredAndLegacyBytesDoNotDegrade(t *testing.T) {
	payload := []byte(`<payload>simple</payload>`)
	digest := sha256.Sum256(payload)
	info := []byte(`<ds:SignedInfo xmlns:ds="` + nsXMLDSig + `"><ds:Reference URI="payload.xml"><ds:DigestMethod Algorithm="` + algSHA256 + `"/><ds:DigestValue>` + base64.StdEncoding.EncodeToString(digest[:]) + `</ds:DigestValue></ds:Reference></ds:SignedInfo>`)
	compatibility, err := assessXMLReferences([]byte(`<root/>`), xmlSignatureContext{document: []byte(`<root/>`)}, info, func(string) ([]byte, error) { return payload, nil })
	if err != nil || len(compatibility) != 0 {
		t.Fatalf("candidatos idénticos degradados: %v %v", compatibility, err)
	}
}

func TestXMLAssessment_DoesNotOmitIncompleteAdditionalSignature(t *testing.T) {
	key, cert := certForTest(t, "XML incomplete signature synthetic")
	payload := []byte("synthetic")
	document := assessmentSignature(t, key, cert, `<root>`, `</root>`, "one", payload, false, false)
	if result, err := assessDocument(t, document, payload); err != nil || !result.Valid {
		t.Fatalf("precondición: %+v %v", result, err)
	}
	for _, incomplete := range []string{
		`<ds:Signature xmlns:ds="` + nsXMLDSig + `"/>`,
		`<ds:Signature xmlns:ds="` + nsXMLDSig + `"><ds:SignatureValue>AAAA</ds:SignatureValue></ds:Signature>`,
	} {
		changed := bytes.Replace(document, []byte(`</root>`), []byte(incomplete+`</root>`), 1)
		if result, err := assessDocument(t, changed, payload); err == nil {
			t.Fatalf("firma adicional incompleta omitida: %+v", result)
		}
	}
}

func TestXMLAssessment_ASiCKeepsHistoricalDiagnosisAndRejectsTampering(t *testing.T) {
	key, cert := certForTest(t, "ASiC compatibility synthetic")
	payload := []byte(`<payload xmlns:unused="urn:unused"><item/></payload>`)
	signed, err := buildASiCXAdESSignatureXML(&LocalSigningKey{Signer: key, Certificate: cert}, "payload.xml", payload, "application/xml", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		document := []byte(signed)
		if legacy {
			// Recrear únicamente las referencias internas de la versión histórica;
			// mantener payload y calcular de nuevo SignatureValue con clave RAM.
			info, err := extractXMLLiteralElement(document, "ds:SignedInfo")
			if err != nil {
				t.Fatal(err)
			}
			var parsed signedInfoForVerify
			if err := xml.Unmarshal(info, &parsed); err != nil {
				t.Fatal(err)
			}
			for _, ref := range parsed.References[1:] {
				needle := `URI="` + ref.URI + `"`
				start := bytes.Index(info, []byte(needle))
				transform := []byte(`<ds:Transforms><ds:Transform Algorithm="` + algExcC14N + `"/></ds:Transforms>`)
				tail := bytes.Replace(info[start:], transform, nil, 1)
				info = append(append([]byte(nil), info[:start]...), tail...)
			}
			original, _ := extractXMLLiteralElement(document, "ds:SignedInfo")
			document = bytes.Replace(document, original, info, 1)
			document = resignAssessment(t, key, document, 0, algExcC14N)
		}
		container, err := asiccontainer.CreateXAdESContainer(document, payload, "payload.xml")
		if err != nil {
			t.Fatal(err)
		}
		result, signers, err := NewASiCXAdESVerifier().Verify(context.Background(), domain.Document{Content: container}, domain.CertificateChain{})
		if err != nil || len(signers) != 1 {
			t.Fatalf("legacy=%v signers=%d err=%v", legacy, len(signers), err)
		}
		if legacy {
			requireCompatibility(t, result)
		} else if !result.Valid || len(xmlCompatibilityDetails(result)) != 0 {
			t.Fatalf("ASiC nueva degradada: %+v", result)
		}
		container, err = asiccontainer.CreateXAdESContainer(document, []byte(`<payload>manipulado</payload>`), "payload.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := NewASiCXAdESVerifier().Verify(context.Background(), domain.Document{Content: container}, domain.CertificateChain{}); err == nil {
			t.Fatal("ASiC manipulado aceptado")
		}
	}
}

func TestXMLAssessment_OOXMLSeparateSignatureEntriesAggregateCompatibility(t *testing.T) {
	key, cert := certForTest(t, "OOXML separate synthetic")
	payload := []byte("synthetic")
	entries := map[string][]byte{
		"payload.xml":             payload,
		"[Content_Types].xml":     []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`),
		"_xmlsignatures/sig1.xml": assessmentSignature(t, key, cert, `<root xmlns:unused="urn:one" xml:lang="es">`, `</root>`, "first", payload, false, false),
		"_xmlsignatures/sig2.xml": assessmentSignature(t, key, cert, `<root xmlns:unused="urn:two" xml:lang="fr">`, `</root>`, "second", payload, true, false),
	}
	result, signers, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{Content: makeZipBytes(t, entries)}, domain.CertificateChain{})
	if err != nil || len(signers) != 2 {
		t.Fatalf("firmas=%d err=%v", len(signers), err)
	}
	requireCompatibility(t, result)
	entries["_xmlsignatures/sig2.xml"] = []byte(`<broken/>`)
	if _, _, err := NewOOXMLVerifier().Verify(context.Background(), domain.Document{Content: makeZipBytes(t, entries)}, domain.CertificateChain{}); err == nil {
		t.Fatal("segunda entrada sin firma ignorada")
	}
}

func TestXMLAssessment_AuthenticatedFieldsIgnoreNestedAndWrongNamespaceLookalikes(t *testing.T) {
	key, cert := certForTest(t, "XML structural fields synthetic")
	payload := []byte("original")
	changed := []byte("manipulado")
	document := assessmentSignature(t, key, cert, `<root>`, `</root>`, "one", payload, false, false)
	info, err := extractXMLLiteralElement(document, "ds:SignedInfo")
	if err != nil {
		t.Fatal(err)
	}
	var parsed signedInfoForVerify
	if err := xml.Unmarshal(info, &parsed); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(changed)
	fakeInfo := bytes.Replace(info, []byte(parsed.References[0].DigestValue), []byte(base64.StdEncoding.EncodeToString(digest[:])), 1)
	nested := append([]byte(`<ds:Object xmlns:ds="`+nsXMLDSig+`">`), fakeInfo...)
	nested = append(nested, []byte(`</ds:Object>`)...)
	attack := bytes.Replace(document, info, append(nested, info...), 1)
	if _, err := assessDocument(t, attack, changed); err == nil {
		t.Fatal("SignedInfo anidado sustituyó referencias directas")
	}
	// Un segundo SignedInfo directo del namespace correcto es ambiguo y se
	// rechaza, aunque sus bytes sean idénticos a los autenticados.
	attack = bytes.Replace(document, info, append(append([]byte(nil), info...), info...), 1)
	if _, err := assessDocument(t, attack, payload); err == nil {
		t.Fatal("SignedInfo directo duplicado aceptado")
	}

	parts, err := extractContextualXMLSignatures(document)
	if err != nil {
		t.Fatal(err)
	}
	values, err := extractRawElementsByName(contextualSignatureFragment(parts[0].fragment, parts[0].context), nsXMLDSig, "SignatureValue")
	if err != nil || len(values) != 1 {
		t.Fatalf("values %d: %v", len(values), err)
	}
	value := values[0]
	badValue := []byte(`<ds:SignatureValue>` + base64.StdEncoding.EncodeToString(make([]byte, 256)) + `</ds:SignatureValue>`)
	for _, location := range []string{"wrong_namespace", "nested", "duplicate"} {
		t.Run("SignatureValue_"+location, func(t *testing.T) {
			var fake []byte
			switch location {
			case "wrong_namespace":
				fake = bytes.Replace(value, []byte(`<ds:SignatureValue>`), []byte(`<ds:SignatureValue xmlns:ds="urn:not-xmldsig">`), 1)
			case "nested":
				fake = append([]byte(`<ds:Object>`), value...)
				fake = append(fake, []byte(`</ds:Object>`)...)
			case "duplicate":
				fake = append([]byte(nil), value...)
			}
			attack := bytes.Replace(document, value, append(fake, badValue...), 1)
			if _, err := assessDocument(t, attack, payload); err == nil {
				t.Fatal("SignatureValue homónimo aceptado en lugar del directo")
			}
		})
	}
	keyInfos, err := extractRawElementsByName(contextualSignatureFragment(parts[0].fragment, parts[0].context), nsXMLDSig, "KeyInfo")
	if err != nil || len(keyInfos) != 1 {
		t.Fatalf("keyinfos %d: %v", len(keyInfos), err)
	}
	keyInfo := keyInfos[0]
	for _, location := range []string{"wrong_namespace", "nested", "duplicate"} {
		t.Run("KeyInfo_"+location, func(t *testing.T) {
			var fake []byte
			switch location {
			case "wrong_namespace":
				fake = bytes.Replace(keyInfo, []byte(nsXMLDSig), []byte("urn:not-xmldsig"), 1)
			case "nested":
				fake = append([]byte(`<ds:Object>`), keyInfo...)
				fake = append(fake, []byte(`</ds:Object>`)...)
			case "duplicate":
				fake = append([]byte(nil), keyInfo...)
			}
			badKeyInfo := bytes.Replace(keyInfo, []byte(base64.StdEncoding.EncodeToString(cert.Raw)), []byte("AAAA"), 1)
			attack := bytes.Replace(document, keyInfo, append(fake, badKeyInfo...), 1)
			if _, err := assessDocument(t, attack, payload); err == nil {
				t.Fatal("KeyInfo homónimo sustituyó el certificado directo")
			}
		})
	}
}

func TestXMLAssessment_RejectsUnsupportedParametersAndEmptyReferences(t *testing.T) {
	key, cert := certForTest(t, "XML unsupported synthetic")
	payload := []byte("synthetic")
	base := assessmentSignature(t, key, cert, `<root xmlns:unused="urn:unused">`, `</root>`, "one", payload, false, false)
	for _, tc := range []struct{ name, old, replacement string }{
		{"signedinfo_parameters", `<ds:CanonicalizationMethod Algorithm="` + algC14N + `"/>`, `<ds:CanonicalizationMethod Algorithm="` + algExcC14N + `"><ec:InclusiveNamespaces xmlns:ec="` + algExcC14N + `" PrefixList="unused"/></ds:CanonicalizationMethod>`},
		{"reference_parameters", `<ds:Transform Algorithm="` + algExcC14N + `"/>`, `<ds:Transform Algorithm="` + algExcC14N + `"><ec:InclusiveNamespaces xmlns:ec="` + algExcC14N + `" PrefixList="unused"/></ds:Transform>`},
		{"unknown_transform", `<ds:Transform Algorithm="` + algExcC14N + `"/>`, `<ds:Transform Algorithm="` + algExcC14N + `"/><ds:Transform Algorithm="urn:unsupported"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := bytes.Replace(base, []byte(tc.old), []byte(tc.replacement), 1)
			changed = resignAssessment(t, key, changed, 0, algExcC14N)
			if _, err := assessDocument(t, changed, payload); err == nil {
				t.Fatal("parámetros/transform ignorados")
			}
		})
	}
	info, err := extractXMLLiteralElement(base, "ds:SignedInfo")
	if err != nil {
		t.Fatal(err)
	}
	first := bytes.Index(info, []byte("<ds:Reference "))
	last := bytes.LastIndex(info, []byte("</ds:Reference>")) + len("</ds:Reference>")
	empty := append(append([]byte(nil), info[:first]...), info[last:]...)
	changed := bytes.Replace(base, info, empty, 1)
	changed = resignAssessment(t, key, changed, 0, algC14N)
	if _, err := assessDocument(t, changed, payload); err == nil {
		t.Fatal("SignedInfo sin referencias aceptado")
	}
}

func resignAssessment(t *testing.T, key crypto.Signer, document []byte, ordinal int, algorithm string) []byte {
	t.Helper()
	current := 0
	canonical, err := canonicalizeXMLSelection(document, algorithm, func(element c14nElement) bool {
		if element.namespaceURI != nsXMLDSig || element.local != "SignedInfo" {
			return false
		}
		selected := current == ordinal
		current++
		return selected
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	signature, err := key.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := extractContextualXMLSignatures(document)
	if err != nil {
		t.Fatal(err)
	}
	part := parts[ordinal]
	values, err := extractRawElementsByName(contextualSignatureFragment(part.fragment, part.context), nsXMLDSig, "SignatureValue")
	if err != nil || len(values) != 1 {
		t.Fatalf("SignatureValue original: count=%d err=%v", len(values), err)
	}
	old := values[0]
	replacement := []byte(`<ds:SignatureValue>` + base64.StdEncoding.EncodeToString(signature) + `</ds:SignatureValue>`)
	changed := bytes.Replace(part.fragment, old, replacement, 1)
	if bytes.Equal(changed, part.fragment) {
		t.Fatal("la prueba no sustituyó SignatureValue")
	}
	output := append([]byte(nil), document[:part.context.signatureStart]...)
	output = append(output, changed...)
	return append(output, document[part.context.signatureEnd:]...)
}

func TestXMLAssessment_EnvelopedRemovesOwningSignatureOnly(t *testing.T) {
	// Sin Id, dos prefijos, dos SignedInfo. El digest de la segunda firma debe
	// conservar completa la primera. No hace falta material de claves aquí.
	first := `<a:Signature xmlns:a="` + nsXMLDSig + `"><a:SignedInfo/></a:Signature>`
	second := `<b:Signature xmlns:b="` + nsXMLDSig + `"><b:SignedInfo/></b:Signature>`
	document := []byte(`<root xmlns:unused="urn:unused" Id="root">` + first + second + `</root>`)
	parts, err := extractContextualXMLSignatures(document)
	if err != nil || len(parts) != 2 {
		t.Fatalf("parts=%d error=%v", len(parts), err)
	}
	for _, uri := range []string{"", "#root"} {
		ref := referenceForVerify{URI: uri, DigestMethod: algorithmAttr{Algorithm: algSHA256}, Transforms: []algorithmAttr{{Algorithm: algEnveloped}, {Algorithm: algC14N}}}
		got, err := declaredReferenceDigest(parts[1].context, ref, document)
		if err != nil {
			t.Fatal(err)
		}
		expectedXML := bytes.Replace(document, []byte(second), nil, 1)
		canonical, err := canonicalizeXML(expectedXML, algC14N)
		if err != nil {
			t.Fatal(err)
		}
		expected := sha256.Sum256(canonical)
		if !bytes.Equal(got, expected[:]) {
			t.Fatalf("enveloped no seleccionó segunda firma: uri=%q", uri)
		}
		wrongXML := bytes.Replace(document, []byte(first), nil, 1)
		wrongCanonical, err := canonicalizeXML(wrongXML, algC14N)
		if err != nil {
			t.Fatal(err)
		}
		wrong := sha256.Sum256(wrongCanonical)
		if bytes.Equal(got, wrong[:]) {
			t.Fatal("se eliminó la primera firma")
		}
	}
}

func TestXMLAssessment_InheritedAlternativeNamespacePrefix(t *testing.T) {
	key, cert := certForTest(t, "XML namespace synthetic")
	payload := []byte("synthetic")
	document := assessmentSignature(t, key, cert, `<root xmlns:ds="`+nsXMLDSig+`">`, `</root>`, "one", payload, false, false)
	// Mover la declaración al ancestro y usar un prefijo no reservado por
	// nuestros generadores no altera qué elemento XMLDSig se está firmando.
	document = bytes.ReplaceAll(document, []byte(` xmlns:ds="`+nsXMLDSig+`"`), nil)
	document = bytes.Replace(document, []byte(`<root>`), []byte(`<root xmlns:ds="`+nsXMLDSig+`">`), 1)
	document = bytes.ReplaceAll(document, []byte("ds:"), []byte("sig:"))
	document = bytes.ReplaceAll(document, []byte("xmlns:ds="), []byte("xmlns:sig="))
	// El cambio de prefijo sí cambia C14N: firmar su SignedInfo real.
	canonical, err := canonicalizeElementInDocument(document, nsXMLDSig, "SignedInfo", algC14N)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	signature, err := key.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	old, err := extractXMLLiteralElement(document, "sig:SignatureValue")
	if err != nil {
		t.Fatal(err)
	}
	document = bytes.Replace(document, old, []byte(`<sig:SignatureValue>`+base64.StdEncoding.EncodeToString(signature)+`</sig:SignatureValue>`), 1)
	// También se debe recalcular KeyInfo, pues contiene ese prefijo.
	keyCanonical, err := canonicalizeElementByIDInDocument(document, "key-one", algExcC14N)
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := sha256.Sum256(keyCanonical)
	info, err := extractXMLLiteralElement(document, "sig:SignedInfo")
	if err != nil {
		t.Fatal(err)
	}
	var parsed signedInfoForVerify
	if err := xml.Unmarshal(info, &parsed); err != nil {
		t.Fatal(err)
	}
	document = bytes.Replace(document, []byte(parsed.References[1].DigestValue), []byte(base64.StdEncoding.EncodeToString(keyDigest[:])), 1)
	canonical, err = canonicalizeElementInDocument(document, nsXMLDSig, "SignedInfo", algC14N)
	if err != nil {
		t.Fatal(err)
	}
	sum = sha256.Sum256(canonical)
	signature, err = key.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	old, err = extractXMLLiteralElement(document, "sig:SignatureValue")
	if err != nil {
		t.Fatal(err)
	}
	document = bytes.Replace(document, old, []byte(`<sig:SignatureValue>`+base64.StdEncoding.EncodeToString(signature)+`</sig:SignatureValue>`), 1)
	result, err := assessDocument(t, document, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(xmlCompatibilityDetails(result)) != 0 {
		t.Fatalf("contexto de prefijo perdido: %+v", result)
	}
}

func TestXMLAssessment_OfficeMultiSignatureContextAndCompatibility(t *testing.T) {
	key, cert := certForTest(t, "Office multi synthetic")
	payload := []byte(`<payload><item/></payload>`)
	for _, format := range []string{"ODF", "OOXML"} {
		for _, legacyPosition := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s_legacy_%d", format, legacyPosition), func(t *testing.T) {
				var parts [][]byte
				for index, lang := range []string{"es", "fr"} {
					prefix := `<signatures xmlns:unused="urn:outer"><section xml:lang="` + lang + `" xmlns:local="urn:` + lang + `">`
					signed := assessmentSignature(t, key, cert, prefix, `</section></signatures>`, fmt.Sprint(index), payload, index == legacyPosition, false)
					sectionStart := bytes.Index(signed, []byte("<section "))
					sectionEnd := bytes.Index(signed, []byte("</section>")) + len("</section>")
					parts = append(parts, append([]byte(nil), signed[sectionStart:sectionEnd]...))
				}
				signatures := append([]byte(`<signatures xmlns:unused="urn:outer">`), parts[0]...)
				signatures = append(signatures, parts[1]...)
				signatures = append(signatures, []byte(`</signatures>`)...)
				verify := func(sig []byte) (domain.VerificationResult, []domain.CertificateRef, error) {
					entries := map[string][]byte{"payload.xml": payload}
					if format == "ODF" {
						entries["mimetype"] = []byte("application/vnd.oasis.opendocument.text")
						entries["META-INF/manifest.xml"] = []byte(`<manifest/>`)
						entries["META-INF/documentsignatures.xml"] = sig
						return NewODFVerifier().Verify(context.Background(), domain.Document{Content: makeZipBytes(t, entries)}, domain.CertificateChain{})
					}
					entries["[Content_Types].xml"] = []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`)
					entries["_xmlsignatures/origin.sigs"] = nil
					entries["_xmlsignatures/sig1.xml"] = sig
					return NewOOXMLVerifier().Verify(context.Background(), domain.Document{Content: makeZipBytes(t, entries)}, domain.CertificateChain{})
				}
				result, signers, err := verify(signatures)
				if err != nil {
					t.Fatal(err)
				}
				if len(signers) != 2 || !containsOfficeDetail(result.Details, "firmas_xml=2") {
					t.Fatalf("faltan firmas: %d %+v", len(signers), result.Details)
				}
				if legacyPosition >= 0 {
					requireCompatibility(t, result)
				} else if !result.Valid || len(xmlCompatibilityDetails(result)) != 0 {
					t.Fatalf("contexto perdido: %+v", result)
				}
				parsed, err := extractContextualXMLSignatures(signatures)
				if err != nil {
					t.Fatal(err)
				}
				for index, part := range parsed {
					info, err := extractXMLLiteralElement(part.fragment, "ds:SignedInfo")
					if err != nil {
						t.Fatal(err)
					}
					var value signedInfoForVerify
					if err := xml.Unmarshal(info, &value); err != nil {
						t.Fatal(err)
					}
					bad := bytes.Replace(part.fragment, []byte(value.References[0].DigestValue), []byte(base64.StdEncoding.EncodeToString(make([]byte, 32))), 1)
					tampered := append([]byte(nil), signatures[:part.context.signatureStart]...)
					tampered = append(tampered, bad...)
					tampered = append(tampered, signatures[part.context.signatureEnd:]...)
					if _, _, err := verify(tampered); err == nil {
						t.Fatalf("firma %d manipulada omitida", index+1)
					}
				}
			})
		}
	}
}
