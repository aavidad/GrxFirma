// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Comando grxfirma-nativehost — host Native Messaging para navegadores.
//
// El navegador lanza este proceso y se comunica con él a través de stdin/stdout
// usando el protocolo Native Messaging (mensajes con framing de 4 bytes little-endian).
//
// Configuración (variables de entorno o config file via browser-bridge.sh):
//
//	GRXFIRMA_PKCS12_DIR      Directorio con ficheros .p12 / .pfx (defecto: ~/.config/grxfirma/pkcs12)
//	GRXFIRMA_PKCS12_PASSWORD Contraseña para los P12 (defecto: "")
//	La firma sin diálogo interactivo solo es posible en despliegues controlados
//	mediante la política de máquina aprobacion_automatica_nativehost (HKLM en
//	Windows, /etc/grxfirma/policy.json en el resto). La variable
//	GRXFIRMA_NATIVEHOST_AUTO_APPROVE ya no desactiva el consentimiento.
//
// Acciones soportadas: ping, getCertificates, sign, verify, proveIdentity
package main

import (
	"context"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	nativehost "grxfirma/internal/adapters/inbound/desktop/nativehost"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/identityjcs"
	"grxfirma/internal/adapters/outbound/common/limits"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/common/metrics"
	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/revocationclient"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/systemtrust"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/adapters/outbound/desktop/certcatalogagg"
	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/adapters/outbound/desktop/nssstore"
	"grxfirma/internal/adapters/outbound/desktop/proxyhttp"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/presentation/desktop/tokenpin"
)

// ---------------------------------------------------------------------------
// Adaptadores mínimos (mismo patrón que cmd/grxfirma/bootstrap.go)
// ---------------------------------------------------------------------------

type catalogoMemoria struct{ certs []domain.CertificateRef }

func (c *catalogoMemoria) List(_ context.Context) ([]domain.CertificateRef, error) {
	return c.certs, nil
}

type proveedorClavesMemoria struct{ claves map[string]ports.SigningKey }

func (p *proveedorClavesMemoria) KeyFor(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	if k, ok := p.claves[ref.ID]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("clave no encontrada para el certificado %s", ref.ID)
}

type proveedorClavesAgregado struct {
	fuentes []ports.SigningKeyProvider
}

func (p *proveedorClavesAgregado) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	var ultimo error
	for _, fuente := range p.fuentes {
		if fuente == nil {
			continue
		}
		clave, err := fuente.KeyFor(ctx, ref)
		if err == nil && clave != nil {
			return clave, nil
		}
		ports.CloseSigningKey(clave)
		if err != nil && !esErrorProveedorNoAplicable(err) {
			ultimo = err
		}
	}
	if ultimo != nil {
		return nil, ultimo
	}
	return nil, fmt.Errorf("clave no encontrada para el certificado %s", ref.ID)
}

func esErrorProveedorNoAplicable(err error) bool {
	return errors.Is(err, macoskeychain.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, wincertstore.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, tokenruntime.ErrNotApplicable) ||
		errors.Is(err, tokenruntime.ErrIdentityUnknown) ||
		strings.Contains(strings.ToLower(err.Error()), "nssstore: solo disponible en linux")
}

type catalogoFirmable struct {
	base ports.CertificateCatalog
}

func (c *catalogoFirmable) List(ctx context.Context) ([]domain.CertificateRef, error) {
	refs, err := c.base.List(ctx)
	if err != nil {
		return nil, err
	}
	resultado := make([]domain.CertificateRef, 0, len(refs))
	for _, ref := range refs {
		// Consultar el catálogo no abre sesiones ni solicita PIN. La evidencia
		// de clave procede del almacén; desconocida no equivale a disponible.
		if ref.HasSigningKey || ref.SigningKeyNeedsUnlock {
			resultado = append(resultado, ref)
		}
	}
	return resultado, nil
}

type aprobacionSistema struct {
	autoApprove bool
	logger      *slog.Logger
}

func (a *aprobacionSistema) Request(ctx context.Context, mensaje string) (bool, error) {
	if a != nil && a.autoApprove {
		if a.logger != nil {
			a.logger.WarnContext(ctx, "nativehost_auto_approval_enabled")
		}
		return true, nil
	}
	if strings.TrimSpace(mensaje) == "" {
		mensaje = "Confirme la operación de firma solicitada desde el navegador."
	}
	ok, err := solicitarAprobacionSistema(ctx, "GrxFirma", mensaje)
	if err != nil {
		return false, err
	}
	return ok, nil
}

type loggerSilencioso struct{}

func (l *loggerSilencioso) Log(_ context.Context, _ ports.Evidence) error { return nil }

type relojReal struct{}

func (r relojReal) Now() time.Time { return time.Now() }

func construirMotorFirmaConHTTP(httpClient *http.Client) *deskSigner.MotorFirmaGo {
	motor := deskSigner.NuevoMotorFirmaGo(relojReal{})
	if tsaURL := strings.TrimSpace(os.Getenv("GRXFIRMA_TSA_URL")); tsaURL != "" {
		tsa := tsaclient.New(tsaURL)
		tsa.HTTPClient = httpClient
		motor.WithTimestampAuthority(tsa)
	}
	motor.WithRevocationProvider(revocationclient.NewWithHTTPClient(httpClient))
	return motor
}

// ---------------------------------------------------------------------------
// Carga de P12 desde directorio
// ---------------------------------------------------------------------------

type entradaCert struct {
	ref   domain.CertificateRef
	clave *deskSigner.ClaveLocal
}

func cargarDirectorioP12(dir, password string, metricas ports.OperationMetrics) ([]entradaCert, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // directorio vacío es aceptable
		}
		return nil, fmt.Errorf("no se pudo leer el directorio de certificados %s: %w", dir, err)
	}

	var resultado []entradaCert
	importador := pkcs12importer.New()
	ctx := context.Background()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if !strings.HasSuffix(name, ".p12") && !strings.HasSuffix(name, ".pfx") {
			continue
		}
		ruta := filepath.Join(dir, e.Name())
		identidad, err := importador.ImportP12File(ctx, ruta, password)
		if err != nil {
			continue // contraseña incorrecta o formato no soportado: lo saltamos
		}
		if metricas != nil {
			metricas.RecordCertificateSource(ctx, "p12")
		}
		clave := deskSigner.NuevaClaveLocalConCadena(identidad.Signer, identidad.Certificate, identidad.Chain)
		ref := identidad.Reference
		resultado = append(resultado, entradaCert{ref: ref, clave: clave})
	}
	return resultado, nil
}

// ---------------------------------------------------------------------------
// Bootstrap del adaptador nativo
// ---------------------------------------------------------------------------

func construirFuentesCertificados(p12Dir, p12Password string, metricas ports.OperationMetrics) (ports.CertificateCatalog, ports.SigningKeyProvider, error) {
	entradas, err := cargarDirectorioP12(p12Dir, p12Password, metricas)
	if err != nil {
		return nil, nil, err
	}

	refs := make([]domain.CertificateRef, 0, len(entradas))
	claves := make(map[string]ports.SigningKey, len(entradas))
	for _, e := range entradas {
		refs = append(refs, e.ref)
		claves[e.ref.ID] = e.clave
	}

	catalogoMem := &catalogoMemoria{certs: refs}
	proveedorMem := &proveedorClavesMemoria{claves: claves}
	nss := nssstore.New()
	mac := macoskeychain.New()
	win := wincertstore.New()

	agregadoCatalogo := certcatalogagg.New(
		nss,
		mac,
		win,
		catalogoMem,
	)
	agregadoClaves := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{
		nss,
		mac,
		win,
		proveedorMem,
	}}

	return &catalogoFirmable{base: agregadoCatalogo}, agregadoClaves, nil
}

func construirAdaptador(configDir, p12Dir, p12Password string, metricas ports.OperationMetrics, logger *slog.Logger) (*nativehost.Adaptador, error) {
	catalogo, proveedor, err := construirFuentesCertificados(p12Dir, p12Password, metricas)
	if err != nil {
		return nil, err
	}
	if tokenruntime.EnabledInBuild() {
		tokens := tokenruntime.New(configDir, tokenpin.New("es").Request)
		catalogo = &catalogoFirmable{base: certcatalogagg.New(catalogo, tokens)}
		proveedor = &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{proveedor, tokens}}
	}
	// El catálogo no se consulta al arrancar: el navegador lanza el host sin
	// acción del usuario y el resultado no se usaba. Se lista al atender una
	// petición que lo necesita.

	aprobador := &aprobacionSistema{
		autoApprove: machinepolicy.OptIn(machinepolicy.AprobacionAutomaticaHost),
		logger:      logger,
	}
	httpClient := proxyhttp.New(configDir)
	motor := construirMotorFirmaConHTTP(httpClient)
	auditor := application.NuevoAuditUseCase(relojReal{}, &loggerSilencioso{})

	ucFirmar := application.NuevoSignDocumentUseCase(catalogo, proveedor, motor, aprobador, auditor, nil).WithMetrics(metricas)
	firmadorIdentidad := application.NuevoSignDocumentUseCase(
		catalogo,
		proveedor,
		motor,
		&aprobacionSistema{logger: logger},
		auditor,
		nil,
	).WithMetrics(metricas)
	ucVerificar := application.NuevoVerifySignatureUseCase(
		systemtrust.New(),
		commonsigner.NewMultiVerifierWithHTTPClient(httpClient),
		auditor,
	)

	canonicalizadorIdentidad, err := identityjcs.Nuevo(identityjcs.ConfiguracionPredeterminada())
	if err != nil {
		return nil, fmt.Errorf("no se pudo configurar identidad local: %w", err)
	}
	generadorIdentidad, err := application.NuevoGeneradorPruebaIdentidadLocal(
		canonicalizadorIdentidad,
		catalogo,
		nuevoSelectorCertificadoIdentidadSistema(localizador.Detectar()),
		firmadorIdentidad,
		relojReal{},
	)
	if err != nil {
		return nil, fmt.Errorf("no se pudo componer identidad local: %w", err)
	}
	adaptador := nativehost.New(catalogo, ucFirmar, ucVerificar, nil).
		WithMetrics(metricas).
		WithGeneradorPruebaIdentidad(generadorIdentidad)
	adaptador.RequireCaller = true
	return adaptador, nil
}

// ---------------------------------------------------------------------------
// Bucle principal
// ---------------------------------------------------------------------------

// version se fija mediante -ldflags "-X main.version=..." al empaquetar.
var version = "dev"

func isVersionRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "--version" || args[0] == "-version")
}

func main() {
	// Diagnóstico explícito, antes de señales, configuración o almacenes. Nunca
	// escribir texto sin framing cuando se reciban argumentos del navegador.
	if isVersionRequest(os.Args[1:]) {
		_, _ = fmt.Fprintln(os.Stdout, "grxfirma-nativehost", version)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := logging.New("cmd/nativehost", os.Stderr)
	metricas := metrics.NewDefault()

	home, _ := os.UserHomeDir()
	executable, err := os.Executable()
	if err != nil {
		writeErrorToLog(logger, ctx, "identificar_ejecutable", err)
		os.Exit(1)
	}
	callerPolicy := newNativeCallerPolicy(
		executable,
		home,
		nativeCallerDevelopmentAllowed(),
	)
	callerID, err := nativeCallerFromArgs(os.Args[1:], callerPolicy)
	if err != nil {
		writeErrorToLog(logger, ctx, "validar_caller", err)
		os.Exit(1)
	}

	configDir := appdirs.Config(home)
	cfg, err := config.Load(configDir)
	if err != nil {
		writeErrorToLog(logger, ctx, "cargar_configuracion", err)
		os.Exit(1)
	}

	p12Dir := cfg.DirectorioP12
	if p12Dir == "" {
		p12Dir = filepath.Join(configDir, "pkcs12")
	}
	if override := os.Getenv("GRXFIRMA_PKCS12_DIR"); override != "" {
		p12Dir = override
	}
	p12Password := os.Getenv("GRXFIRMA_PKCS12_PASSWORD")

	adaptador, err := construirAdaptador(configDir, p12Dir, p12Password, metricas, logger)
	if err != nil {
		writeErrorToLog(logger, ctx, "bootstrap", err)
		os.Exit(1)
	}
	limites := limits.FromEnv(limits.Default())
	operationTimeout := time.Duration(limites.MaxSessionSec) * time.Second
	if operationTimeout <= 0 {
		operationTimeout = time.Duration(limits.Default().MaxSessionSec) * time.Second
	}

	// El navegador mantiene el proceso vivo y envía mensajes hasta desconectarse.
	// Leemos en bucle hasta EOF (desconexión de la extensión).
	logger.DebugContext(ctx, "nativehost_loop_start")
	for {
		err := serveNativeMessage(ctx, adaptador, callerID, os.Stdin, os.Stdout, operationTimeout)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				logger.DebugContext(ctx, "nativehost_loop_end", "reason", "browser_disconnect")
				break // la extensión se desconectó limpiamente
			}
			writeErrorToLog(logger, ctx, "serve_once", err)
			break
		}
	}
}

// serveNativeMessage deja fuera del timeout la espera ociosa entre mensajes.
// El limite de sesion empieza cuando ya existe una solicitud completa que procesar.
func serveNativeMessage(ctx context.Context, adaptador *nativehost.Adaptador, callerID string, r io.Reader, w io.Writer, operationTimeout time.Duration) error {
	payload, err := nativehost.ReadMessage(r)
	if err != nil {
		return err
	}

	opCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	responses, err := adaptador.Process(opCtx, callerID, payload)
	if err != nil {
		return err
	}
	for _, response := range responses {
		if err := nativehost.WriteMessage(w, response); err != nil {
			return err
		}
	}
	return nil
}

func solicitarAprobacionSistema(ctx context.Context, titulo, mensaje string) (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return solicitarAprobacionMacOS(ctx, titulo, mensaje)
	case "windows":
		return solicitarAprobacionWindows(ctx, titulo, mensaje)
	default:
		return solicitarAprobacionLinux(ctx, titulo, mensaje)
	}
}

func solicitarAprobacionLinux(ctx context.Context, titulo, mensaje string) (bool, error) {
	candidatos := []struct {
		bin  string
		args []string
	}{
		{"zenity", []string{"--question", "--title", titulo, "--text", mensaje, "--width", "520", "--ok-label", "Firmar", "--cancel-label", "Cancelar"}},
		{"qarma", []string{"--question", "--title", titulo, "--text", mensaje, "--width", "520", "--ok-label", "Firmar", "--cancel-label", "Cancelar"}},
		{"kdialog", []string{"--warningcontinuecancel", mensaje, "--title", titulo}},
	}
	for _, candidato := range candidatos {
		path, err := exec.LookPath(candidato.bin)
		if err != nil {
			continue
		}
		return ejecutarDialogo(ctx, path, candidato.args...)
	}
	return false, errors.New("no hay dialogo grafico disponible para confirmar la firma; configure zenity/kdialog o, en entornos controlados, la política de máquina aprobacion_automatica_nativehost")
}

func solicitarAprobacionMacOS(ctx context.Context, titulo, mensaje string) (bool, error) {
	path, err := exec.LookPath("osascript")
	if err != nil {
		return false, errors.New("osascript no disponible para confirmar la firma")
	}
	return ejecutarDialogo(ctx, path,
		"-e", "on run argv",
		"-e", "display dialog (item 1 of argv) with title (item 2 of argv) buttons {\"Cancelar\", \"Firmar\"} default button \"Firmar\" cancel button \"Cancelar\" with icon caution",
		"-e", "end run",
		mensaje,
		titulo,
	)
}

func ejecutarDialogo(ctx context.Context, path string, args ...string) (bool, error) {
	// #nosec G204 -- path comes from LookPath over the fixed dialog-tool
	// allowlists above; user-facing text is passed as argv, never as shell code.
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func envBool(nombre string, defecto bool) bool {
	valor := strings.TrimSpace(strings.ToLower(os.Getenv(nombre)))
	if valor == "" {
		return defecto
	}
	switch valor {
	case "1", "true", "t", "yes", "y", "on", "si", "sí":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return defecto
	}
}

// writeErrorToLog escribe los errores en stderr (nunca en stdout, que está
// reservado para el protocolo Native Messaging).
func writeErrorToLog(logger *slog.Logger, ctx context.Context, op string, err error) {
	if err == nil {
		return
	}
	logger.ErrorContext(ctx, "operacion fallida", "op", op, "error", err)
}
