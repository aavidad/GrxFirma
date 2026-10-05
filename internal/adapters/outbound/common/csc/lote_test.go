// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/testsupport/csctest"
)

type resultadoLote struct {
	firma []byte
	err   error
}

// firmarEnLote simula al motor: cada documento firma desde su goroutine y
// avisa al terminar. nil en resumenes significa que el documento falla antes
// de pedir la firma.
func firmarEnLote(lote *csc.LoteFirmas, resumenes [][]byte, opts crypto.SignerOpts) []resultadoLote {
	res := make([]resultadoLote, len(resumenes))
	var wg sync.WaitGroup
	for i := range resumenes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer lote.Terminar(i)
			if resumenes[i] == nil {
				return
			}
			res[i].firma, res[i].err = lote.Firmante(i).Sign(rand.Reader, resumenes[i], opts)
		}(i)
	}
	wg.Wait()
	return res
}

func resumenesDe(textos ...string) [][]byte {
	out := make([][]byte, 0, len(textos))
	for _, t := range textos {
		if t == "" {
			out = append(out, nil)
			continue
		}
		s := sha256.Sum256([]byte(t))
		out = append(out, s[:])
	}
	return out
}

func TestLoteAutorizaUnaVezConUnSoloPINyOTP(t *testing.T) {
	for _, v21 := range []bool{false, true} {
		s := csctest.Nuevo(t)
		s.Configurar(func(s *csctest.Servidor) {
			s.Modo, s.SCAL, s.AuthV21, s.Multisign = "explicit", "2", v21, 5
		})
		// Solo se prevén un PIN y un OTP: un segundo pedido haría fallar la prueba.
		cliente := nuevoCliente(t, s, csctest.PIN, csctest.OTP)
		cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
		if cred.Multisign != 5 {
			t.Fatalf("multisign = %d", cred.Multisign)
		}
		lote, err := cliente.NuevoLote(context.Background(), cred, 4)
		if err != nil {
			t.Fatalf("NuevoLote: %v", err)
		}
		// El tercer documento falla antes de firmar: no debe bloquear al resto.
		resumenes := resumenesDe("uno", "dos", "", "cuatro")
		res := firmarEnLote(lote, resumenes, crypto.SHA256)
		lote.Cerrar()
		for i, r := range res {
			if resumenes[i] == nil {
				continue
			}
			if r.err != nil {
				t.Fatalf("v21=%v documento %d: %v", v21, i, r.err)
			}
			if err := rsa.VerifyPKCS1v15(cred.Certificado.PublicKey.(*rsa.PublicKey), crypto.SHA256, resumenes[i], r.firma); err != nil {
				t.Fatalf("documento %d: la firma no verifica: %v", i, err)
			}
		}
		s.Leer(func(s *csctest.Servidor) {
			if s.Autorizaciones != 1 || s.UltimoNumSignatures != 3 || len(s.ResumenesFirmados) != 3 {
				t.Fatalf("v21=%v: autorizaciones=%d numSignatures=%d firmados=%d", v21,
					s.Autorizaciones, s.UltimoNumSignatures, len(s.ResumenesFirmados))
			}
			if !v21 && s.OTPEnviados != 1 {
				t.Fatalf("OTP enviados = %d", s.OTPEnviados)
			}
		})
	}
}

func TestLoteRepetirFirmaNoPideOtraAutorizacion(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo, s.Multisign = "explicit", 2 })
	cliente := nuevoCliente(t, s, csctest.PIN, csctest.OTP)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	lote, err := cliente.NuevoLote(context.Background(), cred, 2)
	if err != nil {
		t.Fatal(err)
	}
	resumenes := resumenesDe("a", "b")
	for i, r := range firmarEnLote(lote, resumenes, crypto.SHA256) {
		if r.err != nil {
			t.Fatalf("documento %d: %v", i, r.err)
		}
	}
	// Un reintento del motor (p. ej. por falta de espacio en el PDF) no puede
	// gastar otra autorización ni pedir otro PIN.
	_, err = lote.Firmante(0).Sign(rand.Reader, resumenes[0], crypto.SHA256)
	esperarCodigo(t, err, csc.CodigoLoteRepetido)
	lote.Cerrar()
	s.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 1 {
			t.Fatalf("autorizaciones = %d", s.Autorizaciones)
		}
	})
}

func TestLoteConResumenesDistintosRechazaElDiscordante(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Multisign = 3 })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	lote, err := cliente.NuevoLote(context.Background(), cred, 2)
	if err != nil {
		t.Fatal(err)
	}
	r256 := sha256.Sum256([]byte("a"))
	r384 := sha512.Sum384([]byte("b"))
	var wg sync.WaitGroup
	errs := make([]error, 2)
	// El de SHA-256 entra primero y fija el algoritmo del lote.
	primero := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer lote.Terminar(0)
		close(primero)
		_, errs[0] = lote.Firmante(0).Sign(rand.Reader, r256[:], crypto.SHA256)
	}()
	go func() {
		defer wg.Done()
		defer lote.Terminar(1)
		<-primero
		time.Sleep(50 * time.Millisecond)
		_, errs[1] = lote.Firmante(1).Sign(rand.Reader, r384[:], crypto.SHA384)
	}()
	wg.Wait()
	lote.Cerrar()
	if errs[0] != nil {
		t.Fatalf("el primero debió firmarse: %v", errs[0])
	}
	esperarCodigo(t, errs[1], csc.CodigoLoteMixto)
}

func TestLoteSinFirmasNoAutorizaNada(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Multisign = 3 })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	lote, err := cliente.NuevoLote(context.Background(), cred, 3)
	if err != nil {
		t.Fatal(err)
	}
	firmarEnLote(lote, [][]byte{nil, nil, nil}, crypto.SHA256)
	lote.Cerrar()
	s.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 0 {
			t.Fatalf("autorizaciones = %d", s.Autorizaciones)
		}
	})
}

func TestLoteCerrarLiberaAQuienEspera(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Multisign = 3 })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	lote, err := cliente.NuevoLote(context.Background(), cred, 3)
	if err != nil {
		t.Fatal(err)
	}
	hecho := make(chan error, 1)
	go func() {
		r := sha256.Sum256([]byte("solo"))
		_, err := lote.Firmante(0).Sign(rand.Reader, r[:], crypto.SHA256)
		hecho <- err
	}()
	time.Sleep(50 * time.Millisecond)
	lote.Cerrar()
	select {
	case err := <-hecho:
		esperarCodigo(t, err, csc.CodigoSesionCerrada)
	case <-time.After(5 * time.Second):
		t.Fatal("Cerrar no liberó la espera")
	}
}

func TestLoteCancelacionDelContextoLiberaLaEspera(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Multisign = 2 })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	ctx, cancelar := context.WithCancel(context.Background())
	lote, err := cliente.NuevoLote(ctx, cred, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer lote.Cerrar()
	hecho := make(chan error, 1)
	go func() {
		r := sha256.Sum256([]byte("solo"))
		_, err := lote.Firmante(0).Sign(rand.Reader, r[:], crypto.SHA256)
		hecho <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancelar()
	select {
	case err := <-hecho:
		esperarCodigo(t, err, csc.CodigoAutorizacionCaducada)
	case <-time.After(5 * time.Second):
		t.Fatal("la cancelación no liberó la espera")
	}
}

func TestCapacidadLote(t *testing.T) {
	casos := []struct {
		nombre    string
		multisign string
		modo      string
		manual    bool
		total     int
		esperado  int
		codigo    csc.Codigo
	}{
		{"sin multisign", "1", "implicit", false, 10, 1, ""},
		{"multisign ausente", "null", "implicit", false, 10, 1, ""},
		{"multisign 3", "3", "implicit", false, 10, 3, ""},
		{"cabe entero", "50", "implicit", false, 10, 10, ""},
		{"booleano", "true", "implicit", false, 500, csc.MaxFirmasLote, ""},
		{"enorme se acota", "100000", "implicit", false, 500, csc.MaxFirmasLote, ""},
		{"OTP en CLI se trocea", "3", "explicit", false, 10, 3, ""},
		{"OTP único que no cabe", "3", "explicit", true, 10, 0, csc.CodigoOTPLoteExcede},
		{"OTP único que cabe", "10", "explicit", true, 10, 10, ""},
		{"un solo documento", "10", "explicit", true, 1, 1, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := csctest.Nuevo(t)
			s.Configurar(func(s *csctest.Servidor) { s.MultisignJSON, s.Modo = c.multisign, c.modo })
			cliente, err := csc.Nuevo(csc.Opciones{
				URLServicio:    s.URL,
				ClientID:       csctest.ClientID,
				HTTP:           s.Client(),
				AbrirNavegador: s.Navegador(),
				EnvioOTPManual: c.manual,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cliente.Close() }()
			cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
			n, err := cliente.CapacidadLote(cred, c.total)
			if c.codigo != "" {
				esperarCodigo(t, err, c.codigo)
				return
			}
			if err != nil || n != c.esperado {
				t.Fatalf("capacidad = %d, %v; se esperaba %d", n, err, c.esperado)
			}
		})
	}
}

func TestServicioRechazaMasFirmasQueMultisign(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Multisign = 3 })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	// El cliente no deja pasar de la capacidad anunciada.
	if _, err := cliente.NuevoLote(context.Background(), cred, 4); err == nil {
		t.Fatal("NuevoLote aceptó más firmas que multisign")
	}
	// Si el servicio anuncia más de lo que admite, la autorización falla en
	// bloque y ningún documento queda firmado a medias.
	cred.Multisign = 4
	lote, err := cliente.NuevoLote(context.Background(), cred, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range firmarEnLote(lote, resumenesDe("1", "2", "3", "4"), crypto.SHA256) {
		esperarCodigo(t, r.err, csc.CodigoServicio)
	}
	lote.Cerrar()
}
