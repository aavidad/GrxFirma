// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"net/http"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/testsupport/csctest"
)

func autorizar(t *testing.T, cliente *csc.Cliente) {
	t.Helper()
	if err := cliente.Autorizar(context.Background()); err != nil {
		t.Fatalf("Autorizar: %v", err)
	}
}

func TestListadoRecorreTodasLasPaginas(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.CredencialesExtra, s.TamPagina = 5, 2 })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	ids, err := cliente.ListarCredenciales(context.Background())
	if err != nil {
		t.Fatalf("ListarCredenciales: %v", err)
	}
	if len(ids) != 7 || ids[0] != csctest.CredencialRSA || ids[6] != "cred-extra-4" {
		t.Fatalf("ids = %v", ids)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.PeticionesListado != 4 {
			t.Fatalf("páginas pedidas = %d", s.PeticionesListado)
		}
	})
}

func TestListadoMasDeCienCredenciales(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.CredencialesExtra, s.TamPagina = 250, 100 })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	ids, err := cliente.ListarCredenciales(context.Background())
	if err != nil || len(ids) != 252 {
		t.Fatalf("ids = %d, %v", len(ids), err)
	}
}

func TestListadoCiclicoODemasiadoLargoSeCorta(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.CredencialesExtra, s.TamPagina, s.PaginaCiclica = 5, 2, true })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	_, err := cliente.ListarCredenciales(context.Background())
	esperarCodigo(t, err, csc.CodigoRespuestaInvalida)

	largo := csctest.Nuevo(t)
	largo.Configurar(func(s *csctest.Servidor) { s.CredencialesExtra, s.TamPagina = 1100, 100 })
	otro := nuevoCliente(t, largo)
	autorizar(t, otro)
	_, err = otro.ListarCredenciales(context.Background())
	esperarCodigo(t, err, csc.CodigoDemasiadasCredenciales)
}

func TestTokenQueCaducaSeRenuevaConRefreshToken(t *testing.T) {
	s := csctest.Nuevo(t)
	// Una vida de 1 s cae dentro del margen de renovación: cada operación
	// tiene que renovar antes de usar el token.
	s.Configurar(func(s *csctest.Servidor) { s.DarRefresh, s.VidaToken = true, 1 })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	firmante, _ := cliente.Firmante(context.Background(), cred)
	r := sha256.Sum256([]byte("renovado"))
	if _, err := firmante.Sign(rand.Reader, r[:], crypto.SHA256); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Renovaciones < 2 {
			t.Fatalf("renovaciones = %d", s.Renovaciones)
		}
	})
	if err := cliente.Close(); err != nil {
		t.Fatal(err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.RevocadosRefresh != 1 {
			t.Fatalf("refresh_token revocados = %d", s.RevocadosRefresh)
		}
	})
}

func TestRefreshSinRotarConservaElAnterior(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.DarRefresh, s.RefreshSinRotar, s.VidaToken = true, true, 1 })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	for i := 0; i < 3; i++ {
		if _, err := cliente.ListarCredenciales(context.Background()); err != nil {
			t.Fatalf("listado %d: %v", i, err)
		}
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Renovaciones != 3 {
			t.Fatalf("renovaciones = %d", s.Renovaciones)
		}
	})
}

func TestToken401SeRenuevaYRepiteUnaVez(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.DarRefresh = true })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	s.CaducarTokensServicio()
	ids, err := cliente.ListarCredenciales(context.Background())
	if err != nil || len(ids) != 2 {
		t.Fatalf("ListarCredenciales tras 401: %v %v", ids, err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Renovaciones != 1 {
			t.Fatalf("renovaciones = %d", s.Renovaciones)
		}
	})
}

func TestSinRefreshTokenPideConectarDeNuevo(t *testing.T) {
	s := csctest.Nuevo(t)
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	s.CaducarTokensServicio()
	_, err := cliente.ListarCredenciales(context.Background())
	esperarCodigo(t, err, csc.CodigoSesionCaducada)
	// El token ya se da por caducado: la firma tampoco lo usa.
	firmante, _ := cliente.Firmante(context.Background(), cred)
	r := sha256.Sum256([]byte("x"))
	_, err = firmante.Sign(rand.Reader, r[:], crypto.SHA256)
	esperarCodigo(t, err, csc.CodigoSesionCaducada)
	// Autorizar vuelve al navegador y la sesión sigue.
	autorizar(t, cliente)
	if _, err := cliente.ListarCredenciales(context.Background()); err != nil {
		t.Fatalf("tras volver a conectar: %v", err)
	}
}

func TestTokenCaducadoPorTiempoSinRefresh(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.VidaToken = 1 })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	time.Sleep(1100 * time.Millisecond)
	_, err := cliente.ListarCredenciales(context.Background())
	esperarCodigo(t, err, csc.CodigoSesionCaducada)
}

func TestRefreshRechazadoPideConectarDeNuevo(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.DarRefresh, s.VidaToken = true, 1 })
	cliente := nuevoCliente(t, s)
	autorizar(t, cliente)
	time.Sleep(1100 * time.Millisecond)
	s.Configurar(func(s *csctest.Servidor) { s.RefreshRechazado = true })
	_, err := cliente.ListarCredenciales(context.Background())
	esperarCodigo(t, err, csc.CodigoSesionCaducada)
	if strings.Contains(err.Error(), "refresh") && strings.Contains(err.Error(), "=") {
		t.Fatalf("el error no debe llevar datos del formulario: %v", err)
	}
}

func TestDescubrimientoPorOAuth2Issuer(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Emisor, s.DarRefresh = "/emisor", true })
	cliente := nuevoCliente(t, s)
	cred := autorizarYCredencial(t, cliente, csctest.CredencialRSA)
	if !strings.HasSuffix(cliente.ServidorOAuth(), "/as/authorize") {
		t.Fatalf("extremo de autorización = %s", cliente.ServidorOAuth())
	}
	firmante, _ := cliente.Firmante(context.Background(), cred)
	r := sha256.Sum256([]byte("emisor"))
	if _, err := firmante.Sign(rand.Reader, r[:], crypto.SHA256); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	_ = cliente.Close()
	s.Leer(func(s *csctest.Servidor) {
		if s.Revocados != 1 || s.RevocadosRefresh != 1 {
			t.Fatalf("revocación por el extremo de los metadatos: %d/%d", s.Revocados, s.RevocadosRefresh)
		}
	})
}

func TestMetadatosOAuthNoValidosSeRechazan(t *testing.T) {
	casos := []struct {
		nombre  string
		alterar func(s *csctest.Servidor, m map[string]any)
		codigo  csc.Codigo
	}{
		{"emisor distinto", func(s *csctest.Servidor, m map[string]any) { m["issuer"] = s.URL + "/otro" }, csc.CodigoOAuthMetadatos},
		{"sin S256", func(_ *csctest.Servidor, m map[string]any) {
			m["code_challenge_methods_supported"] = []string{"plain"}
		}, csc.CodigoOAuthMetadatos},
		{"token en http", func(s *csctest.Servidor, m map[string]any) {
			m["token_endpoint"] = strings.Replace(s.URL, "https://", "http://", 1) + "/as/token"
		}, csc.CodigoOAuthMetadatos},
		{"autorización en otro host", func(_ *csctest.Servidor, m map[string]any) {
			m["authorization_endpoint"] = "https://atacante.example/authorize"
		}, csc.CodigoOAuthOtroHost},
		{"revocación en otro host", func(_ *csctest.Servidor, m map[string]any) {
			m["revocation_endpoint"] = "https://atacante.example/revoke"
		}, csc.CodigoOAuthOtroHost},
		{"sin extremo de token", func(_ *csctest.Servidor, m map[string]any) { delete(m, "token_endpoint") }, csc.CodigoOAuthMetadatos},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := csctest.Nuevo(t)
			s.Configurar(func(srv *csctest.Servidor) {
				srv.Emisor = "/emisor"
				srv.MetadatosOverride = func(m map[string]any) { c.alterar(srv, m) }
			})
			cliente := nuevoCliente(t, s)
			_, err := cliente.Info(context.Background())
			esperarCodigo(t, err, c.codigo)
		})
	}
}

func TestEmisorEnOtroHostSeRechaza(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(srv *csctest.Servidor) {
		srv.InfoOverride = func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"specs":"2.1.0","authType":["oauth2code"],"oauth2Issuer":"https://atacante.example/emisor"}`))
		}
	})
	cliente := nuevoCliente(t, s)
	_, err := cliente.Info(context.Background())
	esperarCodigo(t, err, csc.CodigoOAuthOtroHost)
}
