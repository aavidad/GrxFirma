// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
	"grxfirma/internal/domain"
)

// EvaluarSelloDocumento comprueba token, impronta y TSA sin efectuar red.
// Un token íntegro con confianza o revocación pendiente nunca resulta válido.
func (e *Evaluador) EvaluarSelloDocumento(ctx context.Context, token, contenido []byte, anclas domain.CertificateChain, referencia time.Time) (domain.DictamenFirmante, domain.AspectoDictamen) {
	base := domain.DictamenFirmante{
		Cadena:      domain.AspectoDictamen{Estado: domain.CadenaNoComprobada},
		Certificado: domain.AspectoDictamen{Estado: domain.CertificadoNoComprobado},
		Revocacion:  domain.AspectoDictamen{Estado: domain.RevocacionNoComprobada},
		SelloTiempo: domain.AspectoDictamen{Estado: domain.SelloNoComprobado},
	}
	parcial := domain.AspectoDictamen{Estado: domain.IntegridadParcial, Motivo: "sello_no_comprobado"}
	noValida := domain.AspectoDictamen{Estado: domain.IntegridadNoValida, Motivo: "sello_documento_no_valido"}
	if ctx.Err() != nil || len(token) == 0 || len(token) > 16<<20 {
		return base, parcial
	}
	// /Contents suele estar reservado con ceros. ASN.1 debe ocupar todo lo
	// anterior al relleno; cualquier otro byte sobrante invalida el token.
	var value asn1.RawValue
	rest, err := asn1.Unmarshal(token, &value)
	if err != nil {
		return base, noValida
	}
	if len(bytes.Trim(rest, "\x00")) != 0 {
		return base, noValida
	}
	sello, err := timestamp.Parse(value.FullBytes)
	if err != nil {
		return base, noValida
	}
	if len(sello.Certificates) == 0 {
		return base, parcial
	}
	if !algoritmoSelloAdmitido(sello.HashAlgorithm) {
		return base, noValida
	}
	digest := sello.HashAlgorithm.New()
	_, _ = digest.Write(contenido)
	if subtle.ConstantTimeCompare(digest.Sum(nil), sello.HashedMessage) != 1 || sello.Time.After(referencia.Add(margenRelojSello)) {
		return base, noValida
	}
	p7, err := pkcs7.Parse(value.FullBytes)
	if err != nil {
		return base, noValida
	}
	tsa := p7.GetOnlySigner()
	if tsa == nil || !tieneUsoSellado(tsa) {
		return base, noValida
	}
	pool, err := poolAnclas(anclas)
	if err != nil {
		return base, parcial
	}
	material := domain.MaterialFirma{CertificadoDER: tsa.Raw}
	for _, cert := range sello.Certificates {
		if cert != nil && !cert.Equal(tsa) {
			material.CertificadosEmbebidosDER = append(material.CertificadosEmbebidosDER, cert.Raw)
		}
	}
	firmante, ok := e.evaluarFirmante(ctx, material, pool, referencia)
	if !ok {
		return base, parcial
	}
	// La cadena de la TSA exige EKU TimeStamping incluso si el certificado
	// contiene un uso de firma genérico aceptado para firmas ordinarias.
	embebidos := parsearEmbebidos(material.CertificadosEmbebidosDER, tsa)
	cadena, aspecto := construirCadena(tsa, embebidos, pool, referencia, []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping})
	firmante.Cadena = aspecto
	if cadena == nil {
		firmante.Revocacion = domain.AspectoDictamen{Estado: domain.RevocacionNoComprobada, Motivo: "cadena_no_construida"}
	} else {
		firmante.Revocacion = e.evaluarRevocacion(ctx, cadena, material, referencia)
	}
	firmante.SelloTiempo = domain.AspectoDictamen{Estado: domain.SelloValido, Fuente: "token_rfc3161", Fecha: sello.Time.UTC()}
	return firmante, domain.AspectoDictamen{Estado: domain.IntegridadValida}
}
