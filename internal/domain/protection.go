// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import "errors"

const MIMETypeProtectedEnvelope = "application/vnd.grxfirma.protected+json"
const MIMETypeProtectedCMS = "application/pkcs7-mime"

// ProtectionProfile describe el perfil criptográfico usado para proteger un fichero.
type ProtectionProfile string

const (
	ProtectionProfileStrong ProtectionProfile = "alto-mlkem768-aes256gcm"
	ProtectionProfileCompat ProtectionProfile = "compat-rsa-oaep-aes256gcm"
)

func (p ProtectionProfile) Validate() error {
	switch p {
	case ProtectionProfileStrong, ProtectionProfileCompat:
		return nil
	default:
		return errors.New("perfil de proteccion no soportado: " + string(p))
	}
}

// ProtectionRecipient contiene el material público mínimo necesario para proteger
// un fichero para un destinatario concreto.
type ProtectionRecipient struct {
	ID string
	// Origin identifica de dónde procede el certificado público.
	Origin string

	// Label es un texto de apoyo para UI/logs. Nunca contiene material sensible.
	Label string

	// RSAOAEP256PublicKeyDER contiene una SubjectPublicKeyInfo DER para RSA-OAEP.
	RSAOAEP256PublicKeyDER []byte

	// CertificateDER contiene el certificado X.509 del destinatario en DER.
	// Se usa para perfiles o contenedores de interoperabilidad CMS.
	CertificateDER []byte

	// MLKEM768PublicKey contiene la encapsulation key codificada de ML-KEM-768.
	MLKEM768PublicKey []byte

	// X25519PublicKey contiene la clave pública X25519 codificada en crudo.
	X25519PublicKey []byte
}

func (r ProtectionRecipient) Supports(profile ProtectionProfile) bool {
	switch profile {
	case ProtectionProfileCompat:
		return len(r.RSAOAEP256PublicKeyDER) > 0
	case ProtectionProfileStrong:
		return len(r.MLKEM768PublicKey) > 0 && len(r.X25519PublicKey) > 0
	default:
		return false
	}
}

func (r ProtectionRecipient) Validate(profile ProtectionProfile) error {
	if r.ID == "" {
		return errors.New("el destinatario de proteccion debe tener identificador")
	}
	if !r.Supports(profile) {
		return errors.New("el destinatario no soporta el perfil de proteccion solicitado")
	}
	return nil
}

// ProtectionKeyMaterial contiene el material privado mínimo necesario para
// descifrar un fichero protegido.
type ProtectionKeyMaterial struct {
	RecipientID string

	// RSAOAEP256PrivateKeyPKCS8 contiene una clave privada RSA en PKCS#8 DER.
	RSAOAEP256PrivateKeyPKCS8 []byte

	// CertificateDER contiene el certificado X.509 asociado a la clave privada
	// cuando el contenedor de protección necesita identificar al destinatario
	// por emisor y número de serie.
	CertificateDER []byte

	// MLKEM768Seed contiene la seed privada de ML-KEM-768 en el formato d||z.
	MLKEM768Seed []byte

	// X25519PrivateKey contiene la clave privada X25519 codificada en crudo.
	X25519PrivateKey []byte

	// SymmetricKey contiene una clave simétrica transitoria para contenedores
	// de compatibilidad interoperable como CMS EncryptedData. Nunca debe
	// persistirse en keyrings ni en configuración.
	SymmetricKey []byte
}

func (k ProtectionKeyMaterial) Supports(profile ProtectionProfile) bool {
	switch profile {
	case ProtectionProfileCompat:
		return len(k.RSAOAEP256PrivateKeyPKCS8) > 0 || len(k.SymmetricKey) > 0
	case ProtectionProfileStrong:
		return len(k.MLKEM768Seed) > 0 && len(k.X25519PrivateKey) > 0
	default:
		return false
	}
}

// ProtectionJob representa un trabajo de cifrado/protección de un documento.
type ProtectionJob struct {
	Document Document
	Profile  ProtectionProfile
	Options  map[string]string

	// SymmetricKey contiene exclusivamente una clave transitoria de la
	// operación actual. El llamador conserva su propiedad y debe borrarla.
	SymmetricKey []byte
}

func (j ProtectionJob) Validate() error {
	if err := j.Profile.Validate(); err != nil {
		return err
	}
	if j.Document.Size() == 0 {
		return errors.New("el trabajo de proteccion debe contener un documento no vacio")
	}
	return nil
}

// ProtectedPayload es el resultado de proteger un documento.
type ProtectedPayload struct {
	Document       Document
	Profile        ProtectionProfile
	RecipientCount int
}

// UnprotectedPayload es el resultado de desproteger un documento.
type UnprotectedPayload struct {
	Document    Document
	Profile     ProtectionProfile
	RecipientID string
}
