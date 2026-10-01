// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// TestXAdESBES_ProduceXMLValido firma un documento y verifica que el resultado
// es XML bien formado con los elementos obligatorios.
func TestXAdESBES_ProduceXMLValido(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>datos</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if result.Format != domain.FormatXAdES {
		t.Fatalf("formato inesperado: %s", result.Format)
	}
	if len(result.Data) == 0 {
		t.Fatal("resultado vacio")
	}

	xmlData := string(result.Data)

	// Verificar que es XML bien formado
	dec := xml.NewDecoder(strings.NewReader(xmlData))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("XML mal formado: %v", err)
		}
	}

	// Verificar elementos obligatorios
	if !strings.Contains(xmlData, "ds:Signature") {
		t.Error("falta elemento ds:Signature")
	}
	if !strings.Contains(xmlData, "xades:QualifyingProperties") {
		t.Error("falta elemento xades:QualifyingProperties")
	}
	if !strings.Contains(xmlData, "xades:SigningCertificate") {
		t.Error("falta elemento xades:SigningCertificate")
	}
	if !strings.Contains(xmlData, "xades:SignedProperties") {
		t.Error("falta elemento xades:SignedProperties")
	}
	if !strings.Contains(xmlData, "xades:SigningTime") {
		t.Error("falta elemento xades:SigningTime")
	}
	if !strings.Contains(xmlData, "<AFIRMA>") {
		t.Error("falta envoltura AFIRMA")
	}
	if !strings.Contains(xmlData, `<CONTENT Name="contrato.xml" MimeType="application/xml">`) {
		t.Error("falta copia embebida del original para verificacion local")
	}
	if !strings.Contains(xmlData, `URI="contrato.xml"`) {
		t.Error("falta referencia detached al documento original")
	}
	if !strings.Contains(xmlData, `URI="#KeyInfo-1"`) {
		t.Error("falta referencia a KeyInfo")
	}
	if !strings.Contains(xmlData, `<xades:MimeType>application/xml</xades:MimeType>`) {
		t.Error("falta MimeType correcto en SignedDataObjectProperties")
	}
}

// TestXAdESBES_DigestDocumentoPresente verifica que el DigestValue del documento
// aparece en la referencia correspondiente.
func TestXAdESBES_DigestDocumentoPresente(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	contenido := []byte("<contrato>datos de prueba</contrato>")
	doc, err := domain.NewDocument("contrato.xml", contenido, "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	contentC14N, err := exclusiveC14N(string(contenido))
	if err != nil {
		t.Fatalf("error canonicalizando contenido: %v", err)
	}
	digest := sha256.Sum256(contentC14N)
	digestB64 := base64.StdEncoding.EncodeToString(digest[:])

	xmlData := string(result.Data)
	if !strings.Contains(xmlData, digestB64) {
		t.Fatalf("DigestValue del documento ausente en la firma.\nEsperado: %s\nXML:\n%s", digestB64, xmlData)
	}
}

func TestXAdESBES_RespetaAlgoritmoYPoliticaAGE18(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>datos</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"algorithm":                     "SHA512withRSA",
			"expPolicy":                     "FirmaAGE18",
			"xadesSignFormat":               "XAdES Detached",
			"includeOnlySigningCertificate": "true",
		},
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if result.Algorithm != "SHA512withRSA" {
		t.Fatalf("algoritmo inesperado: %s", result.Algorithm)
	}
	xmlData := string(result.Data)
	if !strings.Contains(xmlData, algRSASHA512) {
		t.Fatalf("falta SignatureMethod SHA512 en XML: %s", xmlData)
	}
	if !strings.Contains(xmlData, algSHA512) {
		t.Fatalf("falta DigestMethod SHA512 en XML: %s", xmlData)
	}
	if !strings.Contains(xmlData, "urn:oid:2.16.724.1.3.1.1.2.1.8") {
		t.Fatalf("falta policyIdentifier AGE18 en XML: %s", xmlData)
	}
	if !strings.Contains(xmlData, "V8lVVNGDCPen6VELRD1Ja8HARFk=") {
		t.Fatalf("falta policy hash AGE18 XAdES en XML: %s", xmlData)
	}
	if !strings.Contains(xmlData, "http://uri.etsi.org/01903/v1.2.2#SignedProperties") {
		t.Fatalf("falta SignedProperties v1.2.2 para AGE18: %s", xmlData)
	}
	if !strings.Contains(xmlData, "<xades:SigningCertificate>") {
		t.Fatalf("se esperaba SigningCertificate en perfil advanced: %s", xmlData)
	}
	if strings.Contains(xmlData, "SigningCertificateV2") {
		t.Fatalf("no se esperaba SigningCertificateV2 en perfil advanced: %s", xmlData)
	}
	if !strings.Contains(xmlData, `<CONTENT Id="CONTENT-1"><contrato>datos</contrato></CONTENT>`) {
		t.Fatalf("falta CONTENT interno compatible con AutoFirma Java: %s", xmlData)
	}
	if !strings.Contains(xmlData, `Type="http://www.w3.org/2000/09/xmldsig#Object" URI="#CONTENT-1"`) {
		t.Fatalf("falta referencia interna al CONTENT compatible con AutoFirma Java: %s", xmlData)
	}
	if !strings.Contains(xmlData, algC14N) {
		t.Fatalf("falta canonicalizacion inclusiva compatible con AutoFirma Java: %s", xmlData)
	}
}

func TestXAdESBES_ConsumePoliticaTipadaPredeterminada(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument(
		"contrato.xml",
		[]byte("<contrato>política tipada</contrato>"),
		"application/xml",
	)
	if err != nil {
		t.Fatal(err)
	}
	policyID := "urn:oid:1.2.3.4.5"
	policyHash := base64.StdEncoding.EncodeToString(make([]byte, 32))
	policyHashAlgorithm := "SHA-256"
	policyQualifier := "https://sede.example/politica.pdf"
	options := ports.AplicarOpcionesFirmaPredeterminadas(
		ports.DocumentoConfiguracionUsuario{
			XAdES: ports.ConfiguracionUsuarioXAdES{
				PolicyID:            &policyID,
				PolicyHash:          &policyHash,
				PolicyHashAlgorithm: &policyHashAlgorithm,
				PolicyQualifier:     &policyQualifier,
			},
		},
		"XAdES",
		nil,
	)

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options:  options,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	xmlData := string(result.Data)
	for _, expected := range []string{
		policyID,
		policyHash,
		algSHA256,
		policyQualifier,
	} {
		if !strings.Contains(xmlData, expected) {
			t.Fatalf("falta %q de la política tipada en XML: %s", expected, xmlData)
		}
	}
}

func TestXAdESBES_FirmaAGEUsaSigningCertificateClasico(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>datos</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
		Options: map[string]string{
			"algorithm":                     "SHA512withRSA",
			"expPolicy":                     "FirmaAGE",
			"xadesSignFormat":               "XAdES Detached",
			"includeOnlySigningCertificate": "true",
		},
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	xmlData := string(result.Data)
	if !strings.Contains(xmlData, "urn:oid:2.16.724.1.3.1.1.2.1.9") {
		t.Fatalf("falta policyIdentifier AGE en XML: %s", xmlData)
	}
	if !strings.Contains(xmlData, "<xades:SigningCertificate>") {
		t.Fatalf("se esperaba SigningCertificate clasico para FirmaAGE: %s", xmlData)
	}
	if strings.Contains(xmlData, "SigningCertificateV2") {
		t.Fatalf("no se esperaba SigningCertificateV2 para FirmaAGE: %s", xmlData)
	}
	if !strings.Contains(xmlData, `<CONTENT Id="CONTENT-1"><contrato>datos</contrato></CONTENT>`) {
		t.Fatalf("falta CONTENT interno compatible con AutoFirma Java para FirmaAGE: %s", xmlData)
	}
	if !strings.Contains(xmlData, `Type="http://www.w3.org/2000/09/xmldsig#Object" URI="#CONTENT-1"`) {
		t.Fatalf("falta referencia interna al CONTENT para FirmaAGE: %s", xmlData)
	}
}

func TestXAdESBES_IdaYVueltaFirmaLocal(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<contrato>ida y vuelta</contrato>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	if err := verifyLocalXAdESSignature(result.Data); err != nil {
		t.Fatalf("la firma XAdES generada no verifica localmente: %v", err)
	}
}

// TestXAdESBES_SignatureValueNoVacio verifica que SignatureValue no está vacío.
func TestXAdESBES_SignatureValueNoVacio(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<x>datos</x>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	xmlData := string(result.Data)

	// Extraer contenido de ds:SignatureValue
	startTag := "<ds:SignatureValue>"
	endTag := "</ds:SignatureValue>"
	startIdx := strings.Index(xmlData, startTag)
	endIdx := strings.Index(xmlData, endTag)
	if startIdx < 0 || endIdx < 0 {
		t.Fatal("elemento ds:SignatureValue no encontrado en el XML")
	}
	sigValueContent := xmlData[startIdx+len(startTag) : endIdx]
	sigValueContent = strings.TrimSpace(sigValueContent)
	if sigValueContent == "" {
		t.Fatal("ds:SignatureValue está vacío")
	}

	// Verificar que es base64 válido y no trivial
	decoded, err := base64.StdEncoding.DecodeString(sigValueContent)
	if err != nil {
		t.Fatalf("ds:SignatureValue no es base64 válido: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("ds:SignatureValue decodificado está vacío")
	}
}

// TestXAdESBES_ClaveNula_RetornaError verifica que una clave nil retorna error.
func TestXAdESBES_ClaveNula_RetornaError(t *testing.T) {
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<x/>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	_, err = engine.Sign(context.Background(), domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, nil)
	if err == nil {
		t.Fatal("se esperaba error con clave nil")
	}
}

// TestXAdESBES_ContextoCancelado verifica que un contexto cancelado retorna error.
func TestXAdESBES_ContextoCancelado(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-RSA")
	engine := NewXAdESBESDetached()
	doc, err := domain.NewDocument("contrato.xml", []byte("<x/>"), "application/xml")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar de inmediato

	_, err = engine.Sign(ctx, domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatXAdES,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{
		ID:          "clave-xades",
		Signer:      priv,
		Certificate: cert,
	})
	if err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}
}

func verifyLocalXAdESSignature(xmlData []byte) error {
	signedInfo, err := extractXMLElement(string(xmlData), "ds:SignedInfo")
	if err != nil {
		return err
	}
	signatureValue, err := extractElementText(string(xmlData), "ds:SignatureValue")
	if err != nil {
		return err
	}
	certB64, err := extractElementText(string(xmlData), "ds:X509Certificate")
	if err != nil {
		return err
	}

	certDER, err := base64.StdEncoding.DecodeString(compactBase64(certB64))
	if err != nil {
		return fmt.Errorf("certificado base64 invalido: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return err
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("clave publica no RSA")
	}

	c14n, err := exclusiveC14N(signedInfo)
	if err != nil {
		return fmt.Errorf("error en c14n de SignedInfo: %w", err)
	}
	digest := sha256.Sum256(c14n)
	sigBytes, err := base64.StdEncoding.DecodeString(compactBase64(signatureValue))
	if err != nil {
		return fmt.Errorf("signature value base64 invalido: %w", err)
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sigBytes); err != nil {
		return fmt.Errorf("firma RSA invalida: %w", err)
	}
	return nil
}

func extractXMLElement(xmlData, tag string) (string, error) {
	startTag := "<" + tag
	endTag := "</" + tag + ">"
	start := strings.Index(xmlData, startTag)
	if start < 0 {
		return "", fmt.Errorf("tag %s no encontrada", tag)
	}
	end := strings.Index(xmlData[start:], endTag)
	if end < 0 {
		return "", fmt.Errorf("cierre %s no encontrado", tag)
	}
	end += start + len(endTag)
	return xmlData[start:end], nil
}

func extractElementText(xmlData, tag string) (string, error) {
	full, err := extractXMLElement(xmlData, tag)
	if err != nil {
		return "", err
	}
	start := strings.Index(full, ">")
	end := strings.LastIndex(full, "</")
	if start < 0 || end < 0 || end <= start {
		return "", fmt.Errorf("contenido invalido en %s", tag)
	}
	return full[start+1 : end], nil
}

// Los portales (Canarias, MITES) piden XAdES Detached sobre texto plano
// declarando application/xml: debe firmarse en Base64 dentro de CONTENT, como
// AutoFirma Java, y verificarse de forma autónoma.
func TestXAdESBES_DatosNoXMLSeFirmanEnBase64(t *testing.T) {
	priv, cert := certForTest(t, "XAdES-texto")
	engine := NewXAdESBESDetached()
	for _, mime := range []string{"application/xml", "", "text/plain"} {
		doc, err := domain.NewDocument("entrada.bin", []byte("Texto de prueba XAdES"), mime)
		if err != nil {
			t.Fatal(err)
		}
		result, err := engine.Sign(context.Background(), domain.SignatureJob{
			Document: doc,
			Format:   domain.FormatXAdES,
			Action:   domain.ActionSign,
			Options:  map[string]string{"format": "XAdES Detached", "mode": "implicit"},
		}, &LocalSigningKey{ID: "clave", Signer: priv, Certificate: cert})
		if err != nil {
			t.Fatalf("mime=%q: firma XAdES sobre texto: %v", mime, err)
		}
		xmlText := string(result.Data)
		if !strings.Contains(xmlText, `Encoding="Base64"`) || strings.Contains(xmlText, `MimeType="application/xml"`) {
			t.Fatalf("mime=%q: CONTENT no declara Base64 o mantiene un MimeType XML falso", mime)
		}
		firmado, err := domain.NewDocument("entrada.xsig", result.Data, "application/xml")
		if err != nil {
			t.Fatal(err)
		}
		vr, _, err := NewXAdESVerifier().Verify(context.Background(), firmado, domain.CertificateChain{})
		if err != nil || !vr.Valid {
			t.Fatalf("mime=%q: la firma no verifica: %v %s", mime, err, vr.Reason)
		}
	}
}
