// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// firmaXMLDSigJavaConKeyInfo monta una firma XMLdSig enveloped como las de
// AutoFirma (xmldsig_enveloped_SHA1.xml de @firma 1.9, aquí con SHA-256): la
// segunda referencia firma el propio KeyInfo con la cadena [C14N, enveloped,
// XPath]. Los resúmenes esperados se escriben a mano a partir de XMLDSig
// 4.4.3.2: la C14N inicial produce octetos y el resto de la cadena trabaja
// sobre ese documento nuevo, donde la firma que contiene el KeyInfo ya no
// está.
func firmaXMLDSigJavaConKeyInfo(t *testing.T, documento string) []byte {
	t.Helper()
	priv, cert := certForTest(t, "XMLdSig Java KeyInfo")
	certB64 := base64.StdEncoding.EncodeToString(cert.Raw)
	const ns = `xmlns:ds="http://www.w3.org/2000/09/xmldsig#"`
	transformaciones := `<ds:Transforms><ds:Transform Algorithm="` + algC14N + `"></ds:Transform><ds:Transform Algorithm="` + algEnveloped + `"></ds:Transform><ds:Transform Algorithm="` + algXPathFilter + `"><ds:XPath ` + ns + `>not(ancestor-or-self::ds:Signature)</ds:XPath></ds:Transform></ds:Transforms>`
	keyInfo := `<ds:KeyInfo Id="KeyInfo-1"><ds:X509Data><ds:X509Certificate>` + certB64 + `</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`
	// C14N inclusiva del KeyInfo en su contexto: hereda xmlns:ds.
	keyInfoC14N := `<ds:KeyInfo ` + ns + ` Id="KeyInfo-1"><ds:X509Data><ds:X509Certificate>` + certB64 + `</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`
	resumen := func(s string) string {
		suma := sha256.Sum256([]byte(s))
		return base64.StdEncoding.EncodeToString(suma[:])
	}
	referencias := `<ds:Reference URI="">` + transformaciones + `<ds:DigestMethod Algorithm="` + algSHA256 + `"></ds:DigestMethod><ds:DigestValue>` + resumen(documento) + `</ds:DigestValue></ds:Reference>` +
		`<ds:Reference URI="#KeyInfo-1">` + transformaciones + `<ds:DigestMethod Algorithm="` + algSHA256 + `"></ds:DigestMethod><ds:DigestValue>` + resumen(keyInfoC14N) + `</ds:DigestValue></ds:Reference>`
	cuerpoSignedInfo := `<ds:CanonicalizationMethod Algorithm="` + algC14N + `"></ds:CanonicalizationMethod><ds:SignatureMethod Algorithm="` + algRSASHA256 + `"></ds:SignatureMethod>` + referencias + `</ds:SignedInfo>`
	// En la C14N del SignedInfo, el xmlns:ds repetido del XPath sobra.
	signedInfoC14N := `<ds:SignedInfo ` + ns + `>` + strings.ReplaceAll(cuerpoSignedInfo, `<ds:XPath `+ns+`>`, `<ds:XPath>`)
	suma := sha256.Sum256([]byte(signedInfoC14N))
	valor, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, suma[:])
	if err != nil {
		t.Fatal(err)
	}
	firma := `<ds:Signature ` + ns + ` Id="Signature-1"><ds:SignedInfo>` + cuerpoSignedInfo +
		`<ds:SignatureValue>` + base64.StdEncoding.EncodeToString(valor) + `</ds:SignatureValue>` + keyInfo + `</ds:Signature>`
	cierre := strings.LastIndex(documento, "</")
	return []byte(documento[:cierre] + firma + documento[cierre:])
}

// Lote 13b, punto 1 (otras muestras): las firmas XMLdSig enveloped de
// AutoFirma salían como «Compatibilidad histórica» porque la referencia al
// KeyInfo no se calculaba con la semántica estándar.
func TestXMLDSig_ReferenciaJavaAlKeyInfoPorElCaminoEstandar(t *testing.T) {
	documento := `<CATALOG><PLANT><COMMON>Bloodroot</COMMON></PLANT></CATALOG>`
	firmado := firmaXMLDSigJavaConKeyInfo(t, documento)
	result, signers, err := NewXMLDSigVerifier().Verify(context.Background(), domain.Document{Name: "catalogo.xml", MIMEType: "application/xml", Content: firmado}, domain.CertificateChain{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(signers) != 1 || result.Integrity.Status != domain.VerificationStatusValid || result.Coverage != domain.CoberturaCompleta {
		t.Fatalf("integridad=%+v cobertura=%s", result.Integrity, result.Coverage)
	}
	for _, evidencia := range result.Evidence {
		if evidencia.Type == xmlCompatibilityEvidence {
			t.Fatalf("no debe quedar como compatibilidad histórica: %v", evidencia)
		}
	}

	// El KeyInfo firmado y el documento siguen protegidos.
	for nombre, alterado := range map[string][]byte{
		"documento": []byte(strings.Replace(string(firmado), "Bloodroot", "Bloodr00t", 1)),
		"keyinfo":   []byte(strings.Replace(string(firmado), `<ds:KeyInfo Id="KeyInfo-1">`, `<ds:KeyInfo Id="KeyInfo-1" xml:lang="es">`, 1)),
	} {
		if res, _, err := NewXMLDSigVerifier().Verify(context.Background(), domain.Document{Name: "catalogo.xml", MIMEType: "application/xml", Content: alterado}, domain.CertificateChain{}); err == nil && res.Integrity.Status == domain.VerificationStatusValid {
			t.Fatalf("%s alterado no puede dar integridad válida: %+v", nombre, res.Integrity)
		}
	}
}
