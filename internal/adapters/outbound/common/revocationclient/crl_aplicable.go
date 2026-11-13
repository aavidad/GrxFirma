// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package revocationclient

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"strings"
)

var (
	oidDeltaCRLIndicator        = asn1.ObjectIdentifier{2, 5, 29, 27}
	oidIssuingDistributionPoint = asn1.ObjectIdentifier{2, 5, 29, 28}
)

// CRLAplicable decide si una CRL cubre de forma completa al certificado.
// Una CRL delta, indirecta, restringida por motivos o por tipo de
// certificado, o particionada en un punto de distribución que el certificado
// no declara, no puede acreditar que el certificado esté vigente, así que se
// descarta (fallo cerrado). El segundo valor es el código del motivo.
func CRLAplicable(lista *x509.RevocationList, cert *x509.Certificate) (bool, string) {
	if lista == nil || cert == nil {
		return false, "crl_idp_no_interpretable"
	}
	for _, ext := range lista.Extensions {
		if ext.Id.Equal(oidDeltaCRLIndicator) {
			return false, "crl_delta_no_admitida"
		}
	}
	for _, ext := range lista.Extensions {
		if !ext.Id.Equal(oidIssuingDistributionPoint) {
			continue
		}
		idp, err := parsearIDP(ext.Value)
		if err != nil {
			return false, "crl_idp_no_interpretable"
		}
		switch {
		case idp.soloAlgunosMotivos || idp.indirecta || idp.soloAtributos || idp.nombreRelativo:
			return false, "crl_parcial_no_admitida"
		case idp.soloUsuarios && cert.IsCA:
			return false, "crl_no_cubre_ca"
		case idp.soloCA && !cert.IsCA:
			return false, "crl_no_cubre_usuario"
		}
		// Si la CRL declara su punto de distribución, alguno de sus URI tiene
		// que figurar en el CRLDP del certificado. Un punto de distribución
		// sin ningún URI (solo nombres de directorio) no se puede emparejar.
		if idp.conNombre && !intersecta(idp.uris, cert.CRLDistributionPoints) {
			return false, "crl_de_otra_particion"
		}
	}
	return true, ""
}

type puntoDistribucionEmisor struct {
	uris               []string
	conNombre          bool
	nombreRelativo     bool
	soloUsuarios       bool
	soloCA             bool
	soloAlgunosMotivos bool
	indirecta          bool
	soloAtributos      bool
}

// parsearIDP interpreta IssuingDistributionPoint (RFC 5280, 5.2.5).
func parsearIDP(der []byte) (puntoDistribucionEmisor, error) {
	var out puntoDistribucionEmisor
	var secuencia asn1.RawValue
	if resto, err := asn1.Unmarshal(der, &secuencia); err != nil || len(resto) != 0 ||
		secuencia.Class != asn1.ClassUniversal || secuencia.Tag != asn1.TagSequence {
		return out, errors.New("IDP no es una SEQUENCE")
	}
	campos := secuencia.Bytes
	for len(campos) > 0 {
		var campo asn1.RawValue
		var err error
		campos, err = asn1.Unmarshal(campos, &campo)
		if err != nil || campo.Class != asn1.ClassContextSpecific {
			return out, errors.New("campo IDP no válido")
		}
		switch campo.Tag {
		case 0:
			out.conNombre = true
			if err := parsearNombrePuntoDistribucion(campo.Bytes, &out); err != nil {
				return out, err
			}
		case 1:
			out.soloUsuarios = booleanoImplicito(campo)
		case 2:
			out.soloCA = booleanoImplicito(campo)
		case 3:
			out.soloAlgunosMotivos = true
		case 4:
			out.indirecta = booleanoImplicito(campo)
		case 5:
			out.soloAtributos = booleanoImplicito(campo)
		default:
			return out, errors.New("campo IDP desconocido")
		}
	}
	return out, nil
}

func parsearNombrePuntoDistribucion(der []byte, out *puntoDistribucionEmisor) error {
	var nombre asn1.RawValue
	if _, err := asn1.Unmarshal(der, &nombre); err != nil || nombre.Class != asn1.ClassContextSpecific {
		return errors.New("DistributionPointName no válido")
	}
	if nombre.Tag == 1 {
		out.nombreRelativo = true
		return nil
	}
	if nombre.Tag != 0 {
		return errors.New("DistributionPointName desconocido")
	}
	nombres := nombre.Bytes
	for len(nombres) > 0 {
		var general asn1.RawValue
		var err error
		nombres, err = asn1.Unmarshal(nombres, &general)
		if err != nil {
			return errors.New("GeneralName no válido")
		}
		// uniformResourceIdentifier [6] IA5String
		if general.Class == asn1.ClassContextSpecific && general.Tag == 6 {
			out.uris = append(out.uris, string(general.Bytes))
		}
	}
	return nil
}

func booleanoImplicito(campo asn1.RawValue) bool {
	return len(campo.Bytes) == 1 && campo.Bytes[0] != 0
}

func intersecta(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(y)) {
				return true
			}
		}
	}
	return false
}
