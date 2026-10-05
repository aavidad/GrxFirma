// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package cscremota_test

import (
	"context"
	"crypto"
	"crypto/sha256"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/csc"
	"grxfirma/internal/adapters/outbound/desktop/cscremota"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/csctest"
)

func nuevaSesion(t *testing.T, s *csctest.Servidor, permitida *atomic.Bool) (*cscremota.Sesion, string) {
	t.Helper()
	dir := t.TempDir()
	sesion := cscremota.Nueva(cscremota.Opciones{
		ConfigDir: dir,
		HTTP:      s.Client(),
		Navegador: s.Navegador(),
		CargarConfig: func() (config.Config, config.Policy, error) {
			cfg := config.Default()
			cfg.FirmaRemotaCSC = permitida.Load()
			return cfg, config.Policy{}, nil
		},
	})
	t.Cleanup(func() { _ = sesion.Close() })
	return sesion, dir
}

func codigo(t *testing.T, err error, esperado csc.Codigo) {
	t.Helper()
	if got := cscremota.CodigoVisible(err); got != esperado {
		t.Fatalf("código = %q, se esperaba %q (err=%v)", got, esperado, err)
	}
}

func TestSesionDesactivadaNoOfreceNada(t *testing.T) {
	s := csctest.Nuevo(t)
	var permitida atomic.Bool
	sesion, _ := nuevaSesion(t, s, &permitida)
	if sesion.Estado().Permitida {
		t.Fatal("la firma remota no debía estar permitida")
	}
	_, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID)
	codigo(t, err, cscremota.CodigoDesactivada)
	_, _, err = sesion.Conectar(context.Background())
	codigo(t, err, cscremota.CodigoDesactivada)
}

func TestSesionValidaComoLaCLI(t *testing.T) {
	s := csctest.Nuevo(t)
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	ctx := context.Background()
	casos := []struct {
		url, id string
		codigo  csc.Codigo
	}{
		{"http://firma.example", csctest.ClientID, csc.CodigoURLInvalida},
		{"https://usuario@firma.example", csctest.ClientID, csc.CodigoURLInvalida},
		{"https://firma.example/?a=b", csctest.ClientID, csc.CodigoURLInvalida},
		{s.URL, "con espacio", csc.CodigoParametroInvalido},
		{"", csctest.ClientID, csc.CodigoParametroInvalido},
		{s.URL, "", csc.CodigoParametroInvalido},
	}
	for _, c := range casos {
		_, err := sesion.Configurar(ctx, c.url, c.id)
		codigo(t, err, c.codigo)
	}
	_, _, err := sesion.Conectar(ctx)
	codigo(t, err, cscremota.CodigoNoConfigurada)
}

func TestSesionOAuthEnOtroHostSeRechazaAntesDeAbrirElNavegador(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.OAuthURL = "https://otro.example" })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, _ := nuevaSesion(t, s, &permitida)
	_, err := sesion.Configurar(context.Background(), s.URL, csctest.ClientID)
	codigo(t, err, csc.CodigoOAuthOtroHost)
}

func TestSesionCompletaConPINyOTP(t *testing.T) {
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo = "explicit" })
	var permitida atomic.Bool
	permitida.Store(true)
	sesion, dir := nuevaSesion(t, s, &permitida)
	ctx := context.Background()

	d, err := sesion.Configurar(ctx, s.URL, csctest.ClientID)
	if err != nil {
		t.Fatalf("Configurar: %v", err)
	}
	u, _ := url.Parse(s.URL)
	if d.HostServicio != u.Host || d.HostOAuth != u.Host {
		t.Fatalf("hosts = %+v, se esperaba %s", d, u.Host)
	}
	if refs, _ := sesion.List(ctx); len(refs) != 0 {
		t.Fatal("antes de conectar no debe haber certificados remotos")
	}
	creds, descartadas, err := sesion.Conectar(ctx)
	if err != nil || len(creds) != 2 || descartadas != 0 {
		t.Fatalf("Conectar: %d %d %v", len(creds), descartadas, err)
	}
	if !creds[0].PIN || !creds[0].OTP || !creds[0].OTPEnLinea {
		t.Fatalf("credencial explícita sin PIN/OTP: %+v", creds[0])
	}
	refs, _ := sesion.List(ctx)
	if len(refs) != 2 {
		t.Fatalf("List = %d", len(refs))
	}
	var rsaRef domain.CertificateRef
	for _, r := range refs {
		if strings.Contains(r.Subject, "RSA") {
			rsaRef = r
		}
	}
	if _, ok := sesion.Credencial(rsaRef.ID); !ok {
		t.Fatal("Credencial no encuentra el certificado remoto")
	}
	if _, err := sesion.KeyFor(ctx, domain.CertificateRef{ID: "local"}); !errors.Is(err, cscremota.ErrNoAplicable) {
		t.Fatalf("un certificado local debía dar ErrNoAplicable: %v", err)
	}
	if err := sesion.EnviarOTP(ctx, rsaRef.ID); err != nil {
		t.Fatalf("EnviarOTP: %v", err)
	}

	pin, otp := []byte(csctest.PIN), []byte(csctest.OTP)
	ctxFirma, peticion := cscremota.ContextoConSecretos(ctx, pin, otp)
	clave, err := sesion.KeyFor(ctxFirma, rsaRef)
	if err != nil {
		t.Fatalf("KeyFor: %v", err)
	}
	local := clave.(*deskSigner.ClaveLocal)
	resumen := sha256.Sum256([]byte("documento"))
	if _, err := local.SignDigest(resumen[:], crypto.SHA256); err != nil {
		t.Fatalf("firma remota: %v", err)
	}
	// Un segundo documento en la misma petición no puede reutilizar el OTP.
	otro := sha256.Sum256([]byte("otro"))
	_, err = local.SignDigest(otro[:], crypto.SHA256)
	codigo(t, err, cscremota.CodigoOTPLote)
	codigo(t, peticion.Error(), cscremota.CodigoOTPLote)
	s.Leer(func(s *csctest.Servidor) {
		if s.OTPEnviados != 1 || len(s.ResumenesFirmados) != 1 {
			t.Fatalf("OTP enviados %d, firmas %d", s.OTPEnviados, len(s.ResumenesFirmados))
		}
	})

	// Sin secretos en la petición, la firma falla sin pedir nada.
	sinSecretos, _ := sesion.KeyFor(ctx, rsaRef)
	_, err = sinSecretos.(*deskSigner.ClaveLocal).SignDigest(resumen[:], crypto.SHA256)
	codigo(t, err, cscremota.CodigoSecretoNoPedido)

	// La configuración se recuerda, pero no la sesión.
	datos, err := os.ReadFile(filepath.Join(dir, cscremota.FicheroConfiguracion))
	if err != nil || strings.Contains(string(datos), "token") {
		t.Fatalf("configuración guardada: %q %v", datos, err)
	}
	otra := cscremota.Nueva(cscremota.Opciones{ConfigDir: dir, HTTP: s.Client(), CargarConfig: func() (config.Config, config.Policy, error) {
		cfg := config.Default()
		cfg.FirmaRemotaCSC = true
		return cfg, config.Policy{}, nil
	}})
	if e := otra.Estado(); e.URL != s.URL || e.ClientID != csctest.ClientID || e.Conectada || e.Descubierto {
		t.Fatalf("estado recuperado: %+v", e)
	}

	// Si la política la retira, la sesión se cierra y no queda nada.
	permitida.Store(false)
	if refs, _ := sesion.List(ctx); len(refs) != 0 {
		t.Fatal("con la firma remota retirada no deben quedar certificados")
	}
	permitida.Store(true)
	if _, err := sesion.KeyFor(ctx, rsaRef); !errors.Is(err, cscremota.ErrNoAplicable) {
		t.Fatalf("tras cerrar, KeyFor = %v", err)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Revocados != 1 {
			t.Fatalf("el token no se revocó al cerrar: %d", s.Revocados)
		}
	})
}

func TestSecretoValido(t *testing.T) {
	if !cscremota.SecretoValido(nil) || !cscremota.SecretoValido([]byte("1234")) {
		t.Fatal("secretos válidos rechazados")
	}
	if cscremota.SecretoValido([]byte("12\n34")) || cscremota.SecretoValido([]byte{0xff}) ||
		cscremota.SecretoValido([]byte(strings.Repeat("1", 300))) {
		t.Fatal("secreto no válido aceptado")
	}
}
