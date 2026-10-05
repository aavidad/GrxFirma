// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/csctest"
	"grxfirma/internal/testsupport/pdffixture"
)

// conectarLoteCSC prepara una sesión explícita (PIN y OTP en línea) con el
// multisign indicado y devuelve el certificado RSA remoto.
func conectarLoteCSC(t *testing.T, multisign int) (entornoCSCIPC, string) {
	t.Helper()
	e := nuevoEntornoCSCIPC(t, "explicit")
	e.servidor.Configurar(func(s *csctest.Servidor) { s.SCAL, s.Multisign = "2", multisign })
	if resp := pedirIPC(t, e.m, "csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID}); !resp.OK {
		t.Fatalf("csc_configure: %+v", resp)
	}
	resp := pedirIPC(t, e.m, "csc_connect", map[string]any{})
	if !resp.OK {
		t.Fatalf("csc_connect: %+v", resp)
	}
	for _, c := range resp.Data.(resultadoCSCConexion).Credentials {
		if strings.Contains(c.Subject, "RSA") {
			if c.MultiSign != max(multisign, 1) {
				t.Fatalf("multiSign = %d", c.MultiSign)
			}
			return e, c.CertificateID
		}
	}
	t.Fatal("sin credencial RSA")
	return e, ""
}

func pdfsLote(t *testing.T, n int) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	var rutas []string
	for i := 0; i < n; i++ {
		ruta := filepath.Join(dir, fmt.Sprintf("doc%d.pdf", i))
		if err := os.WriteFile(ruta, pdffixture.Minimal(), 0o600); err != nil {
			t.Fatal(err)
		}
		rutas = append(rutas, ruta)
	}
	return dir, rutas
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// TestCSCIPCLoteConOTPAutorizaUnaVez recorre un lote PAdES completo con el
// motor real: tres documentos, un PIN y un OTP, una sola autorización y tres
// firmas que el verificador del motor da por válidas.
func TestCSCIPCLoteConOTPAutorizaUnaVez(t *testing.T) {
	e, cert := conectarLoteCSC(t, 5)
	if resp := pedirIPC(t, e.m, "certificates", map[string]any{}); resp.OK {
		for _, c := range resp.Data.([]certJSON) {
			if c.ID == cert && c.RemoteMultiSign != 5 {
				t.Fatalf("certificates no indica multisign: %+v", c)
			}
		}
	}
	if resp := pedirIPC(t, e.m, "csc_send_otp", map[string]any{"certificateId": cert}); !resp.OK {
		t.Fatalf("csc_send_otp: %+v", resp)
	}
	dir, rutas := pdfsLote(t, 3)
	salida := filepath.Join(dir, "firmados")
	if err := os.Mkdir(salida, 0o700); err != nil {
		t.Fatal(err)
	}
	resp := pedirIPC(t, e.m, "sign_batch", map[string]any{
		"inputPaths": rutas, "outputDir": salida, "certificateId": cert, "format": "pades",
		"remotePin": b64(csctest.PIN), "remoteOtp": b64(csctest.OTP),
	})
	if !resp.OK {
		t.Fatalf("sign_batch: %+v", resp)
	}
	sinSecretosEnRespuesta(t, resp, csctest.PIN, csctest.OTP)
	lote := resp.Data.(resultadoFirmaLote)
	if lote.OkCount != 3 || lote.FailCount != 0 {
		t.Fatalf("lote: %+v", lote)
	}
	e.servidor.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 1 || s.UltimoNumSignatures != 3 || s.OTPEnviados != 1 {
			t.Fatalf("autorizaciones=%d numSignatures=%d otp=%d", s.Autorizaciones, s.UltimoNumSignatures, s.OTPEnviados)
		}
	})
	anclas := domain.CertificateChain{DERCertificates: [][]byte{e.servidor.CA.Raw}}
	for _, item := range lote.Results {
		datos, err := os.ReadFile(item.OutputPath)
		if err != nil {
			t.Fatal(err)
		}
		doc, _ := domain.NewDocument("firmado.pdf", datos, "application/pdf")
		vr, firmantes, err := commonsigner.NewMultiVerifier().Verify(context.Background(), doc, anclas)
		if err != nil || vr.Integrity.Status != domain.VerificationStatusValid || len(firmantes) != 1 {
			t.Fatalf("%s: err=%v integridad=%+v", item.OutputPath, err, vr.Integrity)
		}
	}
}

func TestCSCIPCLoteConOTPQueNoCabeSeRechazaSinFirmar(t *testing.T) {
	e, cert := conectarLoteCSC(t, 2)
	_, rutas := pdfsLote(t, 3)
	resp := pedirIPC(t, e.m, "sign_batch", map[string]any{
		"inputPaths": rutas, "certificateId": cert, "format": "pades",
		"remotePin": b64(csctest.PIN), "remoteOtp": b64(csctest.OTP),
	})
	if resp.OK || resp.ErrorCode != "csc_otp_lote_excede" {
		t.Fatalf("lote mayor que multisign: %+v", resp)
	}
	e.servidor.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 0 || len(s.ResumenesFirmados) != 0 {
			t.Fatal("no debió autorizarse ni firmarse nada")
		}
	})
}

func TestCSCIPCLoteConOTPSinMultisignSigueBloqueado(t *testing.T) {
	e, cert := conectarLoteCSC(t, 1)
	_, rutas := pdfsLote(t, 2)
	resp := pedirIPC(t, e.m, "sign_batch", map[string]any{
		"inputPaths": rutas, "certificateId": cert, "format": "pades",
		"remotePin": b64(csctest.PIN), "remoteOtp": b64(csctest.OTP),
	})
	if resp.OK || resp.ErrorCode != "csc_otp_lote" {
		t.Fatalf("lote con OTP sin multisign: %+v", resp)
	}
	// La multifirma por lotes no usa la autorización conjunta.
	e2, cert2 := conectarLoteCSC(t, 5)
	resp = pedirIPC(t, e2.m, "sign_batch", map[string]any{
		"inputPaths": rutas, "certificateId": cert2, "additionalCertificateIds": []string{strings.Repeat("b", 64)},
		"format": "pades", "remotePin": b64(csctest.PIN), "remoteOtp": b64(csctest.OTP),
	})
	if resp.OK || resp.ErrorCode != "csc_otp_lote" {
		t.Fatalf("multifirma por lotes con OTP: %+v", resp)
	}
}

func TestCSCIPCLoteImplicitoSeAgrupaPorMultisign(t *testing.T) {
	e := nuevoEntornoCSCIPC(t, "implicit")
	e.servidor.Configurar(func(s *csctest.Servidor) { s.Multisign = 3 })
	pedirIPC(t, e.m, "csc_configure", map[string]any{"serviceUrl": e.servidor.URL, "clientId": csctest.ClientID})
	resp := pedirIPC(t, e.m, "csc_connect", map[string]any{})
	if !resp.OK {
		t.Fatalf("csc_connect: %+v", resp)
	}
	var cert string
	for _, c := range resp.Data.(resultadoCSCConexion).Credentials {
		if strings.Contains(c.Subject, "RSA") {
			cert = c.CertificateID
		}
	}
	dir, rutas := pdfsLote(t, 4)
	resp = pedirIPC(t, e.m, "sign_batch", map[string]any{
		"inputPaths": rutas, "outputDir": dir, "certificateId": cert, "format": "pades", "overwrite": "rename",
	})
	if !resp.OK || resp.Data.(resultadoFirmaLote).OkCount != 4 {
		t.Fatalf("sign_batch: %+v", resp)
	}
	// Cuatro documentos con multisign 3: un grupo de tres y uno suelto.
	e.servidor.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 2 || len(s.ResumenesFirmados) != 4 {
			t.Fatalf("autorizaciones=%d firmados=%d", s.Autorizaciones, len(s.ResumenesFirmados))
		}
	})
}
