// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"crypto/x509"
	"errors"
)

// DetalleErrorCadena es la clave de la evidencia «error_cadena=<código>»
// con la que el verificador explica por qué no validó la cadena de un
// firmante. El código se traduce con «verificacion.detalle.valor.<código>»;
// el texto en inglés de crypto/x509 no llega a la persona.
const DetalleErrorCadena = "error_cadena"

// CodigoErrorX509 clasifica un error de x509.Certificate.Verify en un código
// estable y traducible.
func CodigoErrorX509(err error) string {
	var autoridad x509.UnknownAuthorityError
	if errors.As(err, &autoridad) {
		return "x509_autoridad_desconocida"
	}
	var nombre x509.HostnameError
	if errors.As(err, &nombre) {
		return "x509_nombre_no_coincide"
	}
	var inseguro x509.InsecureAlgorithmError
	if errors.As(err, &inseguro) {
		return "x509_algoritmo_inseguro"
	}
	var invalido x509.CertificateInvalidError
	if errors.As(err, &invalido) {
		switch invalido.Reason {
		case x509.Expired:
			return "x509_caducado"
		case x509.IncompatibleUsage:
			return "x509_uso_no_permitido"
		case x509.NotAuthorizedToSign, x509.CANotAuthorizedForThisName, x509.CANotAuthorizedForExtKeyUsage:
			return "x509_emisor_no_autorizado"
		case x509.TooManyIntermediates:
			return "x509_cadena_demasiado_larga"
		}
	}
	return "x509_otro"
}
