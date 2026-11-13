// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"strings"

	"grxfirma/internal/domain"
)

// destinatariosDocumento resume a quién va dirigido un documento protegido sin
// descifrar nada. Sirve para pedir al almacén solo las claves privadas de los
// certificados que figuran como destinatarios (F-01): en NSS cada clave pedida
// se exporta del perfil del navegador a la memoria del proceso.
//
// Es una pista, no una decisión de confianza: el motor de desprotección sigue
// comprobando por su cuenta que la clave corresponde al destinatario. Un
// documento que no se puede interpretar no nombra a nadie y no carga ninguna
// clave.
type destinatariosDocumento struct {
	ids map[string]struct{}
	cms []byte
}

func destinatariosDe(protected domain.Document) destinatariosDocumento {
	content := protected.Content
	if len(content) == 0 {
		return destinatariosDocumento{}
	}
	if looksLikeJSONEnvelope(content) {
		var env protectedEnvelope
		if err := json.Unmarshal(content, &env); err != nil {
			return destinatariosDocumento{}
		}
		ids := make(map[string]struct{}, len(env.Recipients))
		for _, recipient := range env.Recipients {
			if id := strings.TrimSpace(recipient.ID); id != "" {
				ids[id] = struct{}{}
			}
		}
		return destinatariosDocumento{ids: ids}
	}
	switch contentType, err := detectCMSContentType(content); {
	case err != nil:
		return destinatariosDocumento{}
	case contentType == cmsContentTypeEnvelopedData,
		contentType == cmsContentTypeSignedEnvelopedData,
		contentType == cmsContentTypeAuthEnvelopedData:
		return destinatariosDocumento{cms: content}
	default:
		// EncryptedData usa un secreto compartido y el resto de tipos no se
		// descifran: no hace falta ninguna clave privada.
		return destinatariosDocumento{}
	}
}

func (d destinatariosDocumento) incluyeID(id string) bool {
	if d.ids == nil {
		return false
	}
	_, ok := d.ids[strings.TrimSpace(id)]
	return ok
}

// incluyeCertificado indica si el documento nombra como destinatario al
// certificado de ref: en el sobre .afp por su identificador y en CMS por el
// emisor y número de serie o por el identificador de clave del sujeto, que es
// como RecipientInfo identifica al destinatario (RFC 5652, 6.2.1).
func (d destinatariosDocumento) incluyeCertificado(ref domain.CertificateRef) bool {
	if d.ids != nil {
		return d.incluyeID(ref.ID)
	}
	if len(d.cms) == 0 || len(ref.DER) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(ref.DER)
	if err != nil {
		return false
	}
	if issuerAndSerial, err := asn1.Marshal(struct {
		Issuer asn1.RawValue
		Serial *big.Int
	}{
		Issuer: asn1.RawValue{FullBytes: cert.RawIssuer},
		Serial: cert.SerialNumber,
	}); err == nil && bytes.Contains(d.cms, issuerAndSerial) {
		return true
	}
	if ski := cert.SubjectKeyId; len(ski) > 0 && len(ski) < 128 {
		// [0] IMPLICIT SubjectKeyIdentifier dentro de RecipientIdentifier.
		tagged := append([]byte{0x80, byte(len(ski))}, ski...)
		if bytes.Contains(d.cms, tagged) {
			return true
		}
	}
	return false
}
