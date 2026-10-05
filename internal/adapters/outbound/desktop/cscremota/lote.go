// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota

import (
	"context"
	"crypto"
	"io"

	"grxfirma/internal/adapters/outbound/common/csc"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
)

// NuevaClave envuelve una credencial remota en la clave del motor. Si el
// prestador admite varias firmas por autorización (multisign), un lote se
// autoriza por grupos con un solo PIN u OTP. anotar recibe los errores del
// servicio para explicarlos a la persona; puede ser nil.
func NuevaClave(ctx context.Context, cliente *csc.Cliente, cred *csc.Credencial, anotar func(error)) (*deskSigner.ClaveLocal, error) {
	if anotar == nil {
		anotar = func(error) {}
	}
	firmante, err := cliente.Firmante(ctx, cred)
	if err != nil {
		return nil, err
	}
	clave := deskSigner.NuevaClaveLocalConCadena(&firmanteAnotado{firmante: firmante, anotar: anotar}, cred.Certificado, cred.Cadena)
	return clave.ConAutorizadorLote(&autorizadorLote{cliente: cliente, cred: cred, anotar: anotar}), nil
}

type autorizadorLote struct {
	cliente *csc.Cliente
	cred    *csc.Credencial
	anotar  func(error)
}

func (a *autorizadorLote) CapacidadLote(total int) (int, error) {
	n, err := a.cliente.CapacidadLote(a.cred, total)
	if err != nil {
		a.anotar(err)
	}
	return n, err
}

func (a *autorizadorLote) IniciarLote(ctx context.Context, n int) (deskSigner.FirmantesLote, error) {
	lote, err := a.cliente.NuevoLote(ctx, a.cred, n)
	if err != nil {
		a.anotar(err)
		return nil, err
	}
	return &firmantesAnotados{lote: lote, anotar: a.anotar}, nil
}

type firmantesAnotados struct {
	lote   *csc.LoteFirmas
	anotar func(error)
}

func (f *firmantesAnotados) Firmante(i int) crypto.Signer {
	return &firmanteAnotado{firmante: f.lote.Firmante(i), anotar: f.anotar}
}

func (f *firmantesAnotados) Terminar(i int) { f.lote.Terminar(i) }

func (f *firmantesAnotados) Cerrar() { f.lote.Cerrar() }

// firmanteAnotado delega en el firmante remoto y apunta su error, porque el
// motor de firma lo envuelve en mensajes genéricos.
type firmanteAnotado struct {
	firmante crypto.Signer
	anotar   func(error)
}

func (f *firmanteAnotado) Public() crypto.PublicKey { return f.firmante.Public() }

func (f *firmanteAnotado) Sign(r io.Reader, resumen []byte, opts crypto.SignerOpts) ([]byte, error) {
	firma, err := f.firmante.Sign(r, resumen, opts)
	if err != nil {
		f.anotar(err)
	}
	return firma, err
}
