// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"io"
	"math/big"
	"sort"

	"github.com/deatil/go-cryptobin/ber"
	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"
)

// El builder SignedAndEnvelopedData no permite sustituir AddRecipient, que
// fija PKCS#1 v1.5. Capturamos la CEK en la interfaz Cipher de esa operación
// para construir el SET de destinatarios OAEP sin generar destinatarios v1.5.
type cmsCipherConCEK struct {
	cryptobinpkcs7.Cipher
	cek []byte
}

func (c *cmsCipherConCEK) Encrypt(random io.Reader, key, plaintext []byte) ([]byte, []byte, error) {
	// El builder conserva este mismo slice; guardamos su alias para borrarlo
	// al finalizar, también al usar el transporte heredado.
	c.cek = key
	return c.Cipher.Encrypt(random, key, plaintext)
}

// insertarDestinatariosSAED sustituye el SET vacío emitido por Finish() con
// destinatarios RSA-OAEP-SHA-256. El resto del sobre firmado se conserva byte
// por byte, incluidas firma, certificados y parámetros de AES-GCM.
func insertarDestinatariosSAED(blob, cek []byte, recipients []*x509.Certificate) ([]byte, error) {
	if len(cek) != cmsAuthEnvelopedCEKBytes || len(recipients) == 0 || len(recipients) > maxCMSAuthEnvelopedRecipients {
		return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.destinatarios_invalidos"))
	}
	algorithm, err := cmsRSAESOAEP256AlgorithmIdentifier()
	if err != nil {
		return nil, err
	}
	encoded := make([][]byte, 0, len(recipients))
	for _, cert := range recipients {
		if cert == nil || !validCMSAuthEnvelopedIssuerAndSerial(cert.RawIssuer, cert.SerialNumber) {
			return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.certificado_invalido"))
		}
		public, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok || !validCMSAuthEnvelopedRSAKeySize(public) {
			return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.rsa_no_admitido"))
		}
		wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, public, cek, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", localizador.Detectar().T("proteccion.cms.saed.clave_cifrado_error"), err)
		}
		raw, err := asn1.Marshal(cmsKeyTransRecipientInfo{
			Version: 1,
			IssuerAndSerialNumber: cmsIssuerAndSerialNumber{
				IssuerName:   asn1.RawValue{FullBytes: cert.RawIssuer},
				SerialNumber: new(big.Int).Set(cert.SerialNumber),
			},
			KeyEncryptionAlgorithm: algorithm,
			EncryptedKey:           wrapped,
		})
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, raw)
	}
	sort.Slice(encoded, func(i, j int) bool { return bytes.Compare(encoded[i], encoded[j]) < 0 })
	set, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: bytes.Join(encoded, nil)})
	if err != nil {
		return nil, err
	}

	var wrapper cmsContentInfo
	if rest, err := asn1.Unmarshal(blob, &wrapper); err != nil || len(rest) != 0 || !wrapper.ContentType.Equal(oidCMSSignedEnvelopedData) {
		return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.sobre_invalido"))
	}
	var sequence asn1.RawValue
	if rest, err := asn1.Unmarshal(wrapper.Content.Bytes, &sequence); err != nil || len(rest) != 0 || sequence.Tag != asn1.TagSequence {
		return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.estructura_invalida"))
	}
	var version, oldSet asn1.RawValue
	resto, err := asn1.Unmarshal(sequence.Bytes, &version)
	if err != nil || version.Tag != asn1.TagInteger {
		return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.version_invalida"))
	}
	resto, err = asn1.Unmarshal(resto, &oldSet)
	if err != nil || oldSet.Tag != asn1.TagSet || len(oldSet.Bytes) != 0 {
		return nil, errors.New(localizador.Detectar().T("proteccion.cms.saed.destinatarios_existentes"))
	}
	inner, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true,
		Bytes: append(append(append([]byte(nil), version.FullBytes...), set...), resto...),
	})
	if err != nil {
		return nil, err
	}
	wrapper.Content.Bytes, wrapper.Content.FullBytes = inner, nil
	return asn1.Marshal(wrapper)
}

type cmsSAEDDecrypter struct {
	private *rsa.PrivateKey
	oaep    bool
	cek     []byte
}

func (d *cmsSAEDDecrypter) Public() crypto.PublicKey { return &d.private.PublicKey }
func (d *cmsSAEDDecrypter) Decrypt(_ io.Reader, ciphertext []byte, _ crypto.DecrypterOpts) ([]byte, error) {
	var cek []byte
	var err error
	if d.oaep {
		cek, err = rsa.DecryptOAEP(sha256.New(), rand.Reader, d.private, ciphertext, nil)
	} else {
		cek, err = d.private.Decrypt(rand.Reader, ciphertext, nil)
	}
	if err == nil {
		d.cek = cek
	}
	return cek, err
}

// transporteSAEDParaCertificado lee el algoritmo del destinatario concreto
// antes de elegir el descifrador. Los parámetros OAEP se validan aquí: la
// biblioteca original no los comprueba en SignedAndEnvelopedData.
func transporteSAEDParaCertificado(blob []byte, cert *x509.Certificate) (bool, error) {
	cmsCryptobinParseMu.Lock()
	der, err := ber.Ber2der(blob)
	cmsCryptobinParseMu.Unlock()
	if err != nil {
		return false, err
	}
	var wrapper cmsContentInfo
	if rest, err := asn1.Unmarshal(der, &wrapper); err != nil || len(rest) != 0 || !wrapper.ContentType.Equal(oidCMSSignedEnvelopedData) {
		return false, errors.New(localizador.Detectar().T("proteccion.cms.saed.sobre_invalido"))
	}
	var sequence asn1.RawValue
	if rest, err := asn1.Unmarshal(wrapper.Content.Bytes, &sequence); err != nil || len(rest) != 0 || sequence.Tag != asn1.TagSequence {
		return false, errors.New(localizador.Detectar().T("proteccion.cms.saed.estructura_invalida"))
	}
	var version, set asn1.RawValue
	resto, err := asn1.Unmarshal(sequence.Bytes, &version)
	if err != nil {
		return false, err
	}
	_, err = asn1.Unmarshal(resto, &set)
	if err != nil || set.Tag != asn1.TagSet {
		return false, errors.New(localizador.Detectar().T("proteccion.cms.saed.destinatarios_invalidos"))
	}
	for bytesLeft := set.Bytes; len(bytesLeft) > 0; {
		var raw asn1.RawValue
		bytesLeft, err = asn1.Unmarshal(bytesLeft, &raw)
		if err != nil {
			return false, err
		}
		if raw.Tag != asn1.TagSequence {
			continue
		}
		var recipient cmsKeyTransRecipientInfo
		if tail, err := asn1.Unmarshal(raw.FullBytes, &recipient); err != nil || len(tail) != 0 {
			return false, errors.New(localizador.Detectar().T("proteccion.cms.saed.destinatario_invalido"))
		}
		if !cmsRecipientMatchesCertificate(recipient, cert) {
			continue
		}
		if recipient.KeyEncryptionAlgorithm.Algorithm.Equal(oidCMSRSAPKCS1v15) {
			return false, nil
		}
		if err := validateCMSRSAESOAEP256Algorithm(recipient.KeyEncryptionAlgorithm); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, errors.New(localizador.Detectar().T("proteccion.cms.saed.certificado_no_encontrado"))
}
