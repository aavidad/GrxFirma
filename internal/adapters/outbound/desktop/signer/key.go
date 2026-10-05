// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package signer implementa ports.SignerEngine usando exclusivamente la
// biblioteca estandar de Go (crypto/*, encoding/asn1, math/big).
// En Fase 2 se cubre CAdES-BES detached. XAdES y PAdES se anadiran en Fase 3.
package signer

import (
	"crypto"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- SHA-1 is available only for explicitly enabled V1.9 signature interoperability.
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"grxfirma/internal/ports"
	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/signingpolicy"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

// ClaveLocal implementa ports.SigningKey para claves cargadas localmente
// (P12, almacen de sistema, o cualquier crypto.Signer).
// Lleva consigo la clave privada y el certificado de firma.
type ClaveLocal struct {
	id    string
	priv  crypto.Signer
	cert  *x509.Certificate
	chain []*x509.Certificate
	// lote, si no es nil, permite autorizar a la vez las firmas de un lote
	// (véase key_lote.go).
	lote AutorizadorLote
}

// NuevaClaveLocal crea una ClaveLocal a partir de una clave privada y su certificado.
// El identificador se deriva de los primeros 8 bytes de la huella SHA-256 del certificado.
func NuevaClaveLocal(priv crypto.Signer, cert *x509.Certificate) *ClaveLocal {
	h := sha256.Sum256(cert.Raw)
	return &ClaveLocal{
		id:   hex.EncodeToString(h[:8]),
		priv: priv,
		cert: cert,
	}
}

// NuevaClaveLocalConCadena crea una ClaveLocal incluyendo la cadena de certificación
// inmediata para perfiles avanzados que requieren evidencias de revocación.
func NuevaClaveLocalConCadena(priv crypto.Signer, cert *x509.Certificate, chain []*x509.Certificate) *ClaveLocal {
	clave := NuevaClaveLocal(priv, cert)
	clave.chain = append([]*x509.Certificate(nil), chain...)
	return clave
}

// KeyID devuelve un identificador corto derivado de la huella del certificado.
func (c *ClaveLocal) KeyID() string { return c.id }

// ToLocalSigningKey convierte la ClaveLocal a la representacion comun de clave
// que usa el motor CAdES compartido (common/signer).
func (c *ClaveLocal) ToLocalSigningKey() *commonsigner.LocalSigningKey {
	return &commonsigner.LocalSigningKey{
		ID:          c.id,
		Signer:      c.priv,
		Certificate: c.cert,
		Chain:       append([]*x509.Certificate(nil), c.chain...),
	}
}

// SignDigest firma directamente un digest ya calculado. Se usa en protocolos
// trifásicos legacy donde la fase local solo debe producir el PKCS#1 del PRE.
func (c *ClaveLocal) SignDigest(digest []byte, hash crypto.Hash) ([]byte, error) {
	if err := c.validateSigningIdentity(time.Now()); err != nil {
		return nil, err
	}
	if hash == crypto.SHA1 {
		if err := cryptopolicy.RequireLegacySHA1(); err != nil {
			return nil, err
		}
	}
	return c.priv.Sign(rand.Reader, digest, hash)
}

// SignPreData firma un PRE trifásico manteniendo la semántica de V1:
// calcular el hash indicado por el algoritmo y firmar ese digest con PKCS#1.
// options queda reservado para paridad futura con stores externos.
func (c *ClaveLocal) SignPreData(preData []byte, algorithm string, _ map[string]string) (string, error) {
	if err := c.validateSigningIdentity(time.Now()); err != nil {
		return "", err
	}
	hash, err := hashDesdeAlgoritmoLegacy(algorithm)
	if err != nil {
		return "", err
	}
	var digest []byte
	switch hash {
	case crypto.SHA1:
		if err := cryptopolicy.RequireLegacySHA1(); err != nil {
			return "", err
		}
		sum := sha1.Sum(preData) // #nosec G401 -- guarded legacy signature path; SHA-256+ remains the default.
		digest = sum[:]
	case crypto.SHA256:
		sum := sha256.Sum256(preData)
		digest = sum[:]
	case crypto.SHA384:
		sum := sha512.Sum384(preData)
		digest = sum[:]
	case crypto.SHA512:
		sum := sha512.Sum512(preData)
		digest = sum[:]
	default:
		return "", fmt.Errorf("hash legacy no soportado: %v", hash)
	}
	firma, err := c.priv.Sign(rand.Reader, digest, hash)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(firma), nil
}

func (c *ClaveLocal) validateSigningIdentity(now time.Time) error {
	if c == nil || c.priv == nil || c.cert == nil {
		return &signingpolicy.Error{Reason: signingpolicy.MissingIdentity}
	}
	return signingpolicy.ValidateCertificateDER(c.cert.Raw, now)
}

func hashDesdeAlgoritmoLegacy(raw string) (crypto.Hash, error) {
	switch upper := strings.ToUpper(strings.TrimSpace(raw)); {
	case strings.Contains(upper, "SHA512"):
		return crypto.SHA512, nil
	case strings.Contains(upper, "SHA384"):
		return crypto.SHA384, nil
	case strings.Contains(upper, "SHA256"):
		return crypto.SHA256, nil
	case strings.Contains(upper, "SHA1"):
		return crypto.SHA1, nil
	default:
		return 0, fmt.Errorf("algoritmo legacy no soportado: %s", raw)
	}
}

// CertificateChainDER devuelve la cadena de certificados en DER en orden
// hoja -> emisores, sin duplicados.
func (c *ClaveLocal) CertificateChainDER() [][]byte {
	if c == nil || c.cert == nil {
		return nil
	}
	out := make([][]byte, 0, 1+len(c.chain))
	seen := make(map[string]struct{}, 1+len(c.chain))
	appendCert := func(cert *x509.Certificate) {
		if cert == nil || len(cert.Raw) == 0 {
			return
		}
		key := string(cert.Raw)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, append([]byte(nil), cert.Raw...))
	}
	appendCert(c.cert)
	for _, cert := range c.chain {
		appendCert(cert)
	}
	return out
}

// Comprobar en tiempo de compilacion que ClaveLocal implementa ports.SigningKey.
var _ ports.SigningKey = (*ClaveLocal)(nil)
