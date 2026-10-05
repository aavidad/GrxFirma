// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota_test

import (
	"context"
	"crypto"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/desktop/cscremota"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/ports"
	"grxfirma/internal/testsupport/csctest"
)

func TestSesionMuestraElHostDeLosMetadatosOAuth(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Emisor = "/emisor" })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	d, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID)
	if err != nil {
		t.Fatalf("Configurar: %v", err)
	}
	if d.HostOAuth == "" || d.HostOAuth != d.HostServicio {
		t.Fatalf("hosts: %+v", d)
	}
	if _, _, err := sesion.Conectar(context.Background()); err != nil {
		t.Fatalf("Conectar: %v", err)
	}
}

func TestSesionConectaConListadoPaginado(t *testing.T) {
	s := csctest.Nuevo(t)
	// Los extra comparten certificado con la credencial RSA: se descartan
	// por repetidos, pero solo si el listado se recorre entero.
	s.Configurar(func(s *csctest.Servidor) { s.CredencialesExtra, s.TamPagina, s.Multisign = 3, 2, 4 })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	if _, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID); err != nil {
		t.Fatal(err)
	}
	creds, omitidas, err := sesion.Conectar(context.Background())
	if err != nil || len(creds) != 2 || omitidas != 3 {
		t.Fatalf("creds=%d omitidas=%d err=%v", len(creds), omitidas, err)
	}
	for _, c := range creds {
		if c.Multisign != 4 {
			t.Fatalf("multisign = %d", c.Multisign)
		}
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.PeticionesListado != 3 {
			t.Fatalf("páginas = %d", s.PeticionesListado)
		}
	})
}

// En un lote remoto el PIN y el OTP siguen ligados al certificado para el
// que se escribieron: el contexto del lote llega a la clave por otro camino
// (BeginBatch) y no puede llevar los secretos de otro certificado a esta
// credencial.
func TestLoteRemotoNoEntregaSecretosDeOtroCertificado(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo, s.Multisign = "explicit", 4 })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	ctx := context.Background()
	if _, err := sesion.Configurar(ctx, s.URL, csctest.ClientID); err != nil {
		t.Fatal(err)
	}
	creds, _, err := sesion.Conectar(ctx)
	if err != nil || len(creds) != 2 {
		t.Fatalf("Conectar: %d %v", len(creds), err)
	}
	propio, ajeno := creds[0].Ref, creds[1].Ref
	pin, otp := []byte(csctest.PIN), []byte(csctest.OTP)
	ctxPropio, _ := cscremota.ContextoConSecretos(ctx, propio.ID, pin, otp)
	ctxAjeno, _ := cscremota.ContextoConSecretos(ctx, ajeno.ID, pin, otp)

	clave, err := sesion.KeyFor(ctxPropio, propio)
	if err != nil {
		t.Fatalf("KeyFor: %v", err)
	}
	lote, ok := clave.(ports.BatchSigningKey)
	if !ok {
		t.Fatal("la clave remota no admite lotes")
	}
	if n, err := lote.BatchCapacity(2); err != nil || n != 2 {
		t.Fatalf("capacidad = %d %v", n, err)
	}
	// El lote trae los secretos de otro certificado: no se autoriza nada.
	if _, err := lote.BeginBatch(ctxAjeno, 2); cscremota.CodigoVisible(err) != cscremota.CodigoSecretoNoPedido {
		t.Fatalf("lote con secretos ajenos: %v", err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 0 {
			t.Fatalf("autorizaciones con secretos ajenos: %d", s.Autorizaciones)
		}
	})

	// Con sus propios secretos, un solo PIN y OTP autoriza el grupo.
	grupo, err := lote.BeginBatch(ctxPropio, 2)
	if err != nil {
		t.Fatalf("BeginBatch: %v", err)
	}
	defer grupo.Close()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer grupo.Done(i)
			resumen := sha256.Sum256([]byte{byte(i)})
			_, errs[i] = grupo.Key(i).(*deskSigner.ClaveLocal).SignDigest(resumen[:], crypto.SHA256)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("firma %d del lote: %v", i, err)
		}
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 1 || s.UltimoNumSignatures != 2 || len(s.ResumenesFirmados) != 2 {
			t.Fatalf("autorizaciones %d, numSignatures %d, firmas %d", s.Autorizaciones, s.UltimoNumSignatures, len(s.ResumenesFirmados))
		}
	})
}
