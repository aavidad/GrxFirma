// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto"
	"encoding/asn1"
	"strings"
)

var (
	oidDigestSHA384           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidDigestSHA512           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	oidSignatureRSAWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	oidSignatureRSAWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
	oidSignatureECDSAWith384  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidSignatureECDSAWith512  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
)

// cmsHashSpec reúne los identificadores CMS de una función resumen admitida.
type cmsHashSpec struct {
	hash       crypto.Hash
	digestOID  asn1.ObjectIdentifier
	rsaOID     asn1.ObjectIdentifier
	ecdsaOID   asn1.ObjectIdentifier
	rsaLabel   string
	ecdsaLabel string
}

var cmsHashSpecs = []cmsHashSpec{
	{crypto.SHA256, oidDigestSHA256, oidSignatureRSAWithSHA256, oidSignatureECDSAWith256, "SHA256withRSA", "SHA256withECDSA"},
	{crypto.SHA384, oidDigestSHA384, oidSignatureRSAWithSHA384, oidSignatureECDSAWith384, "SHA384withRSA", "SHA384withECDSA"},
	{crypto.SHA512, oidDigestSHA512, oidSignatureRSAWithSHA512, oidSignatureECDSAWith512, "SHA512withRSA", "SHA512withECDSA"},
}

// cmsHashFromOptions elige la función resumen pedida por el portal
// (p. ej. algorithm=SHA512withRSA). SHA-1 y los valores desconocidos se elevan
// a SHA-256: nunca se genera una firma CMS nueva con SHA-1.
func cmsHashFromOptions(options map[string]string) cmsHashSpec {
	raw := strings.ToUpper(strings.TrimSpace(valorOpcion(options, "algorithm")))
	raw = strings.ReplaceAll(raw, "-", "")
	switch {
	case strings.HasPrefix(raw, "SHA384"):
		return cmsHashSpecs[1]
	case strings.HasPrefix(raw, "SHA512"):
		return cmsHashSpecs[2]
	default:
		return cmsHashSpecs[0]
	}
}

// SHA-1 solo para VERIFICAR firmas CMS antiguas (AutoFirma firmaba con
// SHA1withRSA hasta 2014). No está en cmsHashSpecs: nunca se firma con él.
// Ver algoritmo_antiguo.go.
var (
	oidDigestSHA1           = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSignatureRSAWithSHA1 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 5}
	oidSignatureECDSAWith1  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 1}
	cmsHashSHA1Antiguo      = cmsHashSpec{crypto.SHA1, oidDigestSHA1, oidSignatureRSAWithSHA1, oidSignatureECDSAWith1, "SHA1withRSA", "SHA1withECDSA"}
)

// cmsHashParaVerificar admite además SHA-1 y dice si es un algoritmo antiguo.
func cmsHashParaVerificar(oid asn1.ObjectIdentifier) (cmsHashSpec, bool, bool) {
	if spec, ok := cmsHashForDigestOID(oid); ok {
		return spec, false, true
	}
	if oid.Equal(oidDigestSHA1) {
		return cmsHashSHA1Antiguo, true, true
	}
	return cmsHashSpec{}, false, false
}

func cmsHashForDigestOID(oid asn1.ObjectIdentifier) (cmsHashSpec, bool) {
	for _, spec := range cmsHashSpecs {
		if spec.digestOID.Equal(oid) {
			return spec, true
		}
	}
	return cmsHashSpec{}, false
}

func cmsDigest(hash crypto.Hash, data []byte) []byte {
	h := hash.New()
	h.Write(data)
	return h.Sum(nil)
}
