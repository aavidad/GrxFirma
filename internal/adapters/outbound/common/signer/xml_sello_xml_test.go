// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"grxfirma/internal/domain"
)

// selloXMLDePrueba es un sello de tiempo XML de OASIS DSS como los que mete
// AutoFirma en XAdES-T (muestra XAdES-XL_con_sello_xml.xml): una ds:Signature
// de la TSA dentro de dss:Timestamp. Su firma no se comprueba (no hay forma
// de hacerlo sin la TSA); lo que importa es que no se trate como otra firma
// del documento.
const selloXMLDePrueba = `<dss:Timestamp xmlns:dss="urn:oasis:names:tc:dss:1.0:core:schema"><ds:Signature Id="sello-tsa"><ds:SignedInfo><ds:CanonicalizationMethod Algorithm="http://www.w3.org/2001/10/xml-exc-c14n#"/><ds:SignatureMethod Algorithm="http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"/><ds:Reference URI="#tst"><ds:DigestMethod Algorithm="http://www.w3.org/2001/04/xmlenc#sha256"/><ds:DigestValue>AAAA</ds:DigestValue></ds:Reference></ds:SignedInfo><ds:SignatureValue>AAAA</ds:SignatureValue><ds:KeyInfo><ds:X509Data><ds:X509Certificate>AAAA</ds:X509Certificate></ds:X509Data></ds:KeyInfo><ds:Object Id="tst"><dss:TstInfo><dss:SerialNumber>1</dss:SerialNumber></dss:TstInfo></ds:Object></ds:Signature></dss:Timestamp>`

func conPropiedadesNoFirmadas(t *testing.T, firmado []byte, contenido string) []byte {
	t.Helper()
	cierre := []byte("</xades:QualifyingProperties>")
	if bytes.Count(firmado, cierre) != 1 {
		t.Fatalf("la firma de prueba no tiene un único QualifyingProperties:\n%s", firmado)
	}
	return bytes.Replace(firmado, cierre, []byte("<xades:UnsignedProperties><xades:UnsignedSignatureProperties>"+contenido+"</xades:UnsignedSignatureProperties></xades:UnsignedProperties></xades:QualifyingProperties>"), 1)
}

func TestXAdES_SelloXMLNoSeVerificaComoOtraFirma(t *testing.T) {
	firmado := firmarXAdESVariante(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", "XAdES Enveloping")
	conSello := conPropiedadesNoFirmadas(t, firmado,
		`<xades:SignatureTimeStamp><ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315"/><xades:XMLTimeStamp>`+selloXMLDePrueba+`</xades:XMLTimeStamp></xades:SignatureTimeStamp>`)

	vr := verificarXAdESValida(t, conSello)
	if vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("integridad = %s (%s); la firma de la TSA no es una firma del documento", vr.Integrity.Status, vr.Integrity.Reason)
	}

	// La integridad de la firma de verdad se sigue comprobando.
	alterado := bytes.Replace(conSello, []byte("<importe>10<"), []byte("<importe>99<"), 1)
	doc, _ := domain.NewDocument("f.xsig", alterado, "application/xml")
	res, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
	if err == nil && (res.Valid || res.Integrity.Status == domain.VerificationStatusValid) {
		t.Fatal("un documento alterado con un sello XML no puede salir íntegro")
	}
}

// Fuera del sitio exacto de un sello XML (dss:Timestamp hijo de
// xades:XMLTimeStamp dentro de otra firma), una ds:Signature se verifica
// como cualquier otra: no basta con envolverla en dss:Timestamp.
func TestXAdES_FirmaEnDSSTimestampFueraDeSelloSeVerifica(t *testing.T) {
	firmado := firmarXAdESVariante(t, []byte(`<pedido><importe>10</importe></pedido>`), "application/xml", "XAdES Enveloping")
	casos := map[string][]byte{
		"sin XMLTimeStamp": conPropiedadesNoFirmadas(t, firmado, selloXMLDePrueba),
		"fuera de la firma": []byte(strings.Replace(string(firmado), "<ds:Signature ",
			`<raiz><xades:XMLTimeStamp xmlns:xades="http://uri.etsi.org/01903/v1.3.2#" xmlns:ds="http://www.w3.org/2000/09/xmldsig#">`+selloXMLDePrueba+`</xades:XMLTimeStamp><ds:Signature `, 1) + "</raiz>"),
	}
	for nombre, documento := range casos {
		t.Run(nombre, func(t *testing.T) {
			doc, _ := domain.NewDocument("f.xsig", documento, "application/xml")
			res, _, err := NewXAdESVerifier().Verify(context.Background(), doc, domain.CertificateChain{})
			if err == nil {
				t.Fatalf("una ds:Signature falsa fuera de un sello XML no puede pasar sin verificar: integridad=%s", res.Integrity.Status)
			}
		})
	}
}
