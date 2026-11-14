// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/testsupport/csctest"
)

// ID-01/JD-03: otro proceso del mismo usuario que llega al socket no puede
// configurar un servicio CSC, abrir la autorización ni firmar con una
// credencial remota si no es la interfaz vinculada por PID.
func TestCSCIPCSinInterfazVerificadaRechazaLaFirmaRemota(t *testing.T) {
	e := nuevoEntornoCSCIPC(t, "explicit")
	if resp := pedirIPC(t, e.m, "csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID}); !resp.OK {
		t.Fatalf("csc_configure: %+v", resp)
	}
	resp := pedirIPC(t, e.m, "csc_connect", map[string]any{})
	if !resp.OK {
		t.Fatalf("csc_connect: %+v", resp)
	}
	creds := resp.Data.(resultadoCSCConexion).Credentials
	remoto := creds[0].CertificateID
	entrada := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(entrada, []byte("documento"), 0o600); err != nil {
		t.Fatal(err)
	}
	pin := base64.StdEncoding.EncodeToString([]byte(csctest.PIN))
	otp := base64.StdEncoding.EncodeToString([]byte(csctest.OTP))
	local := strings.Repeat("a", 64)

	for _, estado := range []string{"", interfazSinVinculo, interfazNoVerificada} {
		ctx := context.Background()
		if estado != "" {
			ctx = conEstadoInterfaz(ctx, estado)
		}
		for _, c := range []struct {
			accion string
			params map[string]any
		}{
			{"csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID}},
			{"csc_connect", map[string]any{}},
			{"csc_send_otp", map[string]any{"certificateId": remoto}},
			{"sign", map[string]any{"inputPath": entrada, "certificateId": remoto, "format": "cades", "remotePin": pin, "remoteOtp": otp}},
			{"protect_sign", map[string]any{"inputPath": entrada, "certificateId": remoto, "remotePin": pin, "remoteOtp": otp}},
			{"sign_batch", map[string]any{"inputPaths": []string{entrada}, "certificateId": remoto, "format": "cades", "remotePin": pin}},
			// Un firmante adicional remoto tampoco firma desde fuera.
			{"sign_multicosign", map[string]any{"inputPath": entrada, "certificateId": local, "format": "cades", "additionalCertificateIds": []string{remoto}}},
		} {
			raw, _ := json.Marshal(c.params)
			resp := e.m.despachar(ctx, peticion{Action: c.accion, Params: raw})
			if resp.OK || resp.ErrorCode != "csc_interfaz_no_verificada" {
				t.Fatalf("[%q] %s desde una interfaz no verificada: %+v", estado, c.accion, resp)
			}
		}
	}
	// Las firmas locales de un --server manual sin PID siguen como antes.
	raw, _ := json.Marshal(map[string]any{"inputPath": entrada, "certificateId": local, "format": "cades"})
	resp = e.m.despachar(conEstadoInterfaz(context.Background(), interfazSinVinculo), peticion{Action: "sign", Params: raw})
	if resp.ErrorCode == "csc_interfaz_no_verificada" {
		t.Fatalf("una firma local no debe pedir la interfaz vinculada: %+v", resp)
	}
}

func TestServidorMarcaElEstadoDeLaInterfaz(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		vincular bool
		esperado string
	}{
		{"sin_vinculo", false, interfazSinVinculo},
		// net.Pipe no acredita ningún PID: nunca es la interfaz vinculada.
		{"pid_desconocido", true, interfazNoVerificada},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			e := nuevoEntornoCSCIPC(t, "explicit")
			srv := New(e.m)
			if c.vincular {
				if err := srv.peerPID.preparar(); err != nil {
					t.Fatal(err)
				}
				if err := srv.peerPID.publicar(1234); err != nil {
					t.Fatal(err)
				}
			}
			server, client := net.Pipe()
			done := make(chan struct{})
			go func() { defer close(done); srv.servirConexion(context.Background(), server) }()
			defer func() { _ = client.Close(); <-done }()
			_ = client.SetDeadline(time.Now().Add(5 * time.Second))
			enc, dec := json.NewEncoder(client), json.NewDecoder(client)

			params, _ := json.Marshal(map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID})
			if err := enc.Encode(peticion{Action: "csc_configure", Params: params}); err != nil {
				t.Fatal(err)
			}
			var resp respuesta
			if err := dec.Decode(&resp); err != nil {
				t.Fatal(err)
			}
			if resp.OK || resp.ErrorCode != "csc_interfaz_no_verificada" {
				t.Fatalf("csc_configure: %+v", resp)
			}

			if err := enc.Encode(peticion{Action: "export_diagnostic", Params: json.RawMessage(`{}`)}); err != nil {
				t.Fatal(err)
			}
			var diag struct {
				OK   bool `json:"ok"`
				Data struct {
					IPCClientBinding string `json:"ipcClientBinding"`
				} `json:"data"`
			}
			if err := dec.Decode(&diag); err != nil {
				t.Fatal(err)
			}
			if !diag.OK || diag.Data.IPCClientBinding != c.esperado {
				t.Fatalf("diagnóstico: %+v, want %q", diag, c.esperado)
			}
		})
	}
}
