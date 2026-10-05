// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota

import (
	"context"
	"errors"
	"sync"
	"unicode"
	"unicode/utf8"

	"grxfirma/internal/adapters/outbound/common/csc"
)

type claveSecretos struct{}

// Peticion acompaña a una única petición de firma con un certificado remoto.
// Lleva el PIN y el OTP que ha escrito la persona y recoge el último error
// del servicio para que la interfaz lo explique. El PIN se puede leer más de
// una vez (una autorización por documento o por grupo de un lote); el OTP,
// solo una, porque el prestador no lo acepta dos veces: un lote con OTP solo
// se firma si cabe en una autorización conjunta (multisign). Los secretos
// están ligados al certificado para el que se escribieron: la sesión no los
// entrega a otra credencial aunque viaje en el mismo contexto (por ejemplo,
// los firmantes adicionales de una multifirma o el lote de otro certificado).
type Peticion struct {
	mu       sync.Mutex
	certID   string
	pin      []byte
	otp      []byte
	otpUsado bool
	err      error
}

// ContextoConSecretos devuelve un contexto con el PIN y el OTP de esta
// firma (pueden estar vacíos), válidos solo para el certificado certID. No
// copia los valores: quien llama los conserva y debe borrarlos cuando
// termine la petición.
func ContextoConSecretos(ctx context.Context, certID string, pin, otp []byte) (context.Context, *Peticion) {
	p := &Peticion{certID: certID, pin: pin, otp: otp}
	return context.WithValue(ctx, claveSecretos{}, p), p
}

// paraCertificado indica si los secretos de la petición se escribieron para
// este certificado. Un identificador vacío no coincide con ninguno.
func (p *Peticion) paraCertificado(certID string) bool {
	return p != nil && p.certID != "" && p.certID == certID
}

// Error devuelve el último error de la firma remota en esta petición.
func (p *Peticion) Error() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func (p *Peticion) anotar(err error) {
	if p == nil || err == nil {
		return
	}
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()
}

func peticionDe(ctx context.Context) *Peticion {
	p, _ := ctx.Value(claveSecretos{}).(*Peticion)
	return p
}

// SecretoValido acepta un PIN u OTP no vacío, UTF-8, sin caracteres de
// control y de tamaño razonable. Un valor vacío significa «no se ha dado».
func SecretoValido(v []byte) bool {
	if len(v) == 0 {
		return true
	}
	if len(v) > maxSecretoPeticion || !utf8.Valid(v) {
		return false
	}
	for len(v) > 0 {
		r, n := utf8.DecodeRune(v)
		if unicode.IsControl(r) {
			return false
		}
		v = v[n:]
	}
	return true
}

// pedirSecreto entrega al cliente CSC una copia del secreto que trae la
// petición. El cliente borra la copia tras usarla.
func pedirSecreto(ctx context.Context, tipo csc.TipoSecreto) ([]byte, error) {
	s := peticionDe(ctx)
	if s == nil {
		return nil, nuevoError(CodigoSecretoNoPedido)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch tipo {
	case csc.SecretoPIN:
		if len(s.pin) == 0 {
			return nil, nuevoError(CodigoSecretoNoPedido)
		}
		return append([]byte(nil), s.pin...), nil
	case csc.SecretoOTP:
		if len(s.otp) == 0 {
			return nil, nuevoError(CodigoSecretoNoPedido)
		}
		if s.otpUsado {
			return nil, nuevoError(CodigoOTPLote)
		}
		s.otpUsado = true
		return append([]byte(nil), s.otp...), nil
	default:
		return nil, nuevoError(CodigoSecretoNoPedido)
	}
}

// CodigoVisible devuelve el código más concreto de una cadena de errores CSC:
// el cliente envuelve, por ejemplo, «otp_lote» dentro de «secreto», y a la
// persona le sirve más el primero.
func CodigoVisible(err error) csc.Codigo {
	var codigo csc.Codigo
	for err != nil {
		if e, ok := err.(*csc.Error); ok && e != nil {
			codigo = e.Codigo
		}
		err = errors.Unwrap(err)
	}
	return codigo
}
