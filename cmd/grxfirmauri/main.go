// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"errors"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	legacyws "grxfirma/internal/adapters/inbound/legacy/websocket"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/logging"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
)

var version = "dev"

const defaultLaunchedWebSocketPort = 63117
const removeManagedLocalTLSTrustFlag = "--remove-local-tls-trust"
const noPromptLocalTLSTrustFlag = "--no-prompt"

// installManagedLocalTLSTrustFlag genera la CA local del canal WebSocket y la
// instala en la confianza del usuario. El instalador interactivo lo usa para
// pedir la confirmación durante la instalación; /S la difiere al primer uso.
const installManagedLocalTLSTrustFlag = "--install-local-tls-trust"

const legacyLaunchTerminalGrace = 350 * time.Millisecond

var removeManagedLocalTLSTrust = func(ctx context.Context, rootCertFile string) error {
	return localtlstrust.WithLocalTLSStartupLock(ctx, filepath.Dir(filepath.Dir(rootCertFile)), func() error {
		return localtlstrust.RemoveManagedTrusted(ctx, rootCertFile)
	})
}

var retireManagedLocalTLSKey = func(ctx context.Context, rootCertFile string) error {
	if runtime.GOOS != "windows" {
		return localtlstrust.ErrSoporteNoDisponible
	}
	return localtlstrust.WithLocalTLSStartupLock(ctx, filepath.Dir(filepath.Dir(rootCertFile)), func() error {
		return localtlstrust.RetireManagedCAKeyWithoutPrompt(rootCertFile)
	})
}

var installManagedLocalTLSTrust = func(ctx context.Context, certDir string) error {
	return localtlstrust.WithLocalTLSStartupLock(ctx, filepath.Dir(certDir), func() error {
		_, _, rootCertFile, _, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(certDir, "websocket-localhost")
		if err != nil {
			return err
		}
		return localtlstrust.EnsureManagedTrusted(ctx, rootCertFile)
	})
}

type launchTimer struct {
	logger   *slog.Logger
	ctx      context.Context
	start    time.Time
	lastMark time.Time
	flow     string
}

func newLaunchTimer(ctx context.Context, logger *slog.Logger, flow string) *launchTimer {
	if !protocolDebugEnabled() {
		return nil
	}
	now := time.Now()
	return &launchTimer{
		logger:   logger,
		ctx:      ctx,
		start:    now,
		lastMark: now,
		flow:     flow,
	}
}

func (t *launchTimer) Mark(phase string, kv ...any) {
	if t == nil || t.logger == nil {
		return
	}
	now := time.Now()
	fields := []any{
		"pid", os.Getpid(),
		"flow", t.flow,
		"phase", phase,
		"elapsed_ms_total", now.Sub(t.start).Milliseconds(),
		"elapsed_ms_step", now.Sub(t.lastMark).Milliseconds(),
	}
	fields = append(fields, kv...)
	t.lastMark = now
	t.logger.InfoContext(t.ctx, "afirmauri_launch_timing", fields...)
}

type startupProbe struct {
	logger   *slog.Logger
	ctx      context.Context
	start    time.Time
	lastWall time.Time
	lastCPU  time.Duration
	stopOnce sync.Once
	done     chan struct{}
}

func newStartupProbe(ctx context.Context, logger *slog.Logger) *startupProbe {
	if !protocolDebugEnabled() {
		return nil
	}
	now := time.Now()
	return &startupProbe{
		logger:   logger,
		ctx:      ctx,
		start:    now,
		lastWall: now,
		lastCPU:  readProcessCPUTime(),
		done:     make(chan struct{}),
	}
}

func (p *startupProbe) Start() {
	if p == nil || p.logger == nil {
		return
	}
	p.logger.InfoContext(p.ctx, "afirmauri_startup_probe_begin")
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.sample("tick")
			}
		}
	}()
}

func (p *startupProbe) Stop(reason string) {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		p.sample(reason)
		close(p.done)
	})
}

func (p *startupProbe) sample(reason string) {
	if p == nil || p.logger == nil {
		return
	}
	now := time.Now()
	cpuNow := readProcessCPUTime()
	deltaWall := now.Sub(p.lastWall)
	deltaCPU := cpuNow - p.lastCPU
	cpuPct := 0.0
	if deltaWall > 0 {
		cpuPct = 100 * deltaCPU.Seconds() / deltaWall.Seconds()
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	p.logger.InfoContext(
		p.ctx,
		"afirmauri_startup_probe",
		"pid", os.Getpid(),
		"reason", reason,
		"elapsed_ms_total", now.Sub(p.start).Milliseconds(),
		"cpu_pct_interval", cpuPct,
		"cpu_ms_total", cpuNow.Milliseconds(),
		"goroutines", runtime.NumGoroutine(),
		"heap_alloc_mb", bytesToMB(ms.Alloc),
		"heap_sys_mb", bytesToMB(ms.HeapSys),
		"rss_mb", bytesToMB(readProcessRSSBytes()),
	)
	p.lastWall = now
	p.lastCPU = cpuNow
}

func bytesToMB(v uint64) float64 {
	return float64(v) / (1024 * 1024)
}

func protocolDebugEnabled() bool {
	if !logging.DebugAllowed() {
		return false
	}
	if raw := strings.TrimSpace(os.Getenv("GRXFIRMA_DEBUG")); raw != "" && raw != "0" {
		return true
	}
	if raw := strings.TrimSpace(os.Getenv("GRXFIRMA_DEBUG_ENABLED")); raw != "" && raw != "0" {
		return true
	}
	return false
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stderr))
}

type manejadorSolicitudProtocolo func(context.Context, io.Writer, string) int

func run(ctx context.Context, args []string, stderr io.Writer) int {
	return runConDependencias(ctx, args, stderr, handleProtocolRequest)
}

func runConDependencias(
	ctx context.Context,
	args []string,
	stderr io.Writer,
	manejarSolicitud manejadorSolicitudProtocolo,
) int {
	_ = os.Setenv("SYSLOG_IDENTIFIER", "grxfirma-afirmauri")
	setProtocolLocalizer(defaultProtocolConfigDir())
	setStartupTLSTrustNotice("")

	noPromptRemoval := len(args) == 2 && strings.EqualFold(strings.TrimSpace(args[0]), removeManagedLocalTLSTrustFlag) && strings.EqualFold(strings.TrimSpace(args[1]), noPromptLocalTLSTrustFlag)
	if (len(args) == 1 || noPromptRemoval) && strings.EqualFold(
		strings.TrimSpace(args[0]),
		removeManagedLocalTLSTrustFlag,
	) {
		rootCertFile := filepath.Join(
			defaultProtocolConfigDir(),
			"tls",
			"websocket-localhost-root.crt.pem",
		)
		remove := removeManagedLocalTLSTrust
		if noPromptRemoval {
			remove = retireManagedLocalTLSKey
		}
		if err := remove(ctx, rootCertFile); err != nil {
			_, _ = fmt.Fprintln(stderr, tl("error: ")+err.Error())
			return 1
		}
		return 0
	}

	if len(args) == 1 && strings.EqualFold(
		strings.TrimSpace(args[0]),
		installManagedLocalTLSTrustFlag,
	) {
		if err := installManagedLocalTLSTrust(ctx, filepath.Join(defaultProtocolConfigDir(), "tls")); err != nil {
			_, _ = fmt.Fprintln(stderr, tl("error: ")+err.Error())
			return 1
		}
		return 0
	}

	// Invocado por el navegador como manejador de afirma://, el único argumento
	// válido es la URI: nunca se interpretan flags junto a ella, aunque una
	// URI manipulada llegase partida en varios argumentos.
	if invocacionProtocolo(args) {
		if len(args) != 1 {
			_, _ = fmt.Fprintln(stderr, tl("error: ")+tl("invocación del protocolo con argumentos adicionales rechazada"))
			return 1
		}
		return ejecutarURIProtocolo(ctx, stderr, args[0], manejarSolicitud)
	}

	serverCfg, args, err := extraerFlagsServidor(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, tl("error: ")+err.Error())
		return 1
	}
	if serverCfg.habilitado {
		return runServerMode(ctx, stderr, serverCfg)
	}

	if len(args) == 1 && (args[0] == "-version" || args[0] == "--version") {
		_, _ = fmt.Fprintln(stderr, tl("grxfirma-afirmauri %s", version))
		return 0
	}
	if len(args) == 0 {
		if handled, code := maybeRunDesktopService(ctx, stderr); handled {
			return code
		}
		_, _ = fmt.Fprintln(stderr, tl("error: ")+tl("falta la URI afirma:// como argumento"))
		return 1
	}

	return ejecutarURIProtocolo(ctx, stderr, args[0], manejarSolicitud)
}

func invocacionProtocolo(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(arg)), "afirma:") {
			return true
		}
	}
	return false
}

func ejecutarURIProtocolo(
	ctx context.Context,
	stderr io.Writer,
	rawURI string,
	manejarSolicitud manejadorSolicitudProtocolo,
) int {
	// Fyne/GLFW necesita OpenGL. En Windows x64 esta preparación puede activar
	// el fallback Win32 seguro; si ninguna interfaz puede mostrar aprobación o
	// selección, se falla cerrado antes de interpretar o ejecutar la URI.
	if err := prepareProtocolInteractiveUI(stderr); err != nil {
		return graphicsUnavailableExitCode
	}
	if err := prepareStartupLocalTLSTrust(ctx, defaultProtocolConfigDir(), stderr); err != nil {
		return 1
	}
	if forwarded, err := tryForwardLaunchToResident(rawURI); forwarded {
		if err == nil {
			slog.InfoContext(ctx, "legacy_launch_forwarded_to_resident")
			return 0
		}
		slog.WarnContext(ctx, "legacy_launch_forward_failed", "error", err)
	}
	if handled, code := maybeRunProtocolUI(ctx, stderr, rawURI); handled {
		return code
	}
	if manejarSolicitud == nil {
		_, _ = fmt.Fprintln(stderr, tl("error: ")+tl("manejador de protocolo no configurado"))
		return 1
	}
	return manejarSolicitud(ctx, stderr, rawURI)
}

type serverFlags struct {
	habilitado bool
	modo       string
}

func extraerFlagsServidor(args []string) (serverFlags, []string, error) {
	cfg := serverFlags{modo: "websocket"}
	resto := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch strings.ToLower(arg) {
		case "-server", "--server":
			cfg.habilitado = true
		case "-server-modo", "--server-modo":
			if i+1 >= len(args) {
				return cfg, resto, fmt.Errorf("falta valor para %s", arg)
			}
			cfg.habilitado = true
			cfg.modo = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(strings.ToLower(arg), "--server-modo=") || strings.HasPrefix(strings.ToLower(arg), "-server-modo=") {
				cfg.habilitado = true
				cfg.modo = strings.TrimSpace(arg[strings.Index(arg, "=")+1:])
				continue
			}
			resto = append(resto, args[i])
		}
	}
	return cfg, resto, nil
}

func runServerMode(ctx context.Context, stderr io.Writer, cfg serverFlags) int {
	switch strings.ToLower(strings.TrimSpace(cfg.modo)) {
	case "", "websocket", "wss":
		return runExplicitDesktopService(ctx, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "%s%s\n", tl("error: "), tl("modo de servidor no soportado: %s", cfg.modo))
		return 2
	}
}

func handleProtocolRequest(ctx context.Context, stderr io.Writer, rawURI string) int {
	return handleProtocolRequestWithGrace(ctx, stderr, rawURI, nil)
}

//lint:ignore U1000 usado en builds con -tags fyne_gui (protocol_fyne.go)
func handleProtocolRequestNoGrace(ctx context.Context, stderr io.Writer, rawURI string) int {
	grace := time.Duration(0)
	return handleProtocolRequestWithGrace(ctx, stderr, rawURI, &grace)
}

func handleProtocolRequestWithGrace(ctx context.Context, stderr io.Writer, rawURI string, override *time.Duration) int {
	return handleProtocolRequestWithGraceConDependencias(
		ctx,
		stderr,
		rawURI,
		override,
		construirRuntimeAfirmaURI,
		newSignApproval(),
	)
}

type constructorRuntimeAfirmaURI func(configDir, p12Dir, p12Password string) (*runtimeAfirmaURI, error)

func handleProtocolRequestWithGraceConDependencias(
	ctx context.Context,
	stderr io.Writer,
	rawURI string,
	override *time.Duration,
	construirRuntime constructorRuntimeAfirmaURI,
	approval ports.UserApproval,
) int {
	logger := logging.New("cmd/grxfirma-afirmauri", stderr)
	timer := newLaunchTimer(ctx, logger, "entry")
	probe := newStartupProbe(ctx, logger)
	probe.Start()
	defer probe.Stop("return")
	exePath, _ := os.Executable()
	protocolDiagnosticReset(rawURI)
	setProtocolPhase("startup", tl("Arrancando GrxFirma..."), tl("Inicializando el flujo de firma web"))
	timer.Mark("startup_begin", "exe", exePath)

	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	setProtocolLocalizer(configDir)
	setProtocolPhase("config_load", tl("Cargando configuración..."), tl("Preparando preferencias y entorno local"))
	cfg, err := config.Load(configDir)
	if err != nil {
		writeErrorToLog(logger, ctx, "cargar_configuracion", err)
		return 1
	}
	timer.Mark("config_loaded")

	p12Dir := cfg.DirectorioP12
	if p12Dir == "" {
		p12Dir = filepath.Join(configDir, "pkcs12")
	}
	if override := os.Getenv("GRXFIRMA_PKCS12_DIR"); override != "" {
		p12Dir = override
	}
	p12Password := os.Getenv("GRXFIRMA_PKCS12_PASSWORD")

	if req, err := parseWebSocketLaunchURI(rawURI); err == nil {
		if !cfg.PermiteWebSocket() {
			writeErrorToLog(logger, ctx, "websocket_policy", errors.New("WebSocket deshabilitado por configuración o política"))
			return 1
		}
		setProtocolPhase("channel_prepare", tl("Preparando canal local..."), tl("Configurando el canal seguro con el navegador"))
		timer.Mark("launch_uri_parsed", "mode", "websocket", "session_id", maskSessionForLog(req.SessionID))
		return handleWebSocketLaunchRequest(ctx, logger, timer, probe, configDir, cfg, p12Dir, p12Password, exePath, req)
	}
	if req, err := parseServiceLaunchURI(rawURI); err == nil {
		setProtocolPhase("channel_prepare", tl("Preparando servicio local..."), tl("Configurando el canal de compatibilidad"))
		timer.Mark("launch_uri_parsed", "mode", "service", "session_id", maskSessionForLog(req.SessionID))
		return handleServiceLaunchRequest(ctx, logger, timer, probe, configDir, cfg, p12Dir, p12Password, exePath, req)
	}

	setProtocolPhase("launch_parse", tl("Procesando solicitud..."), tl("Interpretando la petición recibida"))
	if construirRuntime == nil {
		writeErrorToLog(logger, ctx, "bootstrap", errors.New("constructor de runtime no configurado"))
		return 1
	}
	runtime, err := construirRuntime(configDir, p12Dir, p12Password)
	if err != nil {
		writeErrorToLog(logger, ctx, "bootstrap", err)
		return 1
	}
	timer.Mark("runtime_ready")

	solicitud, err := runtime.parser.Parse(ctx, rawURI)
	if err != nil {
		writeErrorToLog(logger, ctx, "parse_afirmauri", err)
		return 1
	}
	if err := validateNativeProtocolOperation(solicitud.Operacion); err != nil {
		writeErrorToLog(logger, ctx, "native_ui_operation_policy", err)
		return 1
	}
	if err := solicitarAprobacionFirmaWeb(ctx, solicitud, approval); err != nil {
		writeErrorToLog(logger, ctx, "aprobacion_firma_web", err)
		return 1
	}
	timer.Mark("direct_uri_parsed", "operation", string(solicitud.Operacion))
	probe.Stop("direct_uri_parsed")
	protocolDiagnosticSetOperation(string(solicitud.Operacion))
	setProtocolPhase("sign_execute", tl("Ejecutando operación..."), tl("La solicitud ya está preparada para procesarse"))
	grace := successGracePeriod(rawURI, solicitud)
	if override != nil {
		grace = *override
	}
	logger.InfoContext(
		ctx,
		"arranque protocolo afirmauri",
		"pid",
		os.Getpid(),
		"exe",
		exePath,
		"version",
		version,
		"operacion",
		string(solicitud.Operacion),
		"request_id",
		solicitud.Sesion.RequestID,
		"upload_endpoint",
		sanitizeEndpointForLog(solicitud.Sesion.UploadEndpoint),
		"grace_ms",
		grace.Milliseconds(),
	)
	logRuntimeProxyDiagnostic(ctx, logger, diagnosticarClienteHTTPRuntimeSeguro(configDir))

	if err := runtime.orquestadorPara(solicitud).HandleRequest(ctx, solicitud); err != nil {
		writeErrorToLog(logger, ctx, "handle_request", err)
		return 1
	}
	timer.Mark("direct_request_completed", "operation", string(solicitud.Operacion))
	setProtocolPhase("session_complete", tl("Operación completada"), tl("La operación ha terminado correctamente"))
	switch solicitud.Operacion {
	case afirmauri.OperacionSelectCert:
		portalDeliveredUI("certificate")
	case afirmauri.OperacionFirma, afirmauri.OperacionLote, afirmauri.OperacionSignSave:
		portalDeliveredUI("signature")
	}
	logger.InfoContext(
		ctx,
		"protocolo afirmauri completado",
		"pid",
		os.Getpid(),
		"request_id",
		solicitud.Sesion.RequestID,
		"grace_ms",
		grace.Milliseconds(),
	)
	if grace > 0 {
		time.Sleep(grace)
	}
	return 0
}

func solicitarAprobacionFirmaWeb(
	ctx context.Context,
	solicitud afirmauri.Solicitud,
	approval ports.UserApproval,
) error {
	switch solicitud.Operacion {
	case afirmauri.OperacionFirma, afirmauri.OperacionLote, afirmauri.OperacionSignSave:
	default:
		return nil
	}
	if approval == nil {
		return errors.New("aprobador de firma web no configurado")
	}
	approved, err := approval.Request(ctx,
		fmt.Sprintf("¿Desea continuar con la operación web de firma '%s'?", solicitud.Operacion))
	if err != nil {
		return fmt.Errorf("error al solicitar aprobación de la firma web: %w", err)
	}
	if !approved {
		return errors.New("el usuario ha cancelado la operación web de firma")
	}
	return nil
}

func logRuntimeProxyDiagnostic(ctx context.Context, logger *slog.Logger, diag runtimeProxyDiagnostic) {
	if logger == nil {
		return
	}
	switch diag.Mode {
	case runtimeProxyModeDefaultEnvironment:
		if strings.TrimSpace(diag.Reason) != "" {
			logger.WarnContext(ctx, "proxy runtime: se usara el entorno por error cargando settings", "diagnostic", formatRuntimeProxyDiagnostic(diag))
		}
	case runtimeProxyModeFailClosed:
		logger.WarnContext(ctx, "proxy runtime: configuracion fallando cerrada", "diagnostic", formatRuntimeProxyDiagnostic(diag))
	case runtimeProxyModeManualSecureStore, runtimeProxyModeManualNoSecret, runtimeProxyModeSystem:
		logger.InfoContext(ctx, "proxy runtime activo", "diagnostic", formatRuntimeProxyDiagnostic(diag))
	}
}

type websocketLaunchRequest struct {
	Ports     []int
	SessionID string
	Version   int
}

type serviceLaunchRequest struct {
	Ports     []int
	SessionID string
	Version   int
}

func configureLegacyLaunchLifecycle(serviceCtx context.Context, cancel context.CancelFunc, adapter *legacyws.Adaptador) {
	if adapter == nil || adapter.LegacyRaw == nil || cancel == nil {
		return
	}
	handler, ok := adapter.LegacyRaw.(*legacyWebSocketHandler)
	if !ok || handler == nil {
		return
	}
	var cancelOnce sync.Once
	handler.afterTerminal = func(op afirmauri.TipoOperacion, result string) {
		if !shouldAutoCloseLegacyLaunch(op, result) {
			return
		}
		go func() {
			timer := time.NewTimer(legacyLaunchTerminalGrace)
			defer timer.Stop()
			select {
			case <-serviceCtx.Done():
				return
			case <-timer.C:
				cancelOnce.Do(cancel)
			}
		}()
	}
}

func shouldAutoCloseLegacyLaunch(op afirmauri.TipoOperacion, result string) bool {
	if !isLegacyLaunchTerminalResult(result) {
		return false
	}
	switch op {
	case afirmauri.OperacionSave, afirmauri.OperacionSignSave:
		return true
	default:
		return false
	}
}

func isLegacyLaunchTerminalResult(result string) bool {
	result = strings.ToUpper(strings.TrimSpace(result))
	if result == "" || result == "CANCEL" {
		return false
	}
	if strings.HasPrefix(result, "SAF_") || strings.HasPrefix(result, "ERR-") {
		return false
	}
	return result == "SAVE_OK" || result == "OK"
}

// portalCancelledResult reconoce la respuesta CANCEL: la persona canceló y
// el portal ya lo sabe. No es un fallo de entrega.
func portalCancelledResult(result legacyws.Resultado) bool {
	return strings.EqualFold(strings.TrimSpace(result.Texto), "CANCEL")
}

// showPortalCancelled explica la cancelación con su propio texto; el aviso
// se cierra solo al vencer el plazo, como tras un fallo.
func showPortalCancelled(completion *portalCompletion) {
	completion.failure()
	setProtocolCompletionUI(true)
	setProtocolPhase("operation_cancelled", tl("portal.operation.cancelled_title"), tl("portal.operation.cancelled_body"))
}

func successfulPortalResult(result legacyws.Resultado, expectedType string) bool {
	if !strings.EqualFold(strings.TrimSpace(result.Tipo), expectedType) {
		return false
	}
	value := strings.ToUpper(strings.TrimSpace(result.Texto))
	return value != "" && value != "CANCEL" && !strings.HasPrefix(value, "SAF_") &&
		!strings.HasPrefix(value, "ERR-") && !strings.HasPrefix(value, "ERROR")
}

func handleWebSocketLaunchRequest(
	ctx context.Context,
	logger *slog.Logger,
	timer *launchTimer,
	probe *startupProbe,
	configDir string,
	cfg config.Config,
	p12Dir, p12Password, exePath string,
	req *websocketLaunchRequest,
) int {
	if !cfg.PermiteWebSocket() {
		writeErrorToLog(logger, ctx, "websocket_policy", errors.New("WebSocket deshabilitado por configuración o política"))
		return 1
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	setProtocolCompletionUI(false)
	completion := newPortalCompletion(time.Now)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-serviceCtx.Done():
				return
			case <-ticker.C:
				if completion.expire(cancel) {
					return
				}
				kind, _, _, _ := completion.snapshot()
				if kind != "" {
					completion.display()
				}
			}
		}
	}()

	websocketAdapter, _, err := construirServicioWebSocket(configDir, p12Dir, p12Password)
	if err != nil {
		writeErrorToLog(logger, ctx, "bootstrap_websocket_launch", err)
		return 1
	}
	timer.Mark("websocket_runtime_ready")
	configureLegacyLaunchLifecycle(serviceCtx, cancel, websocketAdapter)
	cleanupControl, err := startLegacyLaunchControlServer(serviceCtx, logger, configDir, cfg, websocketAdapter)
	if err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "no se pudo arrancar el socket de control del lanzamiento websocket", "error", err)
		}
		cleanupControl = func() {}
	}
	defer cleanupControl()
	timer.Mark("websocket_launch_control_ready")

	cfg.WebsocketHabilitado = true
	srv, err := legacyws.StartTLSServerOnPortsWithHooks(
		serviceCtx,
		cfg,
		websocketAdapter,
		filepath.Join(configDir, "tls"),
		req.Ports,
		&legacyws.SessionHooks{
			ExpectedSessionID: req.SessionID,
			OnSocketConnected: func(origin string) {
				timer.Mark("browser_connected", "origin", origin)
				probe.Stop("browser_connected")
				protocolDiagnosticSetOrigin(origin)
				setProtocolPhase("browser_connected", tl("Conectado con la web"), tl("El navegador ya está comunicándose con GrxFirma"))
			},
			OnMessageReceived: func(operation string) {
				completion.received()
				setProtocolCompletionUI(false)
				setProtocolPhase("sign_execute", tl("Procesando solicitud..."), tl("La web ha enviado una nueva operación."))
				avisos.Reiniciar()
				timer.Mark("browser_message_received", "operation", operation)
				probe.Stop("browser_message_received")
				protocolDiagnosticSetOperation(operation)
			},
			OnSocketClosed: func() {
				kind, _, _, failed := completion.snapshot()
				if kind == "" && (!failed || !hasLegacyProtocolWindow()) {
					cancel()
				}
			},
			OnMessageHandled: func(resultado legacyws.Resultado) {
				timer.Mark("browser_message_handled", "operation", string(resultado.Operacion), "result_type", resultado.Tipo)
				switch resultado.Operacion {
				case afirmauri.OperacionFirma:
					if successfulPortalResult(resultado, "firma") {
						completion.delivered("signature")
						completion.display()
					} else if portalCancelledResult(resultado) {
						showPortalCancelled(completion)
					} else {
						completion.failure()
						setProtocolCompletionUI(true)
						setProtocolPhase("operation_failed", tl("Operación no completada"), tl("La firma no se ha entregado correctamente. Puede cerrar esta ventana."))
					}
				case afirmauri.OperacionSave:
					setProtocolPhase("save_execute", tl("Preparando descarga..."), tl("Generando el resultado para devolverlo a la web"))
				case afirmauri.OperacionLoad:
					setProtocolPhase("document_pick", tl("Preparando documento..."), tl("Cargando el fichero solicitado por la web"))
				case afirmauri.OperacionSelectCert:
					if successfulPortalResult(resultado, "selectcert") {
						completion.delivered("certificate")
						completion.display()
					} else if portalCancelledResult(resultado) {
						showPortalCancelled(completion)
					} else {
						completion.failure()
						setProtocolCompletionUI(true)
						setProtocolPhase("operation_failed", tl("Operación no completada"), tl("El certificado no se ha entregado correctamente. Puede cerrar esta ventana."))
					}
				}
			},
			OnMessageError: func(operation string, err error) {
				completion.failure()
				setProtocolCompletionUI(true)
				setProtocolPhase("operation_failed", tl("Operación no completada"), tl("La operación no se ha podido entregar correctamente. Puede cerrar esta ventana."))
				presentLegacySHA1Failure(operation, err)
				protocolDiagnosticSetOperation(operation)
				protocolDiagnosticPersistFailure(operation, err)
			},
		},
	)
	if err != nil {
		writeErrorToLog(logger, ctx, "start_websocket_launch", err)
		return 1
	}
	if srv == nil {
		writeErrorToLog(logger, ctx, "start_websocket_launch", fmt.Errorf("servidor websocket no arrancado"))
		return 1
	}
	timer.Mark("websocket_tls_server_ready", "listen_addr", srv.Addr)

	logger.InfoContext(
		ctx,
		"arranque protocolo afirmauri websocket",
		"pid",
		os.Getpid(),
		"exe",
		exePath,
		"version",
		version,
		"requested_ports",
		req.Ports,
		"session_id",
		maskSessionForLog(req.SessionID),
		"protocol_version",
		req.Version,
		"listen_addr",
		srv.Addr,
	)
	protocolDiagnosticSetLaunchMode("websocket", req.Version, req.SessionID, srv.Addr)
	setProtocolPhase("waiting_browser", tl("Esperando a la web..."), tl("GrxFirma está lista y esperando la conexión del portal"))
	timer.Mark("ui_wait_browser")
	if legacyProtocolUILaunchEnabled() {
		return waitLegacyLaunchUI(serviceCtx, cancel, "websocket", srv.Addr, req.SessionID, configDir)
	}
	return waitLegacyLaunchUI(serviceCtx, cancel, "websocket", srv.Addr, req.SessionID, configDir)
}

func handleServiceLaunchRequest(
	ctx context.Context,
	logger *slog.Logger,
	timer *launchTimer,
	probe *startupProbe,
	configDir string,
	cfg config.Config,
	p12Dir, p12Password, exePath string,
	req *serviceLaunchRequest,
) int {
	serviceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	websocketAdapter, _, err := construirServicioWebSocket(configDir, p12Dir, p12Password)
	if err != nil {
		writeErrorToLog(logger, ctx, "bootstrap_service_launch", err)
		return 1
	}
	timer.Mark("service_runtime_ready")
	configureLegacyLaunchLifecycle(serviceCtx, cancel, websocketAdapter)
	cleanupControl, err := startLegacyLaunchControlServer(serviceCtx, logger, configDir, cfg, websocketAdapter)
	if err != nil {
		if logger != nil {
			logger.WarnContext(ctx, "no se pudo arrancar el socket de control del lanzamiento service", "error", err)
		}
		cleanupControl = func() {}
	}
	defer cleanupControl()
	timer.Mark("service_launch_control_ready")

	srv, err := legacyws.StartLegacySocketServer(
		serviceCtx,
		websocketAdapter,
		filepath.Join(configDir, "tls"),
		req.Ports,
		req.SessionID,
		req.Version,
	)
	if err != nil {
		writeErrorToLog(logger, ctx, "start_service_launch", err)
		return 1
	}
	timer.Mark("service_tls_server_ready", "listen_addr", srv.Addr())

	logger.InfoContext(
		ctx,
		"arranque protocolo afirmauri service",
		"pid",
		os.Getpid(),
		"exe",
		exePath,
		"version",
		version,
		"requested_ports",
		req.Ports,
		"session_id",
		maskSessionForLog(req.SessionID),
		"protocol_version",
		req.Version,
		"listen_addr",
		srv.Addr(),
	)
	protocolDiagnosticSetLaunchMode("service", req.Version, req.SessionID, srv.Addr())
	setProtocolPhase("waiting_browser", tl("Esperando a la web..."), tl("GrxFirma está lista y esperando la conexión del portal"))
	timer.Mark("ui_wait_browser")
	probe.Stop("service_socket_ready")
	if legacyProtocolUILaunchEnabled() {
		return waitLegacyLaunchUI(serviceCtx, cancel, "service", srv.Addr(), req.SessionID, configDir)
	}
	return waitLegacyLaunchUI(serviceCtx, cancel, "service", srv.Addr(), req.SessionID, configDir)
}

func legacyProtocolUILaunchEnabled() bool {
	return strings.TrimSpace(os.Getenv("GRXFIRMA_PROTOCOL_UI")) == "1"
}

func parseWebSocketLaunchURI(uriString string) (*websocketLaunchRequest, error) {
	u, err := url.Parse(strings.TrimSpace(uriString))
	if err != nil {
		return nil, fmt.Errorf("uri invalida: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "afirma") {
		return nil, fmt.Errorf("esquema no soportado")
	}

	action := normalizeProtocolLaunchAction(extractProtocolLaunchAction(u))
	if action == "" {
		action = normalizeProtocolLaunchAction(firstQueryParam(u.Query(), "op", "operation", "action"))
	}
	if action != "websocket" {
		return nil, fmt.Errorf("accion no websocket")
	}

	params := u.Query()
	version := parseRequestedProtocolVersion(params)
	if version != 3 && version != 4 {
		return nil, fmt.Errorf("version de protocolo no soportada: %d", version)
	}

	portsRaw := firstQueryParam(params, "ports", "port", "portsList")
	ports := []int{defaultLaunchedWebSocketPort}
	if strings.TrimSpace(portsRaw) != "" {
		parsed := make([]int, 0, 4)
		for _, chunk := range strings.Split(portsRaw, ",") {
			chunk = strings.TrimSpace(chunk)
			if chunk == "" {
				continue
			}
			port, err := strconv.Atoi(chunk)
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("puerto websocket invalido: %s", chunk)
			}
			parsed = append(parsed, port)
		}
		if len(parsed) > 0 {
			ports = parsed
		}
	}

	return &websocketLaunchRequest{
		Ports:     ports,
		SessionID: firstQueryParam(params, "idsession", "idSession", "sessionid", "sessionId"),
		Version:   version,
	}, nil
}

func parseServiceLaunchURI(uriString string) (*serviceLaunchRequest, error) {
	u, err := url.Parse(strings.TrimSpace(uriString))
	if err != nil {
		return nil, fmt.Errorf("uri invalida: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "afirma") {
		return nil, fmt.Errorf("esquema no soportado")
	}

	action := normalizeProtocolLaunchAction(extractProtocolLaunchAction(u))
	if action == "" {
		action = normalizeProtocolLaunchAction(firstQueryParam(u.Query(), "op", "operation", "action"))
	}
	if action != "service" {
		return nil, fmt.Errorf("accion no service")
	}

	params := u.Query()
	version := parseRequestedProtocolVersion(params)
	if version < 1 || version > 3 {
		return nil, fmt.Errorf("version de protocolo no soportada: %d", version)
	}

	portsRaw := firstQueryParam(params, "ports", "port", "portsList")
	if strings.TrimSpace(portsRaw) == "" {
		return nil, fmt.Errorf("faltan puertos service")
	}
	ports, err := parseLaunchPortsList(portsRaw)
	if err != nil {
		return nil, err
	}
	sessionID := firstQueryParam(params, "idsession", "idSession", "sessionid", "sessionId")
	if sessionID == "" {
		return nil, fmt.Errorf("falta id de sesion service")
	}

	return &serviceLaunchRequest{
		Ports:     ports,
		SessionID: sessionID,
		Version:   version,
	}, nil
}

func extractProtocolLaunchAction(u *url.URL) string {
	if host := strings.TrimSpace(u.Host); host != "" {
		return host
	}
	return strings.Trim(strings.TrimSpace(u.Path), "/")
}

func normalizeProtocolLaunchAction(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func firstQueryParam(values url.Values, names ...string) string {
	for _, name := range names {
		if val := strings.TrimSpace(values.Get(name)); val != "" {
			return val
		}
	}
	return ""
}

func parseRequestedProtocolVersion(values url.Values) int {
	raw := firstQueryParam(values, "v", "ver", "protocolVersion")
	if strings.TrimSpace(raw) == "" {
		return 4
	}
	version, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 4
	}
	return version
}

func parseLaunchPortsList(raw string) ([]int, error) {
	parsed := make([]int, 0, 4)
	for _, chunk := range strings.Split(strings.TrimSpace(raw), ",") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		port, err := strconv.Atoi(chunk)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("puerto invalido: %s", chunk)
		}
		parsed = append(parsed, port)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("sin puertos válidos")
	}
	return parsed, nil
}

func maskSessionForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return "[REDACTED_ID]"
}

func writeErrorToLog(logger *slog.Logger, ctx context.Context, op string, err error) {
	if err == nil {
		return
	}
	reportNativeProtocolFailure(op, err)
	portalErrorUI()
	protocolDiagnosticPersistFailure(op, err)
	logger.ErrorContext(ctx, "operacion fallida", "op", op, "error", err)
}

func setProtocolPhase(phase, status, detail string) {
	protocolDiagnosticMarkPhase(phase, status, detail)
	// Las decisiones automáticas de la operación se explican siempre al
	// usuario junto al estado, no solo en el registro.
	updateProtocolUI(status, detailWithStartupTLSTrustNotice(avisos.ConAvisos(detail)))
	if phase == "operation_failed" || phase == "operation_cancelled" {
		portalErrorUI()
	}
}

func successGracePeriod(rawURI string, solicitud afirmauri.Solicitud) time.Duration {
	const (
		fallback      = 5 * time.Second
		remoteDefault = 15 * time.Second
	)
	raw := os.Getenv("GRXFIRMA_SUCCESS_GRACE_MS")
	if raw == "" {
		if shouldExtendSuccessGrace(rawURI, solicitud) {
			return remoteDefault
		}
		return fallback
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		if shouldExtendSuccessGrace(rawURI, solicitud) {
			return remoteDefault
		}
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func shouldExtendSuccessGrace(rawURI string, solicitud afirmauri.Solicitud) bool {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return false
	}
	uriL := strings.ToLower(rawURI)
	if !strings.HasPrefix(uriL, "afirma://") {
		return false
	}
	if strings.Contains(uriL, "127.0.0.1") || strings.Contains(uriL, "localhost") {
		return false
	}
	if !strings.Contains(uriL, "stservlet=") && !strings.Contains(uriL, "storageservlet=") {
		return false
	}
	if strings.Contains(uriL, "op=batch") || strings.Contains(uriL, "afirma://batch") || strings.Contains(uriL, "afirma://sign") {
		return true
	}

	if solicitud.Sesion.UploadEndpoint != "" {
		return !isLocalEndpoint(solicitud.Sesion.UploadEndpoint)
	}
	return false
}

func isLocalEndpoint(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func sanitizeEndpointForLog(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + u.Hostname()
}
