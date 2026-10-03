// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// grxfirma-gui es el lanzador del núcleo Go y de la interfaz gráfica
// seleccionada.
//
// Secuencia de arranque:
//  1. Construye los servicios hexagonales (certificados, firma, verificacion...).
//  2. Arranca el servidor IPC en un socket Unix o named pipe de Windows.
//  3. Lanza Qt/QML o WinUI 3 pasándole la ruta del socket.
//  4. Espera a que el frontend termine o a que llegue SIGINT/SIGTERM.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"grxfirma/internal/appdirs"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	resttls "grxfirma/internal/adapters/inbound/common/rest"
	"grxfirma/internal/adapters/inbound/common/secretinput"
	"grxfirma/internal/adapters/inbound/desktop/ipc"
	"grxfirma/internal/adapters/outbound/common/config"
	"grxfirma/internal/adapters/outbound/common/hashmanifest"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/adapters/outbound/common/metrics"
	"grxfirma/internal/adapters/outbound/common/pkcs12importer"
	"grxfirma/internal/adapters/outbound/common/protector"
	"grxfirma/internal/adapters/outbound/common/revocationclient"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/adapters/outbound/common/systemtrust"
	"grxfirma/internal/adapters/outbound/common/tsaclient"
	"grxfirma/internal/adapters/outbound/common/updatecheck"
	"grxfirma/internal/adapters/outbound/desktop/certaccess"
	"grxfirma/internal/adapters/outbound/desktop/certcatalogagg"
	"grxfirma/internal/adapters/outbound/desktop/filesystem"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
	"grxfirma/internal/adapters/outbound/desktop/macoskeychain"
	"grxfirma/internal/adapters/outbound/desktop/nssstore"
	"grxfirma/internal/adapters/outbound/desktop/pdfpreview"
	"grxfirma/internal/adapters/outbound/desktop/proxyhttp"
	"grxfirma/internal/adapters/outbound/desktop/proxysecretstore"
	"grxfirma/internal/adapters/outbound/desktop/servicemanager"
	"grxfirma/internal/adapters/outbound/desktop/sessioncertstore"
	deskSigner "grxfirma/internal/adapters/outbound/desktop/signer"
	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/adapters/outbound/desktop/usersettings"
	"grxfirma/internal/adapters/outbound/desktop/wincertstore"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/presentation/desktop/tokenpin"
)

var version = "dev"

func main() {
	bootLoc := localizador.Detectar()
	if err := secretinput.RejectArgv(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, secretinput.UserMessage(err, bootLoc.T))
		os.Exit(2)
	}
	environment, err := secretinput.ConsumeEnvironment(
		[]string{
			"GRXFIRMA_PKCS12_PASSWORD",
			"GRXFIRMA_REST_TOKEN",
			"GRXFIRMA_PROTECTION_SECRET_B64",
		},
		os.LookupEnv,
		os.Unsetenv,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, secretinput.UserMessage(err, bootLoc.T))
		os.Exit(1)
	}
	// Este proceso solo consume P12. Los otros secretos se descartan tras
	// retirarlos del entorno para que tampoco permanezcan en memoria.
	_, _ = environment.Take("GRXFIRMA_REST_TOKEN")
	_, _ = environment.Take("GRXFIRMA_PROTECTION_SECRET_B64")

	var (
		socketPath       = flag.String("ipc-socket", "", "ruta del socket IPC (auto si vacio)")
		frontend         = flag.String("frontend", frontendPorDefecto(runtime.GOOS), "frontend grafico: qt o winui")
		uiBinary         = flag.String("ui-binary", "", "ruta al binario del frontend (auto si vacio)")
		qtBinary         = flag.String("qt-binary", "", "compatibilidad: ruta al binario Qt (auto si vacio)")
		p12              = flag.String("p12", "", "fichero PKCS#12")
		p12PasswordStdin = flag.Bool("p12-password-stdin", false, "lee la contraseña PKCS#12 desde stdin (sin eco en terminal)")
		serverOnly       = flag.Bool("server", false, "arranca solo el servidor interno sin lanzar Qt")
		serverMode       = flag.String("server-modo", "ipc", "modo de servidor (ipc)")
		ipcClientPID     = flag.Uint64("ipc-client-pid", 0, "PID exacto del frontend autorizado para IPC")
		startHidden      = flag.Bool("start-hidden", false, "inicia la interfaz oculta en la bandeja")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)
	loc := localizador.Para(idiomaPreferido(configDir))
	localTLSStartupStatus := ensureLocalTLSStartup(ctx, configDir)

	p12PasswordCompat, _ := environment.Take("GRXFIRMA_PKCS12_PASSWORD")
	p12Password, err := resolverPasswordP12GUI(
		*p12PasswordStdin,
		os.Stdin,
		os.Stderr,
		loc.T("security.secret.prompt.p12"),
		p12PasswordCompat,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, secretinput.UserMessage(err, loc.T))
		os.Exit(1)
	}

	if *serverOnly {
		os.Exit(runServerMode(
			ctx,
			*serverMode,
			*p12,
			p12Password,
			*socketPath,
			*ipcClientPID,
			loc,
			localTLSStartupStatus,
			configDir,
		))
	}

	srv, socketRuta, err := construirServidor(ctx, *p12, p12Password, *socketPath, loc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grxfirma-gui: error arrancando servidor IPC: %v\n", err)
		os.Exit(1)
	}
	srv.SetLocalTLSStartupStatus(localTLSStartupStatus)
	srv.SetLocalTLSStartupRefresh(func(ctx context.Context) ipc.LocalTLSStartupStatus {
		return ensureLocalTLSStartup(ctx, configDir)
	})
	vincularFrontend := ipc.SoportaVinculacionPIDFrontend()
	if vincularFrontend {
		if err := srv.PrepararVinculacionPIDFrontend(); err != nil {
			fmt.Fprintf(
				os.Stderr,
				"grxfirma-gui: no se pudo preparar la vinculacion segura del frontend: %v\n",
				err,
			)
			os.Exit(1)
		}
	}

	// Arrancar servidor IPC en goroutine de fondo.
	ipcListo := make(chan struct{})
	go func() {
		close(ipcListo)
		if err := srv.Escuchar(ctx, socketRuta); err != nil {
			fmt.Fprintf(os.Stderr, "grxfirma-gui: IPC: %v\n", err)
		}
	}()
	<-ipcListo

	frontendSeleccionado, err := normalizarFrontend(*frontend)
	if err != nil {
		srv.DenegarPIDFrontend()
		srv.Cerrar()
		fmt.Fprintf(os.Stderr, "grxfirma-gui: %v\n", err)
		os.Exit(2)
	}
	bin := strings.TrimSpace(*uiBinary)
	if bin == "" && frontendSeleccionado == frontendQt {
		bin = strings.TrimSpace(*qtBinary)
	}
	if bin == "" {
		bin = buscarBinarioFrontend(frontendSeleccionado)
	}
	if bin == "" {
		srv.DenegarPIDFrontend()
		srv.Cerrar()
		fmt.Fprintf(
			os.Stderr,
			"grxfirma-gui: no se encontro el frontend %s. Pasa --ui-binary=<ruta>\n",
			frontendSeleccionado,
		)
		os.Exit(1)
	}

	// #nosec G204 -- bin is either an explicit operator CLI choice or a
	// resolved fixed candidate; socketRuta is passed as a distinct argument.
	frontendArgs := argumentosFrontend(frontendSeleccionado, socketRuta, os.Getpid())
	if *startHidden {
		frontendArgs = append(frontendArgs, "--start-hidden")
	}
	uiCmd := exec.CommandContext(
		ctx,
		bin,
		frontendArgs...,
	)
	uiCmd.Env = entornoFrontend(frontendSeleccionado)
	uiCmd.Stdout = os.Stdout
	uiCmd.Stderr = os.Stderr
	uiCmd.Stdin = os.Stdin

	if err := iniciarFrontendVinculado(uiCmd, srv, vincularFrontend); err != nil {
		fmt.Fprintf(os.Stderr, "grxfirma-gui: no se pudo lanzar %s: %v\n", bin, err)
		srv.Cerrar()
		os.Exit(1)
	}

	// Esperar a que el frontend termine o a que se cancele el contexto.
	done := make(chan error, 1)
	go func() { done <- uiCmd.Wait() }()

	select {
	case <-ctx.Done():
		// Señal recibida; el frontend se cerrará por exec.CommandContext.
		srv.DenegarPIDFrontend()
		srv.Cerrar()
	case err := <-done:
		srv.DenegarPIDFrontend()
		srv.Cerrar()
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"grxfirma-gui: el frontend %s termino con error: %v\n",
				frontendSeleccionado,
				err,
			)
			os.Exit(1)
		}
	}
}

func iniciarFrontendVinculado(
	cmd *exec.Cmd,
	srv *ipc.Servidor,
	vincular bool,
) error {
	if err := cmd.Start(); err != nil {
		if vincular {
			srv.DenegarPIDFrontend()
		}
		return err
	}
	if !vincular {
		return nil
	}
	if cmd.Process == nil || cmd.Process.Pid <= 0 ||
		uint64(cmd.Process.Pid) > uint64(^uint32(0)) {
		srv.DenegarPIDFrontend()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return errors.New("el proceso frontend no proporciono un PID valido")
	}
	if err := srv.PublicarPIDFrontend(uint32(cmd.Process.Pid)); err != nil {
		srv.DenegarPIDFrontend()
		_ = cmd.Process.Kill()
		return errors.Join(errors.New("publicando PID seguro del frontend"), err)
	}
	return nil
}

func resolverPasswordP12GUI(
	fromStdin bool,
	stdin io.Reader,
	stderr io.Writer,
	prompt string,
	fromEnv string,
) (string, error) {
	if fromStdin {
		return secretinput.Read(stdin, stderr, prompt)
	}
	// Compatibilidad para lanzadores existentes. Se retira del entorno antes
	// de lanzar el proceso Qt para que no se propague a hijos.
	return fromEnv, nil
}

// ensureLocalTLSStartup prepara la CA del canal afirma:// para este usuario.
// Se repite en cada arranque para incorporar perfiles Firefox nuevos; la
// implementación de confianza solo modifica los almacenes que lo necesitan.
func ensureLocalTLSStartup(ctx context.Context, configDir string) ipc.LocalTLSStartupStatus {
	status := ipc.LocalTLSStartupStatus{State: "unknown"}
	if runtime.GOOS != "linux" {
		return status
	}
	status.State = "error"
	if !filepath.IsAbs(configDir) {
		return status
	}
	err := localtlstrust.WithLocalTLSStartupLock(ctx, configDir, func() error {
		_, _, rootCertFile, _, err := resttls.EnsureBrowserCompatibleLocalhostCertificate(
			filepath.Join(configDir, "tls"), "websocket-localhost",
		)
		if err != nil {
			return err
		}
		status.Changed, err = localtlstrust.EnsureManagedTrustedWithResult(ctx, rootCertFile)
		return err
	})
	if err != nil {
		return status
	}
	status.State = "ready"
	return status
}

func runServerMode(
	ctx context.Context,
	mode, rutaP12, passwordP12, socketPath string,
	ipcClientPID uint64,
	loc ports.Localizador,
	localTLSStartupStatus ipc.LocalTLSStartupStatus,
	configDir string,
) int {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "ipc":
		pidFrontend, err := normalizarPIDFrontend(ipcClientPID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "grxfirma-gui: PID de cliente IPC no valido: %v\n", err)
			return 2
		}
		srv, socketRuta, err := construirServidor(ctx, rutaP12, passwordP12, socketPath, loc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "grxfirma-gui: error arrancando servidor IPC: %v\n", err)
			return 1
		}
		srv.SetLocalTLSStartupStatus(localTLSStartupStatus)
		srv.SetLocalTLSStartupRefresh(func(ctx context.Context) ipc.LocalTLSStartupStatus {
			return ensureLocalTLSStartup(ctx, configDir)
		})
		if pidFrontend != 0 {
			if err := srv.VincularPIDFrontend(pidFrontend); err != nil {
				fmt.Fprintf(
					os.Stderr,
					"grxfirma-gui: no se pudo vincular el cliente IPC: %v\n",
					err,
				)
				return 1
			}
		}
		fmt.Fprintf(os.Stderr, "grxfirma-gui: servidor IPC escuchando en %s\n", socketRuta)
		if err := srv.Escuchar(ctx, socketRuta); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "grxfirma-gui: IPC: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "grxfirma-gui: modo de servidor no soportado: %s\n", mode)
		return 2
	}
}

func normalizarPIDFrontend(value uint64) (uint32, error) {
	if value == 0 {
		return 0, nil
	}
	if value > uint64(^uint32(0)) {
		return 0, errors.New("fuera del rango de PID admitido")
	}
	return uint32(value), nil
}

func idiomaPreferido(configDir string) string {
	doc, err := usersettings.CargarDocumentoCompat(context.Background(), configDir)
	if err == nil && doc.General.Idioma != nil {
		idioma := strings.TrimSpace(*doc.General.Idioma)
		if idioma != "" {
			return idioma
		}
	}
	return localizador.Detectar().Locale()
}

// construirServidor monta la pila hexagonal completa y devuelve el servidor IPC.
func construirServidor(ctx context.Context, rutaP12, passwordP12, socketPath string, loc ports.Localizador) (*ipc.Servidor, string, error) {
	home, _ := os.UserHomeDir()
	configDir := appdirs.Config(home)

	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, "", fmt.Errorf("cargando configuracion: %w", err)
	}

	p12Dir := cfg.DirectorioP12
	if p12Dir == "" {
		p12Dir = filepath.Join(configDir, "pkcs12")
	}
	if v := os.Getenv("GRXFIRMA_PKCS12_DIR"); v != "" {
		p12Dir = v
	}

	metricas := metrics.NewDefault()

	// Certificados y claves.
	catalogo, claves, temporaryStore, err := construirFuentesCertificados(ctx, rutaP12, passwordP12, p12Dir, metricas)
	if err != nil {
		return nil, "", err
	}
	if tokenruntime.EnabledInBuild() {
		tokens := tokenruntime.New(configDir, tokenpin.New(idiomaPreferido(configDir)).Request)
		catalogo = certcatalogagg.New(catalogo, tokens)
		claves = &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{claves, tokens}}
	}

	// Motor de firma.
	motor := deskSigner.NuevoMotorFirmaGo(relojReal{})
	httpClient := proxyhttp.New(configDir)
	if tsaURL := os.Getenv("GRXFIRMA_TSA_URL"); tsaURL != "" {
		tsa := tsaclient.New(tsaURL)
		tsa.HTTPClient = httpClient
		motor.WithTimestampAuthority(tsa)
	}
	motor.WithRevocationProvider(revocationclient.NewWithHTTPClient(httpClient))

	// Casos de uso.
	auditor := application.NuevoAuditUseCase(relojReal{}, &loggerSilencioso{})
	aprobador := &aprobacionAutomatica{}

	ucFirmar := application.NuevoSignDocumentUseCase(catalogo, claves, motor, aprobador, auditor, nil).
		WithMetrics(metricas)
	ucMultiCofirmar := application.NuevoMultiCoSignUseCase(ucFirmar)
	ucProcesarLote := application.NuevoProcessBatchUseCase(catalogo, claves, motor, aprobador, auditor, nil).
		WithMetrics(metricas)
	ucCrearHash := application.NuevoCreateHashUseCase()
	ucComprobarHash := application.NuevoCheckHashUseCase()
	treeReader := filesystem.NuevoDirectoryTreeReader()
	manifestCodec := hashmanifest.NuevoCodec()
	ucCrearHashDir := application.NuevoCreateDirectoryHashManifestUseCase(treeReader, manifestCodec)
	ucComprobarHashDir := application.NuevoCheckDirectoryHashManifestUseCase(treeReader, manifestCodec)

	ucVerificar := application.NuevoVerifySignatureUseCase(systemtrust.New(), commonsigner.NewMultiVerifierWithHTTPClient(httpClient), auditor)
	keyringProteccion := protector.NuevoLocalCombinedKeyring(configDir, catalogo, claves)
	motorProteccion := protector.NuevoAdaptiveProtector()
	motorProteccionFirmada := protector.NuevoCMSSignedEnvelopedProtector()
	ucProteger := application.NuevoProtectDocumentUseCase(keyringProteccion, motorProteccion, aprobador, auditor, nil)
	ucProtegerFirmando := application.NuevoProtectAndSignDocumentUseCase(catalogo, claves, keyringProteccion, motorProteccionFirmada, aprobador, auditor, nil)
	ucDesproteger := application.NuevoUnprotectDocumentUseCase(keyringProteccion, motorProteccion, auditor, nil)

	ucPreview := application.NuevoPdfPreviewUseCase(pdfpreview.New())

	// Adaptadores outbound nuevos.
	gestorServicio := servicemanager.New("")
	configUsuario := usersettings.New(configDir)
	importador := pkcs12importer.New()

	// Calcular ruta del socket.
	if socketPath == "" {
		socketPath, err = calcularSocketPath()
		if err != nil {
			return nil, "", fmt.Errorf("generando endpoint IPC unico: %w", err)
		}
	}

	srv := ipc.NuevoServidor(
		catalogo,
		ucFirmar,
		ucMultiCofirmar,
		ucProcesarLote,
		ucVerificar,
		ucCrearHash,
		ucComprobarHash,
		ucCrearHashDir,
		ucComprobarHashDir,
		manifestCodec,
		ucPreview,
		gestorServicio,
		configUsuario,
		importador,
		loc,
		configDir,
	)
	srv.WithClaves(claves)
	// In release builds the manager returns an unavailable snapshot without IO.
	// Driver configuration is never injected into REST, URI or Native Messaging.
	srv.WithLocalTokenSettings(tokenruntime.NewManager(configDir))
	srv.WithCertificateAccess(
		application.NuevoCertificateAccessUseCase(certaccess.New(p12Dir)),
		application.NuevoTemporaryCertificateUseCase(temporaryStore),
	)
	srv.WithProxySecrets(proxysecretstore.New())
	srv.WithUpdates(updatecheck.NewWithHTTPClient(httpClient), version)
	srv.WithProtection(ucProteger, ucProtegerFirmando, ucDesproteger, keyringProteccion)
	go func() {
		<-ctx.Done()
		temporaryStore.Clear()
	}()

	return srv, socketPath, nil
}

// ---------------------------------------------------------------------------
// Bootstrapping de certificados (similar a cmd/grxfirma/bootstrap.go)
// ---------------------------------------------------------------------------

type relojReal struct{}

func (r relojReal) Now() time.Time { return time.Now() }

type aprobacionAutomatica struct{}

func (a *aprobacionAutomatica) Request(_ context.Context, _ string) (bool, error) {
	return true, nil
}

type loggerSilencioso struct{}

func (l *loggerSilencioso) Log(_ context.Context, _ ports.Evidence) error { return nil }

type catalogoMemoria struct{ certs []domain.CertificateRef }

func (c *catalogoMemoria) List(_ context.Context) ([]domain.CertificateRef, error) {
	return c.certs, nil
}

type proveedorClavesMemoria struct{ claves map[string]ports.SigningKey }

func (p *proveedorClavesMemoria) KeyFor(_ context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	if k, ok := p.claves[ref.ID]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("clave no encontrada para %s", ref.ID)
}

type proveedorClavesAgregado struct{ fuentes []ports.SigningKeyProvider }

func (p *proveedorClavesAgregado) KeyFor(ctx context.Context, ref domain.CertificateRef) (ports.SigningKey, error) {
	var ultimo error
	for _, f := range p.fuentes {
		if f == nil {
			continue
		}
		k, err := f.KeyFor(ctx, ref)
		if err == nil && k != nil {
			return k, nil
		}
		ports.CloseSigningKey(k)
		if err != nil && !esErrorProveedorNoAplicable(err) {
			ultimo = err
		}
	}
	if ultimo != nil {
		return nil, ultimo
	}
	return nil, fmt.Errorf("clave no encontrada para %s", ref.ID)
}

func esErrorProveedorNoAplicable(err error) bool {
	return errors.Is(err, macoskeychain.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, wincertstore.ErrNoDisponibleEnEstaPlataforma) ||
		errors.Is(err, tokenruntime.ErrNotApplicable) || errors.Is(err, tokenruntime.ErrIdentityUnknown) ||
		strings.Contains(strings.ToLower(err.Error()), "nssstore: solo disponible en linux")
}

func construirFuentesCertificados(
	ctx context.Context,
	rutaP12, password, p12Dir string,
	metricas ports.OperationMetrics,
) (ports.CertificateCatalog, ports.SigningKeyProvider, ports.TemporaryCertificateStore, error) {
	importador := pkcs12importer.New()
	var refs []domain.CertificateRef
	claves := map[string]ports.SigningKey{}

	if rutaP12 != "" {
		id, err := importador.ImportP12File(ctx, rutaP12, password)
		if err != nil {
			return nil, nil, nil, err
		}
		if metricas != nil {
			metricas.RecordCertificateSource(ctx, "p12")
		}
		refs = append(refs, id.Reference)
		claves[id.Reference.ID] = deskSigner.NuevaClaveLocalConCadena(id.Signer, id.Certificate, id.Chain)
	}

	if p12Dir != "" {
		entries, _ := os.ReadDir(p12Dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			nombre := e.Name()
			if filepath.Ext(nombre) != ".p12" && filepath.Ext(nombre) != ".pfx" {
				continue
			}
			id, err := importador.ImportP12File(ctx, filepath.Join(p12Dir, nombre), password)
			if err != nil {
				continue
			}
			if metricas != nil {
				metricas.RecordCertificateSource(ctx, "p12")
			}
			refs = append(refs, id.Reference)
			claves[id.Reference.ID] = deskSigner.NuevaClaveLocalConCadena(id.Signer, id.Certificate, id.Chain)
		}
	}

	catMem := &catalogoMemoria{certs: refs}
	temporaryStore := sessioncertstore.New()
	nss := nssstore.New()
	mac := macoskeychain.New()
	win := wincertstore.New()

	catalogo := certcatalogagg.New(temporaryStore, catMem, nss, mac, win)
	proveedor := &proveedorClavesAgregado{fuentes: []ports.SigningKeyProvider{
		temporaryStore, &proveedorClavesMemoria{claves: claves}, nss, mac, win,
	}}

	return catalogo, proveedor, temporaryStore, nil
}

// ---------------------------------------------------------------------------
// Helpers de plataforma
// ---------------------------------------------------------------------------

type frontendKind string

const (
	frontendQt    frontendKind = "qt"
	frontendWinUI frontendKind = "winui"
)

func normalizarFrontend(raw string) (frontendKind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(frontendQt), "qml":
		return frontendQt, nil
	case string(frontendWinUI), "windows", "native":
		if runtime.GOOS != "windows" {
			return "", errors.New("el frontend WinUI solo está disponible en Windows")
		}
		return frontendWinUI, nil
	default:
		return "", fmt.Errorf("frontend no soportado: %s", raw)
	}
}

func calcularSocketPath() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	sufijo := fmt.Sprintf("%d_%s", os.Getpid(), hex.EncodeToString(nonce[:]))
	if runtime.GOOS == "windows" {
		return `\\.\pipe\grxfirma_ipc_` + sufijo, nil
	}
	nombre := "grxfirma_ipc_" + sufijo + ".sock"
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, nombre), nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		runDir := filepath.Join(home, ".local", "run")
		_ = os.MkdirAll(runDir, 0o700)
		return filepath.Join(runDir, nombre), nil
	}
	return filepath.Join(os.TempDir(), nombre), nil
}

func buscarBinarioQt() string {
	return buscarBinarioFrontend(frontendQt)
}

func buscarBinarioFrontend(frontend frontendKind) string {
	candidatos := candidatosFrontend(frontend)
	if len(candidatos) == 0 {
		return ""
	}
	if runtime.GOOS == "darwin" {
		candidatos = append(candidatos,
			"GrxFirma Desktop Qt.app/Contents/MacOS/grxfirma-gui-qml",
			"GrxFirmaQt.app/Contents/MacOS/GrxFirmaQt",
		)
	}

	// os.Args[0] puede ser arbitrario. Solo se usa como base la ruta real y
	// resuelta del proceso en ejecucion.
	if ejecutable, err := os.Executable(); err == nil {
		if ejecutable, err = filepath.Abs(ejecutable); err == nil {
			if ejecutable, err = filepath.EvalSymlinks(ejecutable); err == nil {
				appDir := filepath.Dir(ejecutable)
				if ruta := buscarCandidatoFrontendEnDirectorio(appDir, candidatos); ruta != "" {
					return ruta
				}
				if runtime.GOOS == "windows" {
					if ruta := buscarFrontendInstaladoEnHermano(appDir, frontend); ruta != "" {
						return ruta
					}
				}
			}
		}
	}

	for _, nombre := range candidatos {
		// PATH is an explicit process-level executable search policy and every
		// candidate name is fixed by the application.
		if filepath.Base(nombre) != nombre {
			continue
		}
		if ruta, err := exec.LookPath(nombre); err == nil {
			return ruta
		}
	}
	return ""
}

func candidatosFrontend(frontend frontendKind) []string {
	switch frontend {
	case frontendQt:
		return []string{"grxfirma-gui-qml", "grxfirma-qt", "GrxFirmaQt"}
	case frontendWinUI:
		return []string{
			"grxfirma-winui.exe",
			filepath.Join("winui", "grxfirma-winui.exe"),
		}
	default:
		return nil
	}
}

// buscarFrontendInstaladoEnHermano cubre la instalación por usuario de Windows:
// DesktopLauncher y las interfaces son hermanos bajo Programs/GrxFirma.
func buscarFrontendInstaladoEnHermano(appDir string, frontend frontendKind) string {
	if !filepath.IsAbs(appDir) || !strings.EqualFold(filepath.Base(appDir), "DesktopLauncher") ||
		!strings.EqualFold(filepath.Base(filepath.Dir(appDir)), "GrxFirma") ||
		!strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(appDir))), "Programs") {
		return ""
	}
	var component, binary string
	switch frontend {
	case frontendQt:
		component, binary = "DesktopQML", "grxfirma-gui-qml.exe"
	case frontendWinUI:
		component, binary = "DesktopWinUI", "grxfirma-winui.exe"
	default:
		return ""
	}
	candidate := filepath.Join(filepath.Dir(appDir), component, binary)
	if info, err := os.Lstat(candidate); err == nil && info.Mode().IsRegular() && esEjecutable(info.Mode()) {
		return candidate
	}
	return ""
}

func argumentosFrontend(frontend frontendKind, socketPath string, backendPID int) []string {
	args := []string{"--ipc-socket", socketPath}
	if frontend == frontendWinUI && backendPID > 0 {
		args = append(args, "--backend-pid", fmt.Sprintf("%d", backendPID))
	}
	return args
}

func buscarCandidatoQtEnDirectorio(appDir string, candidatos []string) string {
	return buscarCandidatoFrontendEnDirectorio(appDir, candidatos)
}

func buscarCandidatoFrontendEnDirectorio(appDir string, candidatos []string) string {
	if !filepath.IsAbs(appDir) {
		return ""
	}
	for _, nombre := range candidatos {
		if !filepath.IsLocal(nombre) {
			continue
		}
		ruta := filepath.Join(appDir, nombre)
		// The only production caller derives appDir from
		// os.Executable+EvalSymlinks; candidatos is the fixed list above.
		if info, err := os.Lstat(ruta); err == nil && info.Mode().IsRegular() && esEjecutable(info.Mode()) {
			return ruta
		}
	}
	return ""
}

func esEjecutable(modo os.FileMode) bool {
	return runtime.GOOS == "windows" || modo.Perm()&0o111 != 0
}

func entornoConBusSesion() []string {
	env := os.Environ()
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return env
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return env
	}
	if !filepath.IsAbs(runtimeDir) || filepath.Clean(runtimeDir) != runtimeDir || strings.IndexByte(runtimeDir, 0) >= 0 {
		return env
	}
	runtimeInfo, err := os.Lstat(runtimeDir) // #nosec G703 -- XDG_RUNTIME_DIR is required to be absolute and is only inspected, never opened or executed.
	if err != nil || runtimeInfo.Mode()&os.ModeSymlink != 0 || !runtimeInfo.IsDir() ||
		runtimeInfo.Mode().Perm()&0o077 != 0 {
		return env
	}
	busPath := filepath.Join(runtimeDir, "bus")
	// busPath combines a validated absolute non-symlink runtime directory with
	// the fixed basename "bus"; Lstat prevents accepting a symlink.
	busInfo, err := os.Lstat(busPath) // #nosec G703 -- fixed "bus" below the absolute, private, non-symlink runtime directory validated above.
	if err != nil || busInfo.Mode()&os.ModeSymlink != 0 || busInfo.Mode()&os.ModeSocket == 0 {
		return env
	}
	return append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+busPath)
}

func entornoFrontend(frontend frontendKind) []string {
	if frontend == frontendQt {
		return entornoConBusSesion()
	}
	return os.Environ()
}

// frontendPorDefecto elige la interfaz nativa de cada sistema: en Windows se
// instala WinUI y no Qt, así que abrir el lanzador sin argumentos (doble clic)
// no debe buscar una interfaz que no existe.
func frontendPorDefecto(goos string) string {
	if goos == "windows" {
		return string(frontendWinUI)
	}
	return "qt"
}
