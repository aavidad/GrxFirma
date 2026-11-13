// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"crypto"
	_ "crypto/sha1" // ESSCertID (signingCertificate v1) usa SHA-1.
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
)

// Atributos firmados que atan la firma CMS a su certificado y a su tipo de
// contenido (RFC 5652 §11.1, RFC 5035, ETSI EN 319 122-1). La lista de
// certificados y el SID del SignerInfo no están firmados: sin estas
// comprobaciones otro certificado con la misma clave podría atribuirse la
// firma.

var oidSigningCertificateV1 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 12}

// detalleSigningCertificateAusente se añade a los detalles de integridad de
// una firma sin signingCertificate(V2); PAdES ETSI lo convierte en aviso.
const detalleSigningCertificateAusente = "signing_certificate=ausente"

// valoresAtributoFirmado devuelve los valores del atributo con ese OID. Un
// atributo repetido es ambiguo y se rechaza.
func valoresAtributoFirmado(attrsDER []byte, oid asn1.ObjectIdentifier) ([]asn1.RawValue, bool, error) {
	remaining := attrsDER
	var valores []asn1.RawValue
	encontrado := false
	for len(remaining) > 0 {
		var rawAttr asn1.RawValue
		var err error
		remaining, err = asn1.Unmarshal(remaining, &rawAttr)
		if err != nil {
			return nil, false, fmt.Errorf("parseando atributos firmados: %w", err)
		}
		var attr attribute
		if _, err := asn1.Unmarshal(rawAttr.FullBytes, &attr); err != nil {
			return nil, false, fmt.Errorf("parseando atributo firmado: %w", err)
		}
		if !attr.Type.Equal(oid) {
			continue
		}
		if encontrado {
			return nil, false, fmt.Errorf("atributo firmado %v repetido", oid)
		}
		encontrado = true
		valores = attr.Values
	}
	return valores, encontrado, nil
}

// verificarContentTypeFirmado exige que el atributo contentType exista y
// coincida con eContentType.
func verificarContentTypeFirmado(signedAttrs asn1.RawValue, encap asn1.RawValue) error {
	var eci encapContentInfoRaw
	if _, err := asn1.Unmarshal(encap.FullBytes, &eci); err != nil {
		return fmt.Errorf("parseando EncapContentInfo: %w", err)
	}
	valores, presente, err := valoresAtributoFirmado(signedAttrs.Bytes, oidContentType)
	if err != nil {
		return err
	}
	if !presente || len(valores) != 1 {
		return errors.New("atributo contentType ausente o con varios valores")
	}
	var declarado asn1.ObjectIdentifier
	if rest, err := asn1.Unmarshal(valores[0].FullBytes, &declarado); err != nil || len(rest) != 0 {
		return errors.New("atributo contentType mal formado")
	}
	if !declarado.Equal(eci.ContentType) {
		return fmt.Errorf("el atributo contentType firmado (%v) no coincide con eContentType (%v)", declarado, eci.ContentType)
	}
	return nil
}

// verificarSigningCertificate comprueba que el primer ESSCertID(v2) del
// atributo firmado identifica el certificado elegido como firmante. Devuelve
// false sin error si la firma no trae el atributo.
func verificarSigningCertificate(signedAttrs asn1.RawValue, cert *x509.Certificate) (bool, error) {
	presente := false
	for _, variante := range []struct {
		oid asn1.ObjectIdentifier
		v2  bool
	}{{oidSigningCertificateV2, true}, {oidSigningCertificateV1, false}} {
		valores, ok, err := valoresAtributoFirmado(signedAttrs.Bytes, variante.oid)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		if len(valores) != 1 {
			return false, errors.New("atributo signingCertificate con varios valores")
		}
		if err := comprobarESSCertID(valores[0].FullBytes, cert, variante.v2); err != nil {
			return false, err
		}
		presente = true
	}
	return presente, nil
}

func comprobarESSCertID(der []byte, cert *x509.Certificate, v2 bool) error {
	malFormado := errors.New("atributo signingCertificate mal formado")
	input := cryptobyte.String(der)
	var signingCert, certs, essCertID cryptobyte.String
	if !input.ReadASN1(&signingCert, cbasn1.SEQUENCE) || !input.Empty() ||
		!signingCert.ReadASN1(&certs, cbasn1.SEQUENCE) ||
		!certs.ReadASN1(&essCertID, cbasn1.SEQUENCE) {
		return malFormado
	}
	hash := crypto.SHA1
	if v2 {
		hash = crypto.SHA256 // DEFAULT de ESSCertIDv2
		if essCertID.PeekASN1Tag(cbasn1.SEQUENCE) {
			var algoritmo cryptobyte.String
			var oid asn1.ObjectIdentifier
			if !essCertID.ReadASN1(&algoritmo, cbasn1.SEQUENCE) || !algoritmo.ReadASN1ObjectIdentifier(&oid) {
				return malFormado
			}
			spec, ok := cmsHashForDigestOID(oid)
			if !ok {
				return fmt.Errorf("algoritmo de signingCertificateV2 no admitido: %v", oid)
			}
			hash = spec.hash
		}
	}
	var certHash []byte
	if !essCertID.ReadASN1Bytes(&certHash, cbasn1.OCTET_STRING) {
		return malFormado
	}
	if !bytes.Equal(certHash, cmsDigest(hash, cert.Raw)) {
		return errors.New("el certificado firmante no es el que identifica el atributo firmado signingCertificate")
	}
	if essCertID.Empty() {
		return nil
	}
	return comprobarIssuerSerial(essCertID, cert)
}

// comprobarIssuerSerial compara el IssuerSerial opcional con el certificado.
func comprobarIssuerSerial(resto cryptobyte.String, cert *x509.Certificate) error {
	malFormado := errors.New("IssuerSerial de signingCertificate mal formado")
	var issuerSerial, nombres cryptobyte.String
	serie := new(big.Int)
	if !resto.ReadASN1(&issuerSerial, cbasn1.SEQUENCE) || !resto.Empty() ||
		!issuerSerial.ReadASN1(&nombres, cbasn1.SEQUENCE) ||
		!issuerSerial.ReadASN1Integer(serie) {
		return malFormado
	}
	if serie.Cmp(cert.SerialNumber) != 0 {
		return errors.New("la serie de signingCertificate no coincide con el certificado firmante")
	}
	directorio := cbasn1.Tag(4).ContextSpecific().Constructed()
	hayDirectorio, coincide := false, false
	for !nombres.Empty() {
		var nombre cryptobyte.String
		var tag cbasn1.Tag
		if !nombres.ReadAnyASN1(&nombre, &tag) {
			return malFormado
		}
		if tag != directorio {
			continue
		}
		hayDirectorio = true
		coincide = coincide || bytes.Equal(nombre, cert.RawIssuer)
	}
	if hayDirectorio && !coincide {
		return errors.New("el emisor de signingCertificate no coincide con el certificado firmante")
	}
	return nil
}

// contarSignerInfosCMS devuelve cuántos SignerInfo trae un CMS SignedData.
func contarSignerInfosCMS(cmsDER []byte) (int, error) {
	ci, err := parseContentInfo(cmsDER)
	if err != nil {
		return 0, err
	}
	sd, err := parseSignedData(ci.Content.Bytes)
	if err != nil {
		return 0, err
	}
	infos, err := parseSignerInfos(sd.SignerInfos)
	if err != nil {
		return 0, err
	}
	return len(infos), nil
}
