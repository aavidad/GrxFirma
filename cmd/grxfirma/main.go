// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Comando grxfirma — cliente de firma electronica en linea de comandos.
//
// Uso basico:
//
//	grxfirma -certificado-pem cert.pem -clave-pem key.pem -entrada doc.pdf -salida doc.csig -formato CAdES
//	grxfirma -certificado-pem cert.pem -clave-pem key.pem -lote manifiesto.json
//
// Flags de credencial (obligatorios):
//
//	-certificado-pem <ruta>   Fichero PEM con el certificado de firma.
//	-clave-pem       <ruta>   Fichero PEM con la clave privada (RSA, ECDSA, PKCS#8).
//
// Convertir P12 a PEM:
//
//	openssl pkcs12 -in cert.p12 -clcerts -nokeys -out cert.pem
//	openssl pkcs12 -in cert.p12 -nocerts -nodes  -out key.pem
//
// El resto de banderas (-entrada, -salida, -formato, -accion, -certificado,
// -lote, -tiempo-espera) son gestionadas por el adaptador CLI interno.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cliin "grxfirma/internal/adapters/inbound/common/cli"
	restin "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/inbound/common/secretinput"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/desktop/pdfpreview"
	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/adapters/outbound/desktop/servicemanager"
	"grxfirma/internal/application"
)

// version se sobreescribe en tiempo de compilacion:
//
//	go build -ldflags "-X main.version=1.0.0" ./cmd/grxfirma
var version = "dev"

const (
	envRESTToken       = "GRXFIRMA_REST_TOKEN"
	envPKCS12Password  = "GRXFIRMA_PKCS12_PASSWORD"
	envProtectionToken = "GRXFIRMA_PROTECTION_SECRET_B64"
	minRESTLifetime    = time.Second
	maxRESTLifetime    = 240 * time.Minute
)

func main() {
	// Cancelacion limpia al recibir Ctrl+C o SIGTERM.
	ctx, stop := newSignalContext(context.Background())
	defer stop()
	logger := logging.New("cmd/grxfirma", os.Stderr)
	loc := localizador.Detectar()

	if err := secretinput.RejectArgv(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, secretinput.UserMessage(err, loc.T))
		os.Exit(2)
	}
	environment, err := secretinput.ConsumeEnvironment(
		[]string{envPKCS12Password, envRESTToken, envProtectionToken},
		os.LookupEnv,
		os.Unsetenv,
	)
	if err != nil {
		logger.ErrorContext(ctx, "no se pudo sanear el entorno de secretos", "op", "secret-env", "error", err)
		fmt.Fprintln(os.Stderr, secretinput.UserMessage(err, loc.T))
		os.Exit(1)
	}

	// Extraer flags de credencial antes de pasarlos al adaptador CLI.
	rutaP12, passwordStdin, rutaCert, rutaClave, argsResto := extraerFlagsCredencial(os.Args[1:])
	acciones, argsResto := extraerAccionesDirectas(argsResto)
	restCfg, argsResto := extraerFlagsREST(argsResto)
	restTokenCompat, restTokenPresent := environment.Take(envRESTToken)
	restCfg = aplicarTokenRESTEntorno(restCfg, func(string) (string, bool) {
		return restTokenCompat, restTokenPresent
	})
	argsResto = acciones.inyectarEnCLI(argsResto)

	if restCfg.parseErr != nil {
		logger.ErrorContext(ctx, "flags REST inválidas", "op", "rest-flags", "error", restCfg.parseErr)
		fmt.Fprintln(os.Stderr, "error:", restCfg.parseErr)
		os.Exit(1)
	}

	// Flags especiales que no necesitan credenciales.
	if len(argsResto) == 1 && (argsResto[0] == "-version" || argsResto[0] == "--version") {
		fmt.Println("grxfirma", version)
		os.Exit(0)
	}
	if acciones.ayudaDetallada {
		imprimirUsoDetallado()
		os.Exit(0)
	}
	if solicitaAyudaGeneral(argsResto) {
		imprimirUso()
		os.Exit(0)
	}

	passwordP12Compat, _ := environment.Take(envPKCS12Password)
	password, err := resolverPasswordP12(
		passwordStdin,
		os.Stdin,
		os.Stderr,
		loc.T("security.secret.prompt.p12"),
		passwordP12Compat,
	)
	if err != nil {
		logger.ErrorContext(ctx, "no se pudo leer la contraseña P12", "op", "secret-input", "error", err)
		fmt.Fprintln(os.Stderr, secretinput.UserMessage(err, loc.T))
		os.Exit(1)
	}

	if restCfg.habilitado && restCfg.soloVerificacion {
		os.Exit(runRESTSoloVerificacion(ctx, logger, restCfg))
	}
	if restCfg.habilitado {
		code := runRESTServer(ctx, logger, rutaP12, password, rutaCert, rutaClave, restCfg)
		os.Exit(code)
	}

	desktop, argsResto, err := extraerModoDesktop(argsResto)
	if err != nil {
		logger.ErrorContext(ctx, "flags desktop inválidas", "op", "desktop-flags", "error", err)
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if desktop.habilitado {
		handled, code := maybeRunDesktopApp(ctx, os.Stderr, desktop.frontend, rutaP12, password, rutaCert, rutaClave)
		if handled {
			os.Exit(code)
		}
	}

	adaptador, err := construirAdaptador(rutaP12, password, rutaCert, rutaClave)
	if err != nil {
		logger.ErrorContext(ctx, "no se pudo arrancar el binario", "op", "bootstrap", "error", err)
		fmt.Fprintln(os.Stderr)
		imprimirUso()
		os.Exit(1)
	}
	protectionCompat, _ := environment.Take(envProtectionToken)
	adaptador.WithCompatibilitySecrets(passwordP12Compat, protectionCompat)

	os.Exit(adaptador.Run(ctx, argsResto))
}

// extraerFlagsCredencial extrae rutas y la fuente segura de la contraseña P12
// del slice de argumentos. El valor del secreto nunca se acepta en argv.
func extraerFlagsCredencial(args []string) (p12 string, passwordStdin bool, cert, key string, resto []string) {
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-p12", "--p12", "-contenedor-p12", "--contenedor-p12":
			if i+1 < len(args) {
				p12 = args[i+1]
				i += 2
				continue
			}
		case "-password-stdin", "--password-stdin", "-contrasena-stdin", "--contrasena-stdin",
			"-p12-password-stdin", "--p12-password-stdin":
			passwordStdin = true
			i++
			continue
		case "-cert", "--cert", "-certificado-pem", "--certificado-pem":
			if i+1 < len(args) {
				cert = args[i+1]
				i += 2
				continue
			}
		case "-key", "--key", "-clave", "--clave", "-clave-pem", "--clave-pem":
			if i+1 < len(args) {
				key = args[i+1]
				i += 2
				continue
			}
		}
		resto = append(resto, args[i])
		i++
	}
	return
}

func resolverPasswordP12(
	fromStdin bool,
	stdin io.Reader,
	stderr io.Writer,
	prompt string,
	fromEnv string,
) (string, error) {
	if fromStdin {
		return secretinput.Read(stdin, stderr, prompt)
	}
	// Compatibilidad para servicios y automatización existentes. Se consume
	// una vez para que no alcance a procesos hijos.
	return fromEnv, nil
}

type restFlags struct {
	habilitado          bool
	addr                string
	token               string
	certFingerprintsCSV string
	sessionTTL          time.Duration
	lifetime            time.Duration
	permitirRemoto      bool
	// soloVerificacion publica únicamente GET /health y POST /verify.
	soloVerificacion bool
	// anclasVerificacion es un fichero o directorio de anclas locales.
	anclasVerificacion string
	// crlVerificacion es un directorio de CRL locales.
	crlVerificacion string
	v2MaxFirmas     int
	v2MaxRevisiones int
	v2MaxPDFMiB     int
	v2MaxCuerpoMiB  int
	parseErr        error
}

const unidadLimiteV2 = "MiB"

type accionesDirectas struct {
	ayudaDetallada         bool
	listarDominios         bool
	anadirDominio          bool
	eliminarDominio        bool
	importarDominios       bool
	importarP12Ruta        string
	exportarDominios       bool
	limpiarDominios        bool
	listarAutoseleccion    bool
	eliminarAutoseleccion  bool
	resetearAutoseleccion  bool
	exportarAutoseleccion  bool
	importarAutoseleccion  bool
	limpiarAutoseleccion   bool
	estadoAlmacenTLS       bool
	limpiarAlmacenTLS      bool
	estadoConfianzaTLS     bool
	generarCertificadosTLS bool
	instalarConfianzaTLS   bool
}

func extraerFlagsREST(args []string) (cfg restFlags, resto []string) {
	cfg.addr = "127.0.0.1:63118"
	cfg.sessionTTL = 10 * time.Minute
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-rest", "--rest", "-servidor-rest", "--servidor-rest":
			cfg.habilitado = true
			i++
			continue
		case "-rest-addr", "--rest-addr", "-direccion-rest", "--direccion-rest":
			if i+1 < len(args) {
				cfg.addr = args[i+1]
				i += 2
				continue
			}
		case "-rest-cert-fingerprints", "--rest-cert-fingerprints", "-huellas-cert-rest", "--huellas-cert-rest":
			if i+1 < len(args) {
				normalizadas, err := normalizarHuellasCertREST(args[i+1])
				if err != nil {
					cfg.parseErr = err
				} else {
					cfg.certFingerprintsCSV = normalizadas
				}
				i += 2
				continue
			}
		case "-rest-session-ttl", "--rest-session-ttl", "-ttl-sesion-rest", "--ttl-sesion-rest":
			if i+1 < len(args) {
				ttl, err := time.ParseDuration(args[i+1])
				if err != nil {
					cfg.parseErr = fmt.Errorf("ttl de sesión REST inválido: %w", err)
				} else {
					cfg.sessionTTL = ttl
				}
				i += 2
				continue
			}
		case "-rest-lifetime", "--rest-lifetime", "-duracion-rest", "--duracion-rest":
			if i+1 < len(args) {
				lifetime, err := time.ParseDuration(args[i+1])
				if err != nil || lifetime < minRESTLifetime || lifetime > maxRESTLifetime {
					cfg.parseErr = fmt.Errorf("duración del servidor REST inválida: debe estar entre %s y %s", minRESTLifetime, maxRESTLifetime)
				} else {
					cfg.lifetime = lifetime
				}
				i += 2
				continue
			}
		case "-rest-publico", "--rest-publico", "-permitir-rest-remoto", "--permitir-rest-remoto":
			cfg.permitirRemoto = true
			i++
			continue
		case "-rest-solo-verificacion", "--rest-solo-verificacion", "-rest-verify-only", "--rest-verify-only":
			cfg.habilitado = true
			cfg.soloVerificacion = true
			i++
			continue
		case "-verificacion-anclas", "--verificacion-anclas", "-verify-anchors", "--verify-anchors":
			if i+1 < len(args) {
				cfg.anclasVerificacion = args[i+1]
				i += 2
				continue
			}
		case "-verificacion-crl", "--verificacion-crl", "-verify-crl-dir", "--verify-crl-dir":
			if i+1 < len(args) {
				cfg.crlVerificacion = args[i+1]
				i += 2
				continue
			}
		case "-verificacion-v2-max-firmas", "--verificacion-v2-max-firmas", "-verify-v2-max-signatures":
			if i+1 < len(args) {
				value, err := strconv.Atoi(args[i+1])
				if err != nil || value < 1 || value > 20 {
					cfg.parseErr = fmt.Errorf("%s: 1..20", localizador.Detectar().T("rest.verify_v2.error.limit"))
				} else {
					cfg.v2MaxFirmas = value
				}
				i += 2
				continue
			}
		case "-verificacion-v2-max-revisiones", "--verificacion-v2-max-revisiones", "-verify-v2-max-revisions":
			if i+1 < len(args) {
				value, err := strconv.Atoi(args[i+1])
				if err != nil || value < 1 || value > 20 {
					cfg.parseErr = fmt.Errorf("%s: 1..20", localizador.Detectar().T("rest.verify_v2.error.limit"))
				} else {
					cfg.v2MaxRevisiones = value
				}
				i += 2
				continue
			}
		case "-verificacion-v2-max-pdf-mib", "--verificacion-v2-max-pdf-mib", "-verify-v2-max-pdf-mib":
			if i+1 < len(args) {
				value, err := strconv.Atoi(args[i+1])
				if err != nil || value < 1 || value > 100 {
					cfg.parseErr = fmt.Errorf("%s: 1..100 %s", localizador.Detectar().T("rest.verify_v2.error.limit"), unidadLimiteV2)
				} else {
					cfg.v2MaxPDFMiB = value
				}
				i += 2
				continue
			}
		case "-verificacion-v2-max-cuerpo-mib", "--verificacion-v2-max-cuerpo-mib", "-verify-v2-max-body-mib":
			if i+1 < len(args) {
				value, err := strconv.Atoi(args[i+1])
				if err != nil || value < 1 || value > 150 {
					cfg.parseErr = fmt.Errorf("%s: 1..150 %s", localizador.Detectar().T("rest.verify_v2.error.limit"), unidadLimiteV2)
				} else {
					cfg.v2MaxCuerpoMiB = value
				}
				i += 2
				continue
			}
		}
		resto = append(resto, args[i])
		i++
	}
	return cfg, resto
}

func normalizarHuellasCertREST(csv string) (string, error) {
	partes := strings.Split(csv, ",")
	normalizadas := make([]string, 0, len(partes))
	vistas := make(map[string]struct{}, len(partes))
	for _, parte := range partes {
		huella := strings.ToLower(strings.TrimSpace(parte))
		huella = strings.ReplaceAll(huella, ":", "")
		if len(huella) != sha256.Size*2 {
			return "", fmt.Errorf("%s: SHA-256/64hex", localizador.Detectar().T("Inválida", "Inválida"))
		}
		if _, err := hex.DecodeString(huella); err != nil {
			return "", fmt.Errorf("%s: SHA-256/64hex", localizador.Detectar().T("Inválida", "Inválida"))
		}
		if _, existe := vistas[huella]; existe {
			continue
		}
		vistas[huella] = struct{}{}
		normalizadas = append(normalizadas, huella)
	}
	if len(normalizadas) == 0 {
		return "", fmt.Errorf("%s: SHA-256/64hex", localizador.Detectar().T("Inválida", "Inválida"))
	}
	return strings.Join(normalizadas, ","), nil
}

func aplicarTokenRESTEntorno(cfg restFlags, lookup func(string) (string, bool)) restFlags {
	if lookup == nil {
		return cfg
	}
	if token, presente := lookup(envRESTToken); presente {
		if token = strings.TrimSpace(token); token != "" {
			// El entorno tiene prioridad: permite que los lanzadores seguros no
			// expongan la credencial en argv ni dependan de un flag heredado.
			cfg.token = token
		}
	}
	return cfg
}

func extraerAccionesDirectas(args []string) (cfg accionesDirectas, resto []string) {
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-ayuda-detallada", "--ayuda-detallada":
			cfg.ayudaDetallada = true
			i++
			continue
		case "-listar-dominios", "--listar-dominios":
			cfg.listarDominios = true
			i++
			continue
		case "-anadir-dominio", "--anadir-dominio", "-añadir-dominio", "--añadir-dominio":
			cfg.anadirDominio = true
			i++
			continue
		case "-eliminar-dominio", "--eliminar-dominio":
			cfg.eliminarDominio = true
			i++
			continue
		case "-importar-dominios", "--importar-dominios":
			cfg.importarDominios = true
			i++
			continue
		case "-importar-p12", "--importar-p12":
			if i+1 < len(args) {
				cfg.importarP12Ruta = args[i+1]
				i += 2
				continue
			}
		case "-exportar-dominios", "--exportar-dominios":
			cfg.exportarDominios = true
			i++
			continue
		case "-limpiar-dominios", "--limpiar-dominios", "-borrar-dominios", "--borrar-dominios":
			cfg.limpiarDominios = true
			i++
			continue
		case "-listar-autoseleccion-cert", "--listar-autoseleccion-cert", "-ver-autoseleccion-cert", "--ver-autoseleccion-cert":
			cfg.listarAutoseleccion = true
			i++
			continue
		case "-eliminar-autoseleccion-cert", "--eliminar-autoseleccion-cert", "-borrar-autoseleccion-portal", "--borrar-autoseleccion-portal":
			cfg.eliminarAutoseleccion = true
			i++
			continue
		case "-resetear-autoseleccion-cert", "--resetear-autoseleccion-cert", "-restablecer-autoseleccion-cert", "--restablecer-autoseleccion-cert":
			cfg.resetearAutoseleccion = true
			i++
			continue
		case "-exportar-autoseleccion-cert", "--exportar-autoseleccion-cert":
			cfg.exportarAutoseleccion = true
			i++
			continue
		case "-importar-autoseleccion-cert", "--importar-autoseleccion-cert":
			cfg.importarAutoseleccion = true
			i++
			continue
		case "-limpiar-autoseleccion-cert", "--limpiar-autoseleccion-cert", "-borrar-autoseleccion-cert", "--borrar-autoseleccion-cert", "-limpiar-seleccion-certificado", "--limpiar-seleccion-certificado":
			cfg.limpiarAutoseleccion = true
			i++
			continue
		case "-estado-almacen-tls", "--estado-almacen-tls":
			cfg.estadoAlmacenTLS = true
			i++
			continue
		case "-limpiar-almacen-tls", "--limpiar-almacen-tls":
			cfg.limpiarAlmacenTLS = true
			i++
			continue
		case "-estado-confianza-tls", "--estado-confianza-tls":
			cfg.estadoConfianzaTLS = true
			i++
			continue
		case "-generar-certificados-tls", "--generar-certificados-tls":
			cfg.generarCertificadosTLS = true
			i++
			continue
		case "-instalar-confianza-tls", "--instalar-confianza-tls":
			cfg.instalarConfianzaTLS = true
			i++
			continue
		}
		resto = append(resto, args[i])
		i++
	}
	return cfg, resto
}

func (cfg accionesDirectas) inyectarEnCLI(args []string) []string {
	op := ""
	switch {
	case cfg.listarDominios:
		op = "listar-dominios"
	case cfg.anadirDominio:
		op = "anadir-dominio"
	case cfg.eliminarDominio:
		op = "eliminar-dominio"
	case cfg.importarDominios:
		op = "importar-dominios"
	case cfg.importarP12Ruta != "":
		op = "importar-p12"
	case cfg.exportarDominios:
		op = "exportar-dominios"
	case cfg.limpiarDominios:
		op = "limpiar-dominios"
	case cfg.listarAutoseleccion:
		op = "listar-autoseleccion-cert"
	case cfg.eliminarAutoseleccion:
		op = "eliminar-autoseleccion-cert"
	case cfg.resetearAutoseleccion:
		op = "resetear-autoseleccion-cert"
	case cfg.exportarAutoseleccion:
		op = "exportar-autoseleccion-cert"
	case cfg.importarAutoseleccion:
		op = "importar-autoseleccion-cert"
	case cfg.limpiarAutoseleccion:
		op = "limpiar-autoseleccion-cert"
	case cfg.estadoAlmacenTLS:
		op = "estado-almacen-tls"
	case cfg.limpiarAlmacenTLS:
		op = "limpiar-almacen-tls"
	case cfg.estadoConfianzaTLS:
		op = "estado-confianza-tls"
	case cfg.generarCertificadosTLS:
		op = "generar-certificados-tls"
	case cfg.instalarConfianzaTLS:
		op = "instalar-confianza-tls"
	}
	if op == "" {
		return args
	}
	out := []string{"-modo-cli", "-operacion", op}
	if cfg.importarP12Ruta != "" {
		out = append(out, "-fichero-p12", cfg.importarP12Ruta)
	}
	return append(out, args...)
}

func runRESTServer(ctx context.Context, logger *slog.Logger, rutaP12, password, rutaCert, rutaClave string, cfg restFlags) int {
	loc := localizador.Detectar()
	if cfg.lifetime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.lifetime)
		defer cancel()
	}
	tokenGenerado := false
	if err := validarPoliticaREST(cfg); err != nil {
		logger.ErrorContext(ctx, "política REST insegura", "op", "rest-security", "error", err, "addr", cfg.addr)
		fmt.Fprintln(os.Stderr, "error de seguridad en servidor REST:", err)
		return 1
	}
	if strings.TrimSpace(cfg.token) == "" && strings.TrimSpace(cfg.certFingerprintsCSV) == "" {
		token, err := generarTokenREST()
		if err != nil {
			logger.ErrorContext(ctx, "no se pudo generar la credencial REST", "op", "rest-security", "error", err)
			fmt.Fprintln(os.Stderr, "error generando la credencial REST:", err)
			return 1
		}
		cfg.token = token
		tokenGenerado = true
		logger.InfoContext(ctx, "credencial REST efímera generada", "op", "rest-security", "addr", cfg.addr)
	}
	servicios, err := construirServicios(rutaP12, password, rutaCert, rutaClave)
	if err != nil {
		logger.ErrorContext(ctx, "no se pudo arrancar el servidor REST", "op", "rest-bootstrap", "error", err)
		fmt.Fprintln(os.Stderr, "error arrancando el servidor REST:", err)
		return 1
	}
	credentialFile := ""
	if tokenGenerado {
		var cleanup func()
		credentialFile, cleanup, err = escribirCredencialRESTPrivada(servicios.configDir, cfg.token)
		if err != nil {
			logger.ErrorContext(ctx, "no se pudo entregar la credencial REST de forma privada", "op", "rest-security", "error", err)
			fmt.Fprintln(os.Stderr, loc.T("rest.error.private_credential"))
			return 1
		}
		defer cleanup()
	}
	if err := configurarVerificacionLocal(servicios.verificar, cfg); err != nil {
		logger.ErrorContext(ctx, "configuración de verificación local inválida", "op", "rest-verify-config", "error", err)
		fmt.Fprintln(os.Stderr, loc.T("rest.verify_only.error.config", err))
		return 1
	}
	selector := application.NuevoSelectCertificateUseCase(servicios.catalogo, &aprobacionAutomatica{}, nil, relojReal{})
	adaptador := restin.New(servicios.firmar, servicios.verificar, selector).WithBearerToken(cfg.token)
	adaptador.WithBatchSigner(servicios.procesarLote)
	adaptador.WithHashes(servicios.crearHash, servicios.comprobarHash)
	adaptador.WithDirectoryHashes(servicios.crearHashDir, servicios.comprobarHashDir)
	adaptador.WithDirectoryHashReports(servicios.informeHashDir)
	adaptador.WithPDFPreview(application.NuevoPdfPreviewUseCase(pdfpreview.New()))
	if esDireccionLoopback(cfg.addr) {
		// La GUI REST local necesita leer el PDF seleccionado para renderizarlo,
		// pero no se habilitan las rutas del resto de endpoints.
		adaptador.WithPDFPreviewPaths()
	}
	if strings.TrimSpace(cfg.certFingerprintsCSV) != "" {
		adaptador.WithCertificateAuth(cfg.certFingerprintsCSV, cfg.sessionTTL)
	}
	adaptador.WithCertificateSources(servicios.catalogo, servicios.claves)
	adaptador.WithProtection(servicios.proteger, servicios.desproteger, servicios.proteccion)
	adaptador.WithSignedProtection(servicios.protegerFirmando)
	adaptador.WithIntercambioProteccion(servicios.exportarProteccion, servicios.importarProteccion)
	adaptador.WithProxySecretStore(proxysecretstore.New())
	adaptador.WithConfigDir(servicios.configDir)
	adaptador.WithServicio(servicemanager.New(""))
	adaptador.WithLocalizador(localizador.Detectar())
	tlsDir := filepath.Join(servicios.configDir, "tls")
	srv, err := restin.StartTLSServer(ctx, cfg.addr, adaptador.Routes(), tlsDir)
	if err != nil {
		logger.ErrorContext(ctx, "no se pudo abrir el servidor REST", "op", "rest-listen", "error", err)
		fmt.Fprintln(os.Stderr, "error abriendo el servidor REST:", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Servidor REST local activo en https://%s\n", srv.Addr)
	fmt.Fprintf(os.Stdout, "OpenAPI: https://%s/openapi.json\n", srv.Addr)
	if cfg.token != "" {
		fmt.Fprintln(os.Stdout, "Autenticación Bearer: activa")
		if tokenGenerado {
			fmt.Fprintln(os.Stdout, loc.T("rest.output.bearer_file", credentialFile))
			fmt.Fprintf(
				os.Stdout,
				"%s\n",
				loc.T(
					"rest.output.curl_usage",
					srv.CertFile,
					credentialFile,
					srv.Addr,
				),
			)
		}
	} else {
		fmt.Fprintln(os.Stdout, "Autenticación Bearer: desactivada")
	}
	if cfg.certFingerprintsCSV != "" {
		fmt.Fprintf(os.Stdout, "Autenticación por certificado: activa (TTL=%s)\n", cfg.sessionTTL)
	} else {
		fmt.Fprintln(os.Stdout, "Autenticación por certificado: desactivada")
	}
	if cfg.lifetime > 0 {
		fmt.Fprintf(os.Stdout, "Caducidad automática del servidor: %s\n", cfg.lifetime)
	}
	<-ctx.Done()
	return 0
}

func generarTokenREST() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func escribirCredencialRESTPrivada(configDir, token string) (string, func(), error) {
	if strings.TrimSpace(token) == "" {
		return "", func() {}, os.ErrInvalid
	}
	if strings.ContainsAny(token, "\"\r\n") {
		return "", func() {}, os.ErrInvalid
	}
	runDir := filepath.Join(configDir, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", func() {}, err
	}
	if err := securefile.ProtectDirectory(runDir, 0o700); err != nil {
		return "", func() {}, err
	}
	path := filepath.Join(runDir, fmt.Sprintf("rest-auth-%d.curl", os.Getpid()))
	payload := []byte(fmt.Sprintf("header = \"Authorization: Bearer %s\"\n", token))
	defer clear(payload)
	if err := securefile.WriteFileAtomic(path, payload, 0o600); err != nil {
		return "", func() {}, err
	}
	if err := securefile.ProtectFile(path, 0o600); err != nil {
		_ = os.Remove(path)
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.Remove(path)
	}
	return path, cleanup, nil
}

func validarPoliticaREST(cfg restFlags) error {
	if raw := strings.TrimSpace(cfg.certFingerprintsCSV); raw != "" {
		if _, err := normalizarHuellasCertREST(raw); err != nil {
			return err
		}
	}
	if esDireccionLoopback(cfg.addr) {
		return nil
	}
	if !cfg.permitirRemoto {
		return fmt.Errorf("la dirección REST %q no es loopback; use -rest-publico solo si desea exponer el servicio deliberadamente", cfg.addr)
	}
	if strings.TrimSpace(cfg.token) == "" && strings.TrimSpace(cfg.certFingerprintsCSV) == "" {
		return fmt.Errorf("la exposición REST fuera de loopback exige token Bearer o huellas de certificado")
	}
	return nil
}

func esDireccionLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func solicitaAyudaGeneral(args []string) bool {
	if solicitaModoCLI(args) {
		return false
	}
	return solicitaAyuda(args)
}

func solicitaAyuda(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-a", "--a", "-h", "--h", "-help", "--help", "-ayuda", "--ayuda", "-ayuda-cli", "--ayuda-cli", "-cli-help", "--cli-help", "-ayuda-detallada", "--ayuda-detallada":
			return true
		}
	}
	return false
}

func solicitaModoCLI(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-cli", "--cli", "-modo-cli", "--modo-cli":
			return true
		}
	}
	return false
}

func imprimirUso() {
	escribirUsoGeneral(os.Stderr)
}

func escribirUsoGeneral(w io.Writer) {
	const plantilla = `Uso:
  grxfirma -contenedor-p12 cert.p12 [-contrasena-stdin] -entrada doc.pdf -salida doc.csig [-formato cades]
  grxfirma -certificado-pem cert.pem -clave-pem key.pem -entrada doc.pdf -salida doc.csig [-formato cades]
  grxfirma -contenedor-p12 cert.p12 [-contrasena-stdin] -lote manifiesto.json
  grxfirma -modo-cli -ayuda

Banderas de credencial:
  -contenedor-p12 <ruta>  Fichero PKCS#12 (.p12 / .pfx) con certificado y clave privada
  -contrasena-stdin       Contraseña P12 por stdin sin eco
  Alias compatibles: -p12, -password-stdin

  -certificado-pem <ruta> Certificado PEM
  -clave-pem       <ruta> Clave privada PEM (RSA, ECDSA o PKCS#8)
  Alias compatibles: -cert, -key, -clave

Banderas de operación:
  -entrada         <ruta> Documento a firmar, verificar, proteger o desproteger
  -salida          <ruta> Fichero de salida con la firma o el resultado de protección
  -formato         <fmt>  {{FORMATOS_FIRMA}}
  -accion          <acc>  firmar (por defecto) | cofirmar | contrafirmar
  -certificado     <id>   Identificador del certificado
  -lote            <ruta> Manifiesto JSON para firma en lote
  -perfil-proteccion <id> compat | alto
  -destinatario    <id>   Destinatario de protección/cifrado (repetible)
  -tiempo-espera   <dur>  Tiempo límite (por defecto: 30s)

Atajos útiles:
  -ayuda, -a, -h
  -operacion, -op
  -entrada, -in, -e
  -salida, -out, -s
  -formato, -f
  -salida-json, -json, -j
  -listar-certificados, -ll
  -comprobar-certificados, -cc

Para ver la ayuda completa del modo CLI:
  grxfirma -modo-cli -ayuda
  grxfirma -ayuda-detallada

Acciones directas disponibles también sin -modo-cli:
  -listar-dominios
  -añadir-dominio -dominio <origen>
  -eliminar-dominio -dominio <origen>
  -importar-dominios -fichero-dominios <ruta>
  -importar-p12 <ruta> [-contrasena-p12-stdin]
  -exportar-dominios -fichero-dominios <ruta>
  -limpiar-dominios
  -listar-autoseleccion-cert
  -eliminar-autoseleccion-cert -dominio <origen>
  -resetear-autoseleccion-cert
  -exportar-autoseleccion-cert -fichero-autoseleccion <ruta>
  -importar-autoseleccion-cert -fichero-autoseleccion <ruta>
  -limpiar-autoseleccion-cert
  -estado-almacen-tls
  -limpiar-almacen-tls
  -estado-confianza-tls
  -generar-certificados-tls
  -instalar-confianza-tls
  -listar-destinatarios-proteccion

Alias compatibles no visibles:
  -anadir-dominio y -borrar-dominios

Círculo de confianza:
  Añadir un dominio significa incorporarlo al círculo de confianza para evitar
  firmar en dominios potencialmente maliciosos o ajenos.
  El primer arranque siembra dominios públicos del Estado, Junta y otras sedes,
  y después puedes listarlos, exportarlos, importarlos o borrarlos por completo.

Modo REST local:
  grxfirma -servidor-rest [-direccion-rest 127.0.0.1:63118]
  grxfirma -servidor-rest [-huellas-cert-rest <sha256,sha256>] [-ttl-sesion-rest 10m]
  Expone HTTPS local con OpenAPI en /openapi.json usando el certificado local de WebSocket.

Endpoints REST disponibles:
  GET  /openapi.json
  POST /auth/challenge
  POST /auth/verify
  POST /hash
  POST /hash/check
  GET  /protection/recipients
  POST /sign
  POST /protect
  POST /unprotect
  POST /verify
  POST /select-certificate

Ejemplos de protección local:
  grxfirma -modo-cli -operacion crear-hash -entrada /ruta/documento.pdf -algoritmo-hash sha256 -formato-hash hex
  grxfirma -modo-cli -operacion comprobar-hash -entrada /ruta/documento.pdf -fichero-hash /ruta/documento.pdf.hexhash
  grxfirma -modo-cli -operacion listar-destinatarios-proteccion -salida-json
  grxfirma -modo-cli -operacion proteger -entrada /ruta/secreto.pdf -destinatario <id> -perfil-proteccion compat
  grxfirma -modo-cli -operacion desproteger -entrada /ruta/secreto.pdf.afp

Ejemplos REST:
  grxfirma -servidor-rest -direccion-rest 127.0.0.1:63118
  grxfirma -servidor-rest -huellas-cert-rest <sha256> -ttl-sesion-rest 10m
  {{REST_PRIVATE_DELIVERY}}

Responsabilidad y garantía:
  GrxFirma: licencia EUPL 1.2 o posterior.
  Se entrega SIN GARANTÍA de ningún tipo.
  El uso de la herramienta, la firma de documentos, la confianza en dominios remotos
  y cualquier consecuencia derivada de errores, incompatibilidades o pérdida de datos
  quedan bajo la responsabilidad del operador o de la organización que la despliega.

Documentación adicional:
  man grxfirma
  man grxfirma-certificados
  man grxfirma-servicios-locales
  man grxfirma-integracion-web
  man grxfirma-lotes
  man grxfirma-protocolos-web
  man grxfirma-recetas
  man grxfirma-diagnostico
  o bien:
  man -l ~/.local/share/man/man1/grxfirma.1`
	ayuda := strings.Replace(
		plantilla,
		"{{FORMATOS_FIRMA}}",
		cliin.SignatureFormatsHelp,
		1,
	)
	ayuda = strings.Replace(
		ayuda,
		"{{REST_PRIVATE_DELIVERY}}",
		localizador.Detectar().T("rest.help.private_delivery"),
		1,
	)
	_, _ = io.WriteString(w, ayuda)
}

func imprimirUsoDetallado() {
	const plantilla = `GrxFirma — ayuda detallada

Modos principales:
  grxfirma -modo-cli ...
    Línea de comandos para firma, verificación, lotes, dominios de confianza y TLS local.

  grxfirma -servidor-rest [-direccion-rest 127.0.0.1:63118]
  grxfirma -servidor-rest [-huellas-cert-rest <sha256,sha256>] [-ttl-sesion-rest 10m]
    {{REST_PRIVATE_DELIVERY}}

  grxfirma-desktop
    Aplicación manual de escritorio para abrir, firmar y verificar documentos.
    También puedes forzar el frontend embebido actual con:
      grxfirma -desktop -frontend fyne
    Y, si existe un binario Qt externo instalado, delegar a él con:
      grxfirma -desktop -frontend qt
      grxfirma -qt

  grxfirma-afirmauri 'afirma://...'
    Adaptador protocolario para portales y sedes electrónicas.

Operaciones directas útiles:
  -listar-dominios
  -añadir-dominio -dominio <origen>
  -eliminar-dominio -dominio <origen>
  -importar-dominios -fichero-dominios <ruta>
  -exportar-dominios -fichero-dominios <ruta>
  -limpiar-dominios
  -listar-autoseleccion-cert
  -eliminar-autoseleccion-cert -dominio <origen>
  -resetear-autoseleccion-cert
  -exportar-autoseleccion-cert -fichero-autoseleccion <ruta>
  -importar-autoseleccion-cert -fichero-autoseleccion <ruta>
  -limpiar-autoseleccion-cert
  -importar-p12 <ruta> [-contrasena-p12-stdin]
  -estado-almacen-tls
  -limpiar-almacen-tls
  -estado-confianza-tls
  -generar-certificados-tls
  -instalar-confianza-tls

Círculo de confianza:
  Añadir un dominio significa incorporarlo al círculo de confianza para evitar
  firmar contra dominios potencialmente maliciosos o ajenos.
  En el primer arranque se siembran dominios públicos habituales del Estado,
  Junta de Andalucía y otras AAPP. Esa semilla es editable y puede borrarse.
  Si el círculo queda vacío y estás en modo interactivo, la CLI ofrece reimportarlo.

REST local:
  -servidor-rest
  -direccion-rest <host:puerto>
  GRXFIRMA_REST_TOKEN
    Compatibilidad automatizada; al omitirlo se genera un token efímero.
  -huellas-cert-rest <sha256,sha256>
  -ttl-sesion-rest 10m
  -rest-publico
    Solo necesario para exponer el REST fuera de loopback. Si se usa, exige autenticación.

Credenciales locales:
  -contenedor-p12 <ruta>
  -contrasena-stdin
  -certificado-pem <ruta>
  -clave-pem <ruta>
  -importar-p12 <ruta> [-contrasena-p12-stdin]

Compatibilidad:
  Se mantienen alias en inglés y alias sin ñ para scripts o terminales antiguos,
  pero la ayuda visible prioriza nombres en castellano.

Software libre y responsabilidad:
  GrxFirma: licencia EUPL 1.2 o posterior.
  Se entrega SIN GARANTÍA de ningún tipo.
  El uso de la herramienta, la firma o verificación de documentos, la confianza
  en dominios remotos y cualquier consecuencia derivada de errores,
  incompatibilidades o pérdida de datos recaen en el operador o la organización.

Documentación:
  man grxfirma
  man grxfirma-certificados
  man grxfirma-servicios-locales
  man grxfirma-integracion-web
  man grxfirma-lotes
  man grxfirma-protocolos-web
  man grxfirma-recetas
  man grxfirma-diagnostico
  man -l ~/.local/share/man/man1/grxfirma.1

Para la ayuda detallada del modo CLI:
  grxfirma -modo-cli -ayuda`
	ayuda := strings.Replace(
		plantilla,
		"{{REST_PRIVATE_DELIVERY}}",
		localizador.Detectar().T("rest.help.private_delivery"),
		1,
	)
	fmt.Fprintln(os.Stderr, ayuda)
}
