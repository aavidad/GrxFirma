// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"grxfirma/internal/security/excepciones"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
	"grxfirma/internal/domain"
)

// margenRelojSelloTiempo tolera la deriva de reloj entre la TSA y este equipo.
const margenRelojSelloTiempo = 5 * time.Minute

// fechaFirmaCMS devuelve el instante de una firma CMS ya comprobada, por este
// orden:
//  1. el sello de tiempo, si se ha podido validar entero (comprobarSelloCMS);
//  2. el atributo firmado signingTime, que protege la firma pero procede del
//     reloj de quien firmó;
//  3. un sello cuya huella corresponde a la firma pero cuya autoridad no se ha
//     podido comprobar, con su propio origen para que nadie lo presente como
//     «según el sello de tiempo» (KD-01).
//
// Sin ninguno no se inventa una fecha. La fecha solo se informa: la vigencia
// del certificado firmante se sigue evaluando en el instante actual.
func fechaFirmaCMS(signerInfo signerInfoRaw, firmante *x509.Certificate, anclas domain.CertificateChain, contexts ...context.Context) (time.Time, string, bool) {
	selloDeclarado, hayDeclarado := time.Time{}, false
	for _, token := range extraerSellosTiempoCMS(signerInfo) {
		fecha, validado, ok := comprobarSelloCMS(token, signerInfo.Signature, firmante, anclas, time.Now(), contexts...)
		if !ok {
			continue
		}
		if validado {
			return fecha, domain.SigningTimeSourceTimestamp, true
		}
		if !hayDeclarado {
			selloDeclarado, hayDeclarado = fecha, true
		}
	}
	if fecha, ok := atributoSigningTime(signerInfo.SignedAttributes.Bytes); ok {
		return fecha, domain.SigningTimeSourceSignedAttribute, true
	}
	if hayDeclarado {
		return selloDeclarado, domain.SigningTimeSourceTimestampUnverified, true
	}
	return time.Time{}, "", false
}

// comprobarSelloCMS analiza un sello RFC 3161 sobre el valor de firma.
// ok indica que el sello está bien formado, su firma corresponde al
// certificado que trae y su huella es la del valor de firma: solo entonces
// su fecha se puede mostrar, como declarada. validado exige además lo mismo
// que el dictamen v2 (verificacionlocal.evaluarSello): algoritmo admitido,
// fecha no futura, uso de clave timeStamping y cadena de la TSA hasta las
// anclas del verificador en el instante del sello; y que ese instante caiga
// dentro de la vigencia del certificado firmante. Sin la cadena, un sello lo
// puede fabricar cualquiera con una «TSA» propia y la hora que quiera.
func comprobarSelloCMS(token, valorFirma []byte, firmante *x509.Certificate, anclas domain.CertificateChain, ahora time.Time, contexts ...context.Context) (fecha time.Time, validado, ok bool) {
	if len(valorFirma) == 0 {
		return time.Time{}, false, false
	}
	p7, err := pkcs7.Parse(token)
	if err != nil || len(p7.Signers) != 1 {
		return time.Time{}, false, false
	}
	ctx := context.Background()
	if len(contexts) > 0 && contexts[0] != nil {
		ctx = contexts[0]
	}
	if p7.Signers[0].DigestAlgorithm.Algorithm.Equal(pkcs7.OIDDigestAlgorithmSHA1) {
		if excepciones.Check("sha1_leer", ctx) != nil {
			domain.AnotarAviso(ctx, "excepciones.sha1_leer.bloqueo")
			return time.Time{}, false, false
		}
	}
	sello, err := timestamp.Parse(token)
	// Sin certificado embebido timestamp.Parse no comprueba la firma.
	if err != nil || sello == nil || len(sello.Certificates) == 0 ||
		!sello.HashAlgorithm.Available() || sello.Time.IsZero() {
		return time.Time{}, false, false
	}
	resumen := sello.HashAlgorithm.New()
	resumen.Write(valorFirma)
	if subtle.ConstantTimeCompare(resumen.Sum(nil), sello.HashedMessage) != 1 {
		return time.Time{}, false, false
	}
	fecha = sello.Time.UTC()
	return fecha, selloCMSValidado(p7, sello, fecha, firmante, anclas, ahora, contexts...), true
}

func selloCMSValidado(p7 *pkcs7.PKCS7, sello *timestamp.Timestamp, fecha time.Time, firmante *x509.Certificate, anclas domain.CertificateChain, ahora time.Time, contexts ...context.Context) bool {
	switch sello.HashAlgorithm {
	case crypto.SHA256, crypto.SHA384, crypto.SHA512:
	default:
		return false
	}
	if fecha.After(ahora.Add(margenRelojSelloTiempo)) {
		return false
	}
	if firmante == nil || fecha.Before(firmante.NotBefore) || fecha.After(firmante.NotAfter) {
		return false
	}
	tsa := p7.GetOnlySigner()
	if tsa != nil {
		if key, ok := tsa.PublicKey.(*rsa.PublicKey); ok && key.N.BitLen() < 2048 {
			return false
		}
	}
	if len(p7.Signers) != 1 {
		return false
	}
	if p7.Signers[0].DigestAlgorithm.Algorithm.Equal(pkcs7.OIDDigestAlgorithmSHA1) {
		ctx := context.Background()
		if len(contexts) > 0 && contexts[0] != nil {
			ctx = contexts[0]
		}
		if excepciones.Require(ctx, "sha1_leer") != nil {
			return false
		}
	}
	if tsa == nil || !usoSelladoTiempo(tsa) {
		return false
	}
	raices, ok := raicesSelloCMS(anclas)
	if !ok {
		return false
	}
	intermedios := x509.NewCertPool()
	for _, cert := range sello.Certificates {
		if cert != nil && !cert.Equal(tsa) {
			intermedios.AddCert(cert)
		}
	}
	_, err := tsa.Verify(x509.VerifyOptions{
		Roots:         raices,
		Intermediates: intermedios,
		CurrentTime:   fecha,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
	})
	return err == nil
}

func usoSelladoTiempo(cert *x509.Certificate) bool {
	for _, uso := range cert.ExtKeyUsage {
		if uso == x509.ExtKeyUsageTimeStamping {
			return true
		}
	}
	return false
}

// raicesSelloCMS reúne las anclas X.509 del verificador para la cadena de la
// TSA: las raíces del sistema si se piden y los certificados DER explícitos.
// Sin ninguna, el sello no se puede validar.
func raicesSelloCMS(anclas domain.CertificateChain) (*x509.CertPool, bool) {
	raices := x509.NewCertPool()
	hay := false
	if anclas.UseSystemRoots {
		if sistema, err := x509.SystemCertPool(); err == nil && sistema != nil {
			raices = sistema.Clone()
			hay = true
		}
	}
	for _, der := range anclas.DERCertificates {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		raices.AddCert(cert)
		hay = true
	}
	return raices, hay
}

// sinFechaFirmada indica que entre las fechas halladas no hay ninguna que
// proteja una firma o un sello validado: ninguna, o solo sellos sin comprobar.
func sinFechaFirmada(fechas []domain.VerificationSigningTime) bool {
	for _, f := range fechas {
		if f.Source != domain.SigningTimeSourceTimestampUnverified {
			return false
		}
	}
	return true
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
