// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"errors"
	"fmt"

	"grxfirma/internal/adapters/outbound/common/anidamientoasn1"
)

// normalizarCMSBER pasa a DER una firma CMS en BER: longitudes indefinidas y
// OCTET STRING troceados (los dos usos de BER que dejan AutoFirma antiguo y
// BouncyCastle en modo flujo). La profundidad se limita antes de recorrerla
// (E2-05). Solo se admiten ceros sobrantes al final, como en /Contents.
func normalizarCMSBER(data []byte) ([]byte, error) {
	if err := anidamientoasn1.Comprobar(data, anidamientoasn1.MaxProfundidad); errors.Is(err, anidamientoasn1.ErrDemasiadoProfundo) {
		return nil, fmt.Errorf("CMS no admitido: %w", err)
	}
	value, offset, err := decodeTLV(data, 0)
	if err != nil {
		return nil, err
	}
	if offset != len(data) && !onlyZeroPadding(data[offset:]) {
		return nil, fmt.Errorf("datos ASN.1 sobrantes: %d bytes", len(data)-offset)
	}
	value, err = aplanarOctetStringBER(value)
	if err != nil {
		return nil, err
	}
	return encodeTLV(value), nil
}

// aplanarOctetStringBER une los trozos de cada OCTET STRING construido en uno
// primitivo, que es su forma DER. El valor es la concatenación de los trozos
// en orden (X.690, 8.7.3), el mismo sobre el que se calculó el resumen.
func aplanarOctetStringBER(node tlvNode) (tlvNode, error) {
	if !node.Constructed {
		return node, nil
	}
	for i := range node.Children {
		hijo, err := aplanarOctetStringBER(node.Children[i])
		if err != nil {
			return tlvNode{}, err
		}
		node.Children[i] = hijo
	}
	if node.TagClass != 0 || node.Tag != 4 {
		return node, nil
	}
	var contenido []byte
	for _, trozo := range node.Children {
		if trozo.TagClass != 0 || trozo.Tag != 4 || trozo.Constructed {
			return tlvNode{}, errors.New("OCTET STRING construido con un trozo que no es OCTET STRING")
		}
		contenido = append(contenido, trozo.Content...)
	}
	if contenido == nil {
		contenido = []byte{}
	}
	return tlvNode{TagClass: 0, Tag: 4, Content: contenido}, nil
}
