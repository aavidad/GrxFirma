// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"io"
	"sort"
	"sync"
)

// MaxFirmasLote es el máximo de resúmenes que el cliente autoriza de una vez,
// aunque el servicio anuncie más. Coincide con el límite del lote del
// escritorio y de la CLI.
const MaxFirmasLote = 128

// CapacidadLote devuelve cuántos documentos de un lote de total se pueden
// autorizar juntos con la credencial. 1 significa que el servicio no admite
// autorizar varias firmas a la vez (multisign) y que cada firma pide la suya,
// como con [FirmanteRemoto].
//
// Si el OTP lo da quien llama una sola vez ([Opciones.EnvioOTPManual]) y el
// lote no cabe en una autorización, se rechaza antes de firmar nada: el
// segundo grupo necesitaría otro código.
func (c *Cliente) CapacidadLote(cred *Credencial, total int) (int, error) {
	if cred == nil || cred.Certificado == nil || total < 1 {
		return 0, nuevoError(CodigoCredencialNoValida, "", nil)
	}
	capacidad := min(cred.Multisign, MaxFirmasLote)
	if capacidad < 2 || total < 2 {
		return 1, nil
	}
	otpUnico := cred.Modo == ModoExplicito && cred.OTP && c.opc.EnvioOTPManual
	if otpUnico && total > capacidad {
		return 0, nuevoError(CodigoOTPLoteExcede, "", nil)
	}
	return min(capacidad, total), nil
}

// LoteFirmas autoriza de una vez las firmas de n documentos. Cada documento
// firma con su propio [crypto.Signer] ([LoteFirmas.Firmante]); el motor los
// usa a la vez desde n goroutines. Cada Sign espera a que todos los
// documentos hayan entregado su resumen o hayan terminado sin firmar
// ([LoteFirmas.Terminar]). Entonces se llama una vez a credentials/authorize
// con numSignatures y la lista de resúmenes (un solo PIN u OTP), una vez a
// signatures/signHash con todos ellos y se borra el SAD.
//
// El SAD solo sirve para los resúmenes autorizados y no se reutiliza: una
// segunda firma del mismo documento (por ejemplo, un reintento del motor)
// falla con [CodigoLoteRepetido] en lugar de pedir otra autorización.
type LoteFirmas struct {
	ctx  context.Context
	c    *Cliente
	cred *Credencial
	n    int

	mu         sync.Mutex
	pendientes int
	usados     []bool
	resumenes  map[int][]byte
	fijado     bool
	h          crypto.Hash
	pss        bool
	disparado  bool
	listo      chan struct{}
	firmas     map[int][]byte
	err        error
}

// NuevoLote prepara la autorización conjunta de n firmas (2 ≤ n ≤
// [Cliente.CapacidadLote]). ctx limita la espera y las peticiones.
func (c *Cliente) NuevoLote(ctx context.Context, cred *Credencial, n int) (*LoteFirmas, error) {
	capacidad, err := c.CapacidadLote(cred, n)
	if err != nil {
		return nil, err
	}
	if n < 2 || n > capacidad {
		return nil, nuevoError(CodigoParametroInvalido, "numSignatures", nil)
	}
	return &LoteFirmas{
		ctx:        ctx,
		c:          c,
		cred:       cred,
		n:          n,
		pendientes: n,
		usados:     make([]bool, n),
		resumenes:  make(map[int][]byte, n),
		listo:      make(chan struct{}),
	}, nil
}

// Tamano devuelve cuántos documentos cubre el lote.
func (l *LoteFirmas) Tamano() int { return l.n }

// Firmante devuelve el crypto.Signer del documento i (0 ≤ i < n).
func (l *LoteFirmas) Firmante(i int) crypto.Signer {
	return &firmanteLote{lote: l, indice: i}
}

// Terminar indica que el documento i ha acabado. Si no llegó a pedir su
// firma (por ejemplo, porque el documento no era válido), deja de esperarse
// y, si era el último, se autoriza el resto. Es idempotente.
func (l *LoteFirmas) Terminar(i int) {
	l.mu.Lock()
	if i < 0 || i >= l.n || l.usados[i] {
		l.mu.Unlock()
		return
	}
	l.usados[i] = true
	disparar := l.retirarLocked()
	l.mu.Unlock()
	if disparar {
		l.ejecutar()
	}
}

// Cerrar libera a quien siga esperando (con [CodigoSesionCerrada]) y olvida
// los resúmenes. El SAD ya se ha borrado al terminar la firma conjunta.
func (l *LoteFirmas) Cerrar() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.disparado {
		l.disparado = true
		l.err = nuevoError(CodigoSesionCerrada, "", nil)
		close(l.listo)
	}
	l.resumenes = nil
}

// retirarLocked descuenta un documento pendiente y dice si hay que lanzar la
// autorización conjunta. Solo devuelve true una vez.
func (l *LoteFirmas) retirarLocked() bool {
	l.pendientes--
	if l.pendientes > 0 || l.disparado {
		return false
	}
	l.disparado = true
	return true
}

// ejecutar autoriza y firma los resúmenes recogidos y despierta a todos.
func (l *LoteFirmas) ejecutar() {
	l.mu.Lock()
	indices := make([]int, 0, len(l.resumenes))
	for i := range l.resumenes {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	lista := make([][]byte, 0, len(indices))
	for _, i := range indices {
		lista = append(lista, l.resumenes[i])
	}
	h, pss := l.h, l.pss
	l.mu.Unlock()

	var (
		firmas map[int][]byte
		err    error
	)
	if len(lista) > 0 {
		firmas, err = l.autorizarYFirmar(indices, lista, h, pss)
	}

	l.mu.Lock()
	l.firmas, l.err = firmas, err
	close(l.listo)
	l.mu.Unlock()
}

func (l *LoteFirmas) autorizarYFirmar(indices []int, lista [][]byte, h crypto.Hash, pss bool) (map[int][]byte, error) {
	c := l.c
	c.mu.Lock()
	defer c.mu.Unlock()
	auth, err := c.autorizarCredencial(l.ctx, l.cred, lista, h)
	if err != nil {
		return nil, err
	}
	// El SAD (o el token de credencial) se borra en cuanto se ha usado.
	defer c.liberarAutorizacion(auth)
	recibidas, err := c.firmarResumenes(l.ctx, l.cred, auth, lista, h, pss)
	if err != nil {
		return nil, err
	}
	firmas := make(map[int][]byte, len(indices))
	for k, i := range indices {
		firmas[i] = recibidas[k]
	}
	return firmas, nil
}

type firmanteLote struct {
	lote   *LoteFirmas
	indice int
}

func (f *firmanteLote) Public() crypto.PublicKey {
	return f.lote.cred.Certificado.PublicKey
}

// Sign entrega el resumen del documento, espera a la autorización conjunta y
// devuelve su firma ya comprobada con la clave pública.
func (f *firmanteLote) Sign(_ io.Reader, resumen []byte, opts crypto.SignerOpts) ([]byte, error) {
	l := f.lote
	if opts == nil {
		l.Terminar(f.indice)
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "", nil)
	}
	h := opts.HashFunc()
	if _, ok := oidHash(h); !ok || len(resumen) != h.Size() {
		l.Terminar(f.indice)
		return nil, nuevoError(CodigoAlgoritmoNoSoportado, "hash", nil)
	}
	pssOpts, pss := opts.(*rsa.PSSOptions)

	l.mu.Lock()
	if f.indice < 0 || f.indice >= l.n || l.usados[f.indice] {
		l.mu.Unlock()
		return nil, nuevoError(CodigoLoteRepetido, "", nil)
	}
	l.usados[f.indice] = true
	var errMixto error
	switch {
	case !l.fijado:
		l.h, l.pss, l.fijado = h, pss, true
	case l.h != h || l.pss != pss:
		errMixto = nuevoError(CodigoLoteMixto, "", nil)
	}
	if errMixto == nil {
		l.resumenes[f.indice] = append([]byte(nil), resumen...)
	}
	disparar := l.retirarLocked()
	l.mu.Unlock()
	if disparar {
		l.ejecutar()
	}
	if errMixto != nil {
		return nil, errMixto
	}

	select {
	case <-l.listo:
	case <-l.ctx.Done():
		return nil, nuevoError(CodigoAutorizacionCaducada, "", l.ctx.Err())
	}
	l.mu.Lock()
	firma, err := l.firmas[f.indice], l.err
	l.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if len(firma) == 0 {
		return nil, nuevoError(CodigoRespuestaInvalida, "signatures", nil)
	}
	return comprobarFirma(l.cred.Certificado.PublicKey, resumen, h, firma, pssOpts)
}

var _ crypto.Signer = (*firmanteLote)(nil)
