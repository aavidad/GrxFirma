// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"

	"grxfirma/internal/domain"
)

const (
	cmsAuthEnvelopedCEKBytes        = 32
	cmsAuthEnvelopedNonceBytes      = 12
	cmsAuthEnvelopedTagBytes        = 16
	cmsAuthEnvelopedMinRSABits      = 2048
	cmsAuthEnvelopedMaxRSABits      = 8192
	maxCMSAuthEnvelopedBytes        = 64 << 20
	maxCMSAuthEnvelopedDERBytes     = maxCMSAuthEnvelopedBytes + (2 << 20)
	maxCMSAuthEnvelopedRecipients   = 64
	maxCMSAuthEnvelopedIssuerBytes  = 16 << 10
	maxCMSAuthEnvelopedSerialBytes  = 20
	maxCMSAuthEnvelopedEncryptedKey = cmsAuthEnvelopedMaxRSABits / 8
)

var (
	oidCMSAES256GCM  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 46}
	oidCMSRSAESOAEP  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 7}
	oidCMSMGF1       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 8}
	oidCMSPSpecified = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 9}

	errCMSAuthEnvelopedIntegrity = errors.New("integridad criptografica de AuthEnvelopedData no valida")
	errCMSAuthEnvelopedDecrypt   = errors.New("no se pudo desproteger el AuthEnvelopedData con las claves disponibles")
)

type cmsAuthEnvelopedData struct {
	Version                  int
	RecipientInfos           []cmsKeyTransRecipientInfo `asn1:"set"`
	AuthEncryptedContentInfo cmsAuthEncryptedContentInfo
	MAC                      []byte
}

type cmsKeyTransRecipientInfo struct {
	Version                int
	IssuerAndSerialNumber  cmsIssuerAndSerialNumber
	KeyEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedKey           []byte
}

type cmsIssuerAndSerialNumber struct {
	IssuerName   asn1.RawValue
	SerialNumber *big.Int
}

type cmsAuthEncryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           asn1.RawValue
}

type cmsGCMParameters struct {
	Nonce  []byte
	ICVLen int `asn1:"optional,default:12"`
}

type cmsRSAESOAEPParameters struct {
	HashAlgorithm    pkix.AlgorithmIdentifier `asn1:"explicit,optional,tag:0"`
	MaskGenAlgorithm pkix.AlgorithmIdentifier `asn1:"explicit,optional,tag:1"`
	PSourceAlgorithm pkix.AlgorithmIdentifier `asn1:"explicit,optional,tag:2"`
}

func encryptCMSAuthEnvelopedData(content []byte, recipients []domain.ProtectionRecipient) ([]byte, int, error) {
	if len(content) > maxCMSAuthEnvelopedBytes {
		return nil, 0, errors.New("tamano de contenido AuthEnvelopedData no permitido")
	}
	if len(recipients) == 0 || len(recipients) > maxCMSAuthEnvelopedRecipients {
		return nil, 0, errors.New("cardinalidad de destinatarios AuthEnvelopedData no permitida")
	}

	cek := make([]byte, cmsAuthEnvelopedCEKBytes)
	if _, err := io.ReadFull(rand.Reader, cek); err != nil {
		return nil, 0, fmt.Errorf("generando CEK AuthEnvelopedData: %w", err)
	}
	defer zeroBytes(cek)

	nonce := make([]byte, cmsAuthEnvelopedNonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, 0, fmt.Errorf("generando nonce AuthEnvelopedData: %w", err)
	}
	defer zeroBytes(nonce)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, 0, fmt.Errorf("inicializando AES-256 para AuthEnvelopedData: %w", err)
	}
	aead, err := cipher.NewGCMWithNonceSize(block, cmsAuthEnvelopedNonceBytes)
	if err != nil {
		return nil, 0, fmt.Errorf("inicializando AES-GCM para AuthEnvelopedData: %w", err)
	}
	sealed := aead.Seal(nil, nonce, content, nil)
	if len(sealed) < cmsAuthEnvelopedTagBytes {
		zeroBytes(sealed)
		return nil, 0, errors.New("resultado AES-GCM AuthEnvelopedData no valido")
	}
	ciphertext := sealed[:len(sealed)-cmsAuthEnvelopedTagBytes]
	mac := sealed[len(sealed)-cmsAuthEnvelopedTagBytes:]

	recipientInfos := make([]cmsKeyTransRecipientInfo, 0, len(recipients))
	oaepAlgorithm, err := cmsRSAESOAEP256AlgorithmIdentifier()
	if err != nil {
		zeroBytes(sealed)
		return nil, 0, err
	}
	for _, recipient := range recipients {
		cert, err := parseCMSRecipientCertificate(recipient, domain.ProtectionProfileCompat)
		if err != nil {
			zeroBytes(sealed)
			return nil, 0, err
		}
		publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			zeroBytes(sealed)
			return nil, 0, fmt.Errorf("el destinatario '%s' no usa clave RSA compatible con AuthEnvelopedData", recipient.ID)
		}
		if !validCMSAuthEnvelopedRSAKeySize(publicKey) {
			zeroBytes(sealed)
			return nil, 0, fmt.Errorf("la clave RSA del destinatario '%s' debe tener entre %d y %d bits", recipient.ID, cmsAuthEnvelopedMinRSABits, cmsAuthEnvelopedMaxRSABits)
		}
		if !validCMSAuthEnvelopedIssuerAndSerial(cert.RawIssuer, cert.SerialNumber) {
			zeroBytes(sealed)
			return nil, 0, fmt.Errorf("el emisor o numero de serie del destinatario '%s' excede el perfil AuthEnvelopedData", recipient.ID)
		}
		encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, cek, nil)
		if err != nil {
			zeroBytes(sealed)
			return nil, 0, fmt.Errorf("cifrando CEK AuthEnvelopedData para '%s': %w", recipient.ID, err)
		}
		recipientInfos = append(recipientInfos, cmsKeyTransRecipientInfo{
			Version: 0,
			IssuerAndSerialNumber: cmsIssuerAndSerialNumber{
				IssuerName:   asn1.RawValue{FullBytes: cert.RawIssuer},
				SerialNumber: new(big.Int).Set(cert.SerialNumber),
			},
			KeyEncryptionAlgorithm: oaepAlgorithm,
			EncryptedKey:           encryptedKey,
		})
	}

	gcmParameters, err := asn1.Marshal(cmsGCMParameters{
		Nonce:  append([]byte(nil), nonce...),
		ICVLen: cmsAuthEnvelopedTagBytes,
	})
	if err != nil {
		zeroBytes(sealed)
		return nil, 0, fmt.Errorf("serializando parametros AES-GCM AuthEnvelopedData: %w", err)
	}
	inner, err := asn1.Marshal(cmsAuthEnvelopedData{
		Version:        0,
		RecipientInfos: recipientInfos,
		AuthEncryptedContentInfo: cmsAuthEncryptedContentInfo{
			ContentType: oidCMSData,
			ContentEncryptionAlgorithm: pkix.AlgorithmIdentifier{
				Algorithm:  oidCMSAES256GCM,
				Parameters: asn1.RawValue{FullBytes: gcmParameters},
			},
			EncryptedContent: asn1.RawValue{
				Class: 2,
				Tag:   0,
				Bytes: append([]byte(nil), ciphertext...),
			},
		},
		MAC: append([]byte(nil), mac...),
	})
	zeroBytes(sealed)
	if err != nil {
		return nil, 0, fmt.Errorf("serializando AuthEnvelopedData: %w", err)
	}
	wrapper, err := asn1.Marshal(cmsContentInfo{
		ContentType: oidCMSAuthEnvelopedData,
		Content: asn1.RawValue{
			Class:      2,
			Tag:        0,
			IsCompound: true,
			Bytes:      inner,
		},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("serializando ContentInfo AuthEnvelopedData: %w", err)
	}
	if len(wrapper) > maxCMSAuthEnvelopedDERBytes {
		zeroBytes(wrapper)
		return nil, 0, errors.New("tamano final de AuthEnvelopedData no permitido")
	}
	return wrapper, len(recipientInfos), nil
}

func decryptCMSAuthEnvelopedData(ctx context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
	if err := ctx.Err(); err != nil {
		return domain.UnprotectedPayload{}, err
	}
	envelope, err := parseCMSAuthEnvelopedData(protected.Content)
	if err != nil {
		return domain.UnprotectedPayload{}, err
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return domain.UnprotectedPayload{}, err
		}
		if !key.Supports(domain.ProtectionProfileCompat) {
			continue
		}
		cert, privateKey, err := parseCMSRSAKeyMaterial(key)
		if err != nil {
			continue
		}
		if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
			continue
		}
		publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok || !validCMSAuthEnvelopedRSAKeySize(publicKey) {
			continue
		}
		for _, recipient := range envelope.RecipientInfos {
			if !cmsRecipientMatchesCertificate(recipient, cert) {
				continue
			}
			if len(recipient.EncryptedKey) != privateKey.Size() {
				continue
			}
			cek, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, recipient.EncryptedKey, nil)
			if err != nil {
				continue
			}
			plaintext, err := openCMSAuthEnvelopedContent(envelope, cek)
			zeroBytes(cek)
			if err != nil {
				continue
			}
			doc, err := domain.NewDocument(unprotectedCMSFileName(protected.Name), plaintext, inferUnprotectedMIME(protected.MIMEType))
			if err != nil {
				zeroBytes(plaintext)
				return domain.UnprotectedPayload{}, err
			}
			return domain.UnprotectedPayload{
				Document:    doc,
				Profile:     domain.ProtectionProfileCompat,
				RecipientID: key.RecipientID,
			}, nil
		}
	}
	// No distinguimos ausencia de clave, fallo OAEP y tag GCM incorrecto: esa
	// diferencia convertiría la API local en un oráculo de descifrado.
	return domain.UnprotectedPayload{}, errCMSAuthEnvelopedDecrypt
}

func parseCMSAuthEnvelopedData(data []byte) (cmsAuthEnvelopedData, error) {
	if len(data) == 0 || len(data) > maxCMSAuthEnvelopedDERBytes {
		return cmsAuthEnvelopedData{}, errors.New("tamano de AuthEnvelopedData no permitido")
	}
	var info cmsContentInfo
	rest, err := asn1.Unmarshal(data, &info)
	if err != nil || len(rest) != 0 || !info.ContentType.Equal(oidCMSAuthEnvelopedData) {
		return cmsAuthEnvelopedData{}, errors.New("contenedor AuthEnvelopedData no valido")
	}
	if !canonicalCMSAuthEnvelopedDER(data, info) {
		return cmsAuthEnvelopedData{}, errors.New("ContentInfo AuthEnvelopedData no es DER canonico")
	}
	var envelope cmsAuthEnvelopedData
	rest, err = asn1.Unmarshal(info.Content.Bytes, &envelope)
	if err != nil || len(rest) != 0 {
		return cmsAuthEnvelopedData{}, errors.New("estructura AuthEnvelopedData no valida")
	}
	if !canonicalCMSAuthEnvelopedDER(info.Content.Bytes, envelope) {
		return cmsAuthEnvelopedData{}, errors.New("estructura AuthEnvelopedData no es DER canonico")
	}
	if envelope.Version != 0 ||
		len(envelope.RecipientInfos) == 0 ||
		len(envelope.RecipientInfos) > maxCMSAuthEnvelopedRecipients {
		return cmsAuthEnvelopedData{}, errors.New("version o cardinalidad AuthEnvelopedData no permitida")
	}
	if !envelope.AuthEncryptedContentInfo.ContentType.Equal(oidCMSData) ||
		!envelope.AuthEncryptedContentInfo.ContentEncryptionAlgorithm.Algorithm.Equal(oidCMSAES256GCM) {
		return cmsAuthEnvelopedData{}, errors.New("AuthEnvelopedData requiere contenido data cifrado con AES-256-GCM")
	}
	encryptedContent := envelope.AuthEncryptedContentInfo.EncryptedContent
	if encryptedContent.Class != 2 || encryptedContent.Tag != 0 || encryptedContent.IsCompound ||
		len(encryptedContent.Bytes) > maxCMSAuthEnvelopedBytes {
		return cmsAuthEnvelopedData{}, errors.New("contenido cifrado AuthEnvelopedData no valido")
	}
	if len(envelope.MAC) != cmsAuthEnvelopedTagBytes {
		return cmsAuthEnvelopedData{}, errors.New("tag AES-GCM AuthEnvelopedData no valido")
	}
	if _, err := parseCMSAuthEnvelopedGCMParameters(envelope.AuthEncryptedContentInfo.ContentEncryptionAlgorithm.Parameters); err != nil {
		return cmsAuthEnvelopedData{}, err
	}
	for _, recipient := range envelope.RecipientInfos {
		if recipient.Version != 0 ||
			!validCMSAuthEnvelopedIssuerAndSerial(
				recipient.IssuerAndSerialNumber.IssuerName.FullBytes,
				recipient.IssuerAndSerialNumber.SerialNumber,
			) ||
			len(recipient.EncryptedKey) == 0 ||
			len(recipient.EncryptedKey) > maxCMSAuthEnvelopedEncryptedKey {
			return cmsAuthEnvelopedData{}, errors.New("destinatario AuthEnvelopedData no valido")
		}
		if err := validateCMSRSAESOAEP256Algorithm(recipient.KeyEncryptionAlgorithm); err != nil {
			return cmsAuthEnvelopedData{}, err
		}
	}
	return envelope, nil
}

func openCMSAuthEnvelopedContent(envelope cmsAuthEnvelopedData, cek []byte) ([]byte, error) {
	if len(cek) != cmsAuthEnvelopedCEKBytes {
		return nil, errors.New("CEK AuthEnvelopedData no valida")
	}
	nonce, err := parseCMSAuthEnvelopedGCMParameters(envelope.AuthEncryptedContentInfo.ContentEncryptionAlgorithm.Parameters)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, errors.New("CEK AES-256 AuthEnvelopedData no valida")
	}
	aead, err := cipher.NewGCMWithNonceSize(block, cmsAuthEnvelopedNonceBytes)
	if err != nil {
		return nil, errors.New("parametros AES-GCM AuthEnvelopedData no validos")
	}
	sealed := make([]byte, 0, len(envelope.AuthEncryptedContentInfo.EncryptedContent.Bytes)+len(envelope.MAC))
	sealed = append(sealed, envelope.AuthEncryptedContentInfo.EncryptedContent.Bytes...)
	sealed = append(sealed, envelope.MAC...)
	plaintext, err := aead.Open(nil, nonce, sealed, nil)
	zeroBytes(sealed)
	if err != nil {
		return nil, errCMSAuthEnvelopedIntegrity
	}
	return plaintext, nil
}

func cmsRSAESOAEP256AlgorithmIdentifier() (pkix.AlgorithmIdentifier, error) {
	hashAlgorithm := pkix.AlgorithmIdentifier{Algorithm: oidCMSSHA256}
	hashDER, err := asn1.Marshal(hashAlgorithm)
	if err != nil {
		return pkix.AlgorithmIdentifier{}, fmt.Errorf("serializando SHA-256 para RSA-OAEP: %w", err)
	}
	parameters, err := asn1.Marshal(cmsRSAESOAEPParameters{
		HashAlgorithm: hashAlgorithm,
		MaskGenAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm:  oidCMSMGF1,
			Parameters: asn1.RawValue{FullBytes: hashDER},
		},
	})
	if err != nil {
		return pkix.AlgorithmIdentifier{}, fmt.Errorf("serializando parametros RSA-OAEP-SHA-256: %w", err)
	}
	return pkix.AlgorithmIdentifier{
		Algorithm:  oidCMSRSAESOAEP,
		Parameters: asn1.RawValue{FullBytes: parameters},
	}, nil
}

func validateCMSRSAESOAEP256Algorithm(algorithm pkix.AlgorithmIdentifier) error {
	if !algorithm.Algorithm.Equal(oidCMSRSAESOAEP) || len(algorithm.Parameters.FullBytes) == 0 {
		return errors.New("AuthEnvelopedData requiere transporte RSA-OAEP-SHA-256")
	}
	var parameters cmsRSAESOAEPParameters
	rest, err := asn1.Unmarshal(algorithm.Parameters.FullBytes, &parameters)
	if err != nil || len(rest) != 0 ||
		!canonicalCMSAuthEnvelopedDER(algorithm.Parameters.FullBytes, parameters) ||
		!parameters.HashAlgorithm.Algorithm.Equal(oidCMSSHA256) ||
		!parameters.MaskGenAlgorithm.Algorithm.Equal(oidCMSMGF1) ||
		!validCMSAlgorithmNullOrAbsent(parameters.HashAlgorithm.Parameters) {
		return errors.New("parametros RSA-OAEP-SHA-256 AuthEnvelopedData no validos")
	}
	var mgfHash pkix.AlgorithmIdentifier
	rest, err = asn1.Unmarshal(parameters.MaskGenAlgorithm.Parameters.FullBytes, &mgfHash)
	if err != nil || len(rest) != 0 ||
		!canonicalCMSAuthEnvelopedDER(parameters.MaskGenAlgorithm.Parameters.FullBytes, mgfHash) ||
		!mgfHash.Algorithm.Equal(oidCMSSHA256) ||
		!validCMSAlgorithmNullOrAbsent(mgfHash.Parameters) {
		return errors.New("MGF1 de AuthEnvelopedData debe usar SHA-256")
	}
	if len(parameters.PSourceAlgorithm.Algorithm) != 0 {
		if !parameters.PSourceAlgorithm.Algorithm.Equal(oidCMSPSpecified) {
			return errors.New("pSource RSA-OAEP AuthEnvelopedData no soportado")
		}
		var label []byte
		rest, err = asn1.Unmarshal(parameters.PSourceAlgorithm.Parameters.FullBytes, &label)
		if err != nil || len(rest) != 0 || len(label) != 0 {
			return errors.New("RSA-OAEP AuthEnvelopedData requiere etiqueta vacia")
		}
	}
	return nil
}

func validCMSAlgorithmNullOrAbsent(parameters asn1.RawValue) bool {
	if len(parameters.FullBytes) == 0 {
		return true
	}
	var value asn1.RawValue
	rest, err := asn1.Unmarshal(parameters.FullBytes, &value)
	return err == nil && len(rest) == 0 &&
		value.Class == 0 && value.Tag == asn1.TagNull &&
		!value.IsCompound && len(value.Bytes) == 0
}

func parseCMSAuthEnvelopedGCMParameters(raw asn1.RawValue) ([]byte, error) {
	if len(raw.FullBytes) == 0 {
		return nil, errors.New("parametros AES-GCM AuthEnvelopedData ausentes")
	}
	var parameters cmsGCMParameters
	rest, err := asn1.Unmarshal(raw.FullBytes, &parameters)
	if err != nil || len(rest) != 0 ||
		!canonicalCMSAuthEnvelopedDER(raw.FullBytes, parameters) ||
		len(parameters.Nonce) != cmsAuthEnvelopedNonceBytes ||
		parameters.ICVLen != cmsAuthEnvelopedTagBytes {
		return nil, errors.New("parametros AES-256-GCM AuthEnvelopedData no validos")
	}
	return append([]byte(nil), parameters.Nonce...), nil
}

func cmsRecipientMatchesCertificate(recipient cmsKeyTransRecipientInfo, cert *x509.Certificate) bool {
	if cert == nil || recipient.IssuerAndSerialNumber.SerialNumber == nil {
		return false
	}
	return cert.SerialNumber.Cmp(recipient.IssuerAndSerialNumber.SerialNumber) == 0 &&
		bytes.Equal(cert.RawIssuer, recipient.IssuerAndSerialNumber.IssuerName.FullBytes)
}

func validCMSAuthEnvelopedRSAKeySize(publicKey *rsa.PublicKey) bool {
	if publicKey == nil || publicKey.N == nil {
		return false
	}
	bits := publicKey.N.BitLen()
	return bits >= cmsAuthEnvelopedMinRSABits && bits <= cmsAuthEnvelopedMaxRSABits
}

func validCMSAuthEnvelopedIssuerAndSerial(issuerDER []byte, serial *big.Int) bool {
	if len(issuerDER) == 0 || len(issuerDER) > maxCMSAuthEnvelopedIssuerBytes ||
		serial == nil || serial.Sign() <= 0 || len(serial.Bytes()) > maxCMSAuthEnvelopedSerialBytes {
		return false
	}
	var issuer asn1.RawValue
	rest, err := asn1.Unmarshal(issuerDER, &issuer)
	return err == nil && len(rest) == 0 &&
		issuer.Class == 0 && issuer.Tag == asn1.TagSequence && issuer.IsCompound
}

func canonicalCMSAuthEnvelopedDER(raw []byte, value any) bool {
	canonical, err := asn1.Marshal(value)
	return err == nil && bytes.Equal(raw, canonical)
}
