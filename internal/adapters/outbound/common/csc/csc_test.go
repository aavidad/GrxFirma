// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"net/http"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/testsupport/csctest"
)

func nuevoCliente(t *testing.T, s *csctest.Servidor, secretos ...string) *csc.Cliente {
	t.Helper()
	pendientes := append([]string(nil), secretos...)
	cliente, err := csc.Nuevo(csc.Opciones{
		URLServicio:        s.URL,
		ClientID:           csctest.ClientID,
		HTTP:               s.Client(),
		AbrirNavegador:     s.Navegador(),
		EsperaAutorizacion: 10 * time.Second,
		TextoCallback:      "ok",
		PedirSecreto: func(_ context.Context, tipo csc.TipoSecreto) ([]byte, error) {
			if len(pendientes) == 0 {
				t.Fatalf("se pidió un secreto %s no previsto", tipo)
			}
			v := pendientes[0]
			pendientes = pendientes[1:]
			return []byte(v), nil
		},
	})
	if err != nil {
		t.Fatalf("Nuevo: %v", err)
	}
	t.Cleanup(func() { _ = cliente.Close() })
	return cliente
}

func autorizarYCredencial(t *testing.T, cliente *csc.Cliente, id string) *csc.Credencial {
	t.Helper()
	ctx := context.Background()
	if _, err := cliente.Info(ctx); err != nil {
		t.Fatalf("Info: %v", err)
	}
	if err := cliente.Autorizar(ctx); err != nil {
		t.Fatalf("Autorizar: %v", err)
	}
	ids, err := cliente.ListarCredenciales(ctx)
	if err != nil || len(ids) != 2 {
		t.Fatalf("ListarCredenciales: %v %v", ids, err)
	}
	cred, err := cliente.Credencial(ctx, id)
	if err != nil {
		t.Fatalf("Credencial: %v", err)
	}
	return cred
}

func esperarCodigo(t *testing.T, err error, codigo csc.Codigo) {
	t.Helper()
	if err == nil {
		t.Fatalf("se esperaba el error %s y no hubo error", codigo)
	}
	if got := csc.CodigoDe(err); got != codigo {
		t.Fatalf("código = %q, se esperaba %q (err=%v)", got, codigo, err)
	}
}

func TestFlujoCompletoImplicitoSoloEnviaElResumen(t *testing.T) {
	s := csctest.Nuevo(t)
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	if cred.SCAL != "1" || cred.Modo != csc.ModoImplicito || len(cred.Cadena) != 1 {
		t.Fatalf("credencial inesperada: %+v", cred)
	}
	if ref := cred.Referencia(); ref.ID == "" || !ref.HasSigningKey || len(ref.ChainDER) != 1 {
		t.Fatalf("referencia inesperada: %+v", ref)
	}
	firmante, err := cliente.Firmante(context.Background(), cred)
	if err != nil {
		t.Fatal(err)
	}
	documento := []byte("documento que nunca debe salir del equipo")
	resumen := sha256.Sum256(documento)
	firma, err := firmante.Sign(rand.Reader, resumen[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(firmante.Public().(*rsa.PublicKey), crypto.SHA256, resumen[:], firma); err != nil {
		t.Fatalf("la firma remota no verifica: %v", err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if len(s.ResumenesFirmados) != 1 || !bytes.Equal(s.ResumenesFirmados[0], resumen[:]) {
			t.Fatalf("el servicio recibió %d resúmenes inesperados", len(s.ResumenesFirmados))
		}
		if s.UltimoSignAlgo != "1.2.840.113549.1.1.11" {
			t.Fatalf("signAlgo = %s", s.UltimoSignAlgo)
		}
	})

	if err := cliente.Close(); err != nil {
		t.Fatal(err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Revocados != 1 {
			t.Fatalf("el token de servicio no se revocó al cerrar (%d)", s.Revocados)
		}
	})
	_, err = firmante.Sign(rand.Reader, resumen[:], crypto.SHA256)
	esperarCodigo(t, err, csc.CodigoSesionCerrada)
}

func TestExplicitoSCAL2PideSecretosYLigaLosResumenes(t *testing.T) {
	for _, v21 := range []bool{false, true} {
		s := csctest.Nuevo(t)
		s.Configurar(func(s *csctest.Servidor) {
			s.Modo, s.SCAL, s.AuthV21 = "explicit", "2", v21
		})
		cliente := nuevoCliente(t, s, csctest.PIN, csctest.OTP)
		cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
		if !cred.PIN || !cred.OTP || cred.SCAL != "2" {
			t.Fatalf("v21=%v: credencial inesperada %+v", v21, cred)
		}
		firmante, _ := cliente.Firmante(context.Background(), cred)
		resumen := sha512.Sum384([]byte("x"))
		if _, err := firmante.Sign(rand.Reader, resumen[:], crypto.SHA384); err != nil {
			t.Fatalf("v21=%v: Sign: %v", v21, err)
		}
		s.Leer(func(s *csctest.Servidor) {
			if s.Autorizaciones != 1 {
				t.Fatalf("v21=%v: autorizaciones = %d", v21, s.Autorizaciones)
			}
			if !v21 && s.OTPEnviados != 1 {
				t.Fatalf("OTP en línea no solicitado: %d", s.OTPEnviados)
			}
		})
	}
}

func TestExplicitoPINIncorrectoNoFirmaNiFiltraElSecreto(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo = "explicit" })
	cliente := nuevoCliente(t, s, "9999", csctest.OTP)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	firmante, _ := cliente.Firmante(context.Background(), cred)
	resumen := sha256.Sum256([]byte("x"))
	_, err := firmante.Sign(rand.Reader, resumen[:], crypto.SHA256)
	esperarCodigo(t, err, csc.CodigoServicio)
	if strings.Contains(err.Error(), "9999") || strings.Contains(err.Error(), csctest.OTP) {
		t.Fatalf("el error contiene un secreto: %v", err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if len(s.ResumenesFirmados) != 0 {
			t.Fatal("no debió llegar ninguna firma")
		}
	})
}

func TestOAuth2CodeAutorizaLaCredencialEnElNavegador(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo, s.SCAL = "oauth2code", "2" })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	firmante, _ := cliente.Firmante(context.Background(), cred)
	resumen := sha256.Sum256([]byte("oauth2code"))
	firma, err := firmante.Sign(rand.Reader, resumen[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(cred.Certificado.PublicKey.(*rsa.PublicKey), crypto.SHA256, resumen[:], firma); err != nil {
		t.Fatal(err)
	}
}

func TestRSAPSSEnviaParametrosYVerifica(t *testing.T) {
	s := csctest.Nuevo(t)
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	cred.Algoritmos = append(cred.Algoritmos, "1.2.840.113549.1.1.10")
	firmante, _ := cliente.Firmante(context.Background(), cred)
	resumen := sha256.Sum256([]byte("pss"))
	opciones := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
	firma, err := firmante.Sign(rand.Reader, resumen[:], opciones)
	if err != nil {
		t.Fatalf("Sign PSS: %v", err)
	}
	if err := rsa.VerifyPSS(firmante.Public().(*rsa.PublicKey), crypto.SHA256, resumen[:], firma, opciones); err != nil {
		t.Fatal(err)
	}
}

func TestECDSAAceptaDERyRSCrudo(t *testing.T) {
	for _, cruda := range []bool{false, true} {
		s := csctest.Nuevo(t)
		s.Configurar(func(s *csctest.Servidor) { s.ECDSACruda = cruda })
		cliente := nuevoCliente(t, s)
		cred := autorizarYCredencial(t, cliente, csctest.CredencialEC)
		firmante, _ := cliente.Firmante(context.Background(), cred)
		resumen := sha256.Sum256([]byte("ec"))
		firma, err := firmante.Sign(rand.Reader, resumen[:], crypto.SHA256)
		if err != nil {
			t.Fatalf("cruda=%v: %v", cruda, err)
		}
		if !ecdsa.VerifyASN1(firmante.Public().(*ecdsa.PublicKey), resumen[:], firma) {
			t.Fatalf("cruda=%v: la firma devuelta no es DER válido", cruda)
		}
		s.Leer(func(s *csctest.Servidor) {
			if s.UltimoSignAlgo != "1.2.840.10045.2.1" {
				t.Fatalf("signAlgo = %s", s.UltimoSignAlgo)
			}
		})
	}
}

func TestFirmaQueNoCorrespondeAlCertificadoSeRechaza(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.FirmaFalsa = true })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	firmante, _ := cliente.Firmante(context.Background(), cred)
	resumen := sha256.Sum256([]byte("x"))
	_, err := firmante.Sign(rand.Reader, resumen[:], crypto.SHA256)
	esperarCodigo(t, err, csc.CodigoFirmaInvalida)
}

func TestSignRechazaResumenesYAlgoritmosNoAdmitidos(t *testing.T) {
	s := csctest.Nuevo(t)
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	firmante, _ := cliente.Firmante(context.Background(), cred)
	_, err := firmante.Sign(rand.Reader, []byte("corto"), crypto.SHA256)
	esperarCodigo(t, err, csc.CodigoParametroInvalido)
	_, err = firmante.Sign(rand.Reader, make([]byte, 20), crypto.SHA1)
	esperarCodigo(t, err, csc.CodigoAlgoritmoNoSoportado)
	_, err = firmante.Sign(rand.Reader, []byte("mensaje"), crypto.Hash(0))
	esperarCodigo(t, err, csc.CodigoAlgoritmoNoSoportado)
}

func TestStateDistintoAbortaLaAutorizacion(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.StateFalso = true })
	cliente := nuevoCliente(t, s)
	err := cliente.Autorizar(context.Background())
	esperarCodigo(t, err, csc.CodigoStateInvalido)
	if _, err := cliente.ListarCredenciales(context.Background()); csc.CodigoDe(err) != csc.CodigoAutorizacionCaducada {
		t.Fatalf("sin token no debe poder listar: %v", err)
	}
}

func TestNoSigueRedireccionesAOtroHost(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) {
		s.InfoOverride = func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://otro-host.invalid/csc/v2/info", http.StatusTemporaryRedirect)
		}
	})
	cliente := nuevoCliente(t, s)
	_, err := cliente.Info(context.Background())
	esperarCodigo(t, err, csc.CodigoRedireccion)
}

func TestRespuestaEnormeSeCorta(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) {
		s.InfoOverride = func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"specs":"2.0.0.2","name":"`))
			relleno := bytes.Repeat([]byte("a"), 64*1024)
			for i := 0; i < 40; i++ {
				_, _ = w.Write(relleno)
			}
			_, _ = w.Write([]byte(`"}`))
		}
	})
	cliente := nuevoCliente(t, s)
	_, err := cliente.Info(context.Background())
	esperarCodigo(t, err, csc.CodigoRespuestaGrande)
}

func TestHTTPPlanoSeRechaza(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8443", "http://csc.example.org", "ftp://csc.example.org", "https://usuario:clave@csc.example.org", "https://csc.example.org/?x=1"} {
		_, err := csc.Nuevo(csc.Opciones{URLServicio: u, ClientID: "c", AbrirNavegador: csc.AbrirNavegadorSistema})
		if err == nil {
			t.Fatalf("%s debió rechazarse", u)
		}
	}
	// El servidor OAuth anunciado por /info también debe ser https.
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) {
		s.InfoOverride = func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"specs":"2.0.0.2","authType":["oauth2code"],"oauth2":"http://127.0.0.1:1"}`))
		}
	})
	_, err := nuevoCliente(t, s).Info(context.Background())
	esperarCodigo(t, err, csc.CodigoSoloHTTPS)
}

func TestServicioSinOAuthSeRechaza(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) {
		s.InfoOverride = func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"specs":"2.0.0.2","authType":["basic"]}`))
		}
	})
	_, err := nuevoCliente(t, s).Info(context.Background())
	esperarCodigo(t, err, csc.CodigoSinOAuth)
}

func TestCertificadoTLSNoConfiableSeRechaza(t *testing.T) {
	s := csctest.Nuevo(t)
	cliente, err := csc.Nuevo(csc.Opciones{URLServicio: s.URL, ClientID: csctest.ClientID, AbrirNavegador: s.Navegador()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cliente.Info(context.Background())
	esperarCodigo(t, err, csc.CodigoRed)
}

func TestClientIDYCredencialSeValidan(t *testing.T) {
	if _, err := csc.Nuevo(csc.Opciones{URLServicio: "https://csc.example.org", ClientID: "con espacio", AbrirNavegador: csc.AbrirNavegadorSistema}); csc.CodigoDe(err) != csc.CodigoParametroInvalido {
		t.Fatalf("client_id con espacio: %v", err)
	}
	s := csctest.Nuevo(t)
	cliente := nuevoCliente(t, s)
	if _, err := cliente.Credencial(context.Background(), "id\x1b[31m"); csc.CodigoDe(err) != csc.CodigoParametroInvalido {
		t.Fatalf("credencial con control: %v", err)
	}
}
