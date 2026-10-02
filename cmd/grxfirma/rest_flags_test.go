// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExtraerFlagsREST(t *testing.T) {
	cfg, resto := extraerFlagsREST([]string{
		"-servidor-rest",
		"-direccion-rest", "127.0.0.1:7443",
		"-modo-cli",
		"-ayuda",
	})
	if !cfg.habilitado {
		t.Fatal("se esperaba modo REST habilitado")
	}
	if cfg.addr != "127.0.0.1:7443" {
		t.Fatalf("addr inesperada: %s", cfg.addr)
	}
	if cfg.token != "" {
		t.Fatalf("el token no debe proceder de argv: %s", cfg.token)
	}
	if cfg.certFingerprintsCSV != "" {
		t.Fatalf("huellas inesperadas: %s", cfg.certFingerprintsCSV)
	}
	if cfg.sessionTTL != 10*time.Minute {
		t.Fatalf("ttl inesperado: %s", cfg.sessionTTL)
	}
	if len(resto) != 2 || resto[0] != "-modo-cli" || resto[1] != "-ayuda" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestExtraerFlagsCredencial_SoloConservaFuenteSegura(t *testing.T) {
	p12, passwordStdin, cert, key, resto := extraerFlagsCredencial([]string{
		"-p12", "/tmp/identidad.p12",
		"-contrasena-stdin",
		"-cert", "/tmp/cert.pem",
		"-key", "/tmp/key.pem",
		"-modo-cli",
	})
	if p12 != "/tmp/identidad.p12" || cert != "/tmp/cert.pem" || key != "/tmp/key.pem" {
		t.Fatalf("rutas inesperadas: p12=%q cert=%q key=%q", p12, cert, key)
	}
	if !passwordStdin {
		t.Fatal("no se conservó la fuente stdin de la contraseña")
	}
	if len(resto) != 1 || resto[0] != "-modo-cli" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestResolverPasswordP12_PriorizaStdinYConservaEntornoComoCompatibilidad(t *testing.T) {
	password, err := resolverPasswordP12(
		true,
		strings.NewReader(" desde-stdin \n"),
		nil,
		"Contraseña: ",
		"desde-entorno",
	)
	if err != nil {
		t.Fatalf("resolverPasswordP12(stdin) error = %v", err)
	}
	if password != " desde-stdin " {
		t.Fatalf("password stdin = %q", password)
	}

	password, err = resolverPasswordP12(
		false,
		nil,
		nil,
		"Contraseña: ",
		"desde-entorno",
	)
	if err != nil || password != "desde-entorno" {
		t.Fatalf("password entorno = %q, err=%v", password, err)
	}
}

func TestGenerarTokenREST_Usa256BitsAleatorios(t *testing.T) {
	primero, err := generarTokenREST()
	if err != nil {
		t.Fatalf("generarTokenREST() error = %v", err)
	}
	segundo, err := generarTokenREST()
	if err != nil {
		t.Fatalf("segunda generarTokenREST() error = %v", err)
	}
	if primero == segundo {
		t.Fatal("dos tokens REST consecutivos no deben coincidir")
	}
	raw, err := base64.RawURLEncoding.DecodeString(primero)
	if err != nil {
		t.Fatalf("token no es base64url valido: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("entropia del token = %d bytes, want 32", len(raw))
	}
}

func TestEscribirCredencialRESTPrivada_EntregaCurlConfigYLaElimina(t *testing.T) {
	const token = "token-generado-no-visible-en-argv"
	path, cleanup, err := escribirCredencialRESTPrivada(t.TempDir(), token)
	if err != nil {
		t.Fatalf("escribirCredencialRESTPrivada() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `header = "Authorization: Bearer ` + token + `"` + "\n"
	if string(raw) != want {
		t.Fatalf("curl config = %q", raw)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("permisos = %04o", info.Mode().Perm())
		}
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("la credencial efímera no se eliminó: %v", err)
	}
}

func TestEscribirCredencialRESTPrivada_RechazaInyeccionCurlConfig(t *testing.T) {
	if _, _, err := escribirCredencialRESTPrivada(t.TempDir(), "token\nheader = \"X: inyectada\""); err == nil {
		t.Fatal("se aceptó un token capaz de inyectar configuración curl")
	}
}

func TestAyudaREST_NoExponeBearerEnArgv(t *testing.T) {
	var output bytes.Buffer
	escribirUsoGeneral(&output)
	text := output.String()
	if strings.Contains(text, "Authorization: Bearer") || strings.Contains(text, " -H ") {
		t.Fatalf("la ayuda recomienda Bearer en argv: %s", text)
	}
	if !strings.Contains(text, "--config") {
		t.Fatalf("la ayuda no documenta el fichero privado para curl: %s", text)
	}
}

func TestExtraerFlagsREST_AuthCertificado(t *testing.T) {
	huellaConSeparadores := strings.Repeat("aa:", 31) + "aa"
	huellaNormalizada := strings.Repeat("aa", 32)
	segundaHuella := strings.Repeat("bb", 32)
	cfg, resto := extraerFlagsREST([]string{
		"-servidor-rest",
		"-huellas-cert-rest", huellaConSeparadores + "," + segundaHuella,
		"-ttl-sesion-rest", "15m",
		"-modo-cli",
	})
	if !cfg.habilitado {
		t.Fatal("se esperaba modo REST habilitado")
	}
	if cfg.certFingerprintsCSV != huellaNormalizada+","+segundaHuella {
		t.Fatalf("huellas inesperadas: %s", cfg.certFingerprintsCSV)
	}
	if cfg.sessionTTL != 15*time.Minute {
		t.Fatalf("ttl inesperado: %s", cfg.sessionTTL)
	}
	if cfg.parseErr != nil {
		t.Fatalf("error de parseo inesperado: %v", cfg.parseErr)
	}
	if len(resto) != 1 || resto[0] != "-modo-cli" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestExtraerFlagsREST_RechazaHuellasCertificadoInvalidas(t *testing.T) {
	for _, raw := range []string{
		":",
		",,,",
		"abcd",
		strings.Repeat("g", 64),
		strings.Repeat("a", 64) + ",",
	} {
		t.Run(raw, func(t *testing.T) {
			cfg, _ := extraerFlagsREST([]string{"-servidor-rest", "-huellas-cert-rest", raw})
			if cfg.parseErr == nil {
				t.Fatalf("huella REST inválida aceptada: %q", raw)
			}
			if cfg.certFingerprintsCSV != "" {
				t.Fatalf("huella inválida conservada en configuración: %q", cfg.certFingerprintsCSV)
			}
		})
	}
}

func TestExtraerFlagsREST_CaducidadServidor(t *testing.T) {
	cfg, resto := extraerFlagsREST([]string{
		"--rest",
		"--rest-lifetime", "30m",
		"--version",
	})
	if cfg.parseErr != nil {
		t.Fatalf("error inesperado: %v", cfg.parseErr)
	}
	if cfg.lifetime != 30*time.Minute {
		t.Fatalf("lifetime = %s", cfg.lifetime)
	}
	if len(resto) != 1 || resto[0] != "--version" {
		t.Fatalf("resto inesperado: %#v", resto)
	}

	for _, raw := range []string{"0s", "500ms", "-1m", "241m", "invalida"} {
		t.Run(raw, func(t *testing.T) {
			invalid, _ := extraerFlagsREST([]string{
				"--rest", "--rest-lifetime", raw,
			})
			if invalid.parseErr == nil {
				t.Fatalf("se esperaba rechazo para %q", raw)
			}
		})
	}
}

func TestAplicarTokenRESTEntorno_PriorizaEntornoSinTocarArgv(t *testing.T) {
	cfg := restFlags{token: "token-en-argv"}
	got := aplicarTokenRESTEntorno(cfg, func(key string) (string, bool) {
		switch key {
		case envRESTToken:
			return " token-privado ", true
		case envRESTTokenHeredado:
			return "", false
		}
		t.Fatalf("clave de entorno inesperada: %s", key)
		return "", false
	})
	if got.token != "token-privado" {
		t.Fatalf("token = %q", got.token)
	}

	unchanged := aplicarTokenRESTEntorno(cfg, func(string) (string, bool) {
		return "   ", true
	})
	if unchanged.token != "token-en-argv" {
		t.Fatalf("un entorno vacío no debe borrar el fallback compatible: %q", unchanged.token)
	}
}

func TestQtRESTTokenSoloViaEntornoNoArgv(t *testing.T) {
	helperPath := filepath.Join("..", "gui-qml", "processenvironment.h")
	helper, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", helperPath, err)
	}
	if !strings.Contains(string(helper), `GRXFIRMA_REST_TOKEN`) ||
		!strings.Contains(string(helper), `forRestToken`) {
		t.Fatal("el helper Qt no configura el token REST mediante entorno privado")
	}
	for _, name := range []string{"backendbridge.cpp", "ipcbridge.cpp"} {
		path := filepath.Join("..", "gui-qml", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		source := string(raw)
		if strings.Contains(source, `QStringLiteral("--rest-token")`) ||
			strings.Contains(source, `args << "--rest-token"`) {
			t.Fatalf("%s vuelve a exponer el token REST mediante argv", path)
		}
		if !strings.Contains(source, `ChildProcessEnvironment::forRestToken`) {
			t.Fatalf("%s no usa el helper de entorno privado para el token REST", path)
		}
	}
}

func TestExtraerFlagsREST_PermiteOptInRemoto(t *testing.T) {
	cfg, resto := extraerFlagsREST([]string{
		"-servidor-rest",
		"-permitir-rest-remoto",
		"-modo-cli",
	})
	if !cfg.habilitado {
		t.Fatal("se esperaba modo REST habilitado")
	}
	if !cfg.permitirRemoto {
		t.Fatal("se esperaba permitirRemoto=true")
	}
	if len(resto) != 1 || resto[0] != "-modo-cli" {
		t.Fatalf("resto inesperado: %#v", resto)
	}
}

func TestValidarPoliticaREST(t *testing.T) {
	t.Run("loopback sin auth permitido", func(t *testing.T) {
		if err := validarPoliticaREST(restFlags{addr: "127.0.0.1:63118"}); err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
	})

	t.Run("remoto sin optin bloqueado", func(t *testing.T) {
		err := validarPoliticaREST(restFlags{addr: "0.0.0.0:63118"})
		if err == nil {
			t.Fatal("se esperaba error para remoto sin opt-in")
		}
	})

	t.Run("remoto con optin y sin auth bloqueado", func(t *testing.T) {
		err := validarPoliticaREST(restFlags{addr: "0.0.0.0:63118", permitirRemoto: true})
		if err == nil {
			t.Fatal("se esperaba error para remoto sin autenticación")
		}
	})

	t.Run("remoto con optin y bearer permitido", func(t *testing.T) {
		err := validarPoliticaREST(restFlags{addr: "0.0.0.0:63118", permitirRemoto: true, token: "valor-prueba"})
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
	})

	t.Run("remoto con huella sintacticamente vacia bloqueado", func(t *testing.T) {
		err := validarPoliticaREST(restFlags{
			addr:                "0.0.0.0:63118",
			permitirRemoto:      true,
			certFingerprintsCSV: ":",
		})
		if err == nil {
			t.Fatal("se esperaba error para allowlist de certificados inválida")
		}
	})
}

func TestExtraerAccionesDirectas(t *testing.T) {
	cfg, resto := extraerAccionesDirectas([]string{
		"-borrar-dominios",
		"-fichero-dominios", "/tmp/dominios.json",
		"-salida-json",
	})
	if !cfg.limpiarDominios {
		t.Fatal("se esperaba accion directa de limpiar dominios")
	}
	if len(resto) != 3 || resto[0] != "-fichero-dominios" || resto[1] != "/tmp/dominios.json" || resto[2] != "-salida-json" {
		t.Fatalf("resto inesperado: %#v", resto)
	}

	cfg, resto = extraerAccionesDirectas([]string{
		"-añadir-dominio",
		"-dominio", "https://sede.ejemplo.es",
	})
	if !cfg.anadirDominio {
		t.Fatal("se esperaba accion directa de añadir dominio")
	}
	if len(resto) != 2 || resto[0] != "-dominio" || resto[1] != "https://sede.ejemplo.es" {
		t.Fatalf("resto inesperado para añadir dominio: %#v", resto)
	}

	cfg, resto = extraerAccionesDirectas([]string{
		"-importar-p12", "/tmp/certificado.p12",
		"-contrasena-p12-stdin",
	})
	if cfg.importarP12Ruta != "/tmp/certificado.p12" {
		t.Fatalf("ruta P12 inesperada: %q", cfg.importarP12Ruta)
	}
	if len(resto) != 1 || resto[0] != "-contrasena-p12-stdin" {
		t.Fatalf("resto inesperado para importar p12: %#v", resto)
	}
}

func TestAccionesDirectasInyectarEnCLI(t *testing.T) {
	t.Run("ayudaDetallada", func(t *testing.T) {
		args := (accionesDirectas{ayudaDetallada: true}).inyectarEnCLI([]string{"-salida-json"})
		if len(args) != 1 || args[0] != "-salida-json" {
			t.Fatalf("args inesperados: %#v", args)
		}
	})

	t.Run("limpiarDominios", func(t *testing.T) {
		args := (accionesDirectas{limpiarDominios: true}).inyectarEnCLI([]string{"-fichero-dominios", "/tmp/dominios.json"})
		if len(args) != 5 {
			t.Fatalf("args inesperados: %#v", args)
		}
		if args[0] != "-modo-cli" || args[1] != "-operacion" || args[2] != "limpiar-dominios" {
			t.Fatalf("prefijo inesperado: %#v", args)
		}
		if args[3] != "-fichero-dominios" || args[4] != "/tmp/dominios.json" {
			t.Fatalf("cola inesperada: %#v", args)
		}
	})

	t.Run("estadoConfianzaTLS", func(t *testing.T) {
		args := (accionesDirectas{estadoConfianzaTLS: true}).inyectarEnCLI(nil)
		if len(args) != 3 || args[0] != "-modo-cli" || args[1] != "-operacion" || args[2] != "estado-confianza-tls" {
			t.Fatalf("args inesperados: %#v", args)
		}
	})

	t.Run("anadirDominio", func(t *testing.T) {
		args := (accionesDirectas{anadirDominio: true}).inyectarEnCLI([]string{"-dominio", "https://sede.ejemplo.es"})
		if len(args) != 5 || args[0] != "-modo-cli" || args[1] != "-operacion" || args[2] != "anadir-dominio" {
			t.Fatalf("args inesperados: %#v", args)
		}
	})

	t.Run("importarP12", func(t *testing.T) {
		args := (accionesDirectas{importarP12Ruta: "/tmp/certificado.p12"}).inyectarEnCLI([]string{"-contrasena-p12-stdin"})
		if len(args) != 6 || args[0] != "-modo-cli" || args[1] != "-operacion" || args[2] != "importar-p12" {
			t.Fatalf("args inesperados: %#v", args)
		}
		if args[3] != "-fichero-p12" || args[4] != "/tmp/certificado.p12" {
			t.Fatalf("ruta p12 inesperada: %#v", args)
		}
	})
}

func TestSolicitaAyudaIncluyeAyudaDetallada(t *testing.T) {
	if !solicitaAyuda([]string{"-ayuda-detallada"}) {
		t.Fatal("se esperaba ayuda para -ayuda-detallada")
	}
}

func TestAyudaGeneralEnumeraTodosLosFormatos(t *testing.T) {
	var stdout bytes.Buffer
	escribirUsoGeneral(&stdout)

	const formatos = "auto|pades|cades|xades|xmldsig|odf|ooxml|facturae|asic-xades"
	if !strings.Contains(stdout.String(), "-formato         <fmt>  "+formatos) {
		t.Fatalf("la ayuda general no enumera todos los formatos públicos: %s", stdout.String())
	}
}

func TestExtraerFlagsRESTLimitesV2(t *testing.T) {
	cfg, rest := extraerFlagsREST([]string{
		"-rest-solo-verificacion", "-verificacion-v2-max-firmas", "12",
		"-verificacion-v2-max-revisiones", "15", "-verificacion-v2-max-pdf-mib", "48",
		"-verificacion-v2-max-cuerpo-mib", "90", "-modo-cli",
	})
	if cfg.parseErr != nil || cfg.v2MaxFirmas != 12 || cfg.v2MaxRevisiones != 15 || cfg.v2MaxPDFMiB != 48 || cfg.v2MaxCuerpoMiB != 90 || len(rest) != 1 || rest[0] != "-modo-cli" {
		t.Fatalf("límites v2 mal interpretados: %+v, resto=%v", cfg, rest)
	}
	for _, flag := range []string{"-verificacion-v2-max-firmas", "-verificacion-v2-max-revisiones", "-verificacion-v2-max-pdf-mib", "-verificacion-v2-max-cuerpo-mib"} {
		cfg, _ = extraerFlagsREST([]string{flag, "999"})
		if cfg.parseErr == nil {
			t.Fatalf("%s aceptó un límite inseguro", flag)
		}
	}
}
