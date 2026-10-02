// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package verificacionlocal

import (
	"context"
	"crypto"
	"crypto/subtle"
	"crypto/x509"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/digitorus/timestamp"
	"grxfirma/internal/domain"
)

const margenRelojSello = 5 * time.Minute

// evaluarSellos verifica los sellos RFC 3161 de la firma. Un sello no es
// obligatorio: su ausencia se declara no_presente y no impide concluir.
func (e *Evaluador) evaluarSellos(ctx context.Context, material domain.MaterialFirma, anclas *x509.CertPool, referencia time.Time) domain.AspectoDictamen {
	if len(material.SellosTiempoDER) == 0 {
		if material.SelloNoEvaluable {
			return domain.AspectoDictamen{Estado: domain.SelloNoComprobado, Motivo: "formato_sin_evaluacion_de_sello"}
		}
		return domain.AspectoDictamen{Estado: domain.SelloNoPresente}
	}
	var peor *domain.AspectoDictamen
	for _, token := range material.SellosTiempoDER {
		aspecto := e.evaluarSello(ctx, token, material.ValorFirma, anclas, referencia)
		switch {
		case peor == nil:
			peor = &aspecto
		case aspecto.Estado == domain.SelloNoValido && peor.Estado != domain.SelloNoValido:
			peor = &aspecto
		case aspecto.Estado == domain.SelloNoComprobado && peor.Estado == domain.SelloValido:
			peor = &aspecto
		case aspecto.Estado == domain.SelloValido && peor.Estado == domain.SelloValido && aspecto.Fecha.Before(peor.Fecha):
			peor = &aspecto
		}
	}
	return *peor
}

// evaluarSello comprueba, en este orden: firma CMS del sello, algoritmo,
// messageImprint sobre el valor de firma, instante, uso de clave de la TSA y
// cadena de la TSA hasta las anclas locales en el instante del sello.
func (e *Evaluador) evaluarSello(ctx context.Context, token, valorFirma []byte, anclas *x509.CertPool, referencia time.Time) domain.AspectoDictamen {
	if len(valorFirma) == 0 {
		return domain.AspectoDictamen{Estado: domain.SelloNoComprobado, Motivo: "sin_valor_de_firma"}
	}
	sello, err := timestamp.Parse(token)
	if err != nil {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "sello_mal_formado_o_firma_no_valida"}
	}
	if len(sello.Certificates) == 0 {
		// Sin certificado embebido timestamp.Parse no ha verificado la firma.
		return domain.AspectoDictamen{Estado: domain.SelloNoComprobado, Motivo: "sello_sin_certificado_tsa"}
	}
	if !algoritmoSelloAdmitido(sello.HashAlgorithm) {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "algoritmo_de_sello_no_admitido"}
	}
	resumen := sello.HashAlgorithm.New()
	resumen.Write(valorFirma)
	esperado := resumen.Sum(nil)
	if subtle.ConstantTimeCompare(esperado, sello.HashedMessage) != 1 {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "sello_no_corresponde_a_la_firma"}
	}
	fecha := sello.Time.UTC()
	if fecha.After(referencia.Add(margenRelojSello)) {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "sello_en_el_futuro", Fecha: fecha}
	}
	p7, err := pkcs7.Parse(token)
	if err != nil {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "sello_mal_formado_o_firma_no_valida"}
	}
	tsa := p7.GetOnlySigner()
	if tsa == nil {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "sello_sin_firmante_unico"}
	}
	if !tieneUsoSellado(tsa) {
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "tsa_sin_uso_de_sellado"}
	}
	intermedios := make([]*x509.Certificate, 0, len(sello.Certificates))
	for _, cert := range sello.Certificates {
		if cert != nil && !cert.Equal(tsa) {
			intermedios = append(intermedios, cert)
		}
	}
	_, cadena := construirCadena(tsa, intermedios, anclas, fecha, []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping})
	switch cadena.Estado {
	case domain.CadenaValida:
		return domain.AspectoDictamen{Estado: domain.SelloValido, Fuente: "tsa_anclada", Fecha: fecha}
	case domain.CadenaNoValida:
		return domain.AspectoDictamen{Estado: domain.SelloNoValido, Motivo: "cadena_tsa_" + cadena.Motivo, Fecha: fecha}
	}
	if e.cfg.SelloRemoto != nil {
		remoto, err := e.cfg.SelloRemoto.ValidarSello(ctx, token, esperado, sello.HashAlgorithm)
		if err == nil && remoto.Valido {
			return domain.AspectoDictamen{Estado: domain.SelloValido, Fuente: "validador_remoto", Fecha: fecha}
		}
	}
	return domain.AspectoDictamen{Estado: domain.SelloNoComprobado, Motivo: "tsa_" + cadena.Motivo, Fecha: fecha}
}

func algoritmoSelloAdmitido(h crypto.Hash) bool {
	switch h {
	case crypto.SHA256, crypto.SHA384, crypto.SHA512:
		return h.Available()
	default:
		return false
	}
}

func tieneUsoSellado(cert *x509.Certificate) bool {
	for _, uso := range cert.ExtKeyUsage {
		if uso == x509.ExtKeyUsageTimeStamping {
			return true
		}
	}
	return false
}
