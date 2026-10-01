// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs12importer

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

const maxIdentityCertificates = 64

func certificadoCorrespondeAClave(cert *x509.Certificate, signer crypto.Signer) bool {
	if cert == nil || signer == nil {
		return false
	}
	publicDER, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return false
	}
	certificatePublicDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return false
	}
	// Comparar la clave, no detalles de codificación del SPKI original.
	return bytes.Equal(publicDER, certificatePublicDER)
}

func construirIdentidadConCadena(signer crypto.Signer, leaf *x509.Certificate, candidates []*x509.Certificate) (IdentidadImportada, error) {
	if !certificadoCorrespondeAClave(leaf, signer) {
		return IdentidadImportada{}, errors.New("el certificado no corresponde a la clave privada")
	}
	if len(candidates) > maxIdentityCertificates {
		return IdentidadImportada{}, errors.New("la credencial contiene demasiados certificados")
	}
	identity := IdentidadImportada{Reference: construirReference(leaf), Signer: signer, Certificate: leaf}
	_, identity.Reference.HasLocalDecryptionKey = signer.(*rsa.PrivateKey)
	seen := map[string]bool{string(leaf.Raw): true}
	current := leaf
	for len(identity.Chain) < maxIdentityCertificates {
		if bytes.Equal(current.RawIssuer, current.RawSubject) && current.CheckSignatureFrom(current) == nil {
			break
		}
		var issuer *x509.Certificate
		for _, candidate := range candidates {
			if candidate == nil || seen[string(candidate.Raw)] || !bytes.Equal(current.RawIssuer, candidate.RawSubject) {
				continue
			}
			if current.CheckSignatureFrom(candidate) == nil {
				issuer = candidate
				break
			}
		}
		if issuer == nil {
			break
		} // Una cadena parcial sigue siendo utilizable.
		seen[string(issuer.Raw)] = true
		identity.Chain = append(identity.Chain, issuer)
		identity.Reference.ChainDER = append(identity.Reference.ChainDER, append([]byte(nil), issuer.Raw...))
		current = issuer
	}
	return identity, nil
}

func parseCertificatesPEM(data []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	for len(data) > 0 {
		block, next := pem.Decode(data)
		if block == nil {
			break
		}
		data = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		if len(certificates) >= maxIdentityCertificates {
			return nil, errors.New("demasiados certificados PEM")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("certificado PEM inválido")
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 {
		return nil, errors.New("no se encontró ningún bloque CERTIFICATE")
	}
	return certificates, nil
}
