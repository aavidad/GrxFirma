// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"errors"
	"strings"
)

// Codigo identifica un fallo del cliente CSC sin ligarlo a un idioma. La
// presentación lo traduce con la clave de catálogo "csc.error.<codigo>".
type Codigo string

const (
	CodigoURLInvalida            Codigo = "url_invalida"
	CodigoSoloHTTPS              Codigo = "solo_https"
	CodigoRedireccion            Codigo = "redireccion"
	CodigoRespuestaGrande        Codigo = "respuesta_grande"
	CodigoRespuestaInvalida      Codigo = "respuesta_invalida"
	CodigoServicio               Codigo = "servicio"
	CodigoRed                    Codigo = "red"
	CodigoSinOAuth               Codigo = "sin_oauth"
	CodigoOAuthOtroHost          Codigo = "oauth_otro_host"
	CodigoAutorizacionDenegada   Codigo = "autorizacion_denegada"
	CodigoAutorizacionCaducada   Codigo = "autorizacion_caducada"
	CodigoNavegador              Codigo = "navegador"
	CodigoCredencialNoValida     Codigo = "credencial_no_valida" // #nosec G101 -- código de error, no una credencial.
	CodigoAlgoritmoNoSoportado   Codigo = "algoritmo_no_soportado"
	CodigoFirmaInvalida          Codigo = "firma_invalida"
	CodigoSecreto                Codigo = "secreto"
	CodigoSesionCerrada          Codigo = "sesion_cerrada"
	CodigoParametroInvalido      Codigo = "parametro_invalido"
	CodigoDemasiadasCredenciales Codigo = "demasiadas_credenciales" // #nosec G101 -- código de error, no una credencial.
	// CodigoSesionCaducada: el token de servicio ha caducado y no se puede
	// renovar; hay que volver a conectar.
	CodigoSesionCaducada Codigo = "sesion_caducada"
	// CodigoOAuthMetadatos: el servidor anunciado en oauth2Issuer no publica
	// unos metadatos RFC 8414 válidos.
	CodigoOAuthMetadatos Codigo = "oauth_metadatos"
	// CodigoLoteRepetido: el motor pidió una segunda firma para el mismo
	// documento de un lote ya autorizado.
	CodigoLoteRepetido Codigo = "lote_repetido"
	// CodigoLoteMixto: los documentos del lote usan resúmenes o esquemas de
	// firma distintos y no caben en una sola autorización.
	CodigoLoteMixto Codigo = "lote_mixto"
	// CodigoOTPLoteExcede: el lote supera las firmas que el prestador admite
	// con un solo código de un solo uso.
	CodigoOTPLoteExcede Codigo = "otp_lote_excede"
)

// Error es el error tipado del paquete. Detalle solo contiene datos técnicos
// no sensibles (por ejemplo, el código de error OAuth o CSC devuelto por el
// servidor, ya saneado). Nunca contiene tokens, PIN, OTP ni SAD.
type Error struct {
	Codigo  Codigo
	Detalle string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "csc"
	}
	if e.Detalle != "" {
		return "csc: " + string(e.Codigo) + " (" + e.Detalle + ")"
	}
	return "csc: " + string(e.Codigo)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// CodigoDe devuelve el código de un error del paquete, o cadena vacía.
func CodigoDe(err error) Codigo {
	var e *Error
	if errors.As(err, &e) {
		return e.Codigo
	}
	return ""
}

func nuevoError(codigo Codigo, detalle string, err error) *Error {
	return &Error{Codigo: codigo, Detalle: sanearDetalle(detalle), Err: err}
}

// sanearDetalle conserva solo caracteres de un identificador técnico y acota
// la longitud: el detalle procede a menudo del servidor y no debe permitir
// inyectar secuencias de terminal ni texto arbitrario.
func sanearDetalle(s string) string {
	const maximo = 64
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= maximo {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.' || r == '/':
			b.WriteRune(r)
		}
	}
	return b.String()
}
