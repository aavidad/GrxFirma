// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/domain"
)

const (
	envelopeVersion        = 1
	strongRecipientAlg     = "mlkem768+x25519+hkdf-sha384+a256gcm-wrap"
	compatRecipientAlg     = "rsa-oaep-sha256"
	contentAEADAlg         = "aes-256-gcm"
	envelopeInfoStrongKEK  = "GrxFirma envelope strong KEK v1"
	envelopeInfoContentAAD = "GrxFirma envelope content v1"
	envelopeLabelCompat    = "GrxFirma envelope compat v1"
)

type EnvelopeProtector struct{}

func NuevoEnvelopeProtector() *EnvelopeProtector {
	return &EnvelopeProtector{}
}

type protectedEnvelope struct {
	Version    int                          `json:"version"`
	Profile    string                       `json:"profile"`
	Algorithm  string                       `json:"algorithm"`
	Name       string                       `json:"name"`
	MIMEType   string                       `json:"mime_type"`
	NonceB64   string                       `json:"nonce_b64"`
	CipherB64  string                       `json:"ciphertext_b64"`
	Recipients []protectedRecipientEnvelope `json:"recipients"`
}

type protectedRecipientEnvelope struct {
	ID                    string `json:"id"`
	Label                 string `json:"label,omitempty"`
	Algorithm             string `json:"algorithm"`
	WrappedKeyB64         string `json:"wrapped_key_b64,omitempty"`
	WrapNonceB64          string `json:"wrap_nonce_b64,omitempty"`
	MLKEMCiphertextB64    string `json:"mlkem_ciphertext_b64,omitempty"`
	X25519EphemeralPubB64 string `json:"x25519_ephemeral_public_b64,omitempty"`
}

func (p *EnvelopeProtector) Protect(_ context.Context, job domain.ProtectionJob, recipients []domain.ProtectionRecipient) (domain.ProtectedPayload, error) {
	if err := job.Validate(); err != nil {
		return domain.ProtectedPayload{}, err
	}
	if len(recipients) == 0 {
		return domain.ProtectedPayload{}, errors.New("debe existir al menos un destinatario de proteccion")
	}

	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("generando clave de contenido: %w", err)
	}
	defer zeroBytes(cek)

	env := protectedEnvelope{
		Version:    envelopeVersion,
		Profile:    string(job.Profile),
		Algorithm:  contentAEADAlg,
		Name:       job.Document.Name,
		MIMEType:   job.Document.MIMEType,
		Recipients: make([]protectedRecipientEnvelope, 0, len(recipients)),
	}

	for _, recipient := range recipients {
		if err := recipient.Validate(job.Profile); err != nil {
			return domain.ProtectedPayload{}, err
		}
		entry, err := p.wrapCEK(job.Profile, recipient, cek)
		if err != nil {
			return domain.ProtectedPayload{}, fmt.Errorf("envolviendo clave para '%s': %w", recipient.ID, err)
		}
		env.Recipients = append(env.Recipients, entry)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("generando nonce de contenido: %w", err)
	}
	defer zeroBytes(nonce)

	contentCipher, err := encryptAESGCM(cek, nonce, job.Document.Content, buildContentAAD(env))
	if err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("cifrando contenido: %w", err)
	}
	env.NonceB64 = base64.StdEncoding.EncodeToString(nonce)
	env.CipherB64 = base64.StdEncoding.EncodeToString(contentCipher)

	raw, err := json.Marshal(env)
	if err != nil {
		return domain.ProtectedPayload{}, fmt.Errorf("serializando sobre protegido: %w", err)
	}
	protectedDoc, err := domain.NewDocument(protectedFileName(job.Document.Name), raw, domain.MIMETypeProtectedEnvelope)
	if err != nil {
		return domain.ProtectedPayload{}, err
	}
	return domain.ProtectedPayload{
		Document:       protectedDoc,
		Profile:        job.Profile,
		RecipientCount: len(env.Recipients),
	}, nil
}

func (p *EnvelopeProtector) Unprotect(_ context.Context, protected domain.Document, keys []domain.ProtectionKeyMaterial) (domain.UnprotectedPayload, error) {
	var env protectedEnvelope
	if err := json.Unmarshal(protected.Content, &env); err != nil {
		return domain.UnprotectedPayload{}, fmt.Errorf("contenedor protegido no valido: %w", err)
	}
	profile := domain.ProtectionProfile(strings.TrimSpace(env.Profile))
	if err := profile.Validate(); err != nil {
		return domain.UnprotectedPayload{}, err
	}
	if len(env.Recipients) == 0 {
		return domain.UnprotectedPayload{}, errors.New("contenedor protegido sin destinatarios")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.NonceB64)
	if err != nil {
		return domain.UnprotectedPayload{}, fmt.Errorf("nonce de contenido no valido: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.CipherB64)
	if err != nil {
		return domain.UnprotectedPayload{}, fmt.Errorf("cifrado de contenido no valido: %w", err)
	}

	var (
		cek         []byte
		recipientID string
	)
	for _, key := range keys {
		if !key.Supports(profile) {
			continue
		}
		for _, recipient := range env.Recipients {
			if recipient.ID != key.RecipientID {
				continue
			}
			cek, err = p.unwrapCEK(profile, recipient, key)
			if err == nil {
				recipientID = recipient.ID
				break
			}
		}
		if len(cek) > 0 {
			break
		}
	}
	if len(cek) == 0 {
		return domain.UnprotectedPayload{}, errors.New("no se pudo desproteger el documento con las claves disponibles")
	}
	defer zeroBytes(cek)

	plaintext, err := decryptAESGCM(cek, nonce, ciphertext, buildContentAAD(env))
	if err != nil {
		return domain.UnprotectedPayload{}, fmt.Errorf("descifrando contenido: %w", err)
	}
	doc, err := domain.NewDocument(env.Name, plaintext, env.MIMEType)
	if err != nil {
		return domain.UnprotectedPayload{}, err
	}
	return domain.UnprotectedPayload{
		Document:    doc,
		Profile:     profile,
		RecipientID: recipientID,
	}, nil
}

func (p *EnvelopeProtector) wrapCEK(profile domain.ProtectionProfile, recipient domain.ProtectionRecipient, cek []byte) (protectedRecipientEnvelope, error) {
	switch profile {
	case domain.ProtectionProfileStrong:
		return wrapStrong(recipient, cek)
	case domain.ProtectionProfileCompat:
		return wrapCompat(recipient, cek)
	default:
		return protectedRecipientEnvelope{}, errors.New("perfil de proteccion no soportado")
	}
}

func (p *EnvelopeProtector) unwrapCEK(profile domain.ProtectionProfile, recipient protectedRecipientEnvelope, key domain.ProtectionKeyMaterial) ([]byte, error) {
	switch profile {
	case domain.ProtectionProfileStrong:
		return unwrapStrong(recipient, key)
	case domain.ProtectionProfileCompat:
		return unwrapCompat(recipient, key)
	default:
		return nil, errors.New("perfil de proteccion no soportado")
	}
}

func wrapStrong(recipient domain.ProtectionRecipient, cek []byte) (protectedRecipientEnvelope, error) {
	mlkemPub, err := mlkem.NewEncapsulationKey768(recipient.MLKEM768PublicKey)
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("clave publica ML-KEM-768 no valida: %w", err)
	}
	x25519Pub, err := ecdh.X25519().NewPublicKey(recipient.X25519PublicKey)
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("clave publica X25519 no valida: %w", err)
	}
	kemShared, kemCipher := mlkemPub.Encapsulate()
	defer zeroBytes(kemShared)
	ephPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("generando clave efimera X25519: %w", err)
	}
	ecdhShared, err := ephPriv.ECDH(x25519Pub)
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("acuerdo X25519: %w", err)
	}
	defer zeroBytes(ecdhShared)
	kek, err := deriveStrongKEK(kemShared, ecdhShared)
	if err != nil {
		return protectedRecipientEnvelope{}, err
	}
	defer zeroBytes(kek)
	wrapNonce := make([]byte, 12)
	if _, err := rand.Read(wrapNonce); err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("generando nonce de envoltura: %w", err)
	}
	defer zeroBytes(wrapNonce)
	wrapped, err := encryptAESGCM(kek, wrapNonce, cek, []byte(recipient.ID))
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("envolviendo CEK: %w", err)
	}
	return protectedRecipientEnvelope{
		ID:                    recipient.ID,
		Label:                 recipient.Label,
		Algorithm:             strongRecipientAlg,
		WrappedKeyB64:         base64.StdEncoding.EncodeToString(wrapped),
		WrapNonceB64:          base64.StdEncoding.EncodeToString(wrapNonce),
		MLKEMCiphertextB64:    base64.StdEncoding.EncodeToString(kemCipher),
		X25519EphemeralPubB64: base64.StdEncoding.EncodeToString(ephPriv.PublicKey().Bytes()),
	}, nil
}

func unwrapStrong(recipient protectedRecipientEnvelope, key domain.ProtectionKeyMaterial) ([]byte, error) {
	dk, err := mlkem.NewDecapsulationKey768(key.MLKEM768Seed)
	if err != nil {
		return nil, fmt.Errorf("clave privada ML-KEM-768 no valida: %w", err)
	}
	kemCipher, err := base64.StdEncoding.DecodeString(recipient.MLKEMCiphertextB64)
	if err != nil {
		return nil, fmt.Errorf("ciphertext ML-KEM no valido: %w", err)
	}
	kemShared, err := dk.Decapsulate(kemCipher)
	if err != nil {
		return nil, fmt.Errorf("decapsulado ML-KEM fallido: %w", err)
	}
	defer zeroBytes(kemShared)
	priv, err := ecdh.X25519().NewPrivateKey(key.X25519PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("clave privada X25519 no valida: %w", err)
	}
	ephPubRaw, err := base64.StdEncoding.DecodeString(recipient.X25519EphemeralPubB64)
	if err != nil {
		return nil, fmt.Errorf("clave publica efimera X25519 no valida: %w", err)
	}
	ephPub, err := ecdh.X25519().NewPublicKey(ephPubRaw)
	if err != nil {
		return nil, fmt.Errorf("clave publica efimera X25519 no valida: %w", err)
	}
	ecdhShared, err := priv.ECDH(ephPub)
	if err != nil {
		return nil, fmt.Errorf("acuerdo X25519 fallido: %w", err)
	}
	defer zeroBytes(ecdhShared)
	kek, err := deriveStrongKEK(kemShared, ecdhShared)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(kek)
	wrapNonce, err := base64.StdEncoding.DecodeString(recipient.WrapNonceB64)
	if err != nil {
		return nil, fmt.Errorf("nonce de envoltura no valido: %w", err)
	}
	wrapped, err := base64.StdEncoding.DecodeString(recipient.WrappedKeyB64)
	if err != nil {
		return nil, fmt.Errorf("clave envuelta no valida: %w", err)
	}
	return decryptAESGCM(kek, wrapNonce, wrapped, []byte(recipient.ID))
}

func wrapCompat(recipient domain.ProtectionRecipient, cek []byte) (protectedRecipientEnvelope, error) {
	pubAny, err := x509.ParsePKIXPublicKey(recipient.RSAOAEP256PublicKeyDER)
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("clave publica RSA no valida: %w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return protectedRecipientEnvelope{}, errors.New("la clave publica de compatibilidad no es RSA")
	}
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, cek, []byte(envelopeLabelCompat))
	if err != nil {
		return protectedRecipientEnvelope{}, fmt.Errorf("envoltura RSA-OAEP fallida: %w", err)
	}
	return protectedRecipientEnvelope{
		ID:            recipient.ID,
		Label:         recipient.Label,
		Algorithm:     compatRecipientAlg,
		WrappedKeyB64: base64.StdEncoding.EncodeToString(wrapped),
	}, nil
}

func unwrapCompat(recipient protectedRecipientEnvelope, key domain.ProtectionKeyMaterial) ([]byte, error) {
	privAny, err := x509.ParsePKCS8PrivateKey(key.RSAOAEP256PrivateKeyPKCS8)
	if err != nil {
		return nil, fmt.Errorf("clave privada RSA no valida: %w", err)
	}
	priv, ok := privAny.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("la clave privada de compatibilidad no es RSA")
	}
	wrapped, err := base64.StdEncoding.DecodeString(recipient.WrappedKeyB64)
	if err != nil {
		return nil, fmt.Errorf("clave envuelta no valida: %w", err)
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, wrapped, []byte(envelopeLabelCompat))
}

func deriveStrongKEK(kemShared, ecdhShared []byte) ([]byte, error) {
	material := make([]byte, 0, len(kemShared)+len(ecdhShared))
	material = append(material, kemShared...)
	material = append(material, ecdhShared...)
	defer zeroBytes(material)
	return hkdf.Key(sha512.New384, material, nil, envelopeInfoStrongKEK, 32)
}

func buildContentAAD(env protectedEnvelope) []byte {
	return []byte(strings.Join([]string{
		envelopeInfoContentAAD,
		fmt.Sprintf("v=%d", env.Version),
		"profile=" + env.Profile,
		"name=" + env.Name,
		"mime=" + env.MIMEType,
	}, "\n"))
}

func protectedFileName(name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "documento"
	}
	return base + ".afp"
}

func encryptAESGCM(key, nonce, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

func decryptAESGCM(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

func zeroBytes(buf []byte) {
	if len(buf) == 0 {
		return
	}
	clear(buf)
}
