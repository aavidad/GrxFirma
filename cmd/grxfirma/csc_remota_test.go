// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"grxfirma/internal/adapters/outbound/common/localizador"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/domain"
	"grxfirma/internal/testsupport/csctest"
)

func TestExtraerFlagsCSC(t *testing.T) {
	opc, resto := extraerFlagsCSC([]string{
		"-operacion", "firmar", "--csc-url", "https://csc.example.org",
		"-csc-client-id=cliente", "-csc-credencial", "cred-1", "-entrada", "a.pdf",
	})
	if opc.url != "https://csc.example.org" || opc.clientID != "cliente" || opc.credencial != "cred-1" || opc.opcionMala != "" {
		t.Fatalf("opciones: %+v", opc)
	}
	if !reflect.DeepEqual(resto, []string{"-operacion", "firmar", "-entrada", "a.pdf"}) {
		t.Fatalf("resto: %v", resto)
	}
	for _, args := range [][]string{{"-csc-url"}, {"-csc-url", "-entrada"}, {"-csc-desconocida"}, {"-csc-activar"}} {
		if opc, _ := extraerFlagsCSC(args); opc.opcionMala == "" || !opc.solicitada() {
			t.Fatalf("%v debió marcarse como opción inválida: %+v", args, opc)
		}
	}
	if opc, _ := extraerFlagsCSC([]string{"-entrada", "a.pdf"}); opc.solicitada() {
		t.Fatal("sin opciones -csc-* no se activa nada")
	}
}

type entornoPruebaCSC struct {
	entornoCSC
	salida, errores *bytes.Buffer
}

func nuevoEntornoPruebaCSC(t *testing.T, s *csctest.Servidor, configJSON, politicaJSON string) *entornoPruebaCSC {
	t.Helper()
	configDir, politicaDir := t.TempDir(), t.TempDir()
	if configJSON != "" {
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if politicaJSON != "" {
		if err := os.WriteFile(filepath.Join(politicaDir, "policy.json"), []byte(politicaJSON), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := &entornoPruebaCSC{salida: &bytes.Buffer{}, errores: &bytes.Buffer{}}
	loc := localizador.Para("es")
	e.entornoCSC = entornoCSC{
		ctx:         context.Background(),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		t:           loc.T,
		idioma:      "es",
		salida:      e.salida,
		errores:     e.errores,
		stdin:       strings.NewReader(""),
		configDir:   configDir,
		politicaDir: politicaDir,
		ejecutarCLI: func(*fuenteRemota, []string) int {
			t.Fatal("no debió llegar a la CLI")
			return 99
		},
	}
	if s != nil {
		e.httpBase = s.Client()
		e.navegador = s.Navegador()
	}
	return e
}

func TestEjecutarCSCDesactivadaYProhibida(t *testing.T) {
	es := localizador.Para("es")
	base := opcionesCSC{url: "https://csc.example.org", clientID: "c", listar: true}

	e := nuevoEntornoPruebaCSC(t, nil, "", "")
	if code := ejecutarCSC(e.entornoCSC, base, false, nil); code != 2 || !strings.Contains(e.errores.String(), es.T("csc.cli.desactivada")) {
		t.Fatalf("sin activar: code=%d salida=%q", code, e.errores.String())
	}

	// La línea de órdenes ya no puede activarla: -csc-activar no existe.
	opc, _ := extraerFlagsCSC([]string{"-csc-activar", "-csc-url", "https://csc.example.org", "-csc-client-id", "c", "-csc-listar-credenciales"})
	e = nuevoEntornoPruebaCSC(t, nil, "", "")
	if code := ejecutarCSC(e.entornoCSC, opc, false, nil); code != 2 || !strings.Contains(e.errores.String(), es.T("csc.cli.opcion_invalida", "-csc-activar")) {
		t.Fatalf("-csc-activar: code=%d salida=%q", code, e.errores.String())
	}

	e = nuevoEntornoPruebaCSC(t, nil, `{"firma_remota_csc": true}`, `{"firma_remota_csc": false}`)
	if code := ejecutarCSC(e.entornoCSC, base, false, nil); code != 2 ||
		!strings.Contains(e.errores.String(), es.T("csc.error.prohibida")) ||
		strings.Contains(e.errores.String(), "config.json") {
		t.Fatalf("la política debe prohibirla sin sugerir config.json: code=%d salida=%q", code, e.errores.String())
	}

	e = nuevoEntornoPruebaCSC(t, nil, `{"firma_remota_csc": true}`, `{"firma_remota_csc": tru`)
	if code := ejecutarCSC(e.entornoCSC, base, false, nil); code != 2 {
		t.Fatalf("una política ilegible no puede autorizar: code=%d", code)
	}

	e = nuevoEntornoPruebaCSC(t, nil, `{"firma_remota_csc": true, "firma_remota_csc_oauth_permitidos": ["mal"]}`, "")
	if code := ejecutarCSC(e.entornoCSC, base, false, nil); code != 2 || !strings.Contains(e.errores.String(), es.T("csc.cli.pares_oauth_invalidos")) {
		t.Fatalf("pares mal formados: code=%d salida=%q", code, e.errores.String())
	}

	e = nuevoEntornoPruebaCSC(t, nil, `{"firma_remota_csc": true}`, "")
	if code := ejecutarCSC(e.entornoCSC, base, true, nil); code != 2 || !strings.Contains(e.errores.String(), es.T("csc.cli.incompatible")) {
		t.Fatalf("con credenciales locales: code=%d salida=%q", code, e.errores.String())
	}

	e = nuevoEntornoPruebaCSC(t, nil, `{"firma_remota_csc": true}`, "")
	if code := ejecutarCSC(e.entornoCSC, opcionesCSC{listar: true}, false, nil); code != 2 || !strings.Contains(e.errores.String(), es.T("csc.cli.faltan_opciones")) {
		t.Fatalf("sin URL: code=%d salida=%q", code, e.errores.String())
	}

	e = nuevoEntornoPruebaCSC(t, nil, `{"firma_remota_csc": true}`, "")
	if code := ejecutarCSC(e.entornoCSC, opcionesCSC{url: "http://csc.example.org", clientID: "c", listar: true}, false, nil); code != 1 || !strings.Contains(e.errores.String(), es.T("csc.error.solo_https")) {
		t.Fatalf("http plano: code=%d salida=%q", code, e.errores.String())
	}
}

func TestEjecutarCSCPermitidaPorPoliticaYOAuthAjenoRechazado(t *testing.T) {
	es := localizador.Para("es")
	s := csctest.Nuevo(t)
	e := nuevoEntornoPruebaCSC(t, s, "", `{"firma_remota_csc": true}`)
	if code := ejecutarCSC(e.entornoCSC, opcionesCSC{url: s.URL, clientID: csctest.ClientID, listar: true}, false, nil); code != 0 {
		t.Fatalf("la política debe bastar: code=%d errores=%q", code, e.errores.String())
	}

	s.Configurar(func(s *csctest.Servidor) { s.OAuthURL = "https://oauth.otro-host.invalid" })
	e = nuevoEntornoPruebaCSC(t, s, `{"firma_remota_csc": true}`, "")
	e.navegador = func(context.Context, string) error {
		t.Fatal("no debe abrirse el navegador hacia un servidor OAuth ajeno")
		return nil
	}
	if code := ejecutarCSC(e.entornoCSC, opcionesCSC{url: s.URL, clientID: csctest.ClientID, listar: true}, false, nil); code != 1 || !strings.Contains(e.errores.String(), es.T("csc.error.oauth_otro_host")) {
		t.Fatalf("OAuth en otro host: code=%d errores=%q", code, e.errores.String())
	}
}

func TestEjecutarCSCListaCredenciales(t *testing.T) {
	s := csctest.Nuevo(t)
	e := nuevoEntornoPruebaCSC(t, s, `{"firma_remota_csc": true}`, "")
	code := ejecutarCSC(e.entornoCSC, opcionesCSC{url: s.URL, clientID: csctest.ClientID, listar: true}, false, nil)
	if code != 0 {
		t.Fatalf("code=%d errores=%q", code, e.errores.String())
	}
	salida := e.salida.String()
	for _, esperado := range []string{csctest.CredencialRSA, csctest.CredencialEC, "Firmante remoto RSA", "CA de pruebas CSC"} {
		if !strings.Contains(salida, esperado) {
			t.Fatalf("falta %q en la salida:\n%s", esperado, salida)
		}
	}
	host := strings.TrimPrefix(s.URL, "https://")
	if !strings.Contains(e.errores.String(), localizador.Para("es").T("csc.cli.hosts", host, host)) {
		t.Fatalf("la CLI debe mostrar los dos hosts: %q", e.errores.String())
	}
	if strings.Contains(e.errores.String(), "state=") || strings.Contains(e.errores.String(), "/oauth2/authorize") {
		t.Fatalf("la CLI no debe escribir el URL con el state: %q", e.errores.String())
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Revocados != 1 {
			t.Fatalf("el token no se revocó al terminar: %d", s.Revocados)
		}
	})
}

// TestEjecutarCSCFirmaConLaCLIReal firma un CAdES con el adaptador CLI real,
// con PIN y OTP canalizados por stdin, y verifica el resultado.
func TestEjecutarCSCFirmaConLaCLIReal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	s := csctest.Nuevo(t)
	s.Configurar(func(s *csctest.Servidor) { s.Modo, s.SCAL = "explicit", "2" })

	dir := t.TempDir()
	entrada := filepath.Join(dir, "documento.txt")
	salida := filepath.Join(dir, "documento.csig")
	contenido := []byte("documento firmado con la CLI y un certificado remoto")
	if err := os.WriteFile(entrada, contenido, 0o600); err != nil {
		t.Fatal(err)
	}

	e := nuevoEntornoPruebaCSC(t, s, `{"firma_remota_csc": true}`, "")
	e.stdin = &lectorByteAByte{r: strings.NewReader(csctest.PIN + "\n" + csctest.OTP + "\n")}
	var argsRecibidos []string
	e.ejecutarCLI = func(remota *fuenteRemota, args []string) int {
		argsRecibidos = args
		adaptador, err := construirAdaptadorCon("", "", "", "", remota)
		if err != nil {
			t.Fatalf("construirAdaptadorCon: %v", err)
		}
		return adaptador.Run(context.Background(), args)
	}
	opc := opcionesCSC{url: s.URL, clientID: csctest.ClientID, credencial: csctest.CredencialRSA}
	args := []string{"-operacion", "firmar", "-entrada", entrada, "-salida", salida, "-formato", "cades"}
	if code := ejecutarCSC(e.entornoCSC, opc, false, args); code != 0 {
		t.Fatalf("code=%d errores=%q salida=%q", code, e.errores.String(), e.salida.String())
	}
	if n := len(argsRecibidos); n < 2 || argsRecibidos[n-2] != "-certificado" {
		t.Fatalf("la CLI debe seleccionar la credencial remota: %v", argsRecibidos)
	}

	firmada, err := os.ReadFile(salida)
	if err != nil {
		t.Fatal(err)
	}
	firmado, _ := domain.NewDocument("documento.csig", firmada, "")
	original, _ := domain.NewDocument("documento.txt", contenido, "text/plain")
	anclas := domain.CertificateChain{DERCertificates: [][]byte{s.CA.Raw}}
	verificador := commonsigner.NewMultiVerifier()
	vr, _, err := verificador.Verify(context.Background(), firmado, anclas)
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		vr, _, err = verificador.VerifyDetached(context.Background(), firmado, original, anclas)
	}
	if err != nil || vr.Integrity.Status != domain.VerificationStatusValid {
		t.Fatalf("la firma no verifica: %v %+v", err, vr.Integrity)
	}
	s.Leer(func(s *csctest.Servidor) {
		if s.Autorizaciones != 1 || s.OTPEnviados != 1 {
			t.Fatalf("autorizaciones=%d otp=%d", s.Autorizaciones, s.OTPEnviados)
		}
	})
	if strings.Contains(e.errores.String(), csctest.PIN) || strings.Contains(e.salida.String(), csctest.OTP) {
		t.Fatal("la salida no puede contener el PIN ni el OTP")
	}
}

func TestTextoSeguroQuitaControles(t *testing.T) {
	if got := textoSeguro("CN=a\x1b[31m\nb"); got != "CN=a?[31m?b" {
		t.Fatalf("textoSeguro = %q", got)
	}
}
