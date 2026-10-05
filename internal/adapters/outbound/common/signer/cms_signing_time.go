// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/subtle"
	"encoding/asn1"
	"time"

	"github.com/digitorus/timestamp"
	"grxfirma/internal/domain"
)

// fechaFirmaCMS devuelve el instante de una firma CMS ya comprobada.
// Prefiere el sello de tiempo, si su firma es correcta y su huella
// corresponde al valor de firma; si no, el atributo firmado signingTime,
// que protege la firma pero procede del reloj de quien firmó. Sin ninguno
// de los dos no se inventa una fecha.
func fechaFirmaCMS(signerInfo signerInfoRaw) (time.Time, string, bool) {
	if fecha, ok := fechaSelloTiempoCMS(signerInfo); ok {
		return fecha, domain.SigningTimeSourceTimestamp, true
	}
	if fecha, ok := atributoSigningTime(signerInfo.SignedAttributes.Bytes); ok {
		return fecha, domain.SigningTimeSourceSignedAttribute, true
	}
	return time.Time{}, "", false
}

func fechaSelloTiempoCMS(signerInfo signerInfoRaw) (time.Time, bool) {
	if len(signerInfo.Signature) == 0 {
		return time.Time{}, false
	}
	for _, token := range extraerSellosTiempoCMS(signerInfo) {
		sello, err := timestamp.Parse(token)
		// Sin certificado embebido timestamp.Parse no comprueba la firma.
		if err != nil || sello == nil || len(sello.Certificates) == 0 ||
			!sello.HashAlgorithm.Available() || sello.Time.IsZero() {
			continue
		}
		resumen := sello.HashAlgorithm.New()
		resumen.Write(signerInfo.Signature)
		if subtle.ConstantTimeCompare(resumen.Sum(nil), sello.HashedMessage) != 1 {
			continue
		}
		return sello.Time.UTC(), true
	}
	return time.Time{}, false
}

func atributoSigningTime(attrsDER []byte) (time.Time, bool) {
	remaining := attrsDER
	for len(remaining) > 0 {
		var rawAttr asn1.RawValue
		var err error
		remaining, err = asn1.Unmarshal(remaining, &rawAttr)
		if err != nil {
			return time.Time{}, false
		}
		var attr attribute
		if _, err := asn1.Unmarshal(rawAttr.FullBytes, &attr); err != nil {
			return time.Time{}, false
		}
		if !attr.Type.Equal(oidSigningTime) {
			continue
		}
		// RFC 5652 §11.3: un único valor, UTCTime o GeneralizedTime.
		if len(attr.Values) != 1 {
			return time.Time{}, false
		}
		var fecha time.Time
		if _, err := asn1.Unmarshal(attr.Values[0].FullBytes, &fecha); err != nil || fecha.IsZero() {
			return time.Time{}, false
		}
		return fecha.UTC(), true
	}
	return time.Time{}, false
}
