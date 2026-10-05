// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"errors"

	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
)

// SignedAndEnvelopedData usa go-cryptobin, cuyo firmante RSA exige una
// *rsa.PrivateKey. El DNIe no exporta su clave: solo firma resúmenes a través
// de externalRSASigner. Este adaptador, registrado solo en el binario móvil,
// calcula el SHA-256 del contenido y delega la firma PKCS#1 v1.5 en el
// firmador externo, que ya comprueba la firma contra el certificado.
//
// La clave del registro es un OID propio (UUID, arco 2.25) para no sustituir
// ninguna entrada de la biblioteca: la verificación sigue resolviéndose por el
// OID real de la firma. Check solo acepta externalRSASigner, así que las claves
// PKCS#12 siguen usando el firmador original de la biblioteca.
// Cada arco cabe en 32 bits: asn1.ObjectIdentifier es []int y gomobile
// compila también para armeabi-v7a.
var externalCMSKeySignRegistration = asn1.ObjectIdentifier{2, 25, 1724470414, 37101873, 1}

func init() {
	cryptobinpkcs7.AddKeySign(externalCMSKeySignRegistration, func() cryptobinpkcs7.KeySign {
		return externalCMSKeySign{}
	})
}

type externalCMSKeySign struct{}

func (externalCMSKeySign) OID() asn1.ObjectIdentifier {
	return cryptobinpkcs7.KeySignWithRSASHA256.OID()
}

func (externalCMSKeySign) HashOID() asn1.ObjectIdentifier {
	return cryptobinpkcs7.KeySignWithRSASHA256.HashOID()
}

func (externalCMSKeySign) Check(pkey any) bool {
	signer, ok := pkey.(*externalRSASigner)
	return ok && signer != nil && signer.publicKey != nil
}

func (externalCMSKeySign) Sign(pkey crypto.PrivateKey, data []byte) ([]byte, []byte, error) {
	signer, ok := pkey.(*externalRSASigner)
	if !ok || signer == nil {
		return nil, nil, errors.New("firmador externo no disponible")
	}
	digest := sha256.Sum256(data)
	signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, nil, err
	}
	return digest[:], signature, nil
}

func (externalCMSKeySign) Verify(pkey crypto.PublicKey, data, signature []byte) (bool, error) {
	public, ok := pkey.(*rsa.PublicKey)
	if !ok || public == nil {
		return false, errors.New("clave pública RSA no válida")
	}
	digest := sha256.Sum256(data)
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature); err != nil {
		return false, err
	}
	return true, nil
}
