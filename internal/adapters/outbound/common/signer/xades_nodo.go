// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"encoding/base64"
	"fmt"
	"time"

	"grxfirma/internal/ports"
	"grxfirma/internal/security/signingpolicy"
)

// FirmarNodoXAdES firma un elemento XML identificado por su Id con una firma
// XAdES separada (referencia "#id") y devuelve el elemento ds:Signature, para
// insertarlo en otra estructura, como el índice de un expediente ENI.
//
// El SignedInfo, las propiedades firmadas, el KeyInfo y el nodo se
// canonicalizan con C14N exclusiva, que no depende de los espacios de nombres
// de los ancestros: la firma sigue siendo válida al colocarla en su destino,
// siempre que el nodo se inserte tal cual y declare sus propios prefijos.
func FirmarNodoXAdES(nodo []byte, id string, key ports.SigningKey, options map[string]string) ([]byte, error) {
	// Las claves de los almacenes del sistema se convierten a la clave local
	// del motor, como en el resto de formatos.
	if conv, ok := key.(interface{ ToLocalSigningKey() *LocalSigningKey }); ok {
		key = conv.ToLocalSigningKey()
	}
	clave, err := requireLocalSigningKeyXAdES(key)
	if err != nil {
		return nil, err
	}
	// Misma política que el resto de firmas: certificado vigente y apto.
	if err := signingpolicy.ValidateCertificateDER(clave.Certificate.Raw, time.Now()); err != nil {
		return nil, err
	}
	algOpts, err := resolveXAdESAlgorithmOptions(options)
	if err != nil {
		return nil, err
	}
	buildOpts := resolveXAdESBuildOptions(options, algOpts)
	buildOpts.CanonicalizationAlg = algExcC14N
	buildOpts.DocumentTransform = algExcC14N
	buildOpts.DocumentTransforms = nil
	buildOpts.OmitDocumentTransform = false
	canon, err := canonicalizeXML(nodo, algExcC14N)
	if err != nil {
		return nil, fmt.Errorf("canonicalizando el nodo a firmar: %w", err)
	}
	resumen, err := digestBytes(algOpts.Hash, canon)
	if err != nil {
		return nil, err
	}
	ids := xadesIDs{sig: "Signature-" + id, keyInfo: "KeyInfo-" + id, signedProps: "SignedProperties-" + id, reference: "Reference-" + id}
	signedInfo, valor, keyInfo, signedProps, err := firmarPartesXAdES(clave, algOpts, buildOpts, ids, "#"+id, base64.StdEncoding.EncodeToString(resumen), "text/xml")
	if err != nil {
		return nil, err
	}
	return []byte(ensamblarFirmaXAdES(ids, buildOpts.Namespace, signedInfo, valor, keyInfo, "", signedProps)), nil
}

// CanonicalizarExclusivo aplica la canonicalización XML exclusiva sin
// comentarios.
func CanonicalizarExclusivo(data []byte) ([]byte, error) {
	return canonicalizeXML(sanitizeXMLDocument(data), algExcC14N)
}
