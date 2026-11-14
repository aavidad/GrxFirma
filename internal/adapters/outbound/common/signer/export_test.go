// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/asn1"
	"fmt"
)

// Manipulaciones de CMS para pruebas de ataque: solo tocan partes que la
// firma no protege (certificados, eContentType y el conjunto SignerInfos).

func reescribirSignedDataParaPrueba(cms []byte, mutar func(*signedDataRaw) error) ([]byte, error) {
	ci, err := parseContentInfo(cms)
	if err != nil {
		return nil, err
	}
	sd, err := parseSignedData(ci.Content.Bytes)
	if err != nil {
		return nil, err
	}
	if err := mutar(&sd); err != nil {
		return nil, err
	}
	sdDER, err := asn1.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("serializando SignedData: %w", err)
	}
	return asn1.Marshal(contentInfoRaw{
		ContentType: ci.ContentType,
		Content:     asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sdDER},
	})
}

// CMSConCertificadosParaPrueba sustituye el conjunto de certificados.
func CMSConCertificadosParaPrueba(cms []byte, certsDER ...[]byte) ([]byte, error) {
	return reescribirSignedDataParaPrueba(cms, func(sd *signedDataRaw) error {
		var contenido []byte
		for _, der := range certsDER {
			contenido = append(contenido, der...)
		}
		sd.Certificates = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: contenido}
		return nil
	})
}

// CMSConEContentTypeParaPrueba declara otro tipo de contenido encapsulado
// (sin contenido) sin tocar los atributos firmados.
func CMSConEContentTypeParaPrueba(cms []byte, oid asn1.ObjectIdentifier) ([]byte, error) {
	return reescribirSignedDataParaPrueba(cms, func(sd *signedDataRaw) error {
		der, err := asn1.Marshal(encapContentInfoRaw{ContentType: oid})
		if err != nil {
			return err
		}
		sd.EncapContentInfo = asn1.RawValue{FullBytes: der}
		return nil
	})
}

// CMSConSignerInfoDuplicadoParaPrueba repite el único SignerInfo.
func CMSConSignerInfoDuplicadoParaPrueba(cms []byte) ([]byte, error) {
	return reescribirSignedDataParaPrueba(cms, func(sd *signedDataRaw) error {
		var si asn1.RawValue
		if _, err := asn1.Unmarshal(sd.SignerInfos.Bytes, &si); err != nil {
			return err
		}
		doble := append(append([]byte(nil), si.FullBytes...), si.FullBytes...)
		sd.SignerInfos = asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: doble}
		return nil
	})
}

// CMSSinSigningCertificateParaPrueba quita el atributo signingCertificateV2
// y vuelve a firmar los atributos con la misma clave RSA (SHA-256).
func CMSSinSigningCertificateParaPrueba(cms []byte, clave *rsa.PrivateKey) ([]byte, error) {
	return reescribirSignedDataParaPrueba(cms, func(sd *signedDataRaw) error {
		infos, err := parseSignerInfos(sd.SignerInfos)
		if err != nil || len(infos) != 1 {
			return fmt.Errorf("se esperaba un SignerInfo: %v", err)
		}
		si := infos[0]
		var conservados []byte
		resto := si.SignedAttributes.Bytes
		for len(resto) > 0 {
			var raw asn1.RawValue
			if resto, err = asn1.Unmarshal(resto, &raw); err != nil {
				return err
			}
			var attr attribute
			if _, err := asn1.Unmarshal(raw.FullBytes, &attr); err != nil {
				return err
			}
			if !attr.Type.Equal(oidSigningCertificateV2) {
				conservados = append(conservados, raw.FullBytes...)
			}
		}
		setDER, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: conservados})
		if err != nil {
			return err
		}
		firma, err := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, cmsDigest(crypto.SHA256, setDER))
		if err != nil {
			return err
		}
		si.SignedAttributes = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: conservados}
		si.Signature = firma
		siDER, err := asn1.Marshal(si)
		if err != nil {
			return err
		}
		sd.SignerInfos = asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: siDER}
		return nil
	})
}
