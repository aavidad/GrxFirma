// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/desktop/cscremota"
	desktopsigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/testsupport/csctest"
)

type entornoCSCIPC struct {
	m         *Manejador
	servidor  *csctest.Servidor
	sesion    *cscremota.Sesion
	permitida *atomic.Bool
	prohibir  *atomic.Bool
}

func nuevoEntornoCSCIPC(t *testing.T, modo string) entornoCSCIPC {
	t.Helper()
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo = modo })
	permitida := &atomic.Bool{}
	permitida.Store(true)
	prohibir := &atomic.Bool{}
	sesion := cscremota.Nueva(cscremota.Opciones{
		ConfigDir: t.TempDir(),
		HTTP:      s.Client(),
		Navegador: s.Navegador(),
		CargarConfig: func() (config.Config, config.Policy, error) {
			cfg := config.Default()
			cfg.FirmaRemotaCSC = permitida.Load()
			if prohibir.Load() {
				falso := false
				return cfg, config.Policy{FirmaRemotaCSC: &falso}, nil
			}
			return cfg, config.Policy{}, nil
		},
	})
	t.Cleanup(func() { _ = sesion.Close() })
	motor := desktopsigner.NuevoMotorFirmaGo(nil)
	firmar := application.NuevoSignDocumentUseCase(sesion, sesion, motor, aprobacionFirmaContrato{}, nil, nil)
	lote := application.NuevoProcessBatchUseCase(sesion, sesion, motor, aprobacionFirmaContrato{}, nil, nil)
	m := &Manejador{Catalogo: sesion, Firmar: firmar, ProcesarLote: lote, CSC: sesion}
	return entornoCSCIPC{m: m, servidor: s, sesion: sesion, permitida: permitida, prohibir: prohibir}
}

func pedirIPC(t *testing.T, m *Manejador, accion string, params any) respuesta {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return m.despachar(context.Background(), peticion{Action: accion, Params: raw})
}

func sinSecretosEnRespuesta(t *testing.T, resp respuesta, secretos ...string) {
	t.Helper()
	data, _ := json.Marshal(resp)
	texto := strings.ToLower(string(data))
	for _, prohibido := range append([]string{"token", "sad", "access_token", "code_verifier"}, secretos...) {
		if strings.Contains(texto, strings.ToLower(prohibido)) {
			t.Fatalf("la respuesta %s contiene %q: %s", resp.Action, prohibido, data)
		}
	}
}

func TestCSCIPCSinSesionNoEstaDisponible(t *testing.T) {
	m := &Manejador{}
	resp := pedirIPC(t, m, "csc_status", map[string]any{})
	if !resp.OK || resp.Data.(resultadoCSCEstado).Allowed {
		t.Fatalf("csc_status sin sesión: %+v", resp)
	}
	resp = pedirIPC(t, m, "csc_connect", map[string]any{})
	if resp.OK || resp.ErrorCode != "csc_desactivada" {
		t.Fatalf("csc_connect sin sesión: %+v", resp)
	}
}

func TestCSCIPCValidaParametrosEstrictamente(t *testing.T) {
	e := nuevoEntornoCSCIPC(t, "implicit")
	casos := []struct {
		accion string
		params any
	}{
		{"csc_status", map[string]any{"extra": 1}},
		{"csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID, "clientSecret": "x"}},
		{"csc_configure", map[string]any{"serviceUrl": e.servidor.URL}},
		{"csc_configure", map[string]any{"serviceUrl": e.servidor.URL + "\n", "clientId": "a\u0000b"}},
		{"csc_configure", map[string]any{"serviceUrl": strings.Repeat("a", 3000), "clientId": "x"}},
		{"csc_configure", []string{"no", "objeto"}},
		{"csc_connect", map[string]any{"serviceUrl": e.servidor.URL}},
		{"csc_send_otp", map[string]any{"certificateId": "../../etc"}},
		{"csc_send_otp", map[string]any{}},
	}
	for _, c := range casos {
		resp := pedirIPC(t, e.m, c.accion, c.params)
		if resp.OK || resp.ErrorCode != "invalid_params" {
			t.Fatalf("%s %v debía rechazarse: %+v", c.accion, c.params, resp)
		}
	}
	resp := pedirIPC(t, e.m, "csc_configure", map[string]any{"serviceUrl": "http://firma.example", "clientId": csctest.ClientID})
	if resp.OK || resp.ErrorCode != "csc_url_invalida" {
		t.Fatalf("http plano: %+v", resp)
	}
	resp = pedirIPC(t, e.m, "csc_connect", map[string]any{})
	if resp.OK || resp.ErrorCode != "csc_no_configurada" {
		t.Fatalf("conectar sin configurar: %+v", resp)
	}
}

func TestCSCIPCFlujoCompletoConPINyOTP(t *testing.T) {
	e := nuevoEntornoCSCIPC(t, "explicit")
	u, _ := url.Parse(e.servidor.URL)

	resp := pedirIPC(t, e.m, "csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID})
	if !resp.OK {
		t.Fatalf("csc_configure: %+v", resp)
	}
	d := resp.Data.(resultadoCSCDescubrimiento)
	if d.ServiceHost != u.Host || d.OAuthHost != u.Host {
		t.Fatalf("hosts: %+v", d)
	}
	resp = pedirIPC(t, e.m, "csc_connect", map[string]any{})
	if !resp.OK {
		t.Fatalf("csc_connect: %+v", resp)
	}
	sinSecretosEnRespuesta(t, resp)
	conexion := resp.Data.(resultadoCSCConexion)
	if len(conexion.Credentials) != 2 {
		t.Fatalf("credenciales: %+v", conexion)
	}
	var certRSA string
	for _, c := range conexion.Credentials {
		if strings.Contains(c.Subject, "RSA") {
			certRSA = c.CertificateID
			if !c.PIN || !c.OTP || !c.OTPOnline || c.Mode != "explicit" {
				t.Fatalf("credencial RSA: %+v", c)
			}
		}
	}

	resp = pedirIPC(t, e.m, "certificates", map[string]any{})
	certs, _ := resp.Data.([]certJSON)
	remotos := 0
	for _, c := range certs {
		if c.Remote && c.RemotePIN && c.RemoteOTP && c.RemoteOTPOnline {
			remotos++
		}
	}
	if !resp.OK || remotos != 2 {
		t.Fatalf("certificates no marca los remotos: %+v", resp)
	}

	if resp := pedirIPC(t, e.m, "csc_send_otp", map[string]any{"certificateId": certRSA}); !resp.OK {
		t.Fatalf("csc_send_otp: %+v", resp)
	}

	dir := t.TempDir()
	entrada := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(entrada, []byte("documento remoto"), 0o600); err != nil {
		t.Fatal(err)
	}
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	// Sin PIN ni OTP la firma falla con un mensaje propio, sin pedir nada.
	resp = pedirIPC(t, e.m, "sign", map[string]any{"inputPath": entrada, "outputPath": filepath.Join(dir, "a.csig"), "certificateId": certRSA, "format": "cades"})
	if resp.OK || resp.ErrorCode != "csc_secreto_no_pedido" {
		t.Fatalf("firma sin secretos: %+v", resp)
	}

	salida := filepath.Join(dir, "doc.csig")
	resp = pedirIPC(t, e.m, "sign", map[string]any{
		"inputPath": entrada, "outputPath": salida, "certificateId": certRSA, "format": "cades",
		"remotePin": b64(csctest.PIN), "remoteOtp": b64(csctest.OTP),
	})
	if !resp.OK {
		t.Fatalf("firma remota: %+v", resp)
	}
	sinSecretosEnRespuesta(t, resp, csctest.PIN, csctest.OTP)
	if info, err := os.Stat(salida); err != nil || info.Size() == 0 {
		t.Fatalf("no se escribió la firma: %v", err)
	}

	// Sin multisign, un lote con OTP no se admite: el código solo vale una vez.
	resp = pedirIPC(t, e.m, "sign_batch", map[string]any{"inputPaths": []string{entrada}, "certificateId": certRSA, "format": "cades", "remotePin": b64(csctest.PIN)})
	if resp.OK || resp.ErrorCode != "csc_otp_lote" {
		t.Fatalf("lote con OTP: %+v", resp)
	}

	// Secretos para un certificado que no es remoto: se rechazan.
	resp = pedirIPC(t, e.m, "sign", map[string]any{"inputPath": entrada, "certificateId": strings.Repeat("a", 64), "remotePin": b64("1234")})
	if resp.OK || resp.ErrorCode != "csc_secreto_no_pedido" {
		t.Fatalf("secreto no pedido: %+v", resp)
	}
	resp = pedirIPC(t, e.m, "sign", map[string]any{"inputPath": entrada, "certificateId": certRSA, "remotePin": b64("12\n34")})
	if resp.OK || resp.ErrorCode != "invalid_params" {
		t.Fatalf("PIN con control: %+v", resp)
	}

	// Una política que la prohíbe se distingue de la desactivada.
	e.prohibir.Store(true)
	resp = pedirIPC(t, e.m, "csc_status", map[string]any{})
	if estado := resp.Data.(resultadoCSCEstado); !resp.OK || estado.Allowed || !estado.ProhibitedByPolicy {
		t.Fatalf("csc_status con política en contra: %+v", resp)
	}
	resp = pedirIPC(t, e.m, "csc_connect", map[string]any{})
	if resp.OK || resp.ErrorCode != "csc_prohibida" {
		t.Fatalf("csc_connect con política en contra: %+v", resp)
	}
	e.prohibir.Store(false)
	pedirIPC(t, e.m, "csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID})
	pedirIPC(t, e.m, "csc_connect", map[string]any{})

	// La configuración la retira: la sesión desaparece.
	e.permitida.Store(false)
	resp = pedirIPC(t, e.m, "csc_status", map[string]any{})
	if estado := resp.Data.(resultadoCSCEstado); !resp.OK || estado.Allowed || estado.ProhibitedByPolicy {
		t.Fatalf("csc_status tras retirar: %+v", resp)
	}
	e.permitida.Store(true)
	if _, ok := e.sesion.Credencial(certRSA); ok {
		t.Fatal("la credencial remota debía haberse olvidado")
	}

	resp = pedirIPC(t, e.m, "csc_disconnect", map[string]any{})
	if !resp.OK {
		t.Fatalf("csc_disconnect: %+v", resp)
	}
}

func TestCSCIPCAccionesSensiblesYLargas(t *testing.T) {
	for _, accion := range []string{"sign", "sign_batch", "sign_multicosign"} {
		if !isSensitiveIPCAction(accion) {
			t.Fatalf("%s debe borrar sus parámetros", accion)
		}
	}
	if desktopIPCTimeoutDe("csc_connect") != longIPCOperationTimeout {
		t.Fatal("csc_connect espera al navegador y necesita el tiempo largo")
	}
}
