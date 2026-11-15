// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"fmt"

	"github.com/deatil/go-cryptobin/ber"
)

// go-cryptobin cifra con OAEP-SHA-1 cuando se selecciona el OID RSAES-OAEP.
// Este adaptador usa SHA-256 y añade sus parámetros ASN.1 al sobre.
type cmsOAEP256KeyEncrypt struct{}

func (cmsOAEP256KeyEncrypt) OID() asn1.ObjectIdentifier { return oidCMSRSAESOAEP }
func (cmsOAEP256KeyEncrypt) Check(key any) bool {
	_, ok := key.(*rsa.PublicKey)
	return ok
}
func (cmsOAEP256KeyEncrypt) Encrypt(plain []byte, key crypto.PublicKey) ([]byte, error) {
	public, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("clave CMS RSA no válida")
	}
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, public, plain, nil)
}
func (cmsOAEP256KeyEncrypt) Decrypt(ciphertext []byte, key crypto.PrivateKey) ([]byte, error) {
	private, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("clave CMS RSA no válida")
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, private, ciphertext, nil)
}

func transformarOAEP256(data []byte, salida bool) ([]byte, bool, error) {
	cmsCryptobinParseMu.Lock()
	der, err := ber.Ber2der(data)
	cmsCryptobinParseMu.Unlock()
	if err != nil {
		return nil, false, err
	}
	var wrapper cmsContentInfo
	if rest, err := asn1.Unmarshal(der, &wrapper); err != nil || len(rest) != 0 || !wrapper.ContentType.Equal(oidCMSEnvelopedData) {
		return nil, false, errors.New("sobre CMS EnvelopedData no válido")
	}
	var sobre asn1.RawValue
	if rest, err := asn1.Unmarshal(wrapper.Content.Bytes, &sobre); err != nil || len(rest) != 0 || sobre.Tag != asn1.TagSequence {
		return nil, false, errors.New("estructura CMS EnvelopedData no válida")
	}
	var version, campo asn1.RawValue
	resto, err := asn1.Unmarshal(sobre.Bytes, &version)
	if err != nil || version.Tag != asn1.TagInteger {
		return nil, false, errors.New("versión CMS EnvelopedData no válida")
	}
	cabecera := append([]byte(nil), version.FullBytes...)
	resto, err = asn1.Unmarshal(resto, &campo)
	if err != nil {
		return nil, false, errors.New("destinatarios CMS EnvelopedData ausentes")
	}
	if campo.Class == asn1.ClassContextSpecific && campo.Tag == 0 {
		cabecera = append(cabecera, campo.FullBytes...)
		resto, err = asn1.Unmarshal(resto, &campo)
		if err != nil {
			return nil, false, errors.New("destinatarios CMS EnvelopedData ausentes")
		}
	}
	if campo.Class != asn1.ClassUniversal || campo.Tag != asn1.TagSet || len(campo.Bytes) == 0 {
		return nil, false, errors.New("CMS EnvelopedData sin destinatarios")
	}
	changed := false
	var destinatarios []byte
	for conjunto := campo.Bytes; len(conjunto) > 0; {
		var raw asn1.RawValue
		conjunto, err = asn1.Unmarshal(conjunto, &raw)
		if err != nil {
			return nil, false, errors.New("destinatario CMS no válido")
		}
		if raw.Class != asn1.ClassUniversal || raw.Tag != asn1.TagSequence {
			// El preflight ya retiró los destinatarios de otros tipos.
			return nil, false, errors.New("destinatario CMS RSA no válido")
		}
		var recipient cmsKeyTransRecipientInfo
		if tail, err := asn1.Unmarshal(raw.FullBytes, &recipient); err != nil || len(tail) != 0 {
			return nil, false, errors.New("destinatario CMS RSA no válido")
		}
		alg := &recipient.KeyEncryptionAlgorithm
		if !alg.Algorithm.Equal(oidCMSRSAESOAEP) {
			destinatarios = append(destinatarios, raw.FullBytes...)
			continue
		}
		if salida {
			if len(alg.Parameters.FullBytes) != 0 {
				return nil, false, errors.New("RSA-OAEP de salida ya tiene parámetros")
			}
			*alg, err = cmsRSAESOAEP256AlgorithmIdentifier()
			if err != nil {
				return nil, false, err
			}
		} else {
			if len(alg.Parameters.FullBytes) == 0 {
				destinatarios = append(destinatarios, raw.FullBytes...)
				continue // OAEP-SHA-1 histórico de la biblioteca.
			}
			if err := validateCMSRSAESOAEP256Algorithm(*alg); err != nil {
				return nil, false, fmt.Errorf("parámetros RSA-OAEP del destinatario CMS: %w", err)
			}
			// La biblioteca identifica OAEP-SHA-256 con este OID, aunque no es
			// apropiado para un CMS externo. Solo lo usamos en su entrada local.
			alg.Algorithm = oidCMSSHA256WithRSA
			alg.Parameters = asn1.RawValue{}
		}
		codificado, err := asn1.Marshal(recipient)
		if err != nil {
			return nil, false, err
		}
		destinatarios = append(destinatarios, codificado...)
		changed = true
	}
	if !changed {
		return data, false, nil
	}
	setDER, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: destinatarios})
	if err != nil {
		return nil, false, err
	}
	inner, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: append(append(cabecera, setDER...), resto...)})
	if err != nil {
		return nil, false, err
	}
	wrapper.Content.Bytes = inner
	wrapper.Content.FullBytes = nil
	encoded, err := asn1.Marshal(wrapper)
	return encoded, true, err
}

func completarParametrosOAEP256(data []byte) ([]byte, error) {
	encoded, changed, err := transformarOAEP256(data, true)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, errors.New("el CMS nuevo no contiene destinatarios RSA-OAEP")
	}
	return encoded, nil
}
