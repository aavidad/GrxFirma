// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// E1-10: el tamaño que declara una entrada ZIP lo pone quien fabrica el
// contenedor; la verificación tiene que cortar al leer, por entrada y en total.
// Las pruebas bajan los topes para no reservar cientos de megas.
func limitarZIPParaPrueba(t *testing.T, porEntrada, total int64) {
	t.Helper()
	antesEntrada, antesTotal := maxEntradaZIPVerificacion, maxTotalZIPVerificacion
	maxEntradaZIPVerificacion, maxTotalZIPVerificacion = porEntrada, total
	t.Cleanup(func() { maxEntradaZIPVerificacion, maxTotalZIPVerificacion = antesEntrada, antesTotal })
}

func odtConEntradas(t *testing.T, extra map[string][]byte) []byte {
	t.Helper()
	priv, cert := certForTest(t, "ODF grande")
	entradas := []entradaZIP{
		{nombre: "mimetype", datos: []byte("application/vnd.oasis.opendocument.text")},
		{nombre: "META-INF/manifest.xml", datos: []byte(`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/></manifest:manifest>`)},
		{nombre: "content.xml", datos: []byte(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body/></office:document-content>`)},
	}
	for nombre, datos := range extra {
		entradas = append(entradas, entradaZIP{nombre: nombre, datos: datos})
	}
	signed, err := NewODFDetached().Sign(context.Background(), domain.SignatureJob{
		Document: domain.Document{Name: "g.odt", MIMEType: "application/vnd.oasis.opendocument.text", Content: zipOrdenado(t, entradas)},
		Format:   formatODF,
		Action:   domain.ActionSign,
	}, &LocalSigningKey{ID: "g", Signer: priv, Certificate: cert})
	if err != nil {
		t.Fatal(err)
	}
	return signed.Data
}

func TestZipEntryResolver_LimitaDescompresion(t *testing.T) {
	grande := odtConEntradas(t, map[string][]byte{"Pictures/ceros.bin": make([]byte, 3<<20)})
	dos := odtConEntradas(t, map[string][]byte{"a.bin": make([]byte, 700<<10), "b.bin": make([]byte, 700<<10)})
	limitarZIPParaPrueba(t, 1<<20, 1<<20+256<<10)

	_, _, err := NewODFVerifier().Verify(context.Background(), domain.Document{Name: "g.odt", Content: grande}, domain.CertificateChain{})
	if err == nil || !strings.Contains(err.Error(), "tamaño máximo") {
		t.Fatalf("una entrada por encima del tope debe rechazarse: %v", err)
	}
	_, _, err = NewODFVerifier().Verify(context.Background(), domain.Document{Name: "g.odt", Content: dos}, domain.CertificateChain{})
	if err == nil || !strings.Contains(err.Error(), "tamaño máximo") {
		t.Fatalf("el total leído por encima del tope debe rechazarse: %v", err)
	}
}

// Un KeyInfo con miles de certificados hacía reanalizar el documento entero
// por cada uno.
func TestXMLVerify_LimitaCertificadosKeyInfo(t *testing.T) {
	firmado := string(operarXAdES(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", domain.ActionSign, map[string]string{"format": "XAdES Enveloped"}, "Victima"))
	certificado := elementoLiteral(t, firmado, "<ds:X509Certificate>", "</ds:X509Certificate>")
	inflado := strings.Replace(firmado, certificado, strings.Repeat(certificado, maxCertificadosKeyInfo+1), 1)
	_, _, err := NewXAdESVerifier().Verify(context.Background(), domain.Document{Name: "p.xsig", Content: []byte(inflado)}, domain.CertificateChain{})
	if err == nil || !strings.Contains(err.Error(), "demasiados certificados") {
		t.Fatalf("se esperaba el error del tope de certificados: %v", err)
	}
}

func TestXMLVerify_LimitaFirmasYReferencias(t *testing.T) {
	firma := `<ds:Signature xmlns:ds="` + nsXMLDSig + `"><ds:SignedInfo/><ds:SignatureValue/><ds:KeyInfo/></ds:Signature>`
	muchas := []byte(`<r>` + strings.Repeat(firma, maxFirmasXML+1) + `</r>`)
	if _, err := extractContextualXMLSignatures(muchas); err == nil || !strings.Contains(err.Error(), "demasiadas firmas") {
		t.Fatalf("se esperaba el tope de firmas: %v", err)
	}
	referencia := `<ds:Reference URI="#a"><ds:DigestMethod Algorithm="` + algSHA256 + `"/><ds:DigestValue>AA==</ds:DigestValue></ds:Reference>`
	info := []byte(`<ds:SignedInfo xmlns:ds="` + nsXMLDSig + `">` + strings.Repeat(referencia, maxReferenciasXML+1) + `</ds:SignedInfo>`)
	if _, err := assessXMLReferences(info, xmlSignatureContext{document: info}, info, nil); err == nil || !strings.Contains(err.Error(), "demasiadas referencias") {
		t.Fatalf("se esperaba el tope de referencias: %v", err)
	}
}
